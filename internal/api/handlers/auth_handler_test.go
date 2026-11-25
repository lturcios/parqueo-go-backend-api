package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/parqueo/api/internal/domain/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func stubUser(t *testing.T, storedPassword string) {
	t.Helper()
	original := getUserByEmail
	getUserByEmail = func(email string) (*models.User, error) {
		return &models.User{Email: email, Nombre: "Admin", Password: storedPassword, UbicacionID: 1, InstitucionID: 1}, nil
	}
	t.Cleanup(func() { getUserByEmail = original })
}

func postLogin(t *testing.T, email, password string) (int, map[string]any) {
	t.Helper()
	t.Setenv("JWT_SECRET", "test-secret")
	app := fiber.New()
	app.Post("/login", Login)

	body, err := json.Marshal(LoginRequest{Email: email, Password: password})
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	req := httptest.NewRequest("POST", "/login", bytes.NewReader(body))
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

func TestLoginRejectsPlainTextStoredPassword(t *testing.T) {
	stubUser(t, "secret123")

	status, payload := postLogin(t, "admin@example.com", "secret123")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
	if payload["error"] != "Invalid credentials" {
		t.Fatalf("expected Invalid credentials error, got %v", payload["error"])
	}
	if _, ok := payload["token"]; ok {
		t.Fatal("expected no token in response")
	}
}

func TestLoginAcceptsCorrectBcryptPassword(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	stubUser(t, string(hash))

	status, payload := postLogin(t, "admin@example.com", "secret123")

	if status != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if token, _ := payload["token"].(string); token == "" {
		t.Fatal("expected a non-empty token")
	}
	user, ok := payload["user"].(map[string]any)
	if !ok {
		t.Fatalf("expected user object, got %v", payload["user"])
	}
	want := map[string]any{
		"email":          "admin@example.com",
		"nombre":         "Admin",
		"ubicacion_id":   float64(1),
		"institucion_id": float64(1),
	}
	for key, value := range want {
		if user[key] != value {
			t.Fatalf("user.%s: expected %v, got %v", key, value, user[key])
		}
	}
}

func TestLoginRejectsWrongPasswordForBcryptHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	stubUser(t, string(hash))

	status, payload := postLogin(t, "admin@example.com", "wrong")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
	if payload["error"] != "Invalid credentials" {
		t.Fatalf("expected Invalid credentials error, got %v", payload["error"])
	}
}

func stubLookupError(t *testing.T, lookupErr error) {
	t.Helper()
	original := getUserByEmail
	getUserByEmail = func(email string) (*models.User, error) { return nil, lookupErr }
	t.Cleanup(func() { getUserByEmail = original })
}

func TestLoginRejectsUnknownEmail(t *testing.T) {
	stubLookupError(t, gorm.ErrRecordNotFound)

	status, payload := postLogin(t, "nobody@example.com", "secret123")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", status)
	}
	if payload["error"] != "Invalid credentials" {
		t.Fatalf("expected Invalid credentials error, got %v", payload["error"])
	}
}

func TestLoginReportsDatabaseFailureAsServerError(t *testing.T) {
	stubLookupError(t, errors.New("dial tcp 10.0.0.5:3306: connect: connection refused"))

	status, payload := postLogin(t, "admin@example.com", "secret123")

	if status != fiber.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", status)
	}
	if payload["error"] != "Internal server error" {
		t.Fatalf("expected a generic error without database details, got %v", payload["error"])
	}
	if _, ok := payload["token"]; ok {
		t.Fatal("expected no token in response")
	}
}
