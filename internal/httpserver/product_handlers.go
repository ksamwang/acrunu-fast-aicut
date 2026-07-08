package httpserver

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ksamwang/acrunu-fast-aicut/internal/services"
)

type createProductRequest struct {
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Metadata    map[string]any `json:"metadata"`
}

type updateProductRequest struct {
	Name        string         `json:"name" binding:"required"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Metadata    map[string]any `json:"metadata"`
}

type createSellingPointRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

type updateSellingPointRequest struct {
	Title       string `json:"title" binding:"required"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

func (s *Server) handleCreateProduct(c *gin.Context) {
	var req createProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid product payload")
		return
	}

	product := s.productAssetService.CreateProduct(services.CreateProductInput(req))
	Created(c, product)
}

func (s *Server) handleListProducts(c *gin.Context) {
	OK(c, s.productAssetService.ListProducts())
}

func (s *Server) handleGetProduct(c *gin.Context) {
	product, err := s.productAssetService.GetProduct(c.Param("productID"))
	if err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, product)
}

func (s *Server) handleUpdateProduct(c *gin.Context) {
	var req updateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid product payload")
		return
	}

	product, err := s.productAssetService.UpdateProduct(c.Param("productID"), services.UpdateProductInput(req))
	if err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, product)
}

func (s *Server) handleArchiveProduct(c *gin.Context) {
	if err := s.productAssetService.ArchiveProduct(c.Param("productID")); err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, gin.H{"archived": true})
}

func (s *Server) handleCreateSellingPoint(c *gin.Context) {
	var req createSellingPointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid selling point payload")
		return
	}

	sellingPoint, err := s.productAssetService.CreateSellingPoint(c.Param("productID"), services.CreateSellingPointInput(req))
	if err != nil {
		handleProductError(c, err)
		return
	}
	Created(c, sellingPoint)
}

func (s *Server) handleListSellingPoints(c *gin.Context) {
	OK(c, s.productAssetService.ListSellingPoints(c.Param("productID")))
}

func (s *Server) handleUpdateSellingPoint(c *gin.Context) {
	var req updateSellingPointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "bad_request", "invalid selling point payload")
		return
	}

	sellingPoint, err := s.productAssetService.UpdateSellingPoint(c.Param("sellingPointID"), services.UpdateSellingPointInput(req))
	if err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, sellingPoint)
}

func (s *Server) handleArchiveSellingPoint(c *gin.Context) {
	if err := s.productAssetService.ArchiveSellingPoint(c.Param("sellingPointID")); err != nil {
		handleProductError(c, err)
		return
	}
	OK(c, gin.H{"archived": true})
}

func handleProductError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrProductNotFound):
		Fail(c, http.StatusNotFound, "not_found", "product not found")
	case errors.Is(err, services.ErrSellingPointNotFound):
		Fail(c, http.StatusNotFound, "not_found", "selling point not found")
	case errors.Is(err, services.ErrAssetNotFound):
		Fail(c, http.StatusNotFound, "not_found", "asset not found")
	default:
		Fail(c, http.StatusInternalServerError, "internal_error", "product service error")
	}
}
