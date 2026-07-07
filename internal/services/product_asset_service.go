package services

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrProductNotFound      = errors.New("product not found")
	ErrSellingPointNotFound = errors.New("selling point not found")
)

type Product struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Category    string         `json:"category,omitempty"`
	Status      string         `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type SellingPoint struct {
	ID          string    `json:"id"`
	ProductID   string    `json:"product_id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Priority    int       `json:"priority"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateProductInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Metadata    map[string]any `json:"metadata"`
}

type UpdateProductInput struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category"`
	Metadata    map[string]any `json:"metadata"`
}

type CreateSellingPointInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

type UpdateSellingPointInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
}

type ProductAssetService struct {
	mu            sync.RWMutex
	products      map[string]Product
	sellingPoints map[string]SellingPoint
}

func NewProductAssetService() *ProductAssetService {
	return &ProductAssetService{
		products:      map[string]Product{},
		sellingPoints: map[string]SellingPoint{},
	}
}

func (s *ProductAssetService) CreateProduct(input CreateProductInput) Product {
	now := time.Now()
	product := Product{
		ID:          uuid.NewString(),
		Name:        input.Name,
		Description: input.Description,
		Category:    input.Category,
		Status:      "active",
		Metadata:    input.Metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.products[product.ID] = product
	return product
}

func (s *ProductAssetService) ListProducts() []Product {
	s.mu.RLock()
	defer s.mu.RUnlock()

	products := make([]Product, 0, len(s.products))
	for _, product := range s.products {
		products = append(products, product)
	}
	sort.Slice(products, func(i, j int) bool {
		return products[i].CreatedAt.After(products[j].CreatedAt)
	})
	return products
}

func (s *ProductAssetService) GetProduct(id string) (Product, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	product, ok := s.products[id]
	if !ok {
		return Product{}, ErrProductNotFound
	}
	return product, nil
}

func (s *ProductAssetService) UpdateProduct(id string, input UpdateProductInput) (Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	product, ok := s.products[id]
	if !ok {
		return Product{}, ErrProductNotFound
	}

	product.Name = input.Name
	product.Description = input.Description
	product.Category = input.Category
	product.Metadata = input.Metadata
	product.UpdatedAt = time.Now()
	s.products[id] = product
	return product, nil
}

func (s *ProductAssetService) ArchiveProduct(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	product, ok := s.products[id]
	if !ok {
		return ErrProductNotFound
	}

	product.Status = "archived"
	product.UpdatedAt = time.Now()
	s.products[id] = product
	return nil
}

func (s *ProductAssetService) CreateSellingPoint(productID string, input CreateSellingPointInput) (SellingPoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.products[productID]; !ok {
		return SellingPoint{}, ErrProductNotFound
	}

	now := time.Now()
	sellingPoint := SellingPoint{
		ID:          uuid.NewString(),
		ProductID:   productID,
		Title:       input.Title,
		Description: input.Description,
		Priority:    input.Priority,
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.sellingPoints[sellingPoint.ID] = sellingPoint
	return sellingPoint, nil
}

func (s *ProductAssetService) ListSellingPoints(productID string) []SellingPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := []SellingPoint{}
	for _, sellingPoint := range s.sellingPoints {
		if sellingPoint.ProductID == productID {
			items = append(items, sellingPoint)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Priority == items[j].Priority {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].Priority > items[j].Priority
	})
	return items
}

func (s *ProductAssetService) UpdateSellingPoint(id string, input UpdateSellingPointInput) (SellingPoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sellingPoint, ok := s.sellingPoints[id]
	if !ok {
		return SellingPoint{}, ErrSellingPointNotFound
	}

	sellingPoint.Title = input.Title
	sellingPoint.Description = input.Description
	sellingPoint.Priority = input.Priority
	sellingPoint.UpdatedAt = time.Now()
	s.sellingPoints[id] = sellingPoint
	return sellingPoint, nil
}

func (s *ProductAssetService) ArchiveSellingPoint(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sellingPoint, ok := s.sellingPoints[id]
	if !ok {
		return ErrSellingPointNotFound
	}

	sellingPoint.Status = "archived"
	sellingPoint.UpdatedAt = time.Now()
	s.sellingPoints[id] = sellingPoint
	return nil
}
