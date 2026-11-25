package handlers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/report"
	"github.com/parqueo/api/internal/repository"
)

type reportCall struct {
	locationID uint
	estado     string
	jornada    report.Jornada
	limit      int
}

func stubReport(t *testing.T, ctx repository.ReportContext, ctxErr error, movements []models.Movement, movErr error) *reportCall {
	t.Helper()
	call := &reportCall{}
	origCtx, origMov := getReportContext, getReportMovements
	getReportContext = func(locationID uint) (repository.ReportContext, error) { return ctx, ctxErr }
	getReportMovements = func(locationID uint, estado string, j report.Jornada, limit int) ([]models.Movement, error) {
		*call = reportCall{locationID, estado, j, limit}
		return movements, movErr
	}
	t.Cleanup(func() { getReportContext, getReportMovements = origCtx, origMov })
	return call
}

func reportApp() *fiber.App {
	app := rateChangeApp(service(nil))
	app.Get("/reports/movements", GetMovementsReport)
	return app
}

func TestMovementsReportReturnsHeaderJornadaRowsAndSummary(t *testing.T) {
	ctx := repository.ReportContext{Institucion: "ALCALDIA MUNICIPAL", Ubicacion: "PARQUEO CENTRAL", HoraCorte: "15:00:00"}
	movements := []models.Movement{{PagoID: "a", SerieEntrada: "E-1", CodigoPresup: 1001}}
	call := stubReport(t, ctx, nil, movements, nil)

	status, payload := send(t, reportApp(), "GET", "/reports/movements?fecha_inicio=2026-11-25&fecha_fin=2026-11-25&estado=activos", "")

	if status != fiber.StatusOK {
		t.Fatalf("expected 200, got %d (%v)", status, payload)
	}
	want := map[string]any{
		"institucion":    "ALCALDIA MUNICIPAL",
		"ubicacion":      "PARQUEO CENTRAL",
		"hora_corte":     "15:00:00",
		"jornada_inicio": "2026-11-24 15:00:01",
		"jornada_fin":    "2026-11-25 15:00:00",
		"fecha_inicio":   "2026-11-25",
		"fecha_fin":      "2026-11-25",
		"estado":         "activos",
		"generado_por":   "admin@example.com",
	}
	for key, value := range want {
		if payload[key] != value {
			t.Errorf("%s: expected %v, got %v", key, value, payload[key])
		}
	}
	if rows, _ := payload["movements"].([]any); len(rows) != 1 {
		t.Errorf("expected 1 movement, got %v", payload["movements"])
	}
	if summary, _ := payload["resumen"].(map[string]any); summary["tickets_utilizados"] != float64(1) {
		t.Errorf("expected the summary in the response, got %v", payload["resumen"])
	}
	if call.locationID != 8 || call.estado != "activos" || call.jornada.After != "2026-11-24 15:00:00" || call.limit != maxReportRows+1 {
		t.Errorf("unexpected repository call: %+v", call)
	}
}

func TestMovementsReportDefaultsEstadoToTodos(t *testing.T) {
	call := stubReport(t, repository.ReportContext{}, nil, nil, nil)

	status, payload := send(t, reportApp(), "GET", "/reports/movements?fecha_inicio=2026-11-25&fecha_fin=2026-11-25", "")

	if status != fiber.StatusOK || call.estado != "todos" || payload["hora_corte"] != "23:59:59" {
		t.Fatalf("expected todos with the default cutoff, got %d %v (%+v)", status, payload, call)
	}
	if rows, ok := payload["movements"].([]any); !ok || len(rows) != 0 {
		t.Fatalf("expected an empty movements array, got %v", payload["movements"])
	}
}

func TestMovementsReportRejections(t *testing.T) {
	tooMany := make([]models.Movement, maxReportRows+1)
	cases := []struct {
		name      string
		query     string
		ctxErr    error
		movements []models.Movement
		movErr    error
		status    int
	}{
		{"missing dates", "", nil, nil, nil, fiber.StatusBadRequest},
		{"reversed dates", "fecha_inicio=2026-11-26&fecha_fin=2026-11-25", nil, nil, nil, fiber.StatusBadRequest},
		{"unknown estado", "fecha_inicio=2026-11-25&fecha_fin=2026-11-25&estado=borrados", nil, nil, nil, fiber.StatusBadRequest},
		{"too many rows", "fecha_inicio=2026-11-01&fecha_fin=2026-11-30", nil, tooMany, nil, fiber.StatusUnprocessableEntity},
		{"context failure", "fecha_inicio=2026-11-25&fecha_fin=2026-11-25", errors.New("dial tcp 10.0.0.5:3306"), nil, nil, fiber.StatusInternalServerError},
		{"movements failure", "fecha_inicio=2026-11-25&fecha_fin=2026-11-25", nil, nil, errors.New("dial tcp 10.0.0.5:3306"), fiber.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubReport(t, repository.ReportContext{}, tc.ctxErr, tc.movements, tc.movErr)

			status, payload := send(t, reportApp(), "GET", fmt.Sprintf("/reports/movements?%s", tc.query), "")

			if status != tc.status {
				t.Fatalf("expected %d, got %d (%v)", tc.status, status, payload)
			}
			if payload["error"] == nil || payload["error"] == "" {
				t.Fatalf("expected an error message, got %v", payload)
			}
			if status == fiber.StatusInternalServerError && payload["error"] != "Internal server error" {
				t.Fatalf("expected a generic message, got %v", payload["error"])
			}
		})
	}
}
