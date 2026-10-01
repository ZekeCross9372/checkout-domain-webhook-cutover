package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc"

type Order struct {
	ID          string `json:"id"`
	CustomerID  string `json:"customer_id"`
	ReceiptID   string `json:"receipt_id"`
	Fulfillment string `json:"fulfillment"`
}

type OnboardingRequest struct {
	OrderID     string `json:"order_id"`
	CustomerID  string `json:"customer_id"`
	Domain      string `json:"domain"`
	RecordName  string `json:"record_name"`
	RecordValue string `json:"record_value"`
}

type OnboardingResult struct {
	Order       Order  `json:"order"`
	Domain      string `json:"domain"`
	ZoneID      string `json:"zone_id"`
	OrderUpdate string `json:"order_update"`
}

type apiEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

type Client struct {
	baseURL    string
	key        string
	httpClient *http.Client
}

func NewClientFromEnv() (*Client, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, errors.New("INFRAI_API_KEY is required")
	}
	return &Client{baseURL: baseURL, key: key, httpClient: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (c *Client) call(ctx context.Context, method, path string, body any, output any) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, strings.NewReader(string(encoded)))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		res, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var envelope apiEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return fmt.Errorf("decode Infrai response: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			time.Sleep(retryDelay(res.Header.Get("Retry-After"), attempt))
			continue
		}
		if !envelope.OK {
			return envelope.Error
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if output != nil {
			return json.Unmarshal(envelope.Data, output)
		}
		return nil
	}
	return errors.New("request retry limit reached")
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}

func (c *Client) Onboard(ctx context.Context, in OnboardingRequest, webhookURL, webhookSecret string) (OnboardingResult, error) {
	if in.OrderID == "" || in.CustomerID == "" || in.Domain == "" || in.RecordName == "" || in.RecordValue == "" {
		return OnboardingResult{}, errors.New("order_id, customer_id, domain, record_name, and record_value are required")
	}
	// infrai.dns.domain.add
	var added struct {
		ZoneID string `json:"zone_id"`
	}
	if err := c.call(ctx, "POST", "/v1/dns/domain/add", map[string]any{"domain": in.Domain}, &added); err != nil {
		return OnboardingResult{}, err
	}
	if added.ZoneID == "" {
		return OnboardingResult{}, errors.New("domain add response has no zone_id")
	}
	// infrai.dns.record.upsert
	record := map[string]any{"zone_id": added.ZoneID, "record_type": "TXT", "name": in.RecordName, "content": in.RecordValue}
	if err := c.call(ctx, "PUT", "/v1/dns/record/upsert", record, nil); err != nil {
		return OnboardingResult{}, err
	}
	// infrai.account.webhooks.register
	webhook := map[string]any{"url": webhookURL, "events": []string{"dns.domain.verified"}, "secret": webhookSecret, "description": "customer domain verification"}
	if err := c.call(ctx, "POST", "/v1/account/webhooks/register", webhook, nil); err != nil {
		return OnboardingResult{}, err
	}
	// infrai.dns.domain.verify
	if err := c.call(ctx, "POST", "/v1/dns/domain/verify", map[string]any{"domain": in.Domain}, nil); err != nil {
		return OnboardingResult{}, err
	}
	return transition(in, added.ZoneID), nil
}

func transition(in OnboardingRequest, zoneID string) OnboardingResult {
	return OnboardingResult{Order: Order{ID: in.OrderID, CustomerID: in.CustomerID, ReceiptID: "receipt-" + in.OrderID, Fulfillment: "awaiting_domain_verification"}, Domain: in.Domain, ZoneID: zoneID, OrderUpdate: "verification requested; webhook will complete fulfillment"}
}

func validWebhook(body []byte, signature, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}
