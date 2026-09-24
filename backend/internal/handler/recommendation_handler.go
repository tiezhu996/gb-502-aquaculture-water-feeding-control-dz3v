package handler

import (
	"aquaculture-water-feeding-control/backend/internal/dto"
	"aquaculture-water-feeding-control/backend/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type RecommendationHandler struct {
	service *service.RecommendationService
}

func NewRecommendationHandler(recommendations *service.RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{service: recommendations}
}

// Generate 生成并保存一条带编号的投喂建议快照。
func (h *RecommendationHandler) Generate(c *gin.Context) {
	var input dto.GenerateRecommendationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, service.NewError(service.CodeValidation, "请选择养殖池并填写天气窗口"))
		return
	}
	result, err := h.service.Generate(input, actorFromContext(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}

// List 按养殖池查看投喂建议快照，可只看有效建议。
func (h *RecommendationHandler) List(c *gin.Context) {
	query, ok := bindPageQuery(c)
	if !ok {
		return
	}
	pondID, _ := strconv.ParseUint(c.Query("pondId"), 10, 64)
	// 默认只看有效建议；显式 validOnly=false（或 includeInvalid=1）时返回全部。
	validOnly := c.Query("validOnly") != "false" && c.Query("includeInvalid") != "1" && c.Query("includeInvalid") != "true"
	result, err := h.service.List(query, uint(pondID), validOnly)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}
