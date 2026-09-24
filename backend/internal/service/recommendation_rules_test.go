package service

import (
	"aquaculture-water-feeding-control/backend/internal/constants"
	"aquaculture-water-feeding-control/backend/internal/model"
	"testing"
	"time"
)

func TestComputeRecommendationScenarios(t *testing.T) {
	plan := model.FeedingPlan{
		Base: model.Base{ID: 1}, Version: 3, DailyAmountKg: 100, FrequencyPerDay: 2, MinOxygen: 5,
	}
	pond := model.Pond{GrowthStage: "成长期"}
	normalReading := model.WaterReading{RiskLevel: constants.RiskNormal, DissolvedOxygen: 7, Temperature: 26}

	tests := []struct {
		name        string
		reading     model.WaterReading
		pond        model.Pond
		weather     string
		wantAction  string
		wantFactor  float64
		wantDailyKg float64
	}{
		{"normal full feed", normalReading, pond, "晴朗", RecommendationActionFeed, 1.0, 100},
		{"warning reduces 30 percent", model.WaterReading{RiskLevel: constants.RiskWarning, DissolvedOxygen: 6, Temperature: 26}, pond, "晴朗", RecommendationActionReduce, 0.7, 70},
		{"warning and cold water", model.WaterReading{RiskLevel: constants.RiskWarning, DissolvedOxygen: 6, Temperature: 18}, pond, "晴朗", RecommendationActionReduce, 0.56, 56},
		{"storm forces hold", normalReading, pond, "暴雨", RecommendationActionHold, 0, 0},
		{"light rain reduces 15 percent", normalReading, pond, "小雨", RecommendationActionReduce, 0.85, 85},
		{"juvenile stage factor", normalReading, model.Pond{GrowthStage: "幼鱼期"}, "晴朗", RecommendationActionFeed, 0.9, 90},
		{"low oxygen forces hold", model.WaterReading{RiskLevel: constants.RiskNormal, DissolvedOxygen: 4, Temperature: 26}, pond, "晴朗", RecommendationActionHold, 0, 0},
		{"critical forces hold regardless of weather", model.WaterReading{RiskLevel: constants.RiskCritical, DissolvedOxygen: 8, Temperature: 26}, pond, "晴朗", RecommendationActionHold, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeRecommendation(plan, tc.reading, tc.pond, tc.weather)
			if got.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", got.Action, tc.wantAction)
			}
			if diff := got.Factor - tc.wantFactor; diff > 0.001 || diff < -0.001 {
				t.Fatalf("factor = %.4f, want %.4f", got.Factor, tc.wantFactor)
			}
			if got.DailyAmountKg != tc.wantDailyKg {
				t.Fatalf("daily = %.2f, want %.2f", got.DailyAmountKg, tc.wantDailyKg)
			}
		})
	}
}

func TestEvaluateRecommendationInvalid(t *testing.T) {
	now := time.Now().UTC()
	rec := &model.FeedingRecommendation{PlanVersion: 2, ReadingMeasuredAt: now.Add(-2 * time.Hour)}
	approved := model.FeedingPlan{Base: model.Base{ID: 9}, Status: constants.PlanStatusApproved, Version: 2}
	latest := model.WaterReading{Base: model.Base{ID: 5}, MeasuredAt: now.Add(-2 * time.Hour)}

	if reason := EvaluateRecommendationInvalid(rec, approved, latest, now); reason != "" {
		t.Fatalf("expected valid, got reason %q", reason)
	}

	t.Run("new reading supersedes", func(t *testing.T) {
		newer := latest
		newer.MeasuredAt = now.Add(-30 * time.Minute)
		if reason := EvaluateRecommendationInvalid(rec, approved, newer, now); reason != InvalidReasonNewReading {
			t.Fatalf("got %q", reason)
		}
	})

	t.Run("plan revoked back to draft", func(t *testing.T) {
		draft := approved
		draft.Status = constants.PlanStatusDraft
		if reason := EvaluateRecommendationInvalid(rec, draft, latest, now); reason != InvalidReasonPlanRevoked {
			t.Fatalf("got %q", reason)
		}
	})

	t.Run("plan version bumped", func(t *testing.T) {
		bumped := approved
		bumped.Version = 3
		if reason := EvaluateRecommendationInvalid(rec, bumped, latest, now); reason == "" {
			t.Fatal("expected version mismatch invalidation")
		}
	})

	t.Run("plan executed", func(t *testing.T) {
		done := approved
		done.Status = constants.PlanStatusExecuted
		if reason := EvaluateRecommendationInvalid(rec, done, latest, now); reason != InvalidReasonPlanExecuted {
			t.Fatalf("got %q", reason)
		}
	})

	t.Run("stale reading beyond 24 hours", func(t *testing.T) {
		old := rec
		old.ReadingMeasuredAt = now.Add(-25 * time.Hour)
		oldReading := latest
		oldReading.MeasuredAt = now.Add(-25 * time.Hour)
		if reason := EvaluateRecommendationInvalid(old, approved, oldReading, now); reason != InvalidReasonReadingStale {
			t.Fatalf("got %q", reason)
		}
	})

	t.Run("pond inactive", func(t *testing.T) {
		withPond := *rec
		withPond.Pond = &model.Pond{Status: constants.PondStatusQuarantine}
		if reason := EvaluateRecommendationInvalid(&withPond, approved, latest, now); reason != InvalidReasonPondState {
			t.Fatalf("got %q", reason)
		}
	})
}

func TestRecommendationNumberFormat(t *testing.T) {
	if got := model.RecommendationNumber(7); got != "REC-000007" {
		t.Fatalf("number = %q", got)
	}
}
