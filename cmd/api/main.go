package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/parqueo/api/internal/api/handlers"
	"github.com/parqueo/api/internal/api/middleware"
	"github.com/parqueo/api/internal/database"
)

func main() {
	// Initialize Database
	database.ConnectDB()

	app := fiber.New()

	// Middleware
	app.Use(logger.New())
	app.Use(cors.New())

	// Routes
	api := app.Group("/api")

	// Auth
	api.Post("/login", handlers.Login)

	// Protected Routes
	api.Use(middleware.AuthMiddleware())

	// Movements & Stats
	api.Get("/dashboard/stats", handlers.GetDashboardStats)
	api.Get("/dashboard/analytics", handlers.GetDashboardAnalytics)
	api.Get("/movements", handlers.GetMovements)
	api.Post("/movements/entrada", handlers.RegisterEntry)
	api.Patch("/movements/salida/:id", handlers.RegisterExit)
	api.Patch("/movements/anular/:id", handlers.RegisterAnnulment)
	api.Get("/rates", handlers.GetRates)
	api.Get("/rates/other", handlers.GetOtherRates)

	// Start Server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Fatal(app.Listen(":" + port))
}
