package service

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/model"
	"aquaculture-water-feeding-control/backend/internal/repository"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ComputedRecommendation 是建议规则的纯计算结果，便于单元测试。
type ComputedRecommendation struct {
	Factor             float64
	Action             string
	AdjustmentPercent  float64
	DailyAmountKg      float64
	AmountPerFeedingKg float64
	Reasons            []string
}

const (
	RecommendationActionFeed   = "feed"
	RecommendationActionReduce = "reduce"
	RecommendationActionHold   = "hold"
)

// 失效原因集中定义，保证页面展示与落库原因一致。
const (
	InvalidReasonNewReading   = "出现新的水质读数，建议依据的水质已更新"
	InvalidReasonPlanRevoked  = "投喂计划已撤销"
	InvalidReasonPlanVersion  = "投喂计划已修订升版"
	InvalidReasonPlanExecuted = "投喂计划已执行完成"
	InvalidReasonPondState    = "养殖池状态已变更，不再处于运行中"
	InvalidReasonReadingStale = "建议依据的水质读数已超过 24 小时"
	InvalidReasonSuperseded   = "已生成更新的投喂建议"
)

// computeRecommendation 依据已批准计划、最新读数、天气窗口和生长阶段计算调整系数。
func computeRecommendation(plan model.FeedingPlan, reading model.WaterReading, pond model.Pond, weather string) ComputedRecommendation {
	factor := 1.0
	reasons := []string{fmt.Sprintf("以已批准计划 v%d 为基准", plan.Version)}
	action := RecommendationActionFeed
	if reading.RiskLevel == constants.RiskCritical || reading.DissolvedOxygen < plan.MinOxygen {
		factor = 0
		action = RecommendationActionHold
		reasons = append(reasons, "水质严重异常或溶解氧低于计划阈值，暂停投喂")
	} else {
		if reading.RiskLevel == constants.RiskWarning {
			factor *= 0.7
			action = RecommendationActionReduce
			reasons = append(reasons, "存在水质预警，建议减量 30%")
		}
		if reading.Temperature < 20 || reading.Temperature > 31 {
			factor *= 0.8
			action = RecommendationActionReduce
			reasons = append(reasons, "水温不在最佳摄食区间，追加减量 20%")
		}
		weatherText := strings.ToLower(strings.TrimSpace(weather))
		if strings.Contains(weatherText, "storm") || strings.Contains(weather, "暴雨") || strings.Contains(weather, "雷雨") {
			factor = 0
			action = RecommendationActionHold
			reasons = append(reasons, "强对流天气窗口不适合投喂")
		} else if strings.Contains(weatherText, "rain") || strings.Contains(weather, "小雨") || strings.Contains(weather, "大风") {
			factor *= 0.85
			action = RecommendationActionReduce
			reasons = append(reasons, "天气窗口不稳定，追加减量 15%")
		}
		if strings.Contains(pond.GrowthStage, "幼") {
			factor *= 0.9
			reasons = append(reasons, "幼体阶段采用少量多餐系数")
		}
	}
	dailyAmount := math.Round(plan.DailyAmountKg*factor*100) / 100
	perFeeding := 0.0
	if plan.FrequencyPerDay > 0 {
		perFeeding = math.Round(dailyAmount/float64(plan.FrequencyPerDay)*100) / 100
	}
	return ComputedRecommendation{
		Factor: factor, Action: action,
		AdjustmentPercent: math.Round((factor-1)*10000) / 100,
		DailyAmountKg:     dailyAmount, AmountPerFeedingKg: perFeeding,
		Reasons: reasons,
	}
}

type RecommendationService struct {
	repo          *repository.RecommendationRepository
	plans         *repository.PlanRepository
	ponds         *repository.PondRepository
	readings      *repository.ReadingRepository
	audit         *AuditService
	transactional bool
}

func (s *RecommendationService) withinTransaction(fn func(*RecommendationService) error) error {
	return s.audit.WithinTransaction(func(tx *gorm.DB, audit *AuditService) error {
		scoped := &RecommendationService{
			repo: repository.NewRecommendationRepository(tx), plans: repository.NewPlanRepository(tx),
			ponds: repository.NewPondRepository(tx), readings: repository.NewReadingRepository(tx),
			audit: audit, transactional: true,
		}
		return fn(scoped)
	})
}

func NewRecommendationService(repo *repository.RecommendationRepository, plans *repository.PlanRepository, ponds *repository.PondRepository, readings *repository.ReadingRepository, audit *AuditService) *RecommendationService {
	return &RecommendationService{repo: repo, plans: plans, ponds: ponds, readings: readings, audit: audit}
}

