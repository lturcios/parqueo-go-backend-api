package repository

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/ratechange"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// dryRunDB returns a MySQL-dialect GORM handle that only builds SQL and never connects.
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "user:pass@tcp(127.0.0.1:1)/none",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
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

func projectionFixture() []models.Movement {
	exit := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	return []models.Movement{
		{PagoID: "open", FechaHoraEntra: exit.Add(-2 * time.Hour), MontoTotal: 0},
		{PagoID: "closed", FechaHoraEntra: exit.Add(-3 * time.Hour), FechaHoraSale: &exit, FechaHoraPago: &exit, MontoTotal: 2.25},
	}
}

func fixedCalculator(entryTime time.Time, budgetCode int, locationID uint) (float64, uint, error) {
	return 9.99, 0, nil
}

func TestProjectOwedAmountsActiveFilterAlwaysProjects(t *testing.T) {
	movements := projectionFixture()[:1]

	projectOwedAmounts(movements, MovementFilter{Estado: "activos"}, fixedCalculator)

	if movements[0].MontoTotal != 9.99 {
		t.Fatalf("expected projected amount 9.99, got %v", movements[0].MontoTotal)
	}
}

func TestProjectOwedAmountsAllFilterWithoutProjectionKeepsStoredAmounts(t *testing.T) {
	movements := projectionFixture()

	projectOwedAmounts(movements, MovementFilter{Estado: "todos"}, fixedCalculator)

	if movements[0].MontoTotal != 0 || movements[1].MontoTotal != 2.25 {
		t.Fatalf("expected stored amounts 0 and 2.25, got %v and %v", movements[0].MontoTotal, movements[1].MontoTotal)
	}
}

func TestProjectOwedAmountsAllFilterWithProjectionOnlyProjectsOpenMovements(t *testing.T) {
	movements := projectionFixture()

	projectOwedAmounts(movements, MovementFilter{Estado: "todos", Proyeccion: true}, fixedCalculator)

	if movements[0].MontoTotal != 9.99 {
		t.Fatalf("expected open movement projected to 9.99, got %v", movements[0].MontoTotal)
	}
	if movements[1].MontoTotal != 2.25 {
		t.Fatalf("expected closed movement to keep 2.25, got %v", movements[1].MontoTotal)
	}
}

func TestProjectOwedAmountsReturnsCalculatorErrors(t *testing.T) {
	movements := projectionFixture()
	failing := func(entryTime time.Time, budgetCode int, locationID uint) (float64, uint, error) {
		return 0, 0, errors.New("rates query failed")
	}

	if err := projectOwedAmounts(movements, MovementFilter{Estado: "activos"}, failing); err == nil {
		t.Fatal("expected the rate lookup error to be returned")
	}
}

func TestCalculateOwedAmountReturnsDatabaseErrors(t *testing.T) {
	useUnreachableDB(t)

	if _, _, err := CalculateOwedAmount(time.Now().Add(-time.Hour), 1211300101, 1); err == nil {
		t.Fatal("expected an error when the database is unreachable")
	}
}

func TestVehicleRateQueryOnlyMatchesCurrentVehicleRatesOfTheLocation(t *testing.T) {
	var rates []models.Rate
	sql := vehicleRateQuery(dryRunDB(t), 1002, 8).Find(&rates).Statement.SQL.String()

	for _, want := range []string{"codigo_presup = ?", "ubicacion_id_fk = ?", "vigente = 1", "(iconfile IS NULL OR iconfile <= 10)", "ORDER BY tiempo_minimo ASC", "LIMIT ?"} {
		if !strings.Contains(sql, want) {
			t.Errorf("expected %q in vehicle rate query, got: %s", want, sql)
		}
	}
}

func TestRateChangeUpdateOnlyTouchesTheThreeFieldsOfAnOpenTicket(t *testing.T) {
	change := ratechange.Change{RateCode: 1002, UnitPrice: 1, Observations: "Tarifa cambiada de A a B por x el 2026-11-25 10:00"}
	stmt := rateChangeUpdate(dryRunDB(t), 8, "G5V._iBQMhib04_P", change).Statement
	sql := stmt.SQL.String()

	if !strings.HasPrefix(sql, "UPDATE `parkmovimientos` SET ") {
		t.Fatalf("expected an UPDATE of parkmovimientos, got: %s", sql)
	}
	set := sql[len("UPDATE `parkmovimientos` SET "):strings.Index(sql, " WHERE ")]
	for _, column := range strings.Split(set, ",") {
		name := strings.Trim(strings.TrimSpace(strings.SplitN(column, "=", 2)[0]), "`")
		if name != "codigo_presup" && name != "precio_unitario" && name != "observaciones" {
			t.Errorf("unexpected column %q in: %s", name, sql)
		}
	}
	if strings.Count(set, "=") != 3 {
		t.Errorf("expected exactly 3 columns, got: %s", set)
	}
	for _, want := range []string{"pago_id = ?", "ubicacion_id_fk = ?", "fecha_horasale IS NULL", "fecha_horapago IS NULL", "fecha_hora_anula IS NULL"} {
		if !strings.Contains(sql, want) {
			t.Errorf("expected %q in: %s", want, sql)
		}
	}
	if len(stmt.Vars) != 5 {
		t.Errorf("expected 3 values and 2 filters, got %v", stmt.Vars)
	}
}
