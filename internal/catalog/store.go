package catalog

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound      = errors.New("product not found")
	ErrDuplicateSKU  = errors.New("SKU already exists")
	ErrInvalidInput  = errors.New("invalid input")
)

// Store is a concurrency-safe in-memory product catalog.
// meta, imageMedia, and videoMedia are intentionally separate structures so
// that List can read only metadata without touching any media URLs.
type Store struct {
	mu         sync.RWMutex
	meta       map[string]Product  // id → Product (no media)
	imageMedia map[string][]string // id → ordered image URLs
	videoMedia map[string][]string // id → ordered video URLs
	order      []string            // insertion order for stable List results
}

// New returns an empty, ready-to-use Store.
func New() *Store {
	return &Store{
		meta:       make(map[string]Product),
		imageMedia: make(map[string][]string),
		videoMedia: make(map[string][]string),
	}
}

// Create adds a new product and returns it. Name and SKU must be non-empty;
// SKU must be unique across the store.
func (s *Store) Create(name, sku string) (Product, error) {
	if name == "" || sku == "" {
		return Product{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.meta {
		if p.SKU == sku {
			return Product{}, ErrDuplicateSKU
		}
	}

	p := Product{
		ID:        uuid.NewString(),
		Name:      name,
		SKU:       sku,
		CreatedAt: time.Now().UTC(),
	}
	s.meta[p.ID] = p
	s.imageMedia[p.ID] = nil
	s.videoMedia[p.ID] = nil
	s.order = append(s.order, p.ID)
	return p, nil
}

// List returns a page of product metadata in insertion order.
// offset and limit follow standard slice semantics; limit ≤ 0 returns all remaining.
func (s *Store) List(offset, limit int) []Product {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if offset < 0 {
		offset = 0
	}
	if offset >= len(s.order) {
		return []Product{}
	}

	ids := s.order[offset:]
	if limit > 0 && limit < len(ids) {
		ids = ids[:limit]
	}

	out := make([]Product, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.meta[id])
	}
	return out
}

// GetByID returns the full product including image and video URLs.
// Returns ErrNotFound when the id is unknown.
func (s *Store) GetByID(id string) (ProductWithMedia, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.meta[id]
	if !ok {
		return ProductWithMedia{}, ErrNotFound
	}

	imgs := copyStrings(s.imageMedia[id])
	vids := copyStrings(s.videoMedia[id])

	return ProductWithMedia{Product: p, ImageURLs: imgs, VideoURLs: vids}, nil
}

// AppendMedia appends image and/or video URLs to an existing product.
// Returns ErrNotFound when the id is unknown.
// Empty or duplicate URLs within each batch are silently skipped.
func (s *Store) AppendMedia(id string, imageURLs, videoURLs []string) error {
	if len(imageURLs) == 0 && len(videoURLs) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.meta[id]; !ok {
		return ErrNotFound
	}

	s.imageMedia[id] = appendUnique(s.imageMedia[id], imageURLs)
	s.videoMedia[id] = appendUnique(s.videoMedia[id], videoURLs)
	return nil
}

func appendUnique(existing, incoming []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, u := range existing {
		seen[u] = struct{}{}
	}
	for _, u := range incoming {
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		existing = append(existing, u)
		seen[u] = struct{}{}
	}
	return existing
}

func copyStrings(src []string) []string {
	out := make([]string, len(src))
	copy(out, src)
	return out
}
