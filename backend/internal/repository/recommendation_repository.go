package repository

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/model"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RecommendationRepository struct {
	db *gorm.DB
}

func NewRecommendationRepository(db *gorm.DB) *RecommendationRepository {
	return &RecommendationRepository{db: db}
}

func (r *RecommendationRepository) List(query dto.PageQuery, pondID uint, status string) ([]model.Recommendation, int64, error) {
	base := r.db.Model(&model.Recommendation{})
	if pondID > 0 {
		base = base.Where("pond_id = ?", pondID)
	}
	if status != "" {
		base = base.Where("status = ?", status)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var recommendations []model.Recommendation
	err := base.Preload("Pond").Preload("FeedingPlan").Order("created_at DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&recommendations).Error
	return recommendations, total, err
}

func (r *RecommendationRepository) Get(id uint) (model.Recommendation, error) {
	var recommendation model.Recommendation
	err := r.db.Preload("Pond").Preload("FeedingPlan").First(&recommendation, id).Error
	return recommendation, err
}

func (r *RecommendationRepository) GetForUpdate(id uint) (model.Recommendation, error) {
	var recommendation model.Recommendation
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Pond").Preload("FeedingPlan").First(&recommendation, id).Error
	return recommendation, err
}

func (r *RecommendationRepository) Create(recommendation *model.Recommendation) error {
	return r.db.Create(recommendation).Error
}

func (r *RecommendationRepository) Save(recommendation *model.Recommendation) error {
	return r.db.Save(recommendation).Error
}

func (r *RecommendationRepository) invalidate(where func(*gorm.DB) *gorm.DB, reason string, now time.Time) (int64, error) {
	result := where(r.db.Model(&model.Recommendation{}).Where("status = ?", constants.RecommendationValid)).
		Updates(map[string]any{
			"status":         constants.RecommendationInvalid,
			"invalid_reason": reason,
			"invalidated_at": now,
		})
	return result.RowsAffected, result.Error
}

func (r *RecommendationRepository) InvalidateValidForPond(pondID uint, reason string, now time.Time) (int64, error) {
	return r.invalidate(func(tx *gorm.DB) *gorm.DB { return tx.Where("pond_id = ?", pondID) }, reason, now)
}

func (r *RecommendationRepository) InvalidateValidForPlan(planID uint, reason string, now time.Time) (int64, error) {
	return r.invalidate(func(tx *gorm.DB) *gorm.DB { return tx.Where("feeding_plan_id = ?", planID) }, reason, now)
}

func (r *RecommendationRepository) InvalidateValidForReading(readingID uint, reason string, now time.Time) (int64, error) {
	return r.invalidate(func(tx *gorm.DB) *gorm.DB { return tx.Where("water_reading_id = ?", readingID) }, reason, now)
}
