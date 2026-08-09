package storesyncagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

type KdzsAccountInput struct {
	Code      string `json:"code"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Mobile    string `json:"mobile"`
	Password  string `json:"password"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
}

type KdzsAccountUpdateInput struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Mobile    string `json:"mobile"`
	Password  string `json:"password"`
	Enabled   *bool  `json:"enabled"`
	SortOrder *int   `json:"sortOrder"`
}

type AccountIDRequest struct {
	AccountID string `json:"accountId"`
}

func (c *Client) ListKdzsAccounts(ctx context.Context, token string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodGet, c.BaseURL+"/api/v1/admin/kdzs/accounts", token, nil)
}

func (c *Client) ListKdzsAccountDetails(ctx context.Context, token string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodGet, c.BaseURL+"/api/v1/admin/kdzs/account-details", token, nil)
}

func (c *Client) ExportKdzsAccounts(ctx context.Context, token string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodGet, c.BaseURL+"/api/v1/admin/kdzs/accounts/export", token, nil)
}

func (c *Client) CreateKdzsAccount(ctx context.Context, token string, body KdzsAccountInput) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost, c.BaseURL+"/api/v1/admin/kdzs/accounts", token, body)
}

func (c *Client) UpdateKdzsAccount(ctx context.Context, token, id string, body KdzsAccountUpdateInput) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPut, c.BaseURL+"/api/v1/admin/kdzs/accounts/"+id, token, body)
}

func (c *Client) DeleteKdzsAccount(ctx context.Context, token, id string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodDelete, c.BaseURL+"/api/v1/admin/kdzs/accounts/"+id, token, nil)
}

func (c *Client) SetDefaultKdzsAccount(ctx context.Context, token, accountID string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost, c.BaseURL+"/api/v1/admin/kdzs/accounts/default", token, AccountIDRequest{AccountID: accountID})
}

func (c *Client) SwitchKdzsAccount(ctx context.Context, token, accountID string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost, c.BaseURL+"/api/v1/admin/kdzs/accounts/switch", token, AccountIDRequest{AccountID: accountID})
}

func (c *Client) ListElecAuth(ctx context.Context, token string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodGet, c.BaseURL+"/api/v1/admin/kdzs/elec-auth", token, nil)
}

func (c *Client) ListExpressTemplates(ctx context.Context, token string) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodGet, c.BaseURL+"/api/v1/admin/kdzs/express-templates", token, nil)
}

func (c *Client) GetBatchPrintURL(ctx context.Context, token, platform string) (json.RawMessage, error) {
	reqURL := c.BaseURL + "/api/v1/admin/kdzs/batch-print-url?platform=" + url.QueryEscape(platform)
	return c.doJSON(ctx, http.MethodGet, reqURL, token, nil)
}

type PrintWaybillQueryItem struct {
	SysTid string `json:"sysTid,omitempty"`
	Tid    string `json:"tid,omitempty"`
}

type PrintWaybillQueryRequest struct {
	Platform string                  `json:"platform"`
	Items    []PrintWaybillQueryItem `json:"items"`
}

func (c *Client) QueryPrintWaybills(ctx context.Context, token string, body PrintWaybillQueryRequest) (json.RawMessage, error) {
	return c.doJSON(ctx, http.MethodPost, c.BaseURL+"/api/v1/admin/kdzs/print-waybills", token, body)
}
