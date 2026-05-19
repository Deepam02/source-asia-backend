package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/deepam02/source-asia-backend/internal/catalog"
	"github.com/deepam02/source-asia-backend/internal/middleware"
	"github.com/deepam02/source-asia-backend/internal/ratelimit"
)

func main() {
	limiter := ratelimit.New()
	store := catalog.New()

	mux := http.NewServeMux()
	mux.HandleFunc("/request", makeRequestHandler(limiter))
	mux.HandleFunc("/stats", makeStatsHandler(limiter))
	registerCatalogRoutes(mux, store)

	log.Println("listening on :3000")
	if err := http.ListenAndServe(":3000", middleware.Logger(mux)); err != nil {
		log.Fatal(err)
	}
}

// requestBody is the expected JSON payload for POST /request.
// Payload is json.RawMessage so it accepts any JSON value (object, array,
// string, number, bool).
type requestBody struct {
	UserID  string          `json:"user_id"`
	Payload json.RawMessage `json:"payload"`
}

func makeRequestHandler(limiter *ratelimit.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var body requestBody
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
			return
		}

		if strings.TrimSpace(body.UserID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
			return
		}
		if len(body.Payload) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload is required"})
			return
		}

		if limiter.Allow(body.UserID) {
			writeJSON(w, http.StatusCreated, map[string]string{"status": "accepted"})
			return
		}
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
	}
}

// statsResponse is the JSON shape returned by GET /stats.
type statsResponse struct {
	UserID   string `json:"user_id"`
	Accepted int64  `json:"accepted"`
	Rejected int64  `json:"rejected"`
}

func makeStatsHandler(limiter *ratelimit.Limiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))
		if userID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id query parameter is required"})
			return
		}

		s := limiter.Stats(userID)
		writeJSON(w, http.StatusOK, statsResponse{
			UserID:   userID,
			Accepted: s.Accepted,
			Rejected: s.Rejected,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
