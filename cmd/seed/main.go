package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

const (
	base    = "http://localhost:3000"
	products = 1000
	images  = 10
	videos  = 2
)

func main() {
	client := &http.Client{Timeout: 30 * time.Second}

	// ------------------------------------------------------------------ //
	// 1. Seed 1,000 products, each with 10 image URLs and 2 video URLs    //
	// ------------------------------------------------------------------ //
	fmt.Printf("Seeding %d products...\n", products)
	seedStart := time.Now()

	for i := 1; i <= products; i++ {
		imgURLs := make([]string, images)
		for j := range imgURLs {
			imgURLs[j] = fmt.Sprintf("https://cdn.example.com/products/sku-%04d/img-%02d.jpg", i, j+1)
		}
		vidURLs := make([]string, videos)
		for j := range vidURLs {
			vidURLs[j] = fmt.Sprintf("https://cdn.example.com/products/sku-%04d/demo-%02d.mp4", i, j+1)
		}

		body := map[string]any{
			"name":       fmt.Sprintf("Product %d", i),
			"sku":        fmt.Sprintf("SKU-%04d", i),
			"image_urls": imgURLs,
			"video_urls": vidURLs,
		}

		if err := postJSON(client, base+"/products", body, nil); err != nil {
			log.Fatalf("seed product %d: %v", i, err)
		}

		if i%100 == 0 {
			fmt.Printf("  created %d/%d\n", i, products)
		}
	}

	fmt.Printf("Seeding done in %s\n\n", time.Since(seedStart).Round(time.Millisecond))

	// ------------------------------------------------------------------ //
	// 2. List products (limit=20) and print latency                       //
	// ------------------------------------------------------------------ //
	fmt.Println("GET /products?limit=20")
	listStart := time.Now()

	resp, err := client.Get(base + "/products?limit=20")
	if err != nil {
		log.Fatalf("list products: %v", err)
	}
	defer resp.Body.Close()
	listBody, _ := io.ReadAll(resp.Body)
	listLatency := time.Since(listStart)

	var listResult struct {
		Products []json.RawMessage `json:"products"`
	}
	_ = json.Unmarshal(listBody, &listResult)

	fmt.Printf("  status:  %s\n", resp.Status)
	fmt.Printf("  items:   %d\n", len(listResult.Products))
	fmt.Printf("  latency: %s\n\n", listLatency.Round(time.Microsecond))

	// ------------------------------------------------------------------ //
	// 3. Fire 7 rapid POST /request calls for the same user               //
	// ------------------------------------------------------------------ //
	fmt.Println("POST /request × 7 (same user_id, expect 5 accepted + 2 rejected)")

	for i := 1; i <= 7; i++ {
		payload := map[string]any{
			"user_id": "seed-test-user",
			"payload": map[string]int{"seq": i},
		}

		var result map[string]string
		statusCode, err := postJSONStatus(client, base+"/request", payload, &result)
		if err != nil {
			log.Fatalf("request %d: %v", i, err)
		}

		verdict := "accepted"
		if statusCode == http.StatusTooManyRequests {
			verdict = "rejected"
		}
		fmt.Printf("  [%d] HTTP %d — %s  (%v)\n", i, statusCode, verdict, result)
	}
}

// postJSON marshals body, POSTs to url, and decodes the response into out (if non-nil).
func postJSON(client *http.Client, url string, body, out any) error {
	_, err := postJSONStatus(client, url, body, out)
	return err
}

// postJSONStatus does the same but also returns the HTTP status code.
func postJSONStatus(client *http.Client, url string, body, out any) (int, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return 0, fmt.Errorf("marshal: %w", err)
	}

	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, fmt.Errorf("post %s: %w", url, err)
	}
	defer resp.Body.Close()

	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response: %w", err)
		}
	}
	return resp.StatusCode, nil
}