// Generate 生成建议并保存为带编号快照；同池旧建议同时标记为被取代。
func (s *RecommendationService) Generate(input dto.GenerateRecommendationInput, actor Actor) (model.FeedingRecommendation, error) {
	if !s.transactional {
		var result model.FeedingRecommendation
		err := s.withinTransaction(func(scoped *RecommendationService) error {
			var inner error
			result, inner = scoped.Generate(input, actor)
			return inner
		})
		return result, err
	}
	pond, err := s.ponds.GetForUpdate(input.PondID)
	if err == gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, NewError(CodeNotFound, "养殖池不存在")
	}
	if err != nil {
		return model.FeedingRecommendation{}, WrapError(CodeInternal, "查询养殖池失败", err)
	}
	if pond.Status != constants.PondStatusActive {
		return model.FeedingRecommendation{}, NewError(CodeConflict, "只能为运行中养殖池生成投喂建议")
	}
	plan, err := s.plans.LatestApprovedForPond(input.PondID)
	if err == gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, NewError(CodeConflict, "当前养殖池没有已批准的投喂计划")
	}
	if err != nil {
		return model.FeedingRecommendation{}, WrapError(CodeInternal, "查询已批准计划失败", err)
	}
	reading, err := s.readings.LatestForPond(input.PondID)
	if err == gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, NewError(CodeConflict, "生成建议前必须有水质读数")
	}
	if err != nil {
		return model.FeedingRecommendation{}, WrapError(CodeInternal, "查询最新水质读数失败", err)
	}
	if time.Since(reading.MeasuredAt) > 24*time.Hour {
		return model.FeedingRecommendation{}, NewError(CodeConflict, "最新水质读数已超过 24 小时，请先采集新读数")
	}

	computed := computeRecommendation(plan, reading, pond, input.Weather)
	rawReasons, err := json.Marshal(computed.Reasons)
	if err != nil {
		return model.FeedingRecommendation{}, WrapError(CodeInternal, "序列化建议依据失败", err)
	}
	readingID := reading.ID
	generatedBy := actor.DisplayName
	if generatedBy == "" {
		generatedBy = actor.Username
	}
	rec := model.FeedingRecommendation{
		PondID: input.PondID, FeedingPlanID: plan.ID, WaterReadingID: &readingID,
		PlanVersion: plan.Version, GrowthStage: pond.GrowthStage,
		ReadingMeasuredAt: reading.MeasuredAt.UTC(), DissolvedOxygen: reading.DissolvedOxygen,
		Temperature: reading.Temperature, PH: reading.PH, Ammonia: reading.Ammonia, Turbidity: reading.Turbidity,
		RiskLevel: reading.RiskLevel, WeatherWindow: strings.TrimSpace(input.Weather), Action: computed.Action,
		DailyAmountKg: computed.DailyAmountKg, AmountPerFeedingKg: computed.AmountPerFeedingKg,
		FrequencyPerDay: plan.FrequencyPerDay, AdjustmentPercent: computed.AdjustmentPercent,
		ReasonsRaw: string(rawReasons), GeneratedBy: generatedBy, Valid: true,
	}
	if err := s.repo.Create(&rec); err != nil {
		return model.FeedingRecommendation{}, WrapError(CodeInternal, "保存投喂建议快照失败", err)
	}
	if _, err := s.repo.MarkSuperseded(input.PondID, reading.MeasuredAt.UTC(), rec.ID, InvalidReasonSuperseded, time.Now().UTC()); err != nil {
		return model.FeedingRecommendation{}, WrapError(CodeInternal, "失效旧投喂建议失败", err)
	}
	hydrateRecommendation(&rec)
	rec.Pond = &pond
	rec.FeedingPlan = &plan
	rec.WaterReading = &reading
	if err := s.audit.Record(actor, "generate", "feeding_recommendation", rec.ID, nil, rec,
		fmt.Sprintf("生成投喂建议 %s（计划 v%d，调整 %.0f%%）", rec.SnapshotNo, plan.Version, computed.AdjustmentPercent)); err != nil {
		return model.FeedingRecommendation{}, err
	}
	return rec, nil
}

