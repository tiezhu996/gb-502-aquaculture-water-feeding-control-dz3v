//go:build integration

package service

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/model"
	"aquaculture-water-feeding-control/backend/internal/repository"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type recEnv struct {
	db         *gorm.DB
	ponds      *repository.PondRepository
	readings   *repository.ReadingRepository
	plans      *repository.PlanRepository
	recs       *repository.RecommendationRepository
	executions *repository.ExecutionRepository
	pondSvc    *PondService
	readingSvc *ReadingService
	planSvc    *PlanService
	recSvc     *RecommendationService
	execSvc    *ExecutionService
	actor      Actor
	pond       model.Pond
	plan       model.FeedingPlan
}

func setupRecEnv(t *testing.T) *recEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "flow.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{}, &model.Pond{}, &model.WaterReading{}, &model.FeedingPlan{},
		&model.FeedingRecommendation{}, &model.ControlExecution{}, &model.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pondRepo := repository.NewPondRepository(db)
	readingRepo := repository.NewReadingRepository(db)
	planRepo := repository.NewPlanRepository(db)
	recRepo := repository.NewRecommendationRepository(db)
	execRepo := repository.NewExecutionRepository(db)
	auditRepo := repository.NewAuditRepository(db)
	auditSvc := NewAuditService(auditRepo)

	env := &recEnv{
		db: db, ponds: pondRepo, readings: readingRepo, plans: planRepo, recs: recRepo, executions: execRepo,
		pondSvc:    NewPondService(pondRepo, recRepo, auditSvc),
		readingSvc: NewReadingService(readingRepo, pondRepo, recRepo, auditSvc),
		planSvc:    NewPlanService(planRepo, pondRepo, readingRepo, recRepo, auditSvc),
		recSvc:     NewRecommendationService(recRepo, planRepo, pondRepo, readingRepo, auditSvc),
		execSvc:    NewExecutionService(execRepo, planRepo, pondRepo, readingRepo, recRepo, auditSvc),
		actor:      Actor{UserID: 1, Username: "operator", DisplayName: "值班员", Role: "operator"},
	}

	pond := model.Pond{
		Code: "P-T1", Name: "测试塘", Species: "鲈鱼", AreaSquareMeters: 1000, CapacityKg: 10000,
		GrowthStage: "成长期", Status: constants.PondStatusActive, Manager: "主管",
	}
	if err := pondRepo.Create(&pond); err != nil {
		t.Fatalf("create pond: %v", err)
	}
	env.pond = pond

	start := time.Now().UTC().Add(-time.Hour)
	end := start.Add(30 * 24 * time.Hour)
	plan := model.FeedingPlan{
		PondID: pond.ID, Name: "测试计划", Version: 1, DailyAmountKg: 100, FrequencyPerDay: 2,
		FeedType: "配合饲料", TargetGrowthStage: "成长期", MinOxygen: 5, StartDate: start, EndDate: end,
		Status: constants.PlanStatusApproved, Rationale: "集成测试用已批准计划依据充分", CreatedBy: "主管", ReviewedBy: "主管",
	}
	if err := planRepo.Create(&plan); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	env.plan = plan

	reading := model.WaterReading{
		PondID: pond.ID, DissolvedOxygen: 7, Temperature: 26, PH: 7.4, Ammonia: 0.1, Turbidity: 20,
		MeasuredAt: time.Now().UTC().Add(-30 * time.Minute), Source: "manual", RiskLevel: constants.RiskNormal,
	}
	if err := readingRepo.Create(&reading); err != nil {
		t.Fatalf("create reading: %v", err)
	}
	return env
}

