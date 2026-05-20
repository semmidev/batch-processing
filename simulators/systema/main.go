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

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"
)

// ── Configuration ─────────────────────────────────────────────────────────────

type AppConfig struct {
	Port               string
	MiddlewareURL      string
	WebhookURL         string
	APIKey             string
	WebhookSecret      string
	AutoTrigger        bool
	AutoBatches        int
	AutoSize           int
	AutoFailCount      int
	SenderIntervalSecs int
	SenderBatchSize    int
	MaxRetries         int
	DBDSN              string
}

func loadConfig() AppConfig {
	dbHost := getEnv("DB_HOST", "postgres")
	dbPort := getEnv("DB_PORT", "5432")
	dbName := getEnv("DB_NAME", "middleware_db")
	dbUser := getEnv("DB_USER", "middleware_user")
	dbPass := getEnv("DB_PASSWORD", "YourStrong!Password")
	dsn := fmt.Sprintf(
		"host=%s port=%s dbname=%s user=%s password=%s sslmode=disable",
		dbHost, dbPort, dbName, dbUser, dbPass,
	)

	autoBatches := 10
	if v, err := strconv.Atoi(getEnv("AUTO_BATCHES", "10")); err == nil && v > 0 {
		autoBatches = v
	}
	autoSize := 500
	if v, err := strconv.Atoi(getEnv("AUTO_SIZE", "500")); err == nil && v > 0 {
		autoSize = v
	}
	autoFailCount := 0
	if v, err := strconv.Atoi(getEnv("AUTO_FAIL_COUNT", "0")); err == nil && v >= 0 {
		autoFailCount = v
	}
	senderInterval := 5
	if v, err := strconv.Atoi(getEnv("SENDER_INTERVAL_SECONDS", "5")); err == nil && v > 0 {
		senderInterval = v
	}
	senderBatchSize := 500
	if v, err := strconv.Atoi(getEnv("SENDER_BATCH_SIZE", "500")); err == nil && v > 0 {
		senderBatchSize = v
	}
	maxRetries := 5
	if v, err := strconv.Atoi(getEnv("MAX_RETRIES", "5")); err == nil && v > 0 {
		maxRetries = v
	}

	return AppConfig{
		Port:               getEnv("SYSTEM_A_PORT", "8082"),
		MiddlewareURL:      getEnv("MIDDLEWARE_URL", "http://middleware:8080"),
		WebhookURL:         getEnv("WEBHOOK_URL", "http://systema:8082/webhook"),
		APIKey:             getEnv("MIDDLEWARE_API_KEY", "key1"),
		WebhookSecret:      getEnv("WEBHOOK_SECRET", "some-generated-secret"),
		AutoTrigger:        getEnv("AUTO_TRIGGER", "true") == "true",
		AutoBatches:        autoBatches,
		AutoSize:           autoSize,
		AutoFailCount:      autoFailCount,
		SenderIntervalSecs: senderInterval,
		SenderBatchSize:    senderBatchSize,
		MaxRetries:         maxRetries,
		DBDSN:              dsn,
	}
}

// ── Domain types ──────────────────────────────────────────────────────────────

// LocalItem is a row in systema_items.
type LocalItem struct {
	ID         string `db:"id"`
	BatchKey   string `db:"batch_key"`
	ExternalID string `db:"external_id"`
	Payload    string `db:"payload"`
	RetryCount int    `db:"retry_count"`
	MaxRetries int    `db:"max_retries"`
}

// FailedItem pairs an external_id with the reason it failed.
type FailedItem struct {
	ExternalID string
	ErrMsg     string
}

// ── Middleware API types ───────────────────────────────────────────────────────

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

// ── Webhook types ─────────────────────────────────────────────────────────────

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

// ── Global state ──────────────────────────────────────────────────────────────

var (
	globalDB  *sqlx.DB
	globalCfg AppConfig
)

