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
	"math"
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

// ── Simulation data pools ────────────────────────────────────────────────────

var productSKUs = []string{
	"SKU-ALPHA-001", "SKU-BETA-002", "SKU-GAMMA-003", "SKU-DELTA-004", "SKU-EPSILON-005",
	"SKU-ZETA-006", "SKU-ETA-007", "SKU-THETA-008", "SKU-IOTA-009", "SKU-KAPPA-010",
	"SKU-LAMBDA-011", "SKU-MU-012", "SKU-NU-013", "SKU-XI-014", "SKU-OMICRON-015",
	"SKU-PI-016", "SKU-RHO-017", "SKU-SIGMA-018", "SKU-TAU-019", "SKU-UPSILON-020",
	"SKU-PHI-021", "SKU-CHI-022", "SKU-PSI-023", "SKU-OMEGA-024", "SKU-PRIME-025",
	"WIDGET-A100", "WIDGET-B200", "WIDGET-C300", "GADGET-X1", "GADGET-Y2",
	"PART-5501", "PART-5502", "PART-5503", "PART-5504", "PART-5505",
	"MODULE-M10", "MODULE-M20", "MODULE-M30", "BUNDLE-PRO", "BUNDLE-LITE",
	"SERVICE-ANNUAL", "SERVICE-MONTHLY", "LICENSE-ENT", "LICENSE-SMB", "LICENSE-TRIAL",
	"ADDON-ANALYTICS", "ADDON-SUPPORT", "ADDON-STORAGE", "ADDON-AI", "ADDON-API",
}

var regions = []string{
	"us-east-1", "us-west-2", "eu-west-1", "eu-central-1", "ap-southeast-1",
	"ap-northeast-1", "ap-south-1", "sa-east-1", "ca-central-1", "af-south-1",
	"me-south-1", "eu-north-1", "ap-east-1", "us-gov-east-1", "cn-north-1",
}

var currencies = []string{
	"USD", "EUR", "GBP", "JPY", "AUD", "CAD", "CHF", "CNY", "INR", "BRL",
	"MXN", "KRW", "SGD", "HKD", "NOK", "SEK", "DKK", "NZD", "ZAR", "THB",
}

var categories = []string{
	"electronics", "software", "hardware", "services", "subscriptions",
	"consulting", "support", "training", "infrastructure", "analytics",
	"security", "networking", "storage", "compute", "database",
}

var paymentMethods = []string{
	"credit_card", "debit_card", "bank_transfer", "wire_transfer",
	"paypal", "stripe", "crypto_btc", "crypto_eth", "invoice_net30",
	"invoice_net60", "ach", "sepa", "direct_debit", "purchase_order",
}

var itemStatuses = []string{
	"pending", "queued", "ready", "validated", "approved",
}

var priorities = []string{"low", "normal", "high", "critical"}

var departments = []string{
	"engineering", "marketing", "finance", "sales", "operations",
	"hr", "legal", "product", "design", "data-science",
}

var tags = []string{
	"bulk", "priority", "recurring", "trial", "enterprise", "migrated",
	"automated", "manual", "legacy", "new", "verified", "unverified",
	"domestic", "international", "vip", "partner", "internal", "external",
	"test", "production", "staging", "canary", "rollback",
}

var channels = []string{
	"api", "web", "mobile", "cli", "sdk", "partner", "reseller", "direct",
}

var environments = []string{"production", "staging", "development", "qa"}

