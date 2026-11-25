package report

import (
	"errors"
	"testing"
	"time"

	"github.com/parqueo/api/internal/domain/models"
)

func TestJornadaBoundsWithAfternoonCutoff(t *testing.T) {
	got, err := JornadaBounds("2026-11-25", "2026-11-25", "15:00:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := Jornada{After: "2026-11-24 15:00:00", Until: "2026-11-25 15:00:00", Cutoff: "15:00:00"}
	if got != want {
		t.Fatalf("expected %+v, got %+v", want, got)
	}
	if got.From() != "2026-11-24 15:00:01" {
		t.Fatalf("expected the jornada to start one second after the cutoff, got %q", got.From())
	}
}

func TestJornadaBoundsForNormalParkingMatchCalendarDays(t *testing.T) {
	got, err := JornadaBounds("2026-11-01", "2026-11-30", "23:59:59")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.After != "2026-10-31 23:59:59" || got.Until != "2026-11-30 23:59:59" || got.From() != "2026-11-01 00:00:00" {
		t.Fatalf("expected calendar days, got %+v (from %s)", got, got.From())
	}
}

func TestJornadaBoundsDefaultsInvalidCutoffToEndOfDay(t *testing.T) {
	for _, cutoff := range []string{"", "  ", "25:00:00", "abc", "15:00:00.000000"} {
		got, err := JornadaBounds("2026-11-25", "2026-11-25", cutoff)
		if cutoff == "15:00:00.000000" {
			if err != nil || got.Cutoff != "15:00:00" {
				t.Errorf("expected fractional seconds to be accepted, got %+v, %v", got, err)
			}
			continue
		}
		if err != nil || got.Cutoff != "23:59:59" {
			t.Errorf("cutoff %q: expected default 23:59:59, got %+v, %v", cutoff, got, err)
		}
	}
}

func TestJornadaBoundsAcceptsShortCutoff(t *testing.T) {
	got, err := JornadaBounds("2026-11-25", "2026-11-25", "15:00")
	if err != nil || got.Cutoff != "15:00:00" {
		t.Fatalf("expected 15:00 to mean 15:00:00, got %+v, %v", got, err)
	}
}

func TestJornadaBoundsRejectsInvalidRanges(t *testing.T) {
	cases := [][2]string{
		{"", "2026-11-25"},
		{"2026-11-25", ""},
		{"25/11/2026", "2026-11-25"},
		{"2026-11-26", "2026-11-25"},
		{"2026-01-01", "2026-12-31"}, // longer than MaxDays
	}
	for _, c := range cases {
		if _, err := JornadaBounds(c[0], c[1], "23:59:59"); !errors.Is(err, ErrInvalidRange) {
			t.Errorf("range %v: expected ErrInvalidRange, got %v", c, err)
		}
	}
}

func ptr[T any](v T) *T { return &v }

func at(hhmm string) time.Time {
	t, _ := time.Parse("2006-01-02 15:04", "2026-11-25 "+hhmm)
	return t
}

func summaryFixture() []models.Movement {
	return []models.Movement{
		// Closed with a separate exit ticket: 2 tickets.
		{PagoID: "a", CodigoPresup: 1001, TarifaDescripcion: "VEHICULO LIVIANO", FechaHoraEntra: at("08:00"), FechaHoraSale: ptr(at("09:30")), FechaHoraPago: ptr(at("09:30")),
			SerieEntrada: "E-001", SerieSalida: ptr("S-001"), MontoTotal: 1.50, TiempoMinutos: ptr(uint(90)), UsuarioSalida: ptr("caja1@x.com")},
		// Closed reusing the entry ticket: 1 ticket; no tiempo_minutos, so the stay comes from the dates.
		{PagoID: "b", CodigoPresup: 1002, TarifaDescripcion: "MOTOCICLETA", FechaHoraEntra: at("10:00"), FechaHoraSale: ptr(at("10:30")), FechaHoraPago: ptr(at("10:30")),
			SerieEntrada: "E-002", SerieSalida: ptr("E-002"), MontoTotal: 1.00, UsuarioSalida: ptr("caja2@x.com")},
		// Closed with an empty exit series: 1 ticket.
		{PagoID: "c", CodigoPresup: 1001, TarifaDescripcion: "VEHICULO LIVIANO", FechaHoraEntra: at("11:00"), FechaHoraSale: ptr(at("12:00")), FechaHoraPago: ptr(at("12:00")),
			SerieEntrada: "E-003", SerieSalida: ptr(""), MontoTotal: 0.75, TiempoMinutos: ptr(uint(60)), UsuarioSalida: ptr("caja1@x.com")},
		// Active: 1 ticket, no revenue yet.
		{PagoID: "d", CodigoPresup: 1001, TarifaDescripcion: "VEHICULO LIVIANO", FechaHoraEntra: at("13:00"), SerieEntrada: "E-004"},
		// Annulled from the web: exit date set, no exit ticket, $0.
		{PagoID: "e", CodigoPresup: 1002, TarifaDescripcion: "MOTOCICLETA", FechaHoraEntra: at("14:00"), FechaHoraSale: ptr(at("14:10")), FechaHoraAnula: ptr(at("14:10")),
			SerieEntrada: "E-005", UsuarioSalida: ptr("admin@x.com")},
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize(summaryFixture())

	if s.Movimientos != 5 || s.Cerrados != 3 || s.Activos != 1 || s.Anulados != 1 {
		t.Fatalf("unexpected counts: %+v", s)
	}
	if s.TicketsEntrada != 5 || s.TicketsSalida != 1 || s.TicketsUtilizados != 6 {
		t.Fatalf("expected 5 entry + 1 exit = 6 tickets, got %d + %d = %d", s.TicketsEntrada, s.TicketsSalida, s.TicketsUtilizados)
	}
	if s.Ingresos != 3.25 {
		t.Fatalf("expected revenue 3.25, got %v", s.Ingresos)
	}
	if s.TiempoPromedioMinutos != 60 { // (90 + 30 + 60) / 3 closed, annulled excluded
		t.Fatalf("expected average stay 60, got %v", s.TiempoPromedioMinutos)
	}
}

func TestSummarizeBreakdowns(t *testing.T) {
	s := Summarize(summaryFixture())

	wantRates := []RateBreakdown{
		{CodigoPresup: 1001, Tarifa: "VEHICULO LIVIANO", Movimientos: 3, Ingresos: 2.25},
		{CodigoPresup: 1002, Tarifa: "MOTOCICLETA", Movimientos: 1, Ingresos: 1.00},
	}
	if len(s.PorTarifa) != len(wantRates) {
		t.Fatalf("expected %d rates, got %+v", len(wantRates), s.PorTarifa)
	}
	for i, want := range wantRates {
		if s.PorTarifa[i] != want {
			t.Errorf("rate %d: expected %+v, got %+v", i, want, s.PorTarifa[i])
		}
	}

	wantOperators := []OperatorBreakdown{
		{Usuario: "caja1@x.com", Cobros: 2, Ingresos: 2.25},
		{Usuario: "caja2@x.com", Cobros: 1, Ingresos: 1.00},
	}
	if len(s.PorOperador) != len(wantOperators) {
		t.Fatalf("expected %d operators, got %+v", len(wantOperators), s.PorOperador)
	}
	for i, want := range wantOperators {
		if s.PorOperador[i] != want {
			t.Errorf("operator %d: expected %+v, got %+v", i, want, s.PorOperador[i])
		}
	}
}

func TestSummarizeEmpty(t *testing.T) {
	s := Summarize(nil)

	if s.Movimientos != 0 || s.TiempoPromedioMinutos != 0 || s.PorTarifa == nil || s.PorOperador == nil {
		t.Fatalf("expected zero summary with empty (non-nil) breakdowns, got %+v", s)
	}
}
