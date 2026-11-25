package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// preflight sends a CORS preflight from origin and returns the allowed origin header.
func preflight(t *testing.T, allowedOrigins, origin string) string {
	t.Helper()
	t.Setenv("CORS_ALLOWED_ORIGINS", allowedOrigins)
	app := fiber.New()
	registerCORS(app)
	app.Get("/api/movements", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest("OPTIONS", "/api/movements", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "GET")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	defer resp.Body.Close()
	return resp.Header.Get("Access-Control-Allow-Origin")
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	got := preflight(t, "http://localhost:5173, https://parqueosv.com", "https://parqueosv.com")

	if got != "https://parqueosv.com" {
		t.Fatalf("expected configured origin to be allowed, got %q", got)
	}
}

func TestCORSRejectsUnlistedOrigin(t *testing.T) {
	got := preflight(t, "http://localhost:5173", "https://evil.example")

	if got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin for an unlisted origin, got %q", got)
	}
}

func TestCORSDisabledWhenNoOriginsConfigured(t *testing.T) {
	got := preflight(t, "", "https://evil.example")

	if got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin without configured origins, got %q", got)
	}
}

func TestCORSIgnoresWildcardOrigins(t *testing.T) {
	for _, configured := range []string{"*", "https://*.example", "*, https://*.example"} {
		if got := preflight(t, configured, "https://evil.example"); got != "" {
			t.Fatalf("expected wildcard %q to be ignored, got %q", configured, got)
		}
	}
}

func TestCORSPreflightAllowsAuthorizationHeaderAndPatch(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")
	app := fiber.New()
	registerCORS(app)
	app.Patch("/api/movements/anular/:id", func(c *fiber.Ctx) error { return c.SendString("ok") })

	req := httptest.NewRequest("OPTIONS", "/api/movements/anular/1", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "PATCH")
	req.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("expected dev origin allowed, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "authorization") {
		t.Fatalf("expected Authorization header allowed, got %q", got)
	}
	if got := resp.Header.Get("Access-Control-Allow-Methods"); !strings.Contains(got, "PATCH") {
		t.Fatalf("expected PATCH allowed, got %q", got)
	}
}
