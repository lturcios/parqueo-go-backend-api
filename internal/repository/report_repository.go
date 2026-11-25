package repository

import (
	"github.com/parqueo/api/internal/database"
	"github.com/parqueo/api/internal/domain/models"
	"github.com/parqueo/api/internal/report"
	"gorm.io/gorm"
)

// ReportContext holds the names printed on the report header and the institution's cutoff.
type ReportContext struct {
	Institucion string
	Ubicacion   string
	HoraCorte   string
}

// GetReportContext reads the location (parkubicaciones) and its institution (instituciones,
// the table the legacy PHP panel uses). A missing institution leaves its name and cutoff empty.
func GetReportContext(locationID uint) (ReportContext, error) {
	var location struct {
		Descripcion   string
		InstitucionID int16 `gorm:"column:institucion_id_fk"`
	}
	result := database.DB.Table("parkubicaciones").
		Select("descripcion, institucion_id_fk").
		Where("id = ?", locationID).
		Limit(1).
		Scan(&location)
	if result.Error != nil {
		return ReportContext{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ReportContext{}, gorm.ErrRecordNotFound
	}

	var institution struct {
		Nombre    string
		HoraCorte *string `gorm:"column:hora_corte"`
	}
	if err := database.DB.Table("instituciones").
		Select("nombre, hora_corte").
		Where("id = ?", location.InstitucionID).
		Limit(1).
		Scan(&institution).Error; err != nil {
		return ReportContext{}, err
	}

	ctx := ReportContext{Institucion: institution.Nombre, Ubicacion: location.Descripcion}
	if institution.HoraCorte != nil {
		ctx.HoraCorte = *institution.HoraCorte
	}
	return ctx, nil
}

// reportMovementsQuery selects the regular movements of the location that entered within the
// jornada span (after, until], oldest first, with the same filters as the movements listing.
func reportMovementsQuery(db *gorm.DB, locationID uint, estado, after, until string) *gorm.DB {
	return movementsQuery(db, MovementFilter{UbicacionID: locationID, Estado: estado, Tipo: "normal"}).
		Where("p.fecha_horaentra > ? AND p.fecha_horaentra <= ?", after, until).
		Select(movementsSelect).
		Order("p.fecha_horaentra ASC")
}

// GetReportMovements returns at most limit movements of the jornada span.
func GetReportMovements(locationID uint, estado string, jornada report.Jornada, limit int) ([]models.Movement, error) {
	movements := []models.Movement{}
	err := reportMovementsQuery(database.DB, locationID, estado, jornada.After, jornada.Until).
		Limit(limit).
		Scan(&movements).Error
	return movements, err
}
