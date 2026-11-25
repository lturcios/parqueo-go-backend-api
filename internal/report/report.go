// Package report builds the data of the movements report: the business-day (jornada) bounds
// and the summary printed on top of the PDF.
package report

import (
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/parqueo/api/internal/domain/models"
)

// MaxDays limits a report to about one quarter of jornadas.
const MaxDays = 93

// DefaultCutoff ends the jornada at midnight, which makes it a calendar day.
const DefaultCutoff = "23:59:59"

var ErrInvalidRange = errors.New("fecha_inicio and fecha_fin must be YYYY-MM-DD, in order, and at most 93 days apart")

const (
	dateLayout     = "2006-01-02"
	dateTimeLayout = "2006-01-02 15:04:05"
)

// Jornada is the time span of a range of business days. Each jornada ends at the institution's
// cutoff (hora_corte) and the next one starts a second later, so the span is (After, Until].
type Jornada struct {
	After  string `json:"-"`
	Until  string `json:"jornada_fin"`
	Cutoff string `json:"hora_corte"`
}

// From is the first instant of the span, for display.
func (j Jornada) From() string {
	after, err := time.Parse(dateTimeLayout, j.After)
	if err != nil {
		return j.After
	}
	return after.Add(time.Second).Format(dateTimeLayout)
}

// normalizeCutoff accepts "HH:MM" or "HH:MM:SS" (fractions ignored) and falls back to
// DefaultCutoff for an empty or malformed value, the setting of a regular parking.
func normalizeCutoff(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		raw = raw[:i]
	}
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.Format("15:04:05")
		}
	}
	return DefaultCutoff
}

// JornadaBounds returns the span of the jornadas fechaInicio..fechaFin: the jornada of day D
// runs from the cutoff of D-1 (exclusive) to the cutoff of D (inclusive).
func JornadaBounds(fechaInicio, fechaFin, cutoff string) (Jornada, error) {
	start, err := time.Parse(dateLayout, fechaInicio)
	if err != nil {
		return Jornada{}, ErrInvalidRange
	}
	end, err := time.Parse(dateLayout, fechaFin)
	if err != nil || end.Before(start) || end.Sub(start) > (MaxDays-1)*24*time.Hour {
		return Jornada{}, ErrInvalidRange
	}
	cut := normalizeCutoff(cutoff)
	return Jornada{
		After:  start.AddDate(0, 0, -1).Format(dateLayout) + " " + cut,
		Until:  end.Format(dateLayout) + " " + cut,
		Cutoff: cut,
	}, nil
}

type RateBreakdown struct {
	CodigoPresup uint    `json:"codigo_presup"`
	Tarifa       string  `json:"tarifa"`
	Movimientos  int     `json:"movimientos"`
	Ingresos     float64 `json:"ingresos"`
}

type OperatorBreakdown struct {
	Usuario  string  `json:"usuario"`
	Cobros   int     `json:"cobros"`
	Ingresos float64 `json:"ingresos"`
}

type Summary struct {
	Movimientos           int                 `json:"movimientos"`
	Cerrados              int                 `json:"cerrados"`
	Activos               int                 `json:"activos"`
	Anulados              int                 `json:"anulados"`
	TicketsEntrada        int                 `json:"tickets_entrada"`
	TicketsSalida         int                 `json:"tickets_salida"`
	TicketsUtilizados     int                 `json:"tickets_utilizados"`
	Ingresos              float64             `json:"ingresos"`
	TiempoPromedioMinutos float64             `json:"tiempo_promedio_minutos"`
	PorTarifa             []RateBreakdown     `json:"por_tarifa"`
	PorOperador           []OperatorBreakdown `json:"por_operador"`
}

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}

// exitTicket reports whether the exit printed its own ticket: a movement whose exit series
// differs from its entry series used two tickets.
func exitTicket(m models.Movement) bool {
	if m.SerieSalida == nil {
		return false
	}
	exit := strings.TrimSpace(*m.SerieSalida)
	return exit != "" && exit != strings.TrimSpace(m.SerieEntrada)
}

func stayMinutes(m models.Movement) float64 {
	if m.TiempoMinutos != nil {
		return float64(*m.TiempoMinutos)
	}
	return m.FechaHoraSale.Sub(m.FechaHoraEntra).Minutes()
}

// Summarize counts movements and tickets, adds up revenue and builds the rate and operator
// breakdowns. Annulled movements count as movements and tickets but not as revenue or stays.
func Summarize(movements []models.Movement) Summary {
	s := Summary{Movimientos: len(movements), PorTarifa: []RateBreakdown{}, PorOperador: []OperatorBreakdown{}}
	rates := map[uint]*RateBreakdown{}
	operators := map[string]*OperatorBreakdown{}
	var totalStay float64

	for _, m := range movements {
		s.TicketsEntrada++
		if exitTicket(m) {
			s.TicketsSalida++
		}

		switch {
		case m.FechaHoraAnula != nil:
			s.Anulados++
			continue
		case m.FechaHoraSale == nil:
			s.Activos++
		default:
			s.Cerrados++
			s.Ingresos += m.MontoTotal
			totalStay += stayMinutes(m)
			user := ""
			if m.UsuarioSalida != nil {
				user = *m.UsuarioSalida
			}
			op := operators[user]
			if op == nil {
				op = &OperatorBreakdown{Usuario: user}
				operators[user] = op
			}
			op.Cobros++
			op.Ingresos += m.MontoTotal
		}

		rate := rates[m.CodigoPresup]
		if rate == nil {
			rate = &RateBreakdown{CodigoPresup: m.CodigoPresup, Tarifa: m.TarifaDescripcion}
			rates[m.CodigoPresup] = rate
		}
		rate.Movimientos++
		if m.FechaHoraSale != nil {
			rate.Ingresos += m.MontoTotal
		}
	}

	s.TicketsUtilizados = s.TicketsEntrada + s.TicketsSalida
	s.Ingresos = roundCents(s.Ingresos)
	if s.Cerrados > 0 {
		s.TiempoPromedioMinutos = math.Round(totalStay / float64(s.Cerrados))
	}

	for _, r := range rates {
		r.Ingresos = roundCents(r.Ingresos)
		s.PorTarifa = append(s.PorTarifa, *r)
	}
	sort.Slice(s.PorTarifa, func(i, j int) bool {
		a, b := s.PorTarifa[i], s.PorTarifa[j]
		if a.Movimientos != b.Movimientos {
			return a.Movimientos > b.Movimientos
		}
		return a.CodigoPresup < b.CodigoPresup
	})

	for _, o := range operators {
		o.Ingresos = roundCents(o.Ingresos)
		s.PorOperador = append(s.PorOperador, *o)
	}
	sort.Slice(s.PorOperador, func(i, j int) bool {
		a, b := s.PorOperador[i], s.PorOperador[j]
		if a.Ingresos != b.Ingresos {
			return a.Ingresos > b.Ingresos
		}
		return a.Usuario < b.Usuario
	})
	return s
}
