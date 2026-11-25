package repository

import (
	"testing"

	"github.com/parqueo/api/internal/database"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// useUnreachableDB points the repository at a MySQL address that refuses connections,
// so every query fails the way it would if the database were down.
func useUnreachableDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "user:pass@tcp(127.0.0.1:1)/none?timeout=1s",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open unreachable db: %v", err)
	}
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
}

func TestGetDashboardStatsReturnsDatabaseErrors(t *testing.T) {
	useUnreachableDB(t)

	stats, err := GetDashboardStats(1)

	if err == nil {
		t.Fatalf("expected an error when the database is unreachable, got stats %+v", stats)
	}
}

func TestGetReportContextReturnsDatabaseErrors(t *testing.T) {
	useUnreachableDB(t)

	if ctx, err := GetReportContext(8); err == nil {
		t.Fatalf("expected an error when the database is unreachable, got %+v", ctx)
	}
}