// ── Simulation data pools ─────────────────────────────────────────────────────

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

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	rand.Seed(time.Now().UnixNano())
	globalCfg = loadConfig()

	log.Printf("==================================================")
	log.Printf("Starting System A Simulator (Postgres-backed)")
	log.Printf("Port            : %s", globalCfg.Port)
	log.Printf("Middleware      : %s", globalCfg.MiddlewareURL)
	log.Printf("Webhook URL     : %s", globalCfg.WebhookURL)
	log.Printf("Sender interval : %ds  batch-size: %d", globalCfg.SenderIntervalSecs, globalCfg.SenderBatchSize)
	log.Printf("Max retries     : %d", globalCfg.MaxRetries)
	log.Printf("Auto-trigger    : %v (%d batches × %d items, %d fail each)",
		globalCfg.AutoTrigger, globalCfg.AutoBatches, globalCfg.AutoSize, globalCfg.AutoFailCount)
	log.Printf("==================================================")

	// Connect to Postgres
	db, err := openDB(globalCfg.DBDSN)
	if err != nil {
		log.Fatalf("[System A] ❌ Cannot connect to DB: %v", err)
	}
	globalDB = db
	defer db.Close()

	// HTTP routes
	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/trigger", handleTrigger)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/stats", handleStats)

	// Background: sender + retry loop
	go runSender(db, globalCfg)

	// Background: auto-seed on startup
	if globalCfg.AutoTrigger {
		go autoSeed(db, globalCfg)
	}

	if err := http.ListenAndServe(":"+globalCfg.Port, nil); err != nil {
		log.Fatalf("[System A] Server error: %v", err)
	}
}

// ── Database layer ────────────────────────────────────────────────────────────

