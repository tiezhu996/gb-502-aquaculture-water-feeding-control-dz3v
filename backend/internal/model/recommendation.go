package model

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"time"
)

// Recommendation 是每次生成投喂建议时保存的带编号快照，
// 记录当时的计划版本、水质读数、天气窗口和调整比例。
type Recommendation struct {
	Base
	Code               string                         `gorm:"size:20;uniqueIndex;not null" json:"code"`
	PondID             uint                           `gorm:"not null;index" json:"pondId"`
	Pond               *Pond                          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"pond,omitempty"`
	FeedingPlanID      uint                           `gorm:"not null;index" json:"feedingPlanId"`
	FeedingPlan        *FeedingPlan                   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"feedingPlan,omitempty"`
	PlanVersion        int                            `gorm:"not null" json:"planVersion"`
	WaterReadingID     uint                           `gorm:"not null;index" json:"waterReadingId"`
	ReadingMeasuredAt  time.Time                      `gorm:"not null" json:"readingMeasuredAt"`
	DissolvedOxygen    float64                        `gorm:"not null" json:"dissolvedOxygen"`
	Temperature        float64                        `gorm:"not null" json:"temperature"`
	PH                 float64                        `gorm:"column:ph;not null" json:"ph"`
	Ammonia            float64                        `gorm:"not null" json:"ammonia"`
	Turbidity          float64                        `gorm:"not null" json:"turbidity"`
	RiskLevel          constants.RiskLevel            `gorm:"size:20;not null" json:"riskLevel"`
	Weather            string                         `gorm:"size:120" json:"weather"`
	Action             string                         `gorm:"size:20;not null" json:"action"`
	DailyAmountKg      float64                        `gorm:"not null" json:"dailyAmountKg"`
	AmountPerFeedingKg float64                        `gorm:"not null" json:"amountPerFeedingKg"`
	FrequencyPerDay    int                            `gorm:"not null" json:"frequencyPerDay"`
	AdjustmentPercent  float64                        `gorm:"not null" json:"adjustmentPercent"`
	Reasons            []string                       `gorm:"type:text;serializer:json" json:"reasons"`
	Status             constants.RecommendationStatus `gorm:"size:20;not null;index" json:"status"`
	InvalidReason      string                         `gorm:"type:text" json:"invalidReason"`
	InvalidatedAt      *time.Time                     `json:"invalidatedAt"`
	GeneratedBy        string                         `gorm:"size:80;not null" json:"generatedBy"`
}
