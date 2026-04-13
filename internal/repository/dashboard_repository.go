package repository

import (
	"time"

	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
)

type DashboardStats struct {
	Total    int64   `json:"total"`
	Activos  int64   `json:"activos"`
	Ingresos float64 `json:"ingresos"`
}

type AnalyticsData struct {
	Hour     int     `json:"hour"`
	Count    int64   `json:"count"`
	Ingresos float64 `json:"ingresos"`
}

func GetDashboardStats(locationID uint) (DashboardStats, error) {
	var stats DashboardStats
	today := time.Now().Format("2006-01-02")

	// Total movements
	database.DB.Model(&models.Movement{}).
		Where("DATE(fecha_horaentra) = ? AND ubicacion_id_fk = ?", today, locationID).
		Count(&stats.Total)

	// Active vehicles
	database.DB.Model(&models.Movement{}).
		Where("DATE(fecha_horaentra) = ? AND ubicacion_id_fk = ? AND fecha_horasale IS NULL", today, locationID).
		Count(&stats.Activos)

	// Total revenue
	database.DB.Model(&models.Movement{}).
		Where("DATE(fecha_horaentra) = ? AND ubicacion_id_fk = ?", today, locationID).
		Select("IFNULL(SUM(monto_total), 0)").
		Row().Scan(&stats.Ingresos)

	return stats, nil
}

func GetDashboardAnalytics(locationID uint) ([]AnalyticsData, error) {
	results := []AnalyticsData{}
	today := time.Now().Format("2006-01-02")

	err := database.DB.Raw(`
		SELECT 
			HOUR(fecha_horaentra) as hour, 
			COUNT(*) as count, 
			SUM(monto_total) as ingresos 
		FROM parkmovimientos 
		WHERE DATE(fecha_horaentra) = ? AND ubicacion_id_fk = ?
		GROUP BY hour 
		ORDER BY hour ASC`, today, locationID).Scan(&results).Error

	return results, err
}
