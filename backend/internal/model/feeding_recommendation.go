package model

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"fmt"
	"time"
)

// FeedingRecommendation 是一次投喂建议生成时保存的带编号快照。
// 它固化当时的计划版本、水质读数、天气窗口和调整比例，供安排执行时引用与审计追溯。
type FeedingRecommendation struct {
	Base
	SnapshotNo string `gorm:"size:24;index;not null" json:"snapshotNo"`

	PondID         uint          `gorm:"not null;index" json:"pondId"`
	Pond           *Pond         `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"pond,omitempty"`
	FeedingPlanID  uint          `gorm:"not null;index" json:"feedingPlanId"`
	FeedingPlan    *FeedingPlan  `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"feedingPlan,omitempty"`
	WaterReadingID *uint         `gorm:"index" json:"waterReadingId"`
	WaterReading   *WaterReading `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"waterReading,omitempty"`

	// 计划版本与生长阶段在生成时固化。
	PlanVersion int    `gorm:"not null;index" json:"planVersion"`
	GrowthStage string `gorm:"size:40;not null" json:"growthStage"`

	// 生成建议所依据的水质读数快照。
	ReadingMeasuredAt time.Time           `gorm:"not null;index" json:"readingMeasuredAt"`
	DissolvedOxygen   float64             `gorm:"not null" json:"dissolvedOxygen"`
	Temperature       float64             `gorm:"not null" json:"temperature"`
	PH                float64             `gorm:"column:ph;not null" json:"ph"`
	Ammonia           float64             `gorm:"not null" json:"ammonia"`
	Turbidity         float64             `gorm:"not null" json:"turbidity"`
	RiskLevel         constants.RiskLevel `gorm:"size:20;not null;index" json:"riskLevel"`

	// 天气窗口与建议结论。
	WeatherWindow      string  `gorm:"size:120" json:"weatherWindow"`
	Action             string  `gorm:"size:20;not null;index" json:"action"`
	DailyAmountKg      float64 `gorm:"not null" json:"dailyAmountKg"`
	AmountPerFeedingKg float64 `gorm:"not null" json:"amountPerFeedingKg"`
	FrequencyPerDay    int     `gorm:"not null" json:"frequencyPerDay"`
	AdjustmentPercent  float64 `gorm:"not null" json:"adjustmentPercent"`

	// Reasons 以 JSON 数组持久化，Reasons 字段由服务层填充后返回给前端。
	ReasonsRaw  string   `gorm:"column:reasons;type:text;not null" json:"-"`
	Reasons     []string `gorm:"-" json:"reasons"`
	GeneratedBy string   `gorm:"size:80;not null" json:"generatedBy"`

	// 有效性：新水质、计划撤销/重批、池塘状态变化等会把建议置为失效并记录原因。
	Valid         bool       `gorm:"not null;default:true;index" json:"valid"`
	InvalidReason string     `gorm:"size:200" json:"invalidReason"`
	InvalidatedAt *time.Time `json:"invalidatedAt"`
}

// RecommendationNumber 依据主键生成人类可读、可口头核对的建议编号。
func RecommendationNumber(id uint) string {
	return fmt.Sprintf("REC-%06d", id)
}
