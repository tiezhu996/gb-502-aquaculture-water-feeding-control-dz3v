package service

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/model"
	"aquaculture-water-feeding-control/backend/internal/repository"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
)

type ExecutionService struct {
	repo            *repository.ExecutionRepository
	plans           *repository.PlanRepository
	ponds           *repository.PondRepository
	readings        *repository.ReadingRepository
	recommendations *repository.RecommendationRepository
	audit           *AuditService
	transactional   bool
}

func (s *ExecutionService) withinTransaction(fn func(*ExecutionService) error) error {
	return s.audit.WithinTransaction(func(tx *gorm.DB, audit *AuditService) error {
		scoped := &ExecutionService{
			repo: repository.NewExecutionRepository(tx), plans: repository.NewPlanRepository(tx),
			ponds: repository.NewPondRepository(tx), readings: repository.NewReadingRepository(tx),
			recommendations: repository.NewRecommendationRepository(tx),
			audit:           audit, transactional: true,
		}
		return fn(scoped)
	})
}

func NewExecutionService(repo *repository.ExecutionRepository, plans *repository.PlanRepository, ponds *repository.PondRepository, readings *repository.ReadingRepository, recommendations *repository.RecommendationRepository, audit *AuditService) *ExecutionService {
	return &ExecutionService{repo: repo, plans: plans, ponds: ponds, readings: readings, recommendations: recommendations, audit: audit}
}