func TestRecommendationSnapshotLifecycle(t *testing.T) {
	env := setupRecEnv(t)

	// 1. 生成快照，编号连续且带版本/调整比例。
	rec1, err := env.recSvc.Generate(dto.GenerateRecommendationInput{PondID: env.pond.ID, Weather: "晴朗"}, env.actor)
	if err != nil {
		t.Fatalf("generate first: %v", err)
	}
	if rec1.SnapshotNo == "" || rec1.PlanVersion != 1 || rec1.Action != "feed" {
		t.Fatalf("unexpected snapshot: %+v", rec1)
	}
	if rec1.DailyAmountKg != 100 {
		t.Fatalf("daily = %v, want 100", rec1.DailyAmountKg)
	}

	// 2. 再次生成，旧快照应标记为“已生成更新的投喂建议”。
	rec2, err := env.recSvc.Generate(dto.GenerateRecommendationInput{PondID: env.pond.ID, Weather: "小雨"}, env.actor)
	if err != nil {
		t.Fatalf("generate second: %v", err)
	}
	stored1, err := env.recs.Get(rec1.ID)
	if err != nil {
		t.Fatalf("reload rec1: %v", err)
	}
	if stored1.Valid {
		t.Fatal("older snapshot should be invalid after a newer generation")
	}
	if stored1.InvalidReason != InvalidReasonSuperseded {
		t.Fatalf("reason = %q", stored1.InvalidReason)
	}
	if !rec2.Valid || rec2.SnapshotNo == rec1.SnapshotNo {
		t.Fatal("new snapshot must be valid with a distinct number")
	}

	// 3. 用有效快照安排执行成功，执行记录写入编号与依据。
	exec, err := env.execSvc.Create(dto.ExecutionInput{
		PondID: env.pond.ID, FeedingPlanID: env.plan.ID, RecommendationSnapshotID: rec2.ID,
		ScheduledAt: time.Now().UTC().Add(2 * time.Hour), PlannedAmountKg: rec2.AmountPerFeedingKg, Weather: "小雨",
	}, env.actor)
	if err != nil {
		t.Fatalf("schedule execution: %v", err)
	}
	if exec.RecommendationNo != rec2.SnapshotNo || exec.Basis == "" {
		t.Fatalf("execution must record snapshot number and basis: %+v", exec)
	}

	// 4. 用已失效快照安排执行必须被拒绝。
	if _, err := env.execSvc.Create(dto.ExecutionInput{
		PondID: env.pond.ID, FeedingPlanID: env.plan.ID, RecommendationSnapshotID: rec1.ID,
		ScheduledAt: time.Now().UTC().Add(3 * time.Hour), PlannedAmountKg: 10, Weather: "晴朗",
	}, env.actor); err == nil {
		t.Fatal("expected conflict when scheduling with an invalid snapshot")
	}
}

func TestNewReadingInvalidatesSnapshots(t *testing.T) {
	env := setupRecEnv(t)
	rec, err := env.recSvc.Generate(dto.GenerateRecommendationInput{PondID: env.pond.ID, Weather: "晴朗"}, env.actor)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 录入更新的水质读数。
	newReading := dto.WaterReadingInput{
		PondID: env.pond.ID, DissolvedOxygen: 6.8, Temperature: 25, PH: 7.2, Ammonia: 0.12, Turbidity: 22,
		MeasuredAt: time.Now().UTC(), Source: "manual",
	}
	if _, err := env.readingSvc.Create(newReading, env.actor); err != nil {
		t.Fatalf("create reading: %v", err)
	}
	stored, err := env.recs.Get(rec.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Valid {
		t.Fatal("snapshot must become invalid when a new reading arrives")
	}
	if stored.InvalidReason != InvalidReasonNewReading {
		t.Fatalf("reason = %q", stored.InvalidReason)
	}
	// 失效快照不能安排执行。
	if _, verr := env.execSvc.Create(dto.ExecutionInput{
		PondID: env.pond.ID, FeedingPlanID: env.plan.ID, RecommendationSnapshotID: rec.ID,
		ScheduledAt: time.Now().UTC().Add(time.Hour), PlannedAmountKg: 10, Weather: "晴朗",
	}, env.actor); verr == nil {
		t.Fatal("expected scheduling to be blocked after new reading")
	}
}

func TestPlanRevokeInvalidatesSnapshots(t *testing.T) {
	env := setupRecEnv(t)
	rec, err := env.recSvc.Generate(dto.GenerateRecommendationInput{PondID: env.pond.ID, Weather: "晴朗"}, env.actor)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 计划当前是 approved；撤销接口要求 pending/approved，先直接走 revoke。
	if _, err := env.planSvc.Revoke(env.plan.ID, "测试撤销", Actor{UserID: 2, DisplayName: "主管", Role: "manager"}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	stored, err := env.recs.Get(rec.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Valid || stored.InvalidReason != InvalidReasonPlanRevoked {
		t.Fatalf("snapshot should be revoked-invalid: valid=%v reason=%q", stored.Valid, stored.InvalidReason)
	}
}
