package repository

import (
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

// Create 先写入取得主键，再回填带编号的 SnapshotNo；编号在业务层唯一且连续可读。
func (r *RecommendationRepository) Create(rec *model.FeedingRecommendation) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(rec).Error; err != nil {
			return err
		}
		rec.SnapshotNo = model.RecommendationNumber(rec.ID)
		return tx.Model(rec).Update("snapshot_no", rec.SnapshotNo).Error
	})
}

func (r *RecommendationRepository) Get(id uint) (model.FeedingRecommendation, error) {
	var rec model.FeedingRecommendation
	err := r.db.Preload("Pond").Preload("FeedingPlan").Preload("WaterReading").First(&rec, id).Error
	return rec, err
}

func (r *RecommendationRepository) GetForUpdate(id uint) (model.FeedingRecommendation, error) {
	var rec model.FeedingRecommendation
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Pond").Preload("FeedingPlan").Preload("WaterReading").First(&rec, id).Error
	return rec, err
}

// ListForPond 返回某养殖池（可选）、按有效性过滤的全部快照，最新在前。
// 数据量以单池为单位可控，由服务层做动态失效判定与分页。
func (r *RecommendationRepository) ListForPond(pondID uint, validOnly bool, limit int) ([]model.FeedingRecommendation, error) {
	query := r.db.Preload("Pond").Preload("FeedingPlan")
	if pondID > 0 {
		query = query.Where("pond_id = ?", pondID)
	}
	if validOnly {
		query = query.Where("valid = ?", true)
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	var recs []model.FeedingRecommendation
	err := query.Order("created_at DESC").Limit(limit).Find(&recs).Error
	return recs, err
}

// CountReferencingReading 返回引用指定水质读数、仍有效的建议数量。
func (r *RecommendationRepository) CountReferencingReading(readingID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.FeedingRecommendation{}).Where("water_reading_id = ?", readingID).Count(&count).Error
	return count, err
}

// CountForPlan 返回引用指定计划的建议总数（含失效快照，用于删除保护）。
func (r *RecommendationRepository) CountForPlan(planID uint) (int64, error) {
	var count int64
	err := r.db.Model(&model.FeedingRecommendation{}).Where("feeding_plan_id = ?", planID).Count(&count).Error
	return count, err
}

// MarkPondInvalid 把某养殖池所有仍有效建议置为失效。
func (r *RecommendationRepository) MarkPondInvalid(pondID uint, reason string, now time.Time) (int64, error) {
	result := r.db.Model(&model.FeedingRecommendation{}).
		Where("pond_id = ? AND valid = ?", pondID, true).
		Updates(map[string]any{"valid": false, "invalid_reason": reason, "invalidated_at": now})
	return result.RowsAffected, result.Error
}

// MarkPlanInvalid 把某计划所有仍有效建议置为失效。
func (r *RecommendationRepository) MarkPlanInvalid(planID uint, reason string, now time.Time) (int64, error) {
	result := r.db.Model(&model.FeedingRecommendation{}).
		Where("feeding_plan_id = ? AND valid = ?", planID, true).
		Updates(map[string]any{"valid": false, "invalid_reason": reason, "invalidated_at": now})
	return result.RowsAffected, result.Error
}

// MarkSuperseded 把某养殖池中、依据水质不晚于指定读数的仍有效建议置为失效。
// 用于新读数录入：以旧读数为依据的建议被新水质取代；excludeID 用于保留本次刚生成的快照。
func (r *RecommendationRepository) MarkSuperseded(pondID uint, measuredAt time.Time, excludeID uint, reason string, now time.Time) (int64, error) {
	result := r.db.Model(&model.FeedingRecommendation{}).
		Where("pond_id = ? AND valid = ? AND reading_measured_at <= ? AND id <> ?", pondID, true, measuredAt, excludeID).
		Updates(map[string]any{"valid": false, "invalid_reason": reason, "invalidated_at": now})
	return result.RowsAffected, result.Error
}

// MarkOne 把单条建议置为失效（动态失效落库）。
func (r *RecommendationRepository) MarkOne(id uint, reason string, now time.Time) error {
	return r.db.Model(&model.FeedingRecommendation{}).
		Where("id = ? AND valid = ?", id, true).
		Updates(map[string]any{"valid": false, "invalid_reason": reason, "invalidated_at": now}).Error
}