func (s *ExecutionService) List(query dto.PageQuery, pondID, planID uint) (dto.PageResult[model.ControlExecution], error) {
	query.Normalize()
	items, total, err := s.repo.List(query, pondID, planID)
	if err != nil {
		return dto.PageResult[model.ControlExecution]{}, WrapError(CodeInternal, "查询执行记录失败", err)
	}
	return dto.PageResult[model.ControlExecution]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (s *ExecutionService) Get(id uint) (model.ControlExecution, error) {
	var execution model.ControlExecution
	var err error
	if s.transactional {
		execution, err = s.repo.GetForUpdate(id)
	} else {
		execution, err = s.repo.Get(id)
	}
	if err == gorm.ErrRecordNotFound {
		return model.ControlExecution{}, NewError(CodeNotFound, "执行记录不存在")
	}
	if err != nil {
		return model.ControlExecution{}, WrapError(CodeInternal, "查询执行记录失败", err)
	}
	return execution, nil
}

func (s *ExecutionService) Create(input dto.ExecutionInput, actor Actor) (model.ControlExecution, error) {
	if !s.transactional {
		var result model.ControlExecution
		err := s.withinTransaction(func(scoped *ExecutionService) error {
			var inner error
			result, inner = scoped.Create(input, actor)
			return inner
		})
		return result, err
	}
	plan, pond, latest, rec, err := s.validateExecution(input.PondID, input.FeedingPlanID, input.RecommendationSnapshotID, input.PlannedAmountKg, input.ScheduledAt, 0)
	if err != nil {
		return model.ControlExecution{}, err
	}
	recID := rec.ID
	execution := model.ControlExecution{
		PondID: input.PondID, FeedingPlanID: input.FeedingPlanID, RecommendationID: &recID,
		RecommendationNo: rec.SnapshotNo, Basis: buildExecutionBasis(rec),
		ScheduledAt: input.ScheduledAt.UTC(), PlannedAmountKg: input.PlannedAmountKg,
		Status:   constants.ExecutionScheduled,
		Operator: actor.DisplayName, Weather: input.Weather, OxygenSnapshot: latest.DissolvedOxygen,
	}
	if execution.Operator == "" {
		execution.Operator = actor.Username
	}
	if err := s.repo.Create(&execution); err != nil {
		return model.ControlExecution{}, WrapError(CodeInternal, "创建执行记录失败", err)
	}
	execution.Pond = &pond
	execution.FeedingPlan = &plan
	execution.Recommendation = &rec
	if err := s.audit.Record(actor, "schedule", "control_execution", execution.ID, nil, execution,
		fmt.Sprintf("依据有效建议 %s 安排投喂（计划 v%d）", rec.SnapshotNo, rec.PlanVersion)); err != nil {
		return model.ControlExecution{}, err
	}
	return execution, nil
}

// buildExecutionBasis 把执行所依据的建议编号、计划版本、水质读数、天气窗口和调整比例固化为文本。
func buildExecutionBasis(rec model.FeedingRecommendation) string {
	return fmt.Sprintf(
		"建议 %s；计划 v%d；水质读数时间 %s（溶氧 %.1fmg/L，%s）；天气窗口：%s；调整比例 %.0f%%；建议 %s，%.2fkg/日",
		rec.SnapshotNo, rec.PlanVersion, rec.ReadingMeasuredAt.Format("2006-01-02 15:04"),
		rec.DissolvedOxygen, riskText(rec.RiskLevel), blankAsDash(rec.WeatherWindow),
		rec.AdjustmentPercent, actionText(rec.Action), rec.DailyAmountKg,
	)
}

func riskText(level constants.RiskLevel) string {
	switch level {
	case constants.RiskNormal:
		return "水质正常"
	case constants.RiskWarning:
		return "水质预警"
	case constants.RiskCritical:
		return "水质严重"
	}
	return string(level)
}

func actionText(action string) string {
	switch action {
	case RecommendationActionFeed:
		return "按计划投喂"
	case RecommendationActionReduce:
		return "减量投喂"
	case RecommendationActionHold:
		return "暂停投喂"
	}
	return action
}

func blankAsDash(value string) string {
	if value == "" {
		return "未填写"
	}
	return value
}

func (s *ExecutionService) Update(id uint, input dto.UpdateExecutionInput, actor Actor) (model.ControlExecution, error) {
	if !s.transactional {
		var result model.ControlExecution
		err := s.withinTransaction(func(scoped *ExecutionService) error {
			var inner error
			result, inner = scoped.Update(id, input, actor)
			return inner
		})
		return result, err
	}
	execution, err := s.Get(id)
	if err != nil {
		return model.ControlExecution{}, err
	}
	if execution.Status == constants.ExecutionCompleted || execution.Status == constants.ExecutionCancelled {
		return model.ControlExecution{}, NewError(CodeConflict, "已完成或已取消记录不能编辑")
	}
	if input.Status != constants.ExecutionScheduled && input.Status != constants.ExecutionRunning && input.Status != constants.ExecutionCancelled {
		return model.ControlExecution{}, NewError(CodeValidation, "执行状态无效")
	}
	if !execution.Status.CanTransitionTo(input.Status) {
		return model.ControlExecution{}, NewError(CodeConflict, "执行状态不允许逆向迁移")
	}
	if input.Status != constants.ExecutionCancelled {
		if execution.RecommendationID == nil {
			return model.ControlExecution{}, NewError(CodeConflict, "该安排缺少投喂建议依据，不能修改，请重新安排")
		}
		if _, _, _, _, verr := s.validateExecution(execution.PondID, execution.FeedingPlanID, *execution.RecommendationID, input.PlannedAmountKg, input.ScheduledAt, execution.ID); verr != nil {
			return model.ControlExecution{}, verr
		}
	}
	before := execution
	execution.ScheduledAt = input.ScheduledAt.UTC()
	execution.PlannedAmountKg = input.PlannedAmountKg
	execution.Weather = input.Weather
	if input.Status == constants.ExecutionRunning && execution.StartedAt == nil {
		now := time.Now().UTC()
		execution.StartedAt = &now
	}
	execution.Status = input.Status
	if err := s.repo.Save(&execution); err != nil {
		return model.ControlExecution{}, WrapError(CodeInternal, "更新执行记录失败", err)
	}
	if err := s.audit.Record(actor, "update", "control_execution", execution.ID, before, execution, "调整时间、数量或执行状态"); err != nil {
		return model.ControlExecution{}, err
	}
	return execution, nil
}

func (s *ExecutionService) Complete(id uint, input dto.CompleteExecutionInput, actor Actor) (model.ControlExecution, error) {
	if !s.transactional {
		var result model.ControlExecution
		err := s.withinTransaction(func(scoped *ExecutionService) error {
			var inner error
			result, inner = scoped.Complete(id, input, actor)
			return inner
		})
		return result, err
	}
	execution, err := s.Get(id)
	if err != nil {
		return model.ControlExecution{}, err
	}
	if execution.Status != constants.ExecutionScheduled && execution.Status != constants.ExecutionRunning {
		return model.ControlExecution{}, NewError(CodeConflict, "当前执行状态不能提交反馈")
	}
	if input.OxygenSnapshot < execution.FeedingPlan.MinOxygen {
		return model.ControlExecution{}, NewError(CodeConflict, "现场溶解氧低于计划阈值，请停止投喂并处置水质")
	}
	if input.ActualAmountKg > execution.FeedingPlan.DailyAmountKg {
		return model.ControlExecution{}, NewError(CodeValidation, "单次实际投喂量不能超过计划日投喂量")
	}
	deviation := math.Abs(input.ActualAmountKg-execution.PlannedAmountKg) / execution.PlannedAmountKg
	if deviation > 0.25 && len([]rune(input.Feedback)) < 10 {
		return model.ControlExecution{}, NewError(CodeValidation, "实际量偏差超过 25% 时需提供至少 10 字说明")
	}
	before := execution
	now := time.Now().UTC()
	if execution.StartedAt == nil {
		execution.StartedAt = &now
	}
	execution.CompletedAt = &now
	execution.ActualAmountKg = input.ActualAmountKg
	execution.OxygenSnapshot = input.OxygenSnapshot
	execution.Feedback = input.Feedback
	execution.Status = constants.ExecutionCompleted
	if err := s.repo.Save(&execution); err != nil {
		return model.ControlExecution{}, WrapError(CodeInternal, "完成执行记录失败", err)
	}
	plan, err := s.plans.Get(execution.FeedingPlanID)
	if err != nil {
		return model.ControlExecution{}, WrapError(CodeInternal, "查询关联计划失败", err)
	}
	openCount, err := s.repo.CountOpenForPlanExcluding(plan.ID, execution.ID)
	if err != nil {
		return model.ControlExecution{}, WrapError(CodeInternal, "检查计划待执行记录失败", err)
	}
	if plan.Status == constants.PlanStatusApproved && openCount == 0 {
		planBefore := plan
		plan.Status = constants.PlanStatusExecuted
		if err := s.plans.Save(&plan); err != nil {
			return model.ControlExecution{}, WrapError(CodeInternal, "更新计划执行状态失败", err)
		}
		if err := s.audit.Record(actor, "execute", "feeding_plan", plan.ID, planBefore, plan, "首次投喂执行已完成"); err != nil {
			return model.ControlExecution{}, err
		}
		if _, err := s.recommendations.MarkPlanInvalid(plan.ID, InvalidReasonPlanExecuted, time.Now().UTC()); err != nil {
			return model.ControlExecution{}, WrapError(CodeInternal, "失效相关投喂建议失败", err)
		}
	}
	if err := s.audit.Record(actor, "complete", "control_execution", execution.ID, before, execution, input.Feedback); err != nil {
		return model.ControlExecution{}, err
	}
	return execution, nil
}

func (s *ExecutionService) Delete(id uint, actor Actor) error {
	if !s.transactional {
		return s.withinTransaction(func(scoped *ExecutionService) error { return scoped.Delete(id, actor) })
	}
	execution, err := s.Get(id)
	if err != nil {
		return err
	}
	if execution.Status != constants.ExecutionScheduled {
		return NewError(CodeConflict, "只有待执行记录可以删除")
	}
	if err := s.repo.Delete(&execution); err != nil {
		return WrapError(CodeInternal, "删除执行记录失败", err)
	}
	return s.audit.Record(actor, "delete", "control_execution", execution.ID, execution, nil, "取消未开始的投喂安排")
}

// loadValidRecommendation 取出建议快照并强制其当前仍有效（动态校验 + 行锁）。
func (s *ExecutionService) loadValidRecommendation(recID uint) (model.FeedingRecommendation, string, error) {
	rec, err := s.recommendations.GetForUpdate(recID)
	if err == gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, "", NewError(CodeValidation, "投喂建议不存在")
	}
	if err != nil {
		return model.FeedingRecommendation{}, "", WrapError(CodeInternal, "查询投喂建议失败", err)
	}
	if !rec.Valid {
		reason := rec.InvalidReason
		if reason == "" {
			reason = "该投喂建议已失效"
		}
		return rec, reason, NewError(CodeConflict, "所选用的投喂建议已失效："+reason)
	}
	plan, perr := s.plans.Get(rec.FeedingPlanID)
	if perr != nil && perr != gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, "", WrapError(CodeInternal, "查询关联计划失败", perr)
	}
	var latest model.WaterReading
	latest, lerr := s.readings.LatestForPond(rec.PondID)
	if lerr != nil && lerr != gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, "", WrapError(CodeInternal, "查询最新水质失败", lerr)
	}
	if reason := EvaluateRecommendationInvalid(&rec, plan, latest, time.Now().UTC()); reason != "" {
		_ = s.recommendations.MarkOne(rec.ID, reason, time.Now().UTC())
		return rec, reason, NewError(CodeConflict, "所选用的投喂建议已失效："+reason)
	}
	return rec, "", nil
}

