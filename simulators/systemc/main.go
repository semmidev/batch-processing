package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProcessRequest struct {
	ExternalID string          `json:"external_id"`
	Payload    json.RawMessage `json:"payload"`
}

type ProcessResponse struct {
	Status      string    `json:"status"`
	ExternalID  string    `json:"external_id"`
	ProcessedAt time.Time `json:"processed_at"`
	Message     string    `json:"message,omitempty"`
}

func main() {
	// Seed random generator
	rand.Seed(time.Now().UnixNano())

	// Configuration from environment variables
	portStr := getEnv("SYSTEM_C_PORT", "8081")
	apiKey := getEnv("SYSTEM_C_API_KEY", "system-c-key")
	latencyMsStr := getEnv("SYSTEM_C_LATENCY_MS", "100")
	failureRateStr := getEnv("SYSTEM_C_FAILURE_RATE", "0.0")
	failPattern := getEnv("SYSTEM_C_FAIL_ID_PATTERN", "")

	latencyMs, err := strconv.Atoi(latencyMsStr)
	if err != nil {
		latencyMs = 100
	}

	failureRate, err := strconv.ParseFloat(failureRateStr, 64)
	if err != nil {
		failureRate = 0.0
	}

	log.Printf("==================================================")
	log.Printf("Starting Mock System C Simulator...")
	log.Printf("Listening on port: :%s", portStr)
	log.Printf("Bearer API Key required: %s", apiKey)
	log.Printf("Simulated Latency: %dms", latencyMs)
	log.Printf("Simulated Failure Rate: %.2f", failureRate)
	if failPattern != "" {
		log.Printf("Simulated Deterministic Failure Pattern: %q", failPattern)
	}
	log.Printf("==================================================")

	http.HandleFunc("/process", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		// 1. Authenticate Request
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + apiKey
		if apiKey != "" && authHeader != expectedAuth {
			log.Printf("[System C] ❌ Unauthorized request from %s", r.RemoteAddr)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"status":"error","message":"unauthorized"}`))
			return
		}

		// 2. Decode Payload
		var req ProcessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Printf("[System C] ❌ Failed to decode request body: %v", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status":"error","message":"invalid payload"}`))
			return
		}

		// Log incoming item processing request
		log.Printf("[System C] 📥 Received item processing request: external_id=%q", req.ExternalID)

		// 3. Simulate Deterministic Failures (e.g. if external_id contains the pattern)
		if failPattern != "" && strings.Contains(req.ExternalID, failPattern) {
			log.Printf("[System C] 💥 DETECTED FORCED FAILURE PATTERN for external_id: %s. Returning 500.", req.ExternalID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"status":"error","message":"injected deterministic server error"}`))
			return
		}

		// 4. Simulate Random Failures
		if failureRate > 0.0 && rand.Float64() < failureRate {
			log.Printf("[System C] 🎲 RANDOM FAILURE triggered for external_id: %s. Returning 500.", req.ExternalID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"status":"error","message":"injected random server error"}`))
			return
		}

		// 5. Simulate Latency
		if latencyMs > 0 {
			time.Sleep(time.Duration(latencyMs) * time.Millisecond)
		}

		// 6. Return Success
		resp := ProcessResponse{
			Status:      "success",
			ExternalID:  req.ExternalID,
			ProcessedAt: time.Now(),
			Message:     "Processed successfully by Mock System C",
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)

		log.Printf("[System C] ✅ Processed item: external_id=%q in %v", req.ExternalID, time.Since(start))
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("System C OK"))
	})

	if err := http.ListenAndServe(":"+portStr, nil); err != nil {
		log.Fatalf("System C failed to start: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