// List 按养殖池查看建议快照，validOnly 只返回当前有效建议，并做动态失效校验。
func (s *RecommendationService) List(query dto.PageQuery, pondID uint, validOnly bool) (dto.PageResult[model.FeedingRecommendation], error) {
	query.Normalize()
	if pondID == 0 {
		return dto.PageResult[model.FeedingRecommendation]{}, NewError(CodeValidation, "请选择养殖池后查看投喂建议")
	}
	recs, err := s.repo.ListForPond(pondID, false, 1000)
	if err != nil {
		return dto.PageResult[model.FeedingRecommendation]{}, WrapError(CodeInternal, "查询投喂建议失败", err)
	}
	var latest model.WaterReading
	latest, lerr := s.readings.LatestForPond(pondID)
	if lerr != nil && lerr != gorm.ErrRecordNotFound {
		return dto.PageResult[model.FeedingRecommendation]{}, WrapError(CodeInternal, "查询最新水质失败", lerr)
	}
	planCache := make(map[uint]model.FeedingPlan)
	now := time.Now().UTC()
	hydrated := make([]model.FeedingRecommendation, 0, len(recs))
	for i := range recs {
		rec := &recs[i]
		hydrateRecommendation(rec)
		if rec.Valid {
			plan, ok := planCache[rec.FeedingPlanID]
			if !ok {
				plan, err = s.plans.Get(rec.FeedingPlanID)
				if err != nil && err != gorm.ErrRecordNotFound {
					return dto.PageResult[model.FeedingRecommendation]{}, WrapError(CodeInternal, "查询关联计划失败", err)
				}
				planCache[rec.FeedingPlanID] = plan
			}
			if reason := EvaluateRecommendationInvalid(rec, plan, latest, now); reason != "" {
				rec.Valid = false
				rec.InvalidReason = reason
				rec.InvalidatedAt = &now
				_ = s.repo.MarkOne(rec.ID, reason, now)
			}
		}
		if !validOnly || rec.Valid {
			hydrated = append(hydrated, *rec)
		}
	}
	total := int64(len(hydrated))
	start := (query.Page - 1) * query.PageSize
	if start > len(hydrated) {
		start = len(hydrated)
	}
	end := start + query.PageSize
	if end > len(hydrated) {
		end = len(hydrated)
	}
	page := hydrated[start:end]
	return dto.PageResult[model.FeedingRecommendation]{Items: page, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

// GetForExecution 取出建议并强制校验其当前仍然有效，失效则返回携带原因的冲突错误。
func (s *RecommendationService) GetForExecution(id uint) (model.FeedingRecommendation, string, error) {
	rec, err := s.repo.Get(id)
	if err == gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, "", NewError(CodeNotFound, "投喂建议不存在")
	}
	if err != nil {
		return model.FeedingRecommendation{}, "", WrapError(CodeInternal, "查询投喂建议失败", err)
	}
	hydrateRecommendation(&rec)
	if !rec.Valid {
		reason := rec.InvalidReason
		if reason == "" {
			reason = "该投喂建议已失效"
		}
		return rec, reason, NewError(CodeConflict, "所选用的投喂建议已失效："+reason)
	}
	plan, err := s.plans.Get(rec.FeedingPlanID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, "", WrapError(CodeInternal, "查询关联计划失败", err)
	}
	latest, lerr := s.readings.LatestForPond(rec.PondID)
	if lerr != nil && lerr != gorm.ErrRecordNotFound {
		return model.FeedingRecommendation{}, "", WrapError(CodeInternal, "查询最新水质失败", lerr)
	}
	if reason := EvaluateRecommendationInvalid(&rec, plan, latest, time.Now().UTC()); reason != "" {
		_ = s.repo.MarkOne(rec.ID, reason, time.Now().UTC())
		return rec, reason, NewError(CodeConflict, "所选用的投喂建议已失效："+reason)
	}
	return rec, "", nil
}

// EvaluateRecommendationInvalid 动态判定一条“标记为有效”的建议是否其实已失效。
// 返回失效原因；空串表示仍有效。可在其它服务的事务内直接调用。
func EvaluateRecommendationInvalid(rec *model.FeedingRecommendation, plan model.FeedingPlan, latest model.WaterReading, now time.Time) string {
	if rec.Pond != nil && rec.Pond.Status != constants.PondStatusActive {
		return InvalidReasonPondState
	}
	if plan.ID == 0 {
		return InvalidReasonPlanRevoked
	}
	switch plan.Status {
	case constants.PlanStatusDraft, constants.PlanStatusPending:
		return InvalidReasonPlanRevoked
	case constants.PlanStatusExecuted:
		return InvalidReasonPlanExecuted
	}
	if plan.Version != rec.PlanVersion {
		return fmt.Sprintf("%s（当前 v%d，建议依据 v%d）", InvalidReasonPlanVersion, plan.Version, rec.PlanVersion)
	}
	if latest.ID != 0 && latest.MeasuredAt.After(rec.ReadingMeasuredAt) {
		return InvalidReasonNewReading
	}
	if now.Sub(rec.ReadingMeasuredAt) > 24*time.Hour {
		return InvalidReasonReadingStale
	}
	return ""
}

// hydrateRecommendation 解析持久化的 JSON 依据，并填充展示字段。
func hydrateRecommendation(rec *model.FeedingRecommendation) {
	if len(rec.Reasons) == 0 && rec.ReasonsRaw != "" {
		var reasons []string
		if err := json.Unmarshal([]byte(rec.ReasonsRaw), &reasons); err == nil {
			rec.Reasons = reasons
		} else {
			rec.Reasons = []string{}
		}
	}
}
