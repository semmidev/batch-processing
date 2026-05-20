package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type SubmitBatchRequest struct {
	CorrelationID string             `json:"correlation_id"`
	WebhookURL    string             `json:"webhook_url,omitempty"`
	Items         []BatchItemRequest `json:"items"`
}

type BatchItemRequest struct {
	ExternalID string          `json:"external_id"`
	Payload    json.RawMessage `json:"payload"`
}

type SubmitBatchResponse struct {
	BatchID string `json:"batch_id"`
	Message string `json:"message"`
}

type WebhookPayload struct {
	Event         string              `json:"event"`
	BatchID       string              `json:"batch_id"`
	CorrelationID string              `json:"correlation_id"`
	Status        string              `json:"status"`
	Summary       WebhookSummary      `json:"summary"`
	FailedItems   []WebhookFailedItem `json:"failed_items,omitempty"`
	Timestamp     string              `json:"timestamp"`
}

type WebhookSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

type WebhookFailedItem struct {
	ExternalID string `json:"external_id"`
	Error      string `json:"error"`
}

var (
	middlewareURL string
	webhookURL    string
	apiKey        string
	webhookSecret string
)

func main() {
	rand.Seed(time.Now().UnixNano())

	portStr := getEnv("SYSTEM_A_PORT", "8082")
	middlewareURL = getEnv("MIDDLEWARE_URL", "http://middleware:8080")
	webhookURL = getEnv("WEBHOOK_URL", "http://systema:8082/webhook")
	apiKey = getEnv("MIDDLEWARE_API_KEY", "key1")
	webhookSecret = getEnv("WEBHOOK_SECRET", "some-generated-secret")
	autoTrigger := getEnv("AUTO_TRIGGER", "true")

	log.Printf("==================================================")
	log.Printf("Starting Mock System A Simulator...")
	log.Printf("Listening on port: :%s", portStr)
	log.Printf("Middleware Target URL: %s", middlewareURL)
	log.Printf("Webhook Callback URL: %s", webhookURL)
	log.Printf("Webhook HMAC Verification Secret: %q", webhookSecret)
	log.Printf("Auto Trigger on Startup: %s", autoTrigger)
	log.Printf("==================================================")

	// Webhook endpoint
	http.HandleFunc("/webhook", handleWebhook)

	// Trigger endpoint
	http.HandleFunc("/trigger", handleTrigger)

	// Health endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("System A OK"))
	})

	// Startup auto-trigger
	if autoTrigger == "true" {
		go func() {
			time.Sleep(5 * time.Second)
			log.Printf("[System A] 🚀 Auto-triggering initial demonstration batch after startup...")
			err := triggerBatch(5, 1, "")
			if err != nil {
				log.Printf("[System A] ❌ Auto-trigger failed: %v", err)
			} else {
				log.Printf("[System A] 🚀 Auto-trigger batch submitted successfully!")
			}
		}()
	}

	if err := http.ListenAndServe(":"+portStr, nil); err != nil {
		log.Fatalf("System A failed to start: %v", err)
	}
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("[System A] Failed to read webhook body: %v", err)
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	// 1. Verify HMAC Signature
	sigHeader := r.Header.Get("X-Signature")
	if sigHeader != "" && webhookSecret != "" {
		sigHex := strings.TrimPrefix(sigHeader, "sha256=")
		expectedSig, err := hex.DecodeString(sigHex)
		if err == nil {
			mac := hmac.New(sha256.New, []byte(webhookSecret))
			mac.Write(bodyBytes)
			computedSig := mac.Sum(nil)

			if !hmac.Equal(expectedSig, computedSig) {
				log.Printf("[System A] ❌ Invalid webhook signature! Rejecting payload.")
				http.Error(w, "Invalid signature", http.StatusUnauthorized)
				return
			}
		} else {
			log.Printf("[System A] ❌ Failed to decode signature hex: %v", err)
			http.Error(w, "Invalid signature format", http.StatusUnauthorized)
			return
		}
	}

	var payload WebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("[System A] ❌ Failed to unmarshal webhook payload: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 2. Log details beautifully
	statusColor := "🟢"
	if payload.Status == "failed" {
		statusColor = "🔴"
	} else if payload.Status == "partial" {
		statusColor = "🟡"
	}

	log.Printf("\n")
	log.Printf("╭──────────────────────────────────────────────────────────────╮")
	log.Printf("│ 📥 RECEIVED COMPLETED BATCH WEBHOOK                          │")
	log.Printf("├──────────────────────────────────────────────────────────────┤")
	log.Printf("│ Event:           %-43s │", payload.Event)
	log.Printf("│ Batch ID:        %-43s │", payload.BatchID)
	log.Printf("│ Correlation ID:  %-43s │", payload.CorrelationID)
	log.Printf("│ Status:          %-2s %-40s │", statusColor, payload.Status)
	log.Printf("│ Timestamp:       %-43s │", payload.Timestamp)
	log.Printf("├──────────────────────────────────────────────────────────────┤")
	log.Printf("│ Summary:                                                     │")
	log.Printf("│   Total Items:   %-43d │", payload.Summary.Total)
	log.Printf("│   Succeeded:     %-43d │", payload.Summary.Success)
	log.Printf("│   Failed:        %-43d │", payload.Summary.Failed)
	if len(payload.FailedItems) > 0 {
		log.Printf("├──────────────────────────────────────────────────────────────┤")
		log.Printf("│ Failed Items Detail:                                         │")
		for _, fi := range payload.FailedItems {
			log.Printf("│   * External ID: %-43s │", fi.ExternalID)
			log.Printf("│     Error:       %-43s │", truncateString(fi.Error, 43))
		}
	}
	log.Printf("╰──────────────────────────────────────────────────────────────╯")
	log.Printf("\n")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Webhook received successfully"))
}

func handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sizeStr := r.URL.Query().Get("size")
	failCountStr := r.URL.Query().Get("fail_count")
	idemKey := r.URL.Query().Get("idempotency_key")

	size := 5
	if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
		size = s
	}

	failCount := 0
	if f, err := strconv.Atoi(failCountStr); err == nil && f >= 0 {
		failCount = f
	}

	log.Printf("[System A] Manual trigger requested: size=%d, fail_count=%d", size, failCount)
	err := triggerBatch(size, failCount, idemKey)
	if err != nil {
		log.Printf("[System A] ❌ Manual trigger failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"status":"error","message":%q}`, err.Error())))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"success","message":"batch triggered successfully"}`))
}

func triggerBatch(size, failCount int, idemKey string) error {
	// Generate unique correlation ID
	correlationID := fmt.Sprintf("corr-%d-%04d", time.Now().Unix(), rand.Intn(10000))

	if idemKey == "" {
		idemKey = fmt.Sprintf("idem-%d-%04d", time.Now().Unix(), rand.Intn(10000))
	}

	items := make([]BatchItemRequest, size)
	for i := 0; i < size; i++ {
		extID := fmt.Sprintf("ext-item-%d-%d", time.Now().Unix(), i)
		if i < failCount {
			// This item will fail downstream because it starts with "fail_"
			extID = fmt.Sprintf("fail-item-%d-%d", time.Now().Unix(), i)
		}

		payloadMap := map[string]interface{}{
			"amount":      rand.Float64() * 500,
			"currency":    "USD",
			"description": fmt.Sprintf("Demo processing for item index %d", i),
		}
		payloadBytes, _ := json.Marshal(payloadMap)

		items[i] = BatchItemRequest{
			ExternalID: extID,
			Payload:    json.RawMessage(payloadBytes),
		}
	}

	reqBody := SubmitBatchRequest{
		CorrelationID: correlationID,
		WebhookURL:    webhookURL,
		Items:         items,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal batch request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, middlewareURL+"/api/v1/batches", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", idemKey)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	log.Printf("[System A] 📤 Sending batch request to middleware (X-Idempotency-Key=%s, items=%d)...", idemKey, size)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("middleware returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var submitResp SubmitBatchResponse
	if err := json.Unmarshal(respBody, &submitResp); err != nil {
		return fmt.Errorf("failed to parse submit response: %w", err)
	}

	log.Printf("[System A] 📥 Batch accepted. Middleware Batch ID: %s. Message: %s", submitResp.BatchID, submitResp.Message)
	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		if maxLen > 3 {
			return s[:maxLen-3] + "..."
		}
		return s[:maxLen]
	}
	return s
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