var companyDomains = []string{
	"acme.com", "globex.io", "umbrella.net", "initech.biz", "hooli.tech",
	"waystar.co", "bluth.org", "dunder.com", "piedpiper.io", "vehement.co",
	"pied-piper.com", "aviato.net", "eklow.co", "bachmanity.io", "raviga.vc",
	"alambic.fr", "bluebell.co.uk", "silverpine.de", "nordsee.at", "taiga.jp",
}

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

	// Startup auto-trigger — 10 batches × 500 items × 50 fail each
	if autoTrigger == "true" {
		go func() {
			time.Sleep(5 * time.Second)
			const (
				autoBatches   = 10
				autoSize      = 500
				autoFailCount = 50
			)
			log.Printf("[System A] 🚀 Auto-triggering %d batches × %d items (%d fail each) = %d total items...",
				autoBatches, autoSize, autoFailCount, autoBatches*autoSize)

			type result struct {
				index int
				err   error
			}
			resultCh := make(chan result, autoBatches)
			for i := 0; i < autoBatches; i++ {
				go func(idx int) {
					key := fmt.Sprintf("idem-auto-%d-%04d-batch%d", time.Now().Unix(), rand.Intn(10000), idx)
					resultCh <- result{index: idx, err: triggerBatch(autoSize, autoFailCount, key)}
				}(i)
			}

			succ, fail := 0, 0
			for i := 0; i < autoBatches; i++ {
				res := <-resultCh
				if res.err != nil {
					log.Printf("[System A] ❌ Auto-trigger batch #%d failed: %v", res.index, res.err)
					fail++
				} else {
					log.Printf("[System A] ✅ Auto-trigger batch #%d submitted", res.index)
					succ++
				}
			}
			log.Printf("[System A] 🏁 Auto-trigger done: %d/%d batches submitted successfully", succ, autoBatches)
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

	q := r.URL.Query()
	sizeStr := q.Get("size")
	failCountStr := q.Get("fail_count")
	idemKey := q.Get("idempotency_key")
	batchesStr := q.Get("batches") // how many separate batch submissions to fire

	// Default: 500 items per batch (massive data simulation)
	size := 500
	if s, err := strconv.Atoi(sizeStr); err == nil && s > 0 {
		size = s
	}

	// Default: 50 fail items per batch
	failCount := 50
	if f, err := strconv.Atoi(failCountStr); err == nil && f >= 0 {
		failCount = f
	}

	// Default: 10 batches; cap at 50 to avoid runaway loads
	numBatches := 10
	if b, err := strconv.Atoi(batchesStr); err == nil && b > 0 {
		if b > 50 {
			b = 50
		}
		numBatches = b
	}

	log.Printf("[System A] 🚀 Manual trigger requested: batches=%d, size=%d/batch, fail_count=%d/batch, total_items=%d",
		numBatches, size, failCount, numBatches*size)

	type result struct {
		index int
		err   error
	}

	resultCh := make(chan result, numBatches)
	for i := 0; i < numBatches; i++ {
		go func(idx int) {
			key := idemKey
			if numBatches > 1 || key == "" {
				key = fmt.Sprintf("idem-manual-%d-%04d-batch%d", time.Now().Unix(), rand.Intn(10000), idx)
			}
			resultCh <- result{index: idx, err: triggerBatch(size, failCount, key)}
		}(i)
	}

	var errs []string
	for i := 0; i < numBatches; i++ {
		res := <-resultCh
		if res.err != nil {
			log.Printf("[System A] ❌ Batch #%d failed: %v", res.index, res.err)
			errs = append(errs, fmt.Sprintf("batch %d: %v", res.index, res.err))
		} else {
			log.Printf("[System A] ✅ Batch #%d submitted successfully", res.index)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if len(errs) == numBatches {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"status":"error","message":%q}`, strings.Join(errs, "; "))))
		return
	}
	if len(errs) > 0 {
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(fmt.Sprintf(`{"status":"partial","batches":%d,"errors":%d,"items_per_batch":%d}`,
			numBatches, len(errs), size)))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"status":"success","batches":%d,"items_per_batch":%d,"total_items":%d}`,
		numBatches, size, numBatches*size)))
}

// randPick returns a random element from a string slice.
func randPick(s []string) string { return s[rand.Intn(len(s))] }

// randTags picks n unique tags from the tags pool.
func randTags(n int) []string {
	perm := rand.Perm(len(tags))
	if n > len(tags) {
		n = len(tags)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = tags[perm[i]]
	}
	return out
}

