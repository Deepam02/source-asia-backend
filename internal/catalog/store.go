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
// meta and media are intentionally separate structures.
type Store struct {
	mu    sync.RWMutex
	meta  map[string]Product  // id → Product (no media)
	media map[string][]string // id → ordered media URLs
	order []string            // insertion order for stable List results
}

// New returns an empty, ready-to-use Store.
func New() *Store {
	return &Store{
		meta:  make(map[string]Product),
		media: make(map[string][]string),
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
	s.media[p.ID] = nil
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

// GetByID returns the full product including media URLs.
// Returns ErrNotFound when the id is unknown.
func (s *Store) GetByID(id string) (ProductWithMedia, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.meta[id]
	if !ok {
		return ProductWithMedia{}, ErrNotFound
	}

	urls := s.media[id]
	snapshot := make([]string, len(urls))
	copy(snapshot, urls)

	return ProductWithMedia{Product: p, Media: snapshot}, nil
}

// AppendMedia adds one or more media URLs to an existing product.
// Returns ErrNotFound when the id is unknown.
// Empty or duplicate URLs within the batch are silently skipped.
func (s *Store) AppendMedia(id string, urls ...string) error {
	if len(urls) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.meta[id]; !ok {
		return ErrNotFound
	}

	existing := make(map[string]struct{}, len(s.media[id]))
	for _, u := range s.media[id] {
		existing[u] = struct{}{}
	}

	for _, u := range urls {
		if u == "" {
			continue
		}
		if _, dup := existing[u]; dup {
			continue
		}
		s.media[id] = append(s.media[id], u)
		existing[u] = struct{}{}
	}
	return nil
}
