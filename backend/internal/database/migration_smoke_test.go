//go:build integration

package database

import (
	"aquaculture-water-feeding-control/backend/internal/model"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestSQLiteMigrationSmoke verifies that all GORM entities (including the new
// recommendation snapshot and the execution snapshot reference) build a
// consistent schema with foreign keys. This runs without PostgreSQL/Redis.
func TestSQLiteMigrationSmoke(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "smoke.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.Pond{},
		&model.WaterReading{},
		&model.FeedingPlan{},
		&model.FeedingRecommendation{},
		&model.ControlExecution{},
		&model.AuditLog{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	// The execution table must carry the snapshot reference columns.
	type columnInfo struct {
		Name string `gorm:"column:name"`
	}
	var cols []columnInfo
	if err := db.Raw("PRAGMA table_info(control_executions)").Scan(&cols).Error; err != nil {
		t.Fatalf("read execution columns: %v", err)
	}
	want := map[string]bool{"recommendation_id": false, "recommendation_no": false, "basis": false}
	for _, c := range cols {
		if _, ok := want[c.Name]; ok {
			want[c.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("control_executions missing column %q", name)
		}
	}

	var recCols []columnInfo
	if err := db.Raw("PRAGMA table_info(feeding_recommendations)").Scan(&recCols).Error; err != nil {
		t.Fatalf("read recommendation columns: %v", err)
	}
	recWant := map[string]bool{"snapshot_no": false, "plan_version": false, "weather_window": false, "adjustment_percent": false, "valid": false, "invalid_reason": false, "reading_measured_at": false}
	for _, c := range recCols {
		if _, ok := recWant[c.Name]; ok {
			recWant[c.Name] = true
		}
	}
	for name, found := range recWant {
		if !found {
			t.Errorf("feeding_recommendations missing column %q", name)
		}
	}
}
