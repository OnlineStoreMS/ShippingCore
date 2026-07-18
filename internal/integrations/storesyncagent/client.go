package storesyncagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8097"
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    httpClient,
	}
}

type OrderQuery struct {
	Platform      string `form:"platform"`
	ShopID        string `form:"shopId"`
	TradeStatus   string `form:"tradeStatus"`
	PageNo        int    `form:"pageNo"`
	PageSize      int    `form:"pageSize"`
	TimeType      int    `form:"timeType"`
	StartDateTime string `form:"startDateTime"`
	EndDateTime   string `form:"endDateTime"`
}

type DecryptOrdersRequest struct {
	Platform    string   `json:"platform"`
	TradeStatus string   `json:"tradeStatus"`
	SysTids     []string `json:"sysTids"`
}

type apiBody struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) ListOrders(ctx context.Context, token string, query OrderQuery) (json.RawMessage, error) {
	if c == nil || c.BaseURL == "" {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	q := url.Values{}
	if query.Platform != "" {
		q.Set("platform", query.Platform)
	}
	if query.ShopID != "" {
		q.Set("shopId", query.ShopID)
	}
	if query.TradeStatus != "" {
		q.Set("tradeStatus", query.TradeStatus)
	}
	if query.PageNo > 0 {
		q.Set("pageNo", fmt.Sprintf("%d", query.PageNo))
	}
	if query.PageSize > 0 {
		q.Set("pageSize", fmt.Sprintf("%d", query.PageSize))
	}
	if query.TimeType > 0 {
		q.Set("timeType", fmt.Sprintf("%d", query.TimeType))
	}
	if query.StartDateTime != "" {
		q.Set("startDateTime", query.StartDateTime)
	}
	if query.EndDateTime != "" {
		q.Set("endDateTime", query.EndDateTime)
	}

	reqURL := c.BaseURL + "/api/v1/admin/orders"
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}
	return c.doJSON(ctx, http.MethodGet, reqURL, token, nil)
}

func (c *Client) DecryptOrders(ctx context.Context, token string, body DecryptOrdersRequest) (json.RawMessage, error) {
	if c == nil || c.BaseURL == "" {
		return nil, fmt.Errorf("storesyncagent 未配置")
	}
	reqURL := c.BaseURL + "/api/v1/admin/orders/decrypt"
	return c.doJSON(ctx, http.MethodPost, reqURL, token, body)
}

func (c *Client) doJSON(ctx context.Context, method, reqURL, token string, body interface{}) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = strings.NewReader(string(payload))
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		if !strings.HasPrefix(token, "Bearer ") {
			token = "Bearer " + token
		}
		req.Header.Set("Authorization", token)
	}

	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storesyncagent request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("storesyncagent http %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}

	var wrapped apiBody
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return raw, nil
	}
	if wrapped.Code != 0 && wrapped.Code != 200 {
		msg := wrapped.Message
		if msg == "" {
			msg = "storesyncagent error"
		}
		return nil, fmt.Errorf("%s", msg)
	}
	if len(wrapped.Data) > 0 {
		return wrapped.Data, nil
	}
	return raw, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
