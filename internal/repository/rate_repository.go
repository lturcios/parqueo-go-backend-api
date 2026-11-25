package repository

import (
	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/ratechange"
	"gorm.io/gorm"
)

func GetRatesForCalculation(budgetCode int, locationID uint) ([]models.Rate, error) {
	var rates []models.Rate
	// Buscamos todas las tarifas vigentes para ese código y ubicación, ordenadas por tiempo_minimo
	err := database.DB.Where("codigo_presup = ? AND ubicacion_id_fk = ? AND vigente = 1", budgetCode, locationID).
		Order("tiempo_minimo ASC").
		Find(&rates).Error
	return rates, err
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

// RateCatalog implements ratechange.Rates on parktarifas.
type RateCatalog struct{}

// vehicleRateQuery selects the first tier of a current vehicle rate of the location.
// Vehicle rates have no icon or an icon up to 10; "otros ingresos" use higher icons.
func vehicleRateQuery(db *gorm.DB, code int, locationID uint) *gorm.DB {
	return db.Where("codigo_presup = ? AND ubicacion_id_fk = ? AND vigente = 1 AND (iconfile IS NULL OR iconfile <= 10)", code, locationID).
		Order("tiempo_minimo ASC").
		Limit(1)
}

// VehicleRate returns the description and first-tier unit price of a current vehicle rate.
func (RateCatalog) VehicleRate(code int, locationID uint) (ratechange.Rate, bool, error) {
	var rates []models.Rate
	if err := vehicleRateQuery(database.DB, code, locationID).Find(&rates).Error; err != nil {
		return ratechange.Rate{}, false, err
	}
	if len(rates) == 0 {
		return ratechange.Rate{}, false, nil
	}
	return ratechange.Rate{Description: rates[0].Descripcion, UnitPrice: rates[0].PrecioUnitario}, true, nil
}

// Description returns the description of a rate code of the location, preferring the
// current rate and its first tier, the same rule the movements listing uses.
func (RateCatalog) Description(code int, locationID uint) (string, error) {
	var rates []models.Rate
	err := database.DB.Where("codigo_presup = ? AND ubicacion_id_fk = ?", code, locationID).
		Order("vigente DESC, tiempo_minimo ASC").
		Limit(1).
		Find(&rates).Error
	if err != nil || len(rates) == 0 {
		return "", err
	}
	return rates[0].Descripcion, nil
}
