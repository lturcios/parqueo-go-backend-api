package main

import (
	"os"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// registerCORS allows cross-origin requests only from the comma-separated origins in
// CORS_ALLOWED_ORIGINS (e.g. "http://localhost:5173" for the Vite dev server).
// When it is empty, CORS stays disabled: in production nginx serves the frontend and
// proxies /api from the same origin, so browsers never need it.
// Each entry must be scheme://host[:port]; Fiber panics at startup on a malformed one.
func registerCORS(app *fiber.App) {
	origins := allowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	if origins == "" {
		return
	}
	app.Use(cors.New(cors.Config{AllowOrigins: origins}))
}

// allowedOrigins trims each entry and drops empty ones and any wildcard ("*" or
// "https://*.example.com"), so the list can only name exact origins.
func allowedOrigins(raw string) string {
	var origins []string
	for _, origin := range strings.Split(raw, ",") {
		origin = strings.TrimSpace(origin)
		if origin == "" || strings.Contains(origin, "*") {
			continue
		}
		origins = append(origins, origin)
	}
	return strings.Join(origins, ",")
}
