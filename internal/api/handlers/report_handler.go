package handlers

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/report"
	"github.com/parqueo/api/internal/repository"
)

// maxReportRows keeps a report within what a browser can render into a PDF comfortably.
const maxReportRows = 10000

// Package-level seams so tests can replace the database lookups.
var (
	getReportContext   = repository.GetReportContext
	getReportMovements = repository.GetReportMovements
)

var reportStates = map[string]bool{"todos": true, "activos": true, "cerrados": true}

// GetMovementsReport returns the data of the movements PDF: header names, the jornada span
// for fecha_inicio..fecha_fin (each jornada ends at the institution's hora_corte), the rows
// and their summary.
func GetMovementsReport(c *fiber.Ctx) error {
	locationID, email := tokenClaims(c)
	fechaInicio, fechaFin := c.Query("fecha_inicio"), c.Query("fecha_fin")
	estado := c.Query("estado", "todos")
	if !reportStates[estado] {
		return c.Status(400).JSON(fiber.Map{"error": "estado must be todos, activos or cerrados"})
	}

	header, err := getReportContext(locationID)
	if err != nil {
		log.Printf("report: location %d context lookup failed: %v", locationID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Internal server error"})
	}

	jornada, err := report.JornadaBounds(fechaInicio, fechaFin, header.HoraCorte)
	if errors.Is(err, report.ErrInvalidRange) {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	movements, err := getReportMovements(locationID, estado, jornada, maxReportRows+1)
	if err != nil {
		log.Printf("report: location %d movements query failed: %v", locationID, err)
		return c.Status(500).JSON(fiber.Map{"error": "Internal server error"})
	}
	if movements == nil {
		movements = []models.Movement{} // always an array for the client
	}
	if len(movements) > maxReportRows {
		return c.Status(422).JSON(fiber.Map{
			"error": fmt.Sprintf("the report has more than %d movements; narrow the date range", maxReportRows),
		})
	}

	return c.JSON(fiber.Map{
		"institucion":    header.Institucion,
		"ubicacion":      header.Ubicacion,
		"hora_corte":     jornada.Cutoff,
		"jornada_inicio": jornada.From(),
		"jornada_fin":    jornada.Until,
		"fecha_inicio":   fechaInicio,
		"fecha_fin":      fechaFin,
		"estado":         estado,
		"generado_por":   email,
		"generado_en":    time.Now().Format("2006-01-02 15:04:05"),
		"movements":      movements,
		"resumen":        report.Summarize(movements),
	})
}