// openDB connects to Postgres with up to 15 retries (2-second back-off).
func openDB(dsn string) (*sqlx.DB, error) {
	var (
		db  *sqlx.DB
		err error
	)
	for i := 1; i <= 15; i++ {
		db, err = sqlx.Open("postgres", dsn)
		if err == nil {
			if pingErr := db.Ping(); pingErr == nil {
				db.SetMaxOpenConns(10)
				db.SetMaxIdleConns(5)
				db.SetConnMaxLifetime(5 * time.Minute)
				log.Printf("[System A] ✅ Connected to Postgres (attempt %d)", i)
				return db, nil
			}
		}
		log.Printf("[System A] ⏳ DB not ready (attempt %d/15): %v — retrying in 2s", i, err)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("could not connect after 15 attempts: %w", err)
}

// insertItems bulk-inserts a slice of LocalItem rows in a single transaction.
// Duplicate external_ids are silently skipped (ON CONFLICT DO NOTHING).
func insertItems(db *sqlx.DB, items []LocalItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(pq.CopyIn(
		"systema_items",
		"id", "batch_key", "external_id", "payload", "max_retries",
	))
	if err != nil {
		return fmt.Errorf("prepare copy: %w", err)
	}

	for _, it := range items {
		if _, err := stmt.Exec(it.ID, it.BatchKey, it.ExternalID, it.Payload, it.MaxRetries); err != nil {
			return fmt.Errorf("copy exec: %w", err)
		}
	}
	if _, err := stmt.Exec(); err != nil {
		return fmt.Errorf("copy flush: %w", err)
	}
	if err := stmt.Close(); err != nil {
		return fmt.Errorf("copy close: %w", err)
	}
	return tx.Commit()
}

// claimPendingBatch atomically marks up to batchSize eligible items as 'sending'
// and returns them. Uses SKIP LOCKED so concurrent senders don't race.
func claimPendingBatch(db *sqlx.DB, batchSize int) ([]LocalItem, error) {
	rows, err := db.Queryx(`
		UPDATE systema_items
		SET    status     = 'sending',
		       updated_at = NOW()
		WHERE  id IN (
		    SELECT id
		    FROM   systema_items
		    WHERE  status IN ('pending', 'failed')
		      AND  next_retry_at <= NOW()
		    ORDER BY next_retry_at ASC
		    LIMIT  $1
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING id, external_id, payload, batch_key, retry_count, max_retries
	`, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []LocalItem
	for rows.Next() {
		var it LocalItem
		if err := rows.StructScan(&it); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// markSent sets status='sent' for the given external IDs.
func markSent(db *sqlx.DB, externalIDs []string) error {
	if len(externalIDs) == 0 {
		return nil
	}
	_, err := db.Exec(`
		UPDATE systema_items
		SET    status     = 'sent',
		       updated_at = NOW()
		WHERE  external_id = ANY($1)
	`, pq.Array(externalIDs))
	return err
}

// markFailedBulk increments retry_count for each item, applies exponential
// back-off to next_retry_at, and marks items 'exhausted' when retries run out.
func markFailedBulk(db *sqlx.DB, items []FailedItem) error {
	if len(items) == 0 {
		return nil
	}
	extIDs := make([]string, len(items))
	errMsgs := make([]string, len(items))
	for i, it := range items {
		extIDs[i] = it.ExternalID
		errMsgs[i] = truncateString(it.ErrMsg, 900)
	}
	_, err := db.Exec(`
		UPDATE systema_items
		SET    status        = CASE
		                         WHEN retry_count + 1 >= max_retries THEN 'exhausted'
		                         ELSE 'failed'
		                       END,
		       retry_count   = retry_count + 1,
		       last_error    = v.err_msg,
		       next_retry_at = NOW() + LEAST(POWER(2, retry_count)::int * 5, 600) * INTERVAL '1 second',
		       updated_at    = NOW()
		FROM   unnest($1::text[], $2::text[]) AS v(ext_id, err_msg)
		WHERE  external_id = v.ext_id
		  AND  status IN ('sending', 'sent', 'failed', 'pending')
	`, pq.Array(extIDs), pq.Array(errMsgs))
	return err
}

// resetStuckSending resets any items stuck in 'sending' for more than 2 minutes
// back to 'failed' so they re-enter the retry queue.
func resetStuckSending(db *sqlx.DB) {
	res, err := db.Exec(`
		UPDATE systema_items
		SET    status        = 'failed',
		       last_error    = 'sender crash / timeout',
		       next_retry_at = NOW() + INTERVAL '10 seconds',
		       updated_at    = NOW()
		WHERE  status = 'sending'
		  AND  updated_at < NOW() - INTERVAL '2 minutes'
	`)
	if err != nil {
		log.Printf("[System A] ⚠️  resetStuckSending error: %v", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("[System A] 🔄 Reset %d stuck 'sending' items → 'failed'", n)
	}
}

// ── Data generation ───────────────────────────────────────────────────────────

// generateItems creates a slice of LocalItem with rich, realistic payloads.
// Items with index < failCount get an external_id starting with "fail-item"
// so System C's deterministic failure pattern triggers for them.
func generateItems(batchKey string, size, failCount, maxRetries int) []LocalItem {
	now := time.Now().UTC()
	items := make([]LocalItem, size)
	for i := 0; i < size; i++ {
		extID := fmt.Sprintf("ext-item-%d-%06d", now.UnixNano(), i)
		if i < failCount {
			extID = fmt.Sprintf("fail-item-%d-%06d", now.UnixNano(), i)
		}

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
		discountPct := rand.Intn(41)
		taxRate := math.Round((rand.Float64()*0.25+0.05)*1000) / 1000
		shippingFee := math.Round(rand.Float64()*50*100) / 100
		weight := math.Round(rand.Float64()*100*100) / 100
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
			"user_id":    userID,
			"email":      fmt.Sprintf("%s@%s", userID, domain),
			"order_id":   orderID,
			"session_id": sessionID,
			"item_index": i,

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

			"payment_method":     payMethod,
			"payment_reference":  fmt.Sprintf("PAY-%s-%08X", strings.ToUpper(payMethod[:3]), rand.Uint32()),
			"payment_gateway":    fmt.Sprintf("gateway-%d", rand.Intn(5)+1),
			"authorization_code": fmt.Sprintf("AUTH%06d", rand.Intn(1000000)),
			"transaction_id":     fmt.Sprintf("txn_%016x", rand.Uint64()),

			"region":      region,
			"channel":     channel,
			"environment": env,
			"department":  dept,
			"priority":    priority,
			"status":      status,
			"version":     version,
			"retry_count": retryCount,
			"tags":        itemTags,

			"created_at":   createdAt.Format(time.RFC3339),
			"scheduled_at": scheduledAt.Format(time.RFC3339),
			"expires_at":   expiredAt.Format(time.RFC3339),

			"client_ip": ip,
			"latitude":  latitude,
			"longitude": longitude,

			"description": fmt.Sprintf(
				"[%s] %s order %s — qty %d x %s @ %.2f %s (ch:%s, prio:%s, dept:%s)",
				strings.ToUpper(env), strings.ToUpper(category),
				orderID, qty, sku, unitPrice, currency,
				channel, priority, dept,
			),

			"metadata": map[string]interface{}{
				"source_system": "system-a-simulator",
				"batch_key":     batchKey,
				"batch_index":   i,
				"schema_ver":    "v3",
				"checksum":      fmt.Sprintf("%08x", rand.Uint32()),
			},
		}
		payloadBytes, _ := json.Marshal(payloadMap)

		items[i] = LocalItem{
			ID:         uuid.New().String(),
			BatchKey:   batchKey,
			ExternalID: extID,
			Payload:    string(payloadBytes),
			MaxRetries: maxRetries,
		}
	}
	return items
}

// seedItems generates items and persists them to systema_items.
// Returns immediately — the sender loop will drain the table.
func seedItems(db *sqlx.DB, size, failCount int) error {
	batchKey := fmt.Sprintf("seed-%d-%04d", time.Now().Unix(), rand.Intn(10000))
	items := generateItems(batchKey, size, failCount, globalCfg.MaxRetries)
	if err := insertItems(db, items); err != nil {
		return fmt.Errorf("insertItems: %w", err)
	}
	log.Printf("[System A] 💾 Seeded %d items (batch_key=%s, fail_count=%d) into systema_items",
		len(items), batchKey, failCount)
	return nil
}

// ── Sender loop ───────────────────────────────────────────────────────────────

// runSender is the background goroutine that:
//  1. Resets any items stuck in 'sending' (crash recovery)
//  2. Claims a batch of pending/failed items
//  3. POSTs them to the middleware as a single batch submission
//  4. Marks items 'sent' on success, or re-queues them with back-off on failure
func runSender(db *sqlx.DB, cfg AppConfig) {
	ticker := time.NewTicker(time.Duration(cfg.SenderIntervalSecs) * time.Second)
	defer ticker.Stop()

	log.Printf("[System A] 🔄 Sender loop started (interval=%ds, batch=%d)",
		cfg.SenderIntervalSecs, cfg.SenderBatchSize)

	// On startup, recover any items left in 'sending' from a previous run
	resetStuckSending(db)

	for range ticker.C {
		resetStuckSending(db)

		items, err := claimPendingBatch(db, cfg.SenderBatchSize)
		if err != nil {
			log.Printf("[System A] ⚠️  claimPendingBatch error: %v", err)
			continue
		}
		if len(items) == 0 {
			continue
		}

		log.Printf("[System A] 📦 Sender claimed %d items — building batch...", len(items))
		if err := sendBatch(db, cfg, items); err != nil {
			log.Printf("[System A] ⚠️  sendBatch error: %v", err)
		}
	}
}

// sendBatch packages the given items into a SubmitBatchRequest and POSTs it to
// the middleware. On success it marks them 'sent'; on failure it re-queues them.
func sendBatch(db *sqlx.DB, cfg AppConfig, items []LocalItem) error {
	correlationID := fmt.Sprintf("corr-%d-%04d", time.Now().Unix(), rand.Intn(10000))
	idemKey := fmt.Sprintf("idem-sender-%d-%06d", time.Now().Unix(), rand.Intn(1000000))

	batchItems := make([]BatchItemRequest, len(items))
	for i, it := range items {
		batchItems[i] = BatchItemRequest{
			ExternalID: it.ExternalID,
			Payload:    json.RawMessage(it.Payload),
		}
	}

	reqBody := SubmitBatchRequest{
		CorrelationID: correlationID,
		WebhookURL:    cfg.WebhookURL,
		Items:         batchItems,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.MiddlewareURL+"/api/v1/batches", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Idempotency-Key", idemKey)
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	log.Printf("[System A] 📤 POST %d items → middleware (idem=%s, ~%.1f KB)",
		len(items), idemKey, float64(len(bodyBytes))/1024)

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Network error — re-queue all items
		failed := make([]FailedItem, len(items))
		for i, it := range items {
			failed[i] = FailedItem{ExternalID: it.ExternalID, ErrMsg: err.Error()}
		}
		if dbErr := markFailedBulk(db, failed); dbErr != nil {
			log.Printf("[System A] ⚠️  markFailedBulk error: %v", dbErr)
		}
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusAccepted {
		// Middleware rejected the batch — re-queue all items
		errMsg := fmt.Sprintf("middleware status %d: %s", resp.StatusCode, string(respBody))
		failed := make([]FailedItem, len(items))
		for i, it := range items {
			failed[i] = FailedItem{ExternalID: it.ExternalID, ErrMsg: errMsg}
		}
		if dbErr := markFailedBulk(db, failed); dbErr != nil {
			log.Printf("[System A] ⚠️  markFailedBulk error: %v", dbErr)
		}
		return fmt.Errorf(errMsg)
	}

	var submitResp SubmitBatchResponse
	if err := json.Unmarshal(respBody, &submitResp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	// Mark all items as sent — individual failures come back via webhook
	extIDs := make([]string, len(items))
	for i, it := range items {
		extIDs[i] = it.ExternalID
	}
	if err := markSent(db, extIDs); err != nil {
		log.Printf("[System A] ⚠️  markSent error: %v", err)
	}

	log.Printf("[System A] ✅ Batch accepted — middleware batch_id=%s (%d items sent)",
		submitResp.BatchID, len(items))
	return nil
}

// ── Auto-seeder ───────────────────────────────────────────────────────────────

// autoSeed fires after a short delay on startup, seeding multiple batches
// concurrently to populate systema_items for the sender loop to drain.
func autoSeed(db *sqlx.DB, cfg AppConfig) {
	time.Sleep(5 * time.Second) // wait for DB + middleware to be ready

	log.Printf("[System A] 🌱 Auto-seeding %d batches × %d items (%d fail each) = %d total rows...",
		cfg.AutoBatches, cfg.AutoSize, cfg.AutoFailCount, cfg.AutoBatches*cfg.AutoSize)

	type result struct {
		index int
		err   error
	}
	resultCh := make(chan result, cfg.AutoBatches)

	for i := 0; i < cfg.AutoBatches; i++ {
		go func(idx int) {
			resultCh <- result{index: idx, err: seedItems(db, cfg.AutoSize, cfg.AutoFailCount)}
		}(i)
	}

	succ, fail := 0, 0
	for i := 0; i < cfg.AutoBatches; i++ {
		res := <-resultCh
		if res.err != nil {
			log.Printf("[System A] ❌ Auto-seed batch #%d failed: %v", res.index, res.err)
			fail++
		} else {
			succ++
		}
	}
	log.Printf("[System A] 🏁 Auto-seed done: %d/%d batches inserted into DB (sender loop will drain them)",
		succ, cfg.AutoBatches)
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

// handleWebhook receives completion callbacks from the middleware.
// If the batch has failed items, those are re-queued in systema_items.
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

	// Verify HMAC signature
	sigHeader := r.Header.Get("X-Signature")
	if sigHeader != "" && globalCfg.WebhookSecret != "" {
		sigHex := strings.TrimPrefix(sigHeader, "sha256=")
		expectedSig, err := hex.DecodeString(sigHex)
		if err != nil {
			log.Printf("[System A] ❌ Invalid signature hex: %v", err)
			http.Error(w, "Invalid signature format", http.StatusUnauthorized)
			return
		}
		mac := hmac.New(sha256.New, []byte(globalCfg.WebhookSecret))
		mac.Write(bodyBytes)
		if !hmac.Equal(expectedSig, mac.Sum(nil)) {
			log.Printf("[System A] ❌ Invalid webhook signature — rejecting")
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	var payload WebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		log.Printf("[System A] ❌ Unmarshal webhook: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Pretty-print webhook summary
	statusIcon := "🟢"
	if payload.Status == "failed" {
		statusIcon = "🔴"
	} else if payload.Status == "partial" {
		statusIcon = "🟡"
	}

	log.Printf("\n")
	log.Printf("╭──────────────────────────────────────────────────────────────╮")
	log.Printf("│ 📥 WEBHOOK RECEIVED                                          │")
	log.Printf("├──────────────────────────────────────────────────────────────┤")
	log.Printf("│ Event:          %-44s│", payload.Event)
	log.Printf("│ Batch ID:       %-44s│", payload.BatchID)
	log.Printf("│ Correlation ID: %-44s│", payload.CorrelationID)
	log.Printf("│ Status:         %-2s %-41s│", statusIcon, payload.Status)
	log.Printf("│ Timestamp:      %-44s│", payload.Timestamp)
	log.Printf("├──────────────────────────────────────────────────────────────┤")
	log.Printf("│ Summary: total=%-4d  success=%-4d  failed=%-4d              │",
		payload.Summary.Total, payload.Summary.Success, payload.Summary.Failed)
	log.Printf("╰──────────────────────────────────────────────────────────────╯")

	// Re-queue items that failed downstream so the retry batcher picks them up
	if len(payload.FailedItems) > 0 {
		failed := make([]FailedItem, len(payload.FailedItems))
		for i, fi := range payload.FailedItems {
			failed[i] = FailedItem{ExternalID: fi.ExternalID, ErrMsg: fi.Error}
		}
		if err := markFailedBulk(globalDB, failed); err != nil {
			log.Printf("[System A] ⚠️  markFailedBulk from webhook: %v", err)
		} else {
			log.Printf("[System A] 🔁 Re-queued %d failed items for retry", len(failed))
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleTrigger seeds new items into Postgres (the sender loop delivers them).
//
//	POST /trigger?batches=10&size=500&fail_count=50
func handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()

	size := 500
	if v, err := strconv.Atoi(q.Get("size")); err == nil && v > 0 {
		size = v
	}
	failCount := 50
	if v, err := strconv.Atoi(q.Get("fail_count")); err == nil && v >= 0 {
		failCount = v
	}
	numBatches := 10
	if v, err := strconv.Atoi(q.Get("batches")); err == nil && v > 0 {
		if v > 50 {
			v = 50
		}
		numBatches = v
	}

	log.Printf("[System A] 🚀 Manual trigger: %d batches × %d items (%d fail) = %d total rows",
		numBatches, size, failCount, numBatches*size)

	type result struct {
		index int
		err   error
	}
	resultCh := make(chan result, numBatches)
	for i := 0; i < numBatches; i++ {
		go func(idx int) {
			resultCh <- result{index: idx, err: seedItems(globalDB, size, failCount)}
		}(i)
	}

	var errList []string
	for i := 0; i < numBatches; i++ {
		res := <-resultCh
		if res.err != nil {
			log.Printf("[System A] ❌ Seed batch #%d: %v", res.index, res.err)
			errList = append(errList, fmt.Sprintf("batch %d: %v", res.index, res.err))
		} else {
			log.Printf("[System A] ✅ Seed batch #%d done", res.index)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if len(errList) == numBatches {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(fmt.Sprintf(`{"status":"error","message":%q}`, strings.Join(errList, "; "))))
		return
	}
	if len(errList) > 0 {
		w.WriteHeader(http.StatusMultiStatus)
		w.Write([]byte(fmt.Sprintf(`{"status":"partial","batches":%d,"errors":%d,"items_per_batch":%d}`,
			numBatches, len(errList), size)))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(
		`{"status":"success","batches":%d,"items_per_batch":%d,"total_seeded":%d,"note":"sender loop will deliver items"}`,
		numBatches, size, numBatches*size,
	)))
}

// handleHealth returns a quick liveness check including DB connectivity.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := globalDB.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(fmt.Sprintf(`{"status":"unhealthy","db":"%v"}`, err)))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","service":"system-a"}`))
}

// handleStats returns a quick count of items by status.
func handleStats(w http.ResponseWriter, r *http.Request) {
	rows, err := globalDB.Query(`
		SELECT status, COUNT(*) AS n
		FROM   systema_items
		GROUP  BY status
		ORDER  BY status
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err == nil {
			counts[status] = n
		}
	}

	b, _ := json.Marshal(counts)
	w.Header().Set("Content-Type", "application/json")
	w.Write(b)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func randPick(s []string) string { return s[rand.Intn(len(s))] }

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
