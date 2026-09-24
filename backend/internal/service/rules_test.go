package service

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/model"
	"strings"
	"testing"
	"time"
)

func TestAssessWaterRisk(t *testing.T) {
	tests := []struct {
		name        string
		input       dto.WaterReadingInput
		wantRisk    constants.RiskLevel
		messagePart string
	}{
		{
			name: "all measurements normal",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 6.2,
				Temperature:     26,
				PH:              7.5,
				Ammonia:         0.1,
				Turbidity:       30,
			},
			wantRisk:    constants.RiskNormal,
			messagePart: "控制范围",
		},
		{
			name: "oxygen warning",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 4.2,
				Temperature:     26,
				PH:              7.5,
				Ammonia:         0.1,
				Turbidity:       30,
			},
			wantRisk:    constants.RiskWarning,
			messagePart: "溶解氧",
		},
		{
			name: "oxygen critical",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 2.8,
				Temperature:     26,
				PH:              7.5,
				Ammonia:         0.1,
				Turbidity:       30,
			},
			wantRisk:    constants.RiskCritical,
			messagePart: "严重偏低",
		},
		{
			name: "ph warning",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 6,
				Temperature:     26,
				PH:              6.2,
				Ammonia:         0.1,
				Turbidity:       30,
			},
			wantRisk:    constants.RiskWarning,
			messagePart: "pH",
		},
		{
			name: "ammonia critical",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 6,
				Temperature:     26,
				PH:              7.5,
				Ammonia:         1.2,
				Turbidity:       30,
			},
			wantRisk:    constants.RiskCritical,
			messagePart: "氨氮",
		},
		{
			name: "temperature warning",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 6,
				Temperature:     33,
				PH:              7.5,
				Ammonia:         0.1,
				Turbidity:       30,
			},
			wantRisk:    constants.RiskWarning,
			messagePart: "水温",
		},
		{
			name: "turbidity warning",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 6,
				Temperature:     26,
				PH:              7.5,
				Ammonia:         0.1,
				Turbidity:       120,
			},
			wantRisk:    constants.RiskWarning,
			messagePart: "浊度",
		},
		{
			name: "critical takes precedence over warnings",
			input: dto.WaterReadingInput{
				DissolvedOxygen: 2,
				Temperature:     33,
				PH:              7.5,
				Ammonia:         0.5,
				Turbidity:       120,
			},
			wantRisk:    constants.RiskCritical,
			messagePart: "浊度",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotRisk, gotMessage := assessWaterRisk(test.input)
			if gotRisk != test.wantRisk {
				t.Fatalf("risk = %q, want %q; message=%q", gotRisk, test.wantRisk, gotMessage)
			}
			if !strings.Contains(gotMessage, test.messagePart) {
				t.Fatalf("message %q does not contain %q", gotMessage, test.messagePart)
			}
		})
	}
}

func TestExecutionStatusCannotMoveBackToScheduled(t *testing.T) {
	if constants.ExecutionRunning.CanTransitionTo(constants.ExecutionScheduled) {
		t.Fatal("running execution must not return to scheduled")
	}
	if !constants.ExecutionScheduled.CanTransitionTo(constants.ExecutionRunning) {
		t.Fatal("scheduled execution should be able to start")
	}
	if constants.ExecutionCompleted.CanTransitionTo(constants.ExecutionRunning) {
		t.Fatal("completed execution must be terminal")
	}
}

func TestComposeExecutionBasis(t *testing.T) {
	recommendation := model.Recommendation{
		Code: "REC-000007", PlanVersion: 3, ReadingMeasuredAt: time.Date(2026, 9, 24, 8, 30, 0, 0, time.UTC),
		DissolvedOxygen: 6.4, RiskLevel: constants.RiskWarning, Weather: "小雨",
		AdjustmentPercent: -30, AmountPerFeedingKg: 23.33, FrequencyPerDay: 3,
	}
	plan := model.FeedingPlan{Name: "秋季育肥计划", DailyAmountKg: 100}
	basis := composeExecutionBasis(recommendation, plan)
	for _, part := range []string{"REC-000007", "秋季育肥计划", "v3", "2026-09-24 08:30", "6.40", "warning", "小雨", "-30.0%", "23.33"} {
		if !strings.Contains(basis, part) {
			t.Fatalf("basis %q does not contain %q", basis, part)
		}
	}
}
