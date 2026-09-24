package model

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"time"
)

type ControlExecution struct {
	Base
	PondID        uint         `gorm:"not null;index" json:"pondId"`
	Pond          *Pond        `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"pond,omitempty"`
	FeedingPlanID uint         `gorm:"not null;index" json:"feedingPlanId"`
	FeedingPlan   *FeedingPlan `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"feedingPlan,omitempty"`
	// 安排执行时引用的有效投喂建议快照，记录编号与制定依据，便于事后追溯“按哪次建议执行”。
	RecommendationID *uint                     `gorm:"index" json:"recommendationId"`
	Recommendation   *FeedingRecommendation    `gorm:"constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"recommendation,omitempty"`
	RecommendationNo string                    `gorm:"size:24" json:"recommendationNo"`
	Basis            string                    `gorm:"type:text" json:"basis"`
	ScheduledAt      time.Time                 `gorm:"not null;index" json:"scheduledAt"`
	StartedAt        *time.Time                `json:"startedAt"`
	CompletedAt      *time.Time                `json:"completedAt"`
	PlannedAmountKg  float64                   `gorm:"not null" json:"plannedAmountKg"`
	ActualAmountKg   float64                   `gorm:"not null;default:0" json:"actualAmountKg"`
	Status           constants.ExecutionStatus `gorm:"size:20;not null;index" json:"status"`
	Operator         string                    `gorm:"size:80;not null" json:"operator"`
	Weather          string                    `gorm:"size:120" json:"weather"`
	OxygenSnapshot   float64                   `json:"oxygenSnapshot"`
	Feedback         string                    `gorm:"type:text" json:"feedback"`
}
