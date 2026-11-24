package repository

import (
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// dryRunDB returns a MySQL-dialect GORM handle that only builds SQL and never connects.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "user:pass@tcp(127.0.0.1:1)/none",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return db
}

// Rates with several tiers share codigo_presup; joining parktarifas used to
// multiply each movement by its number of tiers, inflating the count and revenue sum.
func TestMovementsQueryDoesNotMultiplyRowsByRateTiers(t *testing.T) {
	filters := []MovementFilter{
		{UbicacionID: 1},
		{UbicacionID: 1, Estado: "activos", FechaInicio: "2026-11-24", FechaFin: "2026-11-24"},
		{UbicacionID: 1, Tipo: "otros", TarifaID: 5},
	}

	for _, filter := range filters {
		db := dryRunDB(t)
		var count int64
		countSQL := movementsQuery(db, filter).Count(&count).Statement.SQL.String()
		if strings.Contains(strings.ToUpper(countSQL), "JOIN") {
			t.Errorf("count query must not join rate tiers, got: %s", countSQL)
		}

		var rows []map[string]any
		pageSQL := movementsQuery(dryRunDB(t), filter).Select(movementsSelect).Find(&rows).Statement.SQL.String()
		if strings.Contains(strings.ToUpper(pageSQL), "JOIN") {
			t.Errorf("page query must not join rate tiers, got: %s", pageSQL)
		}
		if !strings.Contains(pageSQL, "LIMIT 1) AS tarifa_descripcion") {
			t.Errorf("page query must pick a single rate description per movement, got: %s", pageSQL)
		}
	}
}