func (s *ExecutionService) validateExecution(pondID, planID, recID uint, amount float64, scheduledAt time.Time, excludedID uint) (model.FeedingPlan, model.Pond, model.WaterReading, model.FeedingRecommendation, error) {
	var plan model.FeedingPlan
	var err error
	if s.transactional {
		plan, err = s.plans.GetForUpdate(planID)
	} else {
		plan, err = s.plans.Get(planID)
	}
	if err == gorm.ErrRecordNotFound {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeValidation, "投喂计划不存在")
	}
	if err != nil {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, WrapError(CodeInternal, "查询投喂计划失败", err)
	}
	if plan.PondID != pondID {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeValidation, "投喂计划与养殖池不匹配")
	}
	if plan.Status != constants.PlanStatusApproved {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "只能使用已批准计划安排执行")
	}
	if scheduledAt.Before(plan.StartDate) || scheduledAt.After(plan.EndDate.Add(24*time.Hour)) {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeValidation, "执行时间必须在计划周期内")
	}
	if amount > plan.DailyAmountKg {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeValidation, "单次计划量不能超过日投喂量")
	}
	// 安排执行必须引用一条当前有效的投喂建议，失效建议不允许再被使用。
	rec, _, recErr := s.loadValidRecommendation(recID)
	if recErr != nil {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, recErr
	}
	if rec.PondID != pondID || rec.FeedingPlanID != planID {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeValidation, "投喂建议与所选养殖池或计划不匹配")
	}
	if rec.PlanVersion != plan.Version {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "投喂建议依据的计划版本与当前计划不一致")
	}
	if rec.Action == RecommendationActionHold {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "所选建议结论为暂停投喂，不能安排执行")
	}
	var pond model.Pond
	if s.transactional {
		pond, err = s.ponds.GetForUpdate(pondID)
	} else {
		pond, err = s.ponds.Get(pondID)
	}
	if err != nil {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, WrapError(CodeInternal, "查询养殖池失败", err)
	}
	if pond.Status != constants.PondStatusActive {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "养殖池非运行状态，不能安排投喂")
	}
	latest, err := s.readings.LatestForPond(pondID)
	if err == gorm.ErrRecordNotFound {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "安排执行前必须有水质读数")
	}
	if err != nil {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, WrapError(CodeInternal, "查询最新水质读数失败", err)
	}
	if time.Since(latest.MeasuredAt) > 24*time.Hour {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "最新水质读数已超过 24 小时")
	}
	if latest.DissolvedOxygen < plan.MinOxygen || latest.RiskLevel == constants.RiskCritical {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "当前水质不满足计划执行条件")
	}
	// 当日累计安排以建议给出的日投喂量为上限（减量建议时随之收紧）。
	dayStart := time.Date(scheduledAt.UTC().Year(), scheduledAt.UTC().Month(), scheduledAt.UTC().Day(), 0, 0, 0, 0, time.UTC)
	plannedForDay, err := s.repo.PlannedAmountForDay(pondID, dayStart, dayStart.Add(24*time.Hour), excludedID)
	if err != nil {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, WrapError(CodeInternal, "核算当日投喂安排失败", err)
	}
	dailyLimit := rec.DailyAmountKg
	if dailyLimit <= 0 {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, "所选建议的日投喂量为 0，请重新生成建议")
	}
	if plannedForDay+amount > dailyLimit+0.0001 {
		return model.FeedingPlan{}, model.Pond{}, model.WaterReading{}, model.FeedingRecommendation{}, NewError(CodeConflict, fmt.Sprintf("当日累计安排不能超过建议日投喂量 %.2f kg", dailyLimit))
	}
	return plan, pond, latest, rec, nil
}
