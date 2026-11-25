package repository

import (
	"math"
	"time"

	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/ratechange"
	"gorm.io/gorm"
)

func GetMovementByID(pagoID string) (*models.Movement, error) {
	var movement models.Movement
	err := database.DB.Where("pago_id = ?", pagoID).First(&movement).Error
	if err != nil {
		return nil, err
	}
	return &movement, nil
}

// CalculateOwedAmount returns the amount owed if the vehicle left now and the minutes elapsed.
// A rate without configured tiers owes nothing; a failed rate lookup returns its error.
func CalculateOwedAmount(entryTime time.Time, budgetCode int, locationID uint) (float64, uint, error) {
	rates, err := GetRatesForCalculation(budgetCode, locationID)
	if err != nil {
		return 0, 0, err
	}
	if len(rates) == 0 {
		return 0, 0, nil
	}

	diff := time.Now().Sub(entryTime).Minutes()
	totalMinutes := uint(diff)
	
	// Por ahora asumimos Periodo 'H' (Horas). Si es 'D' (Días) la lógica se escala.
	// Redondeo hacia arriba a la unidad superior
	totalUnits := int(math.Ceil(diff / 60.0))
	if totalUnits == 0 {
		totalUnits = 1
	}

	var totalOwed float64
	remainingUnits := totalUnits

	for _, rate := range rates {
		if remainingUnits <= 0 {
			break
		}

		// Capacidad de este tramo
		tierSize := (rate.TiempoMaximo - rate.TiempoMinimo) + 1
		
		unitsInThisTier := 0
		if remainingUnits > tierSize {
			unitsInThisTier = tierSize
		} else {
			unitsInThisTier = remainingUnits
		}

		totalOwed += float64(unitsInThisTier) * rate.PrecioUnitario
		remainingUnits -= unitsInThisTier
	}

	// Si sobran unidades y ya no hay más tramos, usamos el precio del último tramo encontrado
	if remainingUnits > 0 && len(rates) > 0 {
		lastRate := rates[len(rates)-1]
		totalOwed += float64(remainingUnits) * lastRate.PrecioUnitario
	}

	return totalOwed, totalMinutes, nil
}