func triggerBatch(size, failCount int, idemKey string) error {
	// Generate unique correlation ID
	correlationID := fmt.Sprintf("corr-%d-%04d", time.Now().Unix(), rand.Intn(10000))

	if idemKey == "" {
		idemKey = fmt.Sprintf("idem-%d-%04d", time.Now().Unix(), rand.Intn(10000))
	}

	now := time.Now().UTC()
	items := make([]BatchItemRequest, size)
	for i := 0; i < size; i++ {
		// Determine whether this item should be a failure item
		extID := fmt.Sprintf("ext-item-%d-%06d", now.UnixNano(), i)
		if i < failCount {
			extID = fmt.Sprintf("fail-item-%d-%06d", now.UnixNano(), i)
		}

		// ── Rich, realistic payload ──────────────────────────────────────
		userID := fmt.Sprintf("usr_%08x", rand.Uint32())
		orderID := fmt.Sprintf("ord_%012x", rand.Uint64()%0x1000000000000)
		sessionID := fmt.Sprintf("sess_%016x", rand.Uint64())
		sku := randPick(productSKUs)
		region := randPick(regions)
		currency := randPick(currencies)
		category := randPick(categories)
		payMethod := randPick(paymentMethods)
		priority := randPick(priorities)
		dept := randPick(departments)
		channel := randPick(channels)
		env := randPick(environments)
		domain := randPick(companyDomains)
		status := randPick(itemStatuses)

		qty := rand.Intn(99) + 1
		unitPrice := math.Round((rand.Float64()*4990+10)*100) / 100
		totalAmount := math.Round(float64(qty)*unitPrice*100) / 100
		discountPct := rand.Intn(41) // 0–40 %
		taxRate := math.Round((rand.Float64()*0.25+0.05)*1000) / 1000
		shippingFee := math.Round(rand.Float64()*50*100) / 100
		weight := math.Round(rand.Float64()*100*100) / 100 // kg
		retryCount := rand.Intn(4)
		version := rand.Intn(5) + 1

		createdAt := now.Add(-time.Duration(rand.Intn(30*24)) * time.Hour)
		scheduledAt := now.Add(time.Duration(rand.Intn(72)) * time.Hour)
		expiredAt := scheduledAt.Add(time.Duration(rand.Intn(7*24)+24) * time.Hour)

		itemTags := randTags(rand.Intn(4) + 1)
		ip := fmt.Sprintf("%d.%d.%d.%d", rand.Intn(224)+1, rand.Intn(256), rand.Intn(256), rand.Intn(256))
		latitude := math.Round((rand.Float64()*180-90)*1e6) / 1e6
		longitude := math.Round((rand.Float64()*360-180)*1e6) / 1e6

		payloadMap := map[string]interface{}{
			// Identity
			"user_id":    userID,
			"email":      fmt.Sprintf("%s@%s", userID, domain),
			"order_id":   orderID,
			"session_id": sessionID,
			"item_index": i,

			// Product
			"sku":          sku,
			"category":     category,
			"quantity":     qty,
			"unit_price":   unitPrice,
			"total_amount": totalAmount,
			"discount_pct": discountPct,
			"tax_rate":     taxRate,
			"shipping_fee": shippingFee,
			"weight_kg":    weight,
			"currency":     currency,

			// Payment
			"payment_method":       payMethod,
			"payment_reference":    fmt.Sprintf("PAY-%s-%08X", strings.ToUpper(payMethod[:3]), rand.Uint32()),
			"payment_gateway":      fmt.Sprintf("gateway-%d", rand.Intn(5)+1),
			"authorization_code":   fmt.Sprintf("AUTH%06d", rand.Intn(1000000)),
			"transaction_id":       fmt.Sprintf("txn_%016x", rand.Uint64()),

			// Routing & infra
			"region":      region,
			"channel":     channel,
			"environment": env,
			"department":  dept,
			"priority":    priority,
			"status":      status,
			"version":     version,
			"retry_count": retryCount,
			"tags":        itemTags,

			// Timestamps
			"created_at":   createdAt.Format(time.RFC3339),
			"scheduled_at": scheduledAt.Format(time.RFC3339),
			"expires_at":   expiredAt.Format(time.RFC3339),

			// Network / geo
			"client_ip": ip,
			"latitude":  latitude,
			"longitude": longitude,

			// Free-text description
			"description": fmt.Sprintf(
				"[%s] %s order %s — qty %d x %s @ %.2f %s (ch:%s, prio:%s, dept:%s)",
				strings.ToUpper(env), strings.ToUpper(category),
				orderID, qty, sku, unitPrice, currency,
				channel, priority, dept,
			),

			// Nested metadata for downstream enrichment
			"metadata": map[string]interface{}{
				"source_system": "system-a-simulator",
				"batch_index":   i,
				"schema_ver":    "v2",
				"checksum":      fmt.Sprintf("%08x", rand.Uint32()),
			},
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

	log.Printf("[System A] 📤 Sending massive batch to middleware (idem=%s, items=%d, ~%.1f KB)...",
		idemKey, size, float64(len(bodyBytes))/1024)
	client := &http.Client{Timeout: 60 * time.Second} // larger timeout for big payloads
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

	log.Printf("[System A] 📥 Batch accepted — Middleware Batch ID: %s | %s", submitResp.BatchID, submitResp.Message)
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
