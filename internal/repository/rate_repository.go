package repository

import (
	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
)

func GetRatesForCalculation(budgetCode int, locationID uint) ([]models.Rate, error) {
	var rates []models.Rate
	// Buscamos todas las tarifas vigentes para ese código y ubicación, ordenadas por tiempo_minimo
	err := database.DB.Where("codigo_presup = ? AND ubicacion_id_fk = ? AND vigente = 1", budgetCode, locationID).
		Order("tiempo_minimo ASC").
		Find(&rates).Error
	return rates, err
}

func GetRate(budgetCode int, locationID uint) (*models.Rate, error) {
	var rate models.Rate
	err := database.DB.Where("codigo_presup = ? AND ubicacion_id_fk = ? AND vigente = 1", budgetCode, locationID).First(&rate).Error
	if err != nil {
		return nil, err
	}
	return &rate, nil
}

func GetRatesByLocation(locationID uint) ([]models.Rate, error) {
	var rates []models.Rate
	err := database.DB.Where("ubicacion_id_fk = ? AND vigente = 1", locationID).Find(&rates).Error
	return rates, err
}

func GetOtherIncomeRatesByLocation(locationID uint) ([]models.Rate, error) {
	var rates []models.Rate
	err := database.DB.Where("ubicacion_id_fk = ? AND iconfile > 10 AND vigente = 1", locationID).Find(&rates).Error
	return rates, err
}
