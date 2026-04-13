package repository

import (
	"fmt"
	"math"
	"time"

	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
)

func RegisterEntry(movement *models.Movement) error {
	movement.PagoID = fmt.Sprintf("%d%s", time.Now().Unix(), movement.Placa)
	movement.FechaHoraEntra = time.Now()
	return database.DB.Create(movement).Error
}

func GetMovementByID(pagoID string) (*models.Movement, error) {
	var movement models.Movement
	err := database.DB.Where("pago_id = ?", pagoID).First(&movement).Error
	if err != nil {
		return nil, err
	}
	return &movement, nil
}

func RegisterExit(pagoID string, userEmail string) (*models.Movement, error) {
	movement, err := GetMovementByID(pagoID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	movement.FechaHoraSale = &now
	movement.UsuarioSalida = &userEmail

	rate, err := GetRate(int(movement.CodigoPresup), movement.UbicacionID)
	if err != nil {
		return nil, err
	}

	diff := now.Sub(movement.FechaHoraEntra).Minutes()
	movement.TiempoMinutos = new(uint)
	*movement.TiempoMinutos = uint(diff)

	horas := math.Ceil(diff / 60.0)
	if horas == 0 {
		horas = 1
	}
	movement.MontoTotal = horas * rate.PrecioUnitario
	movement.FechaHoraPago = &now

	err = database.DB.Save(movement).Error
	return movement, err
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
}

func GetMovements(filter MovementFilter, page int, pageSize int) ([]models.Movement, int64, float64, error) {
	var movements []models.Movement
	var totalCount int64
	var totalIngresos float64

	// 1. Construir la consulta base con los filtros comunes
	baseQuery := database.DB.Table("parkmovimientos as p").
		Joins("LEFT JOIN parktarifas pt ON p.codigo_presup = pt.codigo_presup AND p.ubicacion_id_fk = pt.ubicacion_id_fk").
		Where("p.ubicacion_id_fk = ?", filter.UbicacionID)

	// Filtrar por tipo (normal vs otros)
	if filter.Tipo == "otros" {
		baseQuery = baseQuery.Where("LOWER(p.placa) = ?", "otros")
		if filter.TarifaID > 0 {
			baseQuery = baseQuery.Where("p.codigo_presup = ?", filter.TarifaID)
		}
	} else {
		baseQuery = baseQuery.Where("LOWER(p.placa) != ?", "otros")
	}

	if filter.FechaInicio != "" && filter.FechaFin != "" {
		if filter.FechaInicio == filter.FechaFin {
			baseQuery = baseQuery.Where("DATE(p.fecha_horaentra) = ?", filter.FechaInicio)
		} else {
			baseQuery = baseQuery.Where("DATE(p.fecha_horaentra) BETWEEN ? AND ?", filter.FechaInicio, filter.FechaFin)
		}
	}

	switch filter.Estado {
	case "activos":
		baseQuery = baseQuery.Where("p.fecha_horasale IS NULL")
	case "cerrados":
		baseQuery = baseQuery.Where("p.fecha_horasale IS NOT NULL")
	}

	// 2. Ejecutar conteo total (usando una copia de la query base)
	if err := baseQuery.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// 3. Calcular ingresos totales (usando otra copia de la query base)
	if err := baseQuery.Select("IFNULL(SUM(p.monto_total), 0)").Row().Scan(&totalIngresos); err != nil {
		return nil, 0, 0, err
	}

	// 4. Obtener registros paginados
	offset := (page - 1) * pageSize
	err := baseQuery.Select("p.*, pt.descripcion as tarifa_descripcion").
		Order("p.fecha_horaentra DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&movements).Error

	return movements, totalCount, totalIngresos, err
}
