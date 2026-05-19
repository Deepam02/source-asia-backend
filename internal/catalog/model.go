package catalog

import "time"

// Product holds core metadata only — no media.
type Product struct {
	ID        string
	Name      string
	SKU       string
	CreatedAt time.Time
}

// ProductWithMedia is returned by GetByID; embeds metadata plus its media URLs.
type ProductWithMedia struct {
	Product
	Media []string
}
