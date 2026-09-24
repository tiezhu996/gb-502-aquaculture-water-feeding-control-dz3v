package dto

type RecommendationInput struct {
	PondID  uint   `json:"pondId" binding:"required"`
	Weather string `json:"weather" binding:"max=120"`
}
