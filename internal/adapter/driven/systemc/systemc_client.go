package systemc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/semmidev/batch-processing/internal/config"
	"github.com/semmidev/batch-processing/internal/observability"
	"github.com/semmidev/batch-processing/internal/port/output"
	"github.com/sony/gobreaker/v2"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

type systemCClient struct {
	client      *http.Client
	baseURL     string
	apiKey      string
	cb          *gobreaker.CircuitBreaker[[]byte]
	rateLimiter *rate.Limiter
}

// NewSystemCClient creates a new systemCClient implementing output.SystemCClient.
func NewSystemCClient(cfg *config.Config) output.SystemCClient {
	httpClient := &http.Client{
		Timeout: time.Duration(cfg.SystemC.TimeoutSeconds) * time.Second,
	}

	st := gobreaker.Settings{
		Name:        "SystemC",
		MaxRequests: uint32(cfg.CB.MaxRequestsHalfOpen),
		Interval:    10 * time.Second, // rolling window for counting failures
		Timeout:     time.Duration(cfg.CB.OpenTimeoutSeconds) * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Only evaluate after we have a meaningful sample size
			if counts.Requests < uint32(cfg.CB.MinRequestsToTrip) {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.CB.FailureRatioThreshold
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			observability.Log.Warn("circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		},
	}
	cb := gobreaker.NewCircuitBreaker[[]byte](st)
	limiter := rate.NewLimiter(rate.Limit(cfg.SystemC.RateLimitPerSecond), cfg.SystemC.RateLimitPerSecond)

	return &systemCClient{
		client:      httpClient,
		baseURL:     cfg.SystemC.BaseURL,
		apiKey:      cfg.SystemC.APIKey,
		cb:          cb,
		rateLimiter: limiter,
	}
}

func (s *systemCClient) SendItem(ctx context.Context, externalID string, payload string) ([]byte, error) {
	if err := s.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}

	reqBody := map[string]interface{}{
		"external_id": externalID,
		"payload":     json.RawMessage(payload),
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := s.cb.Execute(func() ([]byte, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/process", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		if s.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+s.apiKey)
		}

		res, err := s.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()

		resBody, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, err
		}

		if res.StatusCode >= 400 {
			// for 4xx errors we usually don't want to trip the circuit breaker, but for 5xx we do.
			// simplifed logic: trip on 5xx
			if res.StatusCode >= 500 {
				return nil, fmt.Errorf("system C server error: status %d, body: %s", res.StatusCode, string(resBody))
			}
			return resBody, fmt.Errorf("system C client error: status %d", res.StatusCode)
		}

		return resBody, nil
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return nil, fmt.Errorf("system C unavailable (circuit breaker open): %w", err)
		}
		return nil, err
	}

	return resp, nil
}
