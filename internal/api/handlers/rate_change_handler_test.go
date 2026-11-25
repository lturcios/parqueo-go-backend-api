package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/parqueo/api/internal/ratechange"
)

type stubStore struct {
	connected bool
	configErr error
	updateErr error
	ticket    ratechange.Ticket
	written   *ratechange.Change
}

func (s *stubStore) MovementsDoc(ctx context.Context, locationID uint) (string, bool, error) {
	return "LT-8", s.connected, s.configErr
}

func (s *stubStore) UpdateTicket(ctx context.Context, docID, pagoID string, apply func(ratechange.Ticket) (ratechange.Change, error)) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	change, err := apply(s.ticket)
	if err != nil {
		return err
	}
	s.written = &change
	return nil
}

type stubRates struct{}

func (stubRates) VehicleRate(code int, locationID uint) (ratechange.Rate, bool, error) {
	if code == 1002 {
		return ratechange.Rate{Description: "MOTOCICLETA", UnitPrice: 1}, true, nil
	}
	return ratechange.Rate{}, false, nil
}

func (stubRates) Description(code int, locationID uint) (string, error) {
	return "VEHICULO LIVIANO", nil
}

func rateChangeApp(svc *ratechange.Service) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		token := jwt.New(jwt.SigningMethodHS256)
		token.Claims = jwt.MapClaims{"email": "admin@example.com", "ubicacion_id": float64(8)}
		c.Locals("user", token)
		return c.Next()
	})
	app.Get("/features", GetFeatures(svc))
	app.Patch("/movements/:pagoId/tarifa", ChangeRate(svc))
	return app
}

func send(t *testing.T, app *fiber.App, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, payload
}

func service(store ratechange.Store) *ratechange.Service {
	return &ratechange.Service{Store: store, Rates: stubRates{}, Now: time.Now}
}

func TestChangeRateEndpointWritesTheChange(t *testing.T) {
	store := &stubStore{connected: true, ticket: ratechange.Ticket{RateCode: 1001, Active: true}}

	status, payload := send(t, rateChangeApp(service(store)), "PATCH", "/movements/G5V._iBQMhib04_P/tarifa", `{"codigo_presup":1002}`)

	if status != fiber.StatusOK {
		t.Fatalf("expected 200, got %d (%v)", status, payload)
	}
	if store.written == nil || store.written.RateCode != 1002 || store.written.UnitPrice != 1 {
		t.Fatalf("expected rate 1002 at $1 written, got %+v", store.written)
	}
	if payload["codigo_presup"] != float64(1002) {
		t.Fatalf("expected codigo_presup 1002 in response, got %v", payload)
	}
}

func TestChangeRateEndpointErrorMapping(t *testing.T) {
	active := ratechange.Ticket{RateCode: 1001, Active: true}
	cases := []struct {
		name   string
		store  ratechange.Store
		body   string
		status int
	}{
		{"malformed body", &stubStore{connected: true, ticket: active}, `{`, fiber.StatusBadRequest},
		{"missing rate", &stubStore{connected: true, ticket: active}, `{}`, fiber.StatusBadRequest},
		{"unknown rate", &stubStore{connected: true, ticket: active}, `{"codigo_presup":7777}`, fiber.StatusBadRequest},
		{"same rate", &stubStore{connected: true, ticket: ratechange.Ticket{RateCode: 1002, Active: true}}, `{"codigo_presup":1002}`, fiber.StatusBadRequest},
		{"closed vehicle", &stubStore{connected: true, ticket: ratechange.Ticket{RateCode: 1001}}, `{"codigo_presup":1002}`, fiber.StatusConflict},
		{"other income", &stubStore{connected: true, ticket: ratechange.Ticket{RateCode: 1001, Active: true, OtherIncome: true}}, `{"codigo_presup":1002}`, fiber.StatusConflict},
		{"missing movement", &stubStore{connected: true, updateErr: ratechange.ErrNotFound}, `{"codigo_presup":1002}`, fiber.StatusNotFound},
		{"location without Firestore", &stubStore{connected: false}, `{"codigo_presup":1002}`, fiber.StatusForbidden},
		{"Firestore not configured", nil, `{"codigo_presup":1002}`, fiber.StatusForbidden},
		{"Firestore failure", &stubStore{connected: true, updateErr: errors.New("rpc error: code = Unavailable desc = 10.0.0.5")}, `{"codigo_presup":1002}`, fiber.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := service(tc.store)
			if tc.store == nil {
				svc.Store = nil
			}

			status, payload := send(t, rateChangeApp(svc), "PATCH", "/movements/G5V._iBQMhib04_P/tarifa", tc.body)

			if status != tc.status {
				t.Fatalf("expected %d, got %d (%v)", tc.status, status, payload)
			}
			if payload["error"] == nil || payload["error"] == "" {
				t.Fatalf("expected an error message, got %v", payload)
			}
			if status == fiber.StatusInternalServerError && payload["error"] != "Internal server error" {
				t.Fatalf("expected a generic message without Firestore details, got %v", payload["error"])
			}
		})
	}
}

func TestFeaturesEndpointReportsAvailability(t *testing.T) {
	cases := []struct {
		name  string
		store ratechange.Store
		want  bool
	}{
		{"connected location", &stubStore{connected: true}, true},
		{"location without Firestore", &stubStore{connected: false}, false},
		{"Firestore failure", &stubStore{configErr: errors.New("unavailable")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, payload := send(t, rateChangeApp(service(tc.store)), "GET", "/features", "")

			if status != fiber.StatusOK || payload["cambio_tarifa"] != tc.want {
				t.Fatalf("expected 200 with cambio_tarifa=%v, got %d %v", tc.want, status, payload)
			}
		})
	}

	svc := service(nil)
	svc.Store = nil
	if _, payload := send(t, rateChangeApp(svc), "GET", "/features", ""); payload["cambio_tarifa"] != false {
		t.Fatalf("expected cambio_tarifa=false without Firestore, got %v", payload)
	}
}

type stubMirror struct {
	updated bool
	err     error
}

func (m stubMirror) ApplyRateChange(ctx context.Context, locationID uint, pagoID string, change ratechange.Change) (bool, error) {
	return m.updated, m.err
}

func TestChangeRateEndpointReportsMySQLCopy(t *testing.T) {
	cases := []struct {
		name   string
		mirror stubMirror
		want   bool
	}{
		{"MySQL updated", stubMirror{updated: true}, true},
		{"movement not in MySQL yet", stubMirror{updated: false}, false},
		{"MySQL failure keeps the Firestore change", stubMirror{err: errors.New("mysql down")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubStore{connected: true, ticket: ratechange.Ticket{RateCode: 1001, Active: true}}
			svc := service(store)
			svc.Mirror = tc.mirror

			status, payload := send(t, rateChangeApp(svc), "PATCH", "/movements/G5V._iBQMhib04_P/tarifa", `{"codigo_presup":1002}`)

			if status != fiber.StatusOK {
				t.Fatalf("expected 200, got %d (%v)", status, payload)
			}
			if payload["mysql_actualizado"] != tc.want {
				t.Fatalf("expected mysql_actualizado=%v, got %v", tc.want, payload)
			}
			if store.written == nil {
				t.Fatal("expected the Firestore change to be written")
			}
		})
	}
}
