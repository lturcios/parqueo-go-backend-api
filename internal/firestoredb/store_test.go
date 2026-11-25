package firestoredb

import (
	"testing"

	"github.com/parqueo/api/internal/ratechange"
)

func configData() map[string]any {
	return map[string]any{
		"ubicaciones": []any{"1", "2", "8", "11", "12"},
		"prefijos": []any{map[string]any{
			"1": "SM-", "2": "CJ-", "6": "RI-", "7": "", "8": "LT-", "11": "RI-",
		}},
	}
}

func TestMovementsDocFromConfig(t *testing.T) {
	cases := []struct {
		location uint
		wantDoc  string
		wantOK   bool
	}{
		{8, "LT-8", true},
		{11, "RI-11", true},
		{1, "SM-1", true},
		{6, "", false},  // has a prefix but is not connected to Firestore
		{12, "", false}, // connected but without a prefix
		{3, "", false},  // neither
	}
	for _, tc := range cases {
		doc, ok := movementsDocFromConfig(configData(), tc.location)
		if doc != tc.wantDoc || ok != tc.wantOK {
			t.Errorf("location %d: expected (%q, %v), got (%q, %v)", tc.location, tc.wantDoc, tc.wantOK, doc, ok)
		}
	}
}

func TestMovementsDocFromConfigAcceptsNumericIDsAndEmptyPrefix(t *testing.T) {
	data := map[string]any{
		"ubicaciones": []any{int64(7), float64(8)},
		"prefijos":    []any{map[string]any{"7": "", "8": "LT-"}},
	}

	if doc, ok := movementsDocFromConfig(data, 7); !ok || doc != "7" {
		t.Errorf("expected (\"7\", true), got (%q, %v)", doc, ok)
	}
	if doc, ok := movementsDocFromConfig(data, 8); !ok || doc != "LT-8" {
		t.Errorf("expected (\"LT-8\", true), got (%q, %v)", doc, ok)
	}
}

func TestMovementsDocFromMalformedConfig(t *testing.T) {
	for _, data := range []map[string]any{
		nil,
		{"ubicaciones": "8"},
		{"ubicaciones": []any{"8"}},
		{"ubicaciones": []any{"8"}, "prefijos": []any{}},
		{"ubicaciones": []any{"8"}, "prefijos": []any{"LT-"}},
	} {
		if doc, ok := movementsDocFromConfig(data, 8); ok {
			t.Errorf("expected unavailable for %v, got %q", data, doc)
		}
	}
}

func TestTicketFromData(t *testing.T) {
	active := map[string]any{
		"codigoPresupuestario": int64(1001),
		"fechaHoraentra":       "2026-11-24T23:13:03",
		"fechaHorasale":        nil,
		"fechaHorapago":        nil,
		"fechaHoraAnula":       nil,
		"observaciones":        nil,
		"placa":                "P3650",
	}
	if got := ticketFromData(active); got != (ratechange.Ticket{RateCode: 1001, Active: true}) {
		t.Errorf("unexpected active ticket: %+v", got)
	}

	closed := map[string]any{
		"codigoPresupuestario": float64(1001),
		"fechaHorasale":        "2026-11-24T23:14:42",
		"fechaHorapago":        "2026-11-24T23:14:42",
		"observaciones":        "NOTA",
		"placa":                "P3650",
	}
	if got := ticketFromData(closed); got != (ratechange.Ticket{RateCode: 1001, Observations: "NOTA"}) {
		t.Errorf("unexpected closed ticket: %+v", got)
	}

	annulled := map[string]any{"codigoPresupuestario": int64(1001), "fechaHoraAnula": "2026-11-24T23:14:42"}
	if ticketFromData(annulled).Active {
		t.Error("expected an annulled ticket to be inactive")
	}

	otros := map[string]any{"codigoPresupuestario": int64(2001), "placa": "OTROS", "fechaHorasale": ""}
	if got := ticketFromData(otros); !got.OtherIncome || !got.Active {
		t.Errorf("expected an active other-income ticket, got %+v", got)
	}
}
