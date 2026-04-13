package models

import (
	"time"
)

type User struct {
	Email           string    `gorm:"primaryKey;type:varchar(120)" json:"email"`
	Nombre          string    `gorm:"type:varchar(120);not null" json:"nombre"`
	Password        string    `gorm:"type:varchar(60);not null" json:"-"`
	LastAction      string    `gorm:"type:varchar(10);default:''" json:"last_action"`
	LastTime        time.Time `gorm:"autoUpdateNow" json:"last_time"`
	InstitucionID   int16     `gorm:"column:institucion_id_fk;not null" json:"institucion_id"`
	UbicacionID     uint      `gorm:"column:ubicacion_id_fk;not null" json:"ubicacion_id"`
}

func (User) TableName() string {
	return "parkusuarios"
}

type Location struct {
	ID            uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	Descripcion   string `gorm:"type:varchar(50)" json:"descripcion"`
	Observacion   string `gorm:"type:varchar(200)" json:"observacion"`
	InstitucionID int16  `gorm:"column:institucion_id_fk;not null" json:"institucion_id"`
}

func (Location) TableName() string {
	return "parkubicaciones"
}

type Rate struct {
	ID               int       `gorm:"primaryKey;autoIncrement" json:"id"`
	InstitucionID    int16     `gorm:"column:institucion_id_fk;not null" json:"institucion_id"`
	CodigoPresup     int       `gorm:"column:codigo_presup;not null" json:"codigo_presup"`
	Descripcion      string    `gorm:"type:varchar(50)" json:"descripcion"`
	PrecioUnitario   float64   `gorm:"not null" json:"precio_unitario"`
	TiempoMinimo     int       `gorm:"not null" json:"tiempo_minimo"`
	TiempoMaximo     int       `gorm:"not null" json:"tiempo_maximo"`
	TiempoTolerancia int       `gorm:"not null" json:"tiempo_tolerancia"`
	Periodo          string    `gorm:"type:varchar(6);default:'0';not null" json:"periodo"`
	Referencia       string    `gorm:"type:varchar(200)" json:"referencia"`
	Vigencia         time.Time `gorm:"type:date;not null" json:"vigencia"`
	Vigente          uint8     `gorm:"type:tinyint;default:0;not null" json:"vigente"`
	UbicacionID      uint      `gorm:"column:ubicacion_id_fk;not null" json:"ubicacion_id"`
	IconFile         *int      `gorm:"column:iconfile" json:"icon_file"`
}

func (Rate) TableName() string {
	return "parktarifas"
}

type Movement struct {
	PagoID          string     `gorm:"primaryKey;column:pago_id" json:"pago_id"`
	FechaHoraEntra  time.Time  `gorm:"column:fecha_horaentra;not null" json:"fecha_hora_entra"`
	FechaHoraSale   *time.Time `gorm:"column:fecha_horasale" json:"fecha_hora_sale"`
	CodigoPresup    uint       `gorm:"column:codigo_presup;default:0;not null" json:"codigo_presup"`
	Placa           string     `gorm:"column:placa;type:varchar(10);default:'0';not null" json:"placa"`
	PrecioUnitario  float64    `gorm:"column:precio_unitario;not null" json:"precio_unitario"`
	TiempoMinutos   *uint      `gorm:"column:tiempo_minutos" json:"tiempo_minutos"`
	MontoTotal      float64    `gorm:"column:monto_total;not null" json:"monto_total"`
	SerieEntrada    string     `gorm:"column:serie_entrada;not null" json:"serie_entrada"`
	SerieSalida     *string    `gorm:"column:serie_salida" json:"serie_salida"`
	FechaHoraPago   *time.Time `gorm:"column:fecha_horapago" json:"fecha_hora_pago"`
	Observaciones   *string    `gorm:"column:observaciones" json:"observaciones"`
	UbicacionID     uint       `gorm:"column:ubicacion_id_fk;not null" json:"ubicacion_id"`
	UsuarioEntrada  string     `gorm:"column:usuario_entrada;not null" json:"usuario_entrada"`
	UsuarioSalida   *string    `gorm:"column:usuario_salida" json:"usuario_salida"`
	FechaHoraAnula  *time.Time `gorm:"column:fecha_hora_anula" json:"fecha_hora_anula"`
	FechaHoraUpdate time.Time  `gorm:"column:fecha_hora_update;autoUpdateNow" json:"fecha_hora_update"`
	TarifaDescripcion string  `gorm:"->" json:"tarifa_descripcion"`
}

func (Movement) TableName() string {
	return "parkmovimientos"
}
