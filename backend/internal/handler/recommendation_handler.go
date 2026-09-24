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

func (h *RecommendationHandler) List(c *gin.Context) {
	query, ok := bindPageQuery(c)
	if !ok {
		return
	}
	pondID, _ := strconv.ParseUint(c.Query("pondId"), 10, 64)
	result, err := h.service.List(query, uint(pondID), c.Query("status"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *RecommendationHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	result, err := h.service.Get(id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *RecommendationHandler) Create(c *gin.Context) {
	var input dto.RecommendationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		respondError(c, service.NewError(service.CodeValidation, "建议生成参数不完整或格式无效"))
		return
	}
	result, err := h.service.Generate(input, actorFromContext(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": result})
}
