package handlers

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/parqueo/api/internal/ratechange"
)

// firestoreTimeout bounds every Firestore round trip a request makes.
const firestoreTimeout = 15 * time.Second

type changeRateRequest struct {
	CodigoPresup int `json:"codigo_presup"`
}

func tokenClaims(c *fiber.Ctx) (locationID uint, email string) {
	claims := c.Locals("user").(*jwt.Token).Claims.(jwt.MapClaims)
	location, _ := claims["ubicacion_id"].(float64)
	email, _ = claims["email"].(string)
	return uint(location), email
}

// GetFeatures tells the web which optional actions the user's location supports.
func GetFeatures(svc *ratechange.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		locationID, _ := tokenClaims(c)
		ctx, cancel := context.WithTimeout(c.UserContext(), firestoreTimeout)
		defer cancel()

		available, err := svc.Available(ctx, locationID)
		if err != nil {
			// The rest of the dashboard keeps working; only the rate change stays hidden.
			log.Printf("features: Firestore config lookup failed: %v", err)
			available = false
		}
		return c.JSON(fiber.Map{"cambio_tarifa": available})
	}
}

// ChangeRate sets a new rate on an active ticket of a Firestore client.
func ChangeRate(svc *ratechange.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req changeRateRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Cannot parse JSON"})
		}
		if req.CodigoPresup <= 0 {
			return c.Status(400).JSON(fiber.Map{"error": "codigo_presup is required"})
		}

		locationID, email := tokenClaims(c)
		pagoID := c.Params("pagoId")
		ctx, cancel := context.WithTimeout(c.UserContext(), firestoreTimeout)
		defer cancel()

		result, err := svc.ChangeRate(ctx, ratechange.Request{
			LocationID:  locationID,
			PagoID:      pagoID,
			NewRateCode: req.CodigoPresup,
			UserEmail:   email,
		})
		switch {
		case err == nil:
			if result.MySQLErr != nil {
				// Firestore already has the change; the Cloud Function sync remains the fallback.
				log.Printf("change rate: movement %s at location %d stored in Firestore, MySQL copy failed: %v", pagoID, locationID, result.MySQLErr)
			}
			return c.JSON(fiber.Map{
				"pago_id":           pagoID,
				"codigo_presup":     req.CodigoPresup,
				"mysql_actualizado": result.MySQLUpdated,
			})
		case errors.Is(err, ratechange.ErrUnavailable):
			return c.Status(403).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, ratechange.ErrNotFound):
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, ratechange.ErrNotActive), errors.Is(err, ratechange.ErrNotVehicle):
			return c.Status(409).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, ratechange.ErrInvalidRate), errors.Is(err, ratechange.ErrSameRate):
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		default:
			log.Printf("change rate: movement %s at location %d failed: %v", pagoID, locationID, err)
			return c.Status(500).JSON(fiber.Map{"error": "Internal server error"})
		}
	}
}
