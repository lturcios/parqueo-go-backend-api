package handlers

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v4"
	"github.com/parqueo/api/internal/repository"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// getUserByEmail is a package-level seam so tests can replace the database lookup.
var getUserByEmail = repository.GetUserByEmail

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	user, err := getUserByEmail(req.Email)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid credentials"})
	}
	if err != nil {
		// A database failure is not a wrong password: report it as such, without leaking details.
		log.Printf("login: user lookup failed: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "Internal server error"})
	}

	// Only bcrypt hashes are accepted; plain-text stored passwords are rejected.
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid credentials"})
	}

	// Create JWT token
	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["email"] = user.Email
	claims["ubicacion_id"] = user.UbicacionID
	claims["exp"] = time.Now().Add(time.Hour * 15).Unix()

	t, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "Could not generate token"})
	}

	return c.JSON(fiber.Map{
		"token": t,
		"user": fiber.Map{
			"email":          user.Email,
			"nombre":         user.Nombre,
			"ubicacion_id":   user.UbicacionID,
			"institucion_id": user.InstitucionID,
		},
	})
}
