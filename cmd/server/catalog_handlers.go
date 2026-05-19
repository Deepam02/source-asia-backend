package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/deepam02/source-asia-backend/internal/catalog"
)

const (
	maxMediaPerArray = 20
	maxURLLength     = 2048
)

func registerCatalogRoutes(mux *http.ServeMux, store *catalog.Store) {
	mux.HandleFunc("POST /products", makeCreateProductHandler(store))
	mux.HandleFunc("GET /products", makeListProductsHandler(store))
	mux.HandleFunc("GET /products/{id}", makeGetProductHandler(store))
	mux.HandleFunc("POST /products/{id}/media", makeAppendMediaHandler(store))
}

// --- POST /products ---

type createProductRequest struct {
	Name      string   `json:"name"`
	SKU       string   `json:"sku"`
	ImageURLs []string `json:"image_urls"`
	VideoURLs []string `json:"video_urls"`
}

func makeCreateProductHandler(store *catalog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createProductRequest
		if err := decodeJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}

		if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.SKU) == "" {
			writeJSON(w, http.StatusBadRequest, errResp("name and sku are required"))
			return
		}
		if len(body.ImageURLs) > maxMediaPerArray {
			writeJSON(w, http.StatusBadRequest, errResp("image_urls must contain at most 20 items"))
			return
		}
		if len(body.VideoURLs) > maxMediaPerArray {
			writeJSON(w, http.StatusBadRequest, errResp("video_urls must contain at most 20 items"))
			return
		}
		if err := validateURLSlice(body.ImageURLs); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}
		if err := validateURLSlice(body.VideoURLs); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}

		p, err := store.Create(strings.TrimSpace(body.Name), strings.TrimSpace(body.SKU))
		if errors.Is(err, catalog.ErrDuplicateSKU) {
			writeJSON(w, http.StatusConflict, errResp("SKU already exists"))
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}

		if len(body.ImageURLs) > 0 || len(body.VideoURLs) > 0 {
			_ = store.AppendMedia(p.ID, body.ImageURLs, body.VideoURLs)
		}

		full, _ := store.GetByID(p.ID)
		writeJSON(w, http.StatusCreated, full)
	}
}

// --- GET /products ---

type listProductsResponse struct {
	Products []catalog.Product `json:"products"`
}

func makeListProductsHandler(store *catalog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		offset := queryInt(r, "offset", 0)
		limit := queryInt(r, "limit", 20)

		if offset < 0 {
			writeJSON(w, http.StatusBadRequest, errResp("offset must be >= 0"))
			return
		}
		if limit < 1 {
			writeJSON(w, http.StatusBadRequest, errResp("limit must be >= 1"))
			return
		}

		products := store.List(offset, limit)
		if products == nil {
			products = []catalog.Product{}
		}

		writeJSON(w, http.StatusOK, listProductsResponse{Products: products})
	}
}

// --- GET /products/{id} ---

func makeGetProductHandler(store *catalog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		p, err := store.GetByID(id)
		if errors.Is(err, catalog.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errResp("product not found"))
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errResp("internal error"))
			return
		}

		writeJSON(w, http.StatusOK, p)
	}
}

// --- POST /products/{id}/media ---

type appendMediaRequest struct {
	ImageURLs []string `json:"image_urls"`
	VideoURLs []string `json:"video_urls"`
}

func makeAppendMediaHandler(store *catalog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		var body appendMediaRequest
		if err := decodeJSON(r, &body); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}

		if len(body.ImageURLs) == 0 && len(body.VideoURLs) == 0 {
			writeJSON(w, http.StatusBadRequest, errResp("at least one of image_urls or video_urls is required"))
			return
		}
		if len(body.ImageURLs) > maxMediaPerArray {
			writeJSON(w, http.StatusBadRequest, errResp("image_urls must contain at most 20 items"))
			return
		}
		if len(body.VideoURLs) > maxMediaPerArray {
			writeJSON(w, http.StatusBadRequest, errResp("video_urls must contain at most 20 items"))
			return
		}
		if err := validateURLSlice(body.ImageURLs); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}
		if err := validateURLSlice(body.VideoURLs); err != nil {
			writeJSON(w, http.StatusBadRequest, errResp(err.Error()))
			return
		}

		err := store.AppendMedia(id, body.ImageURLs, body.VideoURLs)
		if errors.Is(err, catalog.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, errResp("product not found"))
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errResp("internal error"))
			return
		}

		p, _ := store.GetByID(id)
		writeJSON(w, http.StatusOK, p)
	}
}

// --- helpers ---

func validateURLSlice(urls []string) error {
	for _, u := range urls {
		if len(u) > maxURLLength {
			return errors.New("url exceeds 2048 characters")
		}
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return errors.New("url must use http or https scheme: " + u)
		}
	}
	return nil
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return n
}

func errResp(msg string) map[string]string {
	return map[string]string{"error": msg}
}
