package catalog

import "time"

// Product holds core metadata only — no media.
type Product struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SKU       string    `json:"sku"`
	CreatedAt time.Time `json:"created_at"`
}

// ProductWithMedia is returned by GetByID and create; embeds metadata plus
// its separate image and video URL lists.
type ProductWithMedia struct {
	Product
	ImageURLs []string `json:"image_urls"`
	VideoURLs []string `json:"video_urls"`
}
