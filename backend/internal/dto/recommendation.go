package dto

// GenerateRecommendationInput 是生成并保存投喂建议快照的请求。
type GenerateRecommendationInput struct {
	PondID  uint   `json:"pondId" binding:"required"`
	Weather string `json:"weather" binding:"max:120"`
}
