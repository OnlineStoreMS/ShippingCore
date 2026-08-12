package ordercore

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
		baseURL = "http://127.0.0.1:8098"
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

type OrderQuery struct {
	Page            int    `form:"page"`
	PageSize        int    `form:"pageSize"`
	ShipStatus      string `form:"shipStatus"`
	AllocType       string `form:"allocType"`
	Platform        string `form:"platform"`
	Keyword         string `form:"keyword"`
	PlatformSysTid  string `form:"platformSysTid"`
	SourceChannel   string `form:"sourceChannel"`
	OrderedAtStart  string `form:"orderedAtStart"`
	OrderedAtEnd    string `form:"orderedAtEnd"`
	PayTimeStart    string `form:"payTimeStart"`
	PayTimeEnd      string `form:"payTimeEnd"`
}

type ShipRequest struct {
	ExpressCompany string          `json:"expressCompany"`
	ExpressNo      string          `json:"expressNo"`
	Remark         string          `json:"remark,omitempty"`
	Callback       bool            `json:"callback"`
	Items          []ShipItemInput `json:"items,omitempty"`
}

type ShipItemInput struct {
	OrderItemID uint64 `json:"orderItemId"`
	Qty         int    `json:"qty"`
}

type apiBody struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) ListOrders(ctx context.Context, token string, query OrderQuery) (json.RawMessage, error) {
	if c == nil || c.BaseURL == "" {
		return nil, fmt.Errorf("ordercore 未配置")
	}
	q := url.Values{}
	if query.Page > 0 {
		q.Set("page", fmt.Sprintf("%d", query.Page))
	}
	if query.PageSize > 0 {
		q.Set("pageSize", fmt.Sprintf("%d", query.PageSize))
	}
	if query.ShipStatus != "" {
		q.Set("shipStatus", query.ShipStatus)
	}
	if query.AllocType != "" {
		q.Set("allocType", query.AllocType)
	}
	if query.Platform != "" {
		q.Set("platform", query.Platform)
	}
	if query.Keyword != "" {
		q.Set("keyword", query.Keyword)
	}
	if query.PlatformSysTid != "" {
		q.Set("platformSysTid", query.PlatformSysTid)
	}
	if query.SourceChannel != "" {
		q.Set("sourceChannel", query.SourceChannel)
	}
	if query.OrderedAtStart != "" {
		q.Set("orderedAtStart", query.OrderedAtStart)
	}
	if query.OrderedAtEnd != "" {
		q.Set("orderedAtEnd", query.OrderedAtEnd)
	}
	if query.PayTimeStart != "" {
		q.Set("payTimeStart", query.PayTimeStart)
	}
	if query.PayTimeEnd != "" {
		q.Set("payTimeEnd", query.PayTimeEnd)
	}

	reqURL := c.BaseURL + "/api/v1/admin/orders"
	if encoded := q.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}
	return c.doJSON(ctx, http.MethodGet, reqURL, token, nil)
}

func (c *Client) GetOrder(ctx context.Context, token string, id uint64) (json.RawMessage, error) {
	if c == nil || c.BaseURL == "" {
		return nil, fmt.Errorf("ordercore 未配置")
	}
	reqURL := fmt.Sprintf("%s/api/v1/admin/orders/%d", c.BaseURL, id)
	return c.doJSON(ctx, http.MethodGet, reqURL, token, nil)
}

func (c *Client) Ship(ctx context.Context, token string, orderID uint64, body ShipRequest) (json.RawMessage, error) {
	if c == nil || c.BaseURL == "" {
		return nil, fmt.Errorf("ordercore 未配置")
	}
	reqURL := fmt.Sprintf("%s/api/v1/admin/orders/%d/ship", c.BaseURL, orderID)
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
		return nil, fmt.Errorf("ordercore request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("ordercore http %d: %s", resp.StatusCode, truncate(string(raw), 256))
	}

	var wrapped apiBody
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return raw, nil
	}
	if wrapped.Code != 0 && wrapped.Code != 200 {
		msg := wrapped.Message
		if msg == "" {
			msg = "ordercore error"
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