func RegisterAnnulment(pagoID string, userEmail string) (*models.Movement, error) {
	movement, err := GetMovementByID(pagoID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	obs := "TICKET ANULADO"
	
	movement.FechaHoraSale = &now
	movement.FechaHoraAnula = &now
	movement.UsuarioSalida = &userEmail
	movement.MontoTotal = 0
	movement.Observaciones = &obs

	err = database.DB.Save(movement).Error
	return movement, err
}

type MovementFilter struct {
	FechaInicio string
	FechaFin    string
	Estado      string
	UbicacionID uint
	Tipo        string // "normal" u "otros"
	TarifaID    uint   // Filtro específico para otros ingresos
	Proyeccion  bool   // With Estado "todos", project the owed amount of open movements
}

// movementsSelect picks one rate description per movement. A codigo_presup can have
// several tiers (rows in parktarifas), so joining the table would duplicate movements;
// the current (vigente) rate and its first tier win.
const movementsSelect = "p.*, (SELECT pt.descripcion FROM parktarifas pt" +
	" WHERE pt.codigo_presup = p.codigo_presup AND pt.ubicacion_id_fk = p.ubicacion_id_fk" +
	" ORDER BY pt.vigente DESC, pt.tiempo_minimo ASC LIMIT 1) AS tarifa_descripcion"

// movementsQuery builds the filtered base query shared by the count, the revenue sum and the page.
func movementsQuery(db *gorm.DB, filter MovementFilter) *gorm.DB {
	tx := db.Table("parkmovimientos as p").
		Where("p.ubicacion_id_fk = ?", filter.UbicacionID)

	// Filtrar por tipo (normal vs otros)
	if filter.Tipo == "otros" {
		tx = tx.Where("LOWER(p.placa) = ?", "otros")
		if filter.TarifaID > 0 {
			tx = tx.Where("p.codigo_presup = ?", filter.TarifaID)
		}
	} else {
		tx = tx.Where("LOWER(p.placa) != ?", "otros")
	}

	if filter.FechaInicio != "" && filter.FechaFin != "" {
		if filter.FechaInicio == filter.FechaFin {
			tx = tx.Where("DATE(p.fecha_horaentra) = ?", filter.FechaInicio)
		} else {
			tx = tx.Where("DATE(p.fecha_horaentra) BETWEEN ? AND ?", filter.FechaInicio, filter.FechaFin)
		}
	}

	switch filter.Estado {
	case "activos":
		tx = tx.Where("p.fecha_horasale IS NULL")
	case "cerrados":
		tx = tx.Where("p.fecha_horasale IS NOT NULL")
	}

	return tx
}

func GetMovements(filter MovementFilter, page int, pageSize int) ([]models.Movement, int64, float64, error) {
	movements := []models.Movement{}
	var totalCount int64
	var totalIngresos float64

	// 1. Construir la consulta base con los filtros comunes
	tx := movementsQuery(database.DB, filter)

	// 2. Ejecutar conteo total usando una sesión independiente
	if err := tx.Session(&gorm.Session{}).Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// 3. Calcular ingresos totales usando una sesión independiente
	if err := tx.Session(&gorm.Session{}).Select("IFNULL(SUM(p.monto_total), 0)").Row().Scan(&totalIngresos); err != nil {
		return nil, 0, 0, err
	}

	// 4. Obtener registros paginados usando una sesión independiente
	offset := (page - 1) * pageSize
	err := tx.Session(&gorm.Session{}).
		Select(movementsSelect).
		Order("p.fecha_horaentra DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&movements).Error

	if err != nil {
		return nil, 0, 0, err
	}

	if err := projectOwedAmounts(movements, filter, CalculateOwedAmount); err != nil {
		return nil, 0, 0, err
	}

	return movements, totalCount, totalIngresos, nil
}

type owedAmountCalculator func(entryTime time.Time, budgetCode int, locationID uint) (float64, uint, error)

// projectOwedAmounts replaces monto_total of open movements (no exit yet) with the amount
// owed if the vehicle left now. The "activos" filter always projects; "todos" projects only
// when requested. Closed or annulled movements keep the amount stored by the app.
// A failed rate lookup aborts the projection instead of showing $0 as if nothing were owed.
func projectOwedAmounts(movements []models.Movement, filter MovementFilter, calculate owedAmountCalculator) error {
	projectAll := filter.Proyeccion && filter.Estado != "cerrados"
	if filter.Estado != "activos" && !projectAll {
		return nil
	}
	for i := range movements {
		if movements[i].FechaHoraSale != nil {
			continue
		}
		monto, _, err := calculate(movements[i].FechaHoraEntra, int(movements[i].CodigoPresup), movements[i].UbicacionID)
		if err != nil {
			return err
		}
		movements[i].MontoTotal = monto
	}
	return nil
}

// MovementMirror implements ratechange.Mirror on parkmovimientos.
type MovementMirror struct{}

// rateChangeUpdate writes the three rate-change fields to the movement while it is still open.
// fecha_hora_update refreshes on its own (ON UPDATE CURRENT_TIMESTAMP).
func rateChangeUpdate(db *gorm.DB, locationID uint, pagoID string, change ratechange.Change) *gorm.DB {
	return db.Table("parkmovimientos").
		Where("pago_id = ? AND ubicacion_id_fk = ?", pagoID, locationID).
		Where("fecha_horasale IS NULL AND fecha_horapago IS NULL AND fecha_hora_anula IS NULL").
		Updates(map[string]any{
			"codigo_presup":   change.RateCode,
			"precio_unitario": change.UnitPrice,
			"observaciones":   change.Observations,
		})
}

// ApplyRateChange copies a rate change to MySQL; updated is false when no open movement matched.
func (MovementMirror) ApplyRateChange(locationID uint, pagoID string, change ratechange.Change) (bool, error) {
	result := rateChangeUpdate(database.DB, locationID, pagoID, change)
	return result.RowsAffected > 0, result.Error
}
