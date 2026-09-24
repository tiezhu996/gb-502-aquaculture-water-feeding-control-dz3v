package service

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/model"
	"aquaculture-water-feeding-control/backend/internal/repository"
	"fmt"
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

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

func (s *RecommendationService) List(query dto.PageQuery, pondID uint, status string) (dto.PageResult[model.Recommendation], error) {
	query.Normalize()
	if status != "" && !constants.RecommendationStatus(status).Valid() {
		return dto.PageResult[model.Recommendation]{}, NewError(CodeValidation, "建议快照状态无效")
	}
	items, total, err := s.repo.List(query, pondID, status)
	if err != nil {
		return dto.PageResult[model.Recommendation]{}, WrapError(CodeInternal, "查询建议快照失败", err)
	}
	return dto.PageResult[model.Recommendation]{Items: items, Total: total, Page: query.Page, PageSize: query.PageSize}, nil
}

func (s *RecommendationService) Get(id uint) (model.Recommendation, error) {
	recommendation, err := s.repo.Get(id)
	if err == gorm.ErrRecordNotFound {
		return model.Recommendation{}, NewError(CodeNotFound, "建议快照不存在")
	}
	if err != nil {
		return model.Recommendation{}, WrapError(CodeInternal, "查询建议快照失败", err)
	}
	return recommendation, nil
}

// Generate 计算投喂建议并保存为带编号快照，之后的执行安排必须引用有效快照。
func (s *RecommendationService) Generate(input dto.RecommendationInput, actor Actor) (model.Recommendation, error) {
	if !s.transactional {
		var result model.Recommendation
		err := s.withinTransaction(func(scoped *RecommendationService) error {
			var inner error
			result, inner = scoped.Generate(input, actor)
			return inner
		})
		return result, err
	}
	pond, err := s.ponds.GetForUpdate(input.PondID)
	if err == gorm.ErrRecordNotFound {
		return model.Recommendation{}, NewError(CodeNotFound, "养殖池不存在")
	}
	if err != nil {
		return model.Recommendation{}, WrapError(CodeInternal, "查询养殖池失败", err)
	}
	if pond.Status != constants.PondStatusActive {
		return model.Recommendation{}, NewError(CodeConflict, "只能为运行中养殖池生成投喂建议")
	}
	plan, err := s.plans.LatestApprovedForPond(input.PondID)
	if err == gorm.ErrRecordNotFound {
		return model.Recommendation{}, NewError(CodeConflict, "当前养殖池没有已批准的投喂计划")
	}
	if err != nil {
		return model.Recommendation{}, WrapError(CodeInternal, "查询已批准计划失败", err)
	}
	reading, err := s.readings.LatestForPond(input.PondID)
	if err == gorm.ErrRecordNotFound {
		return model.Recommendation{}, NewError(CodeConflict, "生成建议前必须有水质读数")
	}
	if err != nil {
		return model.Recommendation{}, WrapError(CodeInternal, "查询最新水质读数失败", err)
	}
	if time.Since(reading.MeasuredAt) > 24*time.Hour {
		return model.Recommendation{}, NewError(CodeConflict, "最新水质读数已超过 24 小时，请先采集新读数")
	}

	weather := strings.TrimSpace(input.Weather)
	factor := 1.0
	reasons := []string{"以已批准计划 v" + fmt.Sprint(plan.Version) + " 为基准"}
	action := "feed"
	if reading.RiskLevel == constants.RiskCritical || reading.DissolvedOxygen < plan.MinOxygen {
		factor = 0
		action = "hold"
		reasons = append(reasons, "水质严重异常或溶解氧低于计划阈值，暂停投喂")
	} else {
		if reading.RiskLevel == constants.RiskWarning {
			factor *= 0.7
			action = "reduce"
			reasons = append(reasons, "存在水质预警，建议减量 30%")
		}
		if reading.Temperature < 20 || reading.Temperature > 31 {
			factor *= 0.8
			action = "reduce"
			reasons = append(reasons, "水温不在最佳摄食区间，追加减量 20%")
		}
		weatherText := strings.ToLower(weather)
		if strings.Contains(weatherText, "storm") || strings.Contains(weather, "暴雨") || strings.Contains(weather, "雷雨") {
			factor = 0
			action = "hold"
			reasons = append(reasons, "强对流天气窗口不适合投喂")
		} else if strings.Contains(weatherText, "rain") || strings.Contains(weather, "小雨") || strings.Contains(weather, "大风") {
			factor *= 0.85
			action = "reduce"
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
	recommendation := model.Recommendation{
		PondID: pond.ID, FeedingPlanID: plan.ID, PlanVersion: plan.Version,
		WaterReadingID: reading.ID, ReadingMeasuredAt: reading.MeasuredAt,
		DissolvedOxygen: reading.DissolvedOxygen, Temperature: reading.Temperature, PH: reading.PH,
		Ammonia: reading.Ammonia, Turbidity: reading.Turbidity, RiskLevel: reading.RiskLevel,
		Weather: weather, Action: action, DailyAmountKg: dailyAmount, AmountPerFeedingKg: perFeeding,
		FrequencyPerDay: plan.FrequencyPerDay, AdjustmentPercent: math.Round((factor-1)*10000) / 100,
		Reasons: reasons, Status: constants.RecommendationValid, GeneratedBy: actor.DisplayName,
	}
	if recommendation.GeneratedBy == "" {
		recommendation.GeneratedBy = actor.Username
	}
	if err := s.repo.Create(&recommendation); err != nil {
		return model.Recommendation{}, WrapError(CodeInternal, "保存建议快照失败", err)
	}
	recommendation.Code = fmt.Sprintf("REC-%06d", recommendation.ID)
	if err := s.repo.Save(&recommendation); err != nil {
		return model.Recommendation{}, WrapError(CodeInternal, "生成建议编号失败", err)
	}
	recommendation.Pond = &pond
	recommendation.FeedingPlan = &plan
	if err := s.audit.Record(actor, "generate", "recommendation", recommendation.ID, nil, recommendation, "生成投喂建议快照 "+recommendation.Code); err != nil {
		return model.Recommendation{}, err
	}
	return recommendation, nil
}
