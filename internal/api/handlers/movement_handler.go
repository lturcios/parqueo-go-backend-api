package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/repository"
	"github.com/spf13/cast"
)

func GetDashboardStats(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	locationID := uint(claims["ubicacion_id"].(float64))

	stats, err := repository.GetDashboardStats(locationID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(stats)
}

func GetDashboardAnalytics(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	locationID := uint(claims["ubicacion_id"].(float64))

	data, err := repository.GetDashboardAnalytics(locationID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(data)
}

func GetMovements(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	locationID := uint(claims["ubicacion_id"].(float64))

	page := cast.ToInt(c.Query("page", "1"))
	pageSize := cast.ToInt(c.Query("pageSize", "15"))

	filter := repository.MovementFilter{
		FechaInicio: c.Query("fecha_inicio"),
		FechaFin:    c.Query("fecha_fin"),
		Estado:      c.Query("estado"),
		Tipo:        c.Query("tipo", "normal"),
		TarifaID:    uint(cast.ToInt(c.Query("tarifa_id", "0"))),
		UbicacionID: locationID,
	}

	movements, totalCount, totalIngresos, err := repository.GetMovements(filter, page, pageSize)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"movements":       movements,
		"total_count":     totalCount,
		"total_ingresos": totalIngresos,
		"page":           page,
		"page_size":      pageSize,
	})
}

func GetRates(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	locationID := uint(claims["ubicacion_id"].(float64))

	rates, err := repository.GetRatesByLocation(locationID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(rates)
}

func GetOtherRates(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	locationID := uint(claims["ubicacion_id"].(float64))

	rates, err := repository.GetOtherIncomeRatesByLocation(locationID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(rates)
}

func RegisterEntry(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	locationID := uint(claims["ubicacion_id"].(float64))
	userEmail := claims["email"].(string)

	var movement models.Movement
	if err := c.BodyParser(&movement); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	movement.UbicacionID = locationID
	movement.UsuarioEntrada = userEmail
	
	rate, err := repository.GetRate(int(movement.CodigoPresup), locationID)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid rate code"})
	}
	movement.PrecioUnitario = rate.PrecioUnitario

	if err := repository.RegisterEntry(&movement); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(movement)
}

func RegisterExit(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userEmail := claims["email"].(string)

	pagoID := c.Params("id")
	if pagoID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "ID is required"})
	}

	movement, err := repository.RegisterExit(pagoID, userEmail)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(movement)
}

func RegisterAnnulment(c *fiber.Ctx) error {
	user := c.Locals("user").(*jwt.Token)
	claims := user.Claims.(jwt.MapClaims)
	userEmail := claims["email"].(string)

	pagoID := c.Params("id")
	if pagoID == "" {
		return c.Status(400).JSON(fiber.Map{"error": "ID is required"})
	}

	movement, err := repository.RegisterAnnulment(pagoID, userEmail)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(movement)
}
