package httpserver

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type assetSemanticSpaceQueryRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (s *Server) handleGetAssetSemanticSpace(c *gin.Context) {
	result, err := s.assetEmbeddingService.GetAssetSemanticSpace(c.Request.Context(), assetFiltersFromRequest(c))
	if err != nil {
		Fail(c, http.StatusBadGateway, "semantic_space_failed", err.Error())
		return
	}
	OK(c, result)
}

func (s *Server) handleQueryAssetSemanticSpace(c *gin.Context) {
	var input assetSemanticSpaceQueryRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		Fail(c, http.StatusBadRequest, "invalid_request", "invalid semantic space query")
		return
	}
	if strings.TrimSpace(input.Query) == "" {
		Fail(c, http.StatusBadRequest, "invalid_request", "semantic query is required")
		return
	}
	result, err := s.assetEmbeddingService.QueryAssetSemanticSpace(c.Request.Context(), input.Query, assetFiltersFromRequest(c), input.Limit)
	if err != nil {
		Fail(c, http.StatusBadGateway, "semantic_search_failed", err.Error())
		return
	}
	OK(c, result)
}

func (s *Server) handleGetAssetSemanticNeighbors(c *gin.Context) {
	limit := parsePositiveInt(c.Query("limit"), 24)
	result, err := s.assetEmbeddingService.FindAssetSemanticNeighbors(c.Request.Context(), c.Param("assetID"), assetFiltersFromRequest(c), limit)
	if err != nil {
		Fail(c, http.StatusBadGateway, "semantic_neighbors_failed", err.Error())
		return
	}
	OK(c, result)
}
