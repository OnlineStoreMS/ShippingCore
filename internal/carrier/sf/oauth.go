package sf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	// SandboxOAuthURL 沙箱 OAuth2 取 token（云打印插件 SCPPrint.print 的 accessToken）
	SandboxOAuthURL = "https://sfapi-sbox.sf-express.com/oauth2/accessToken"
	// ProdOAuthURL 生产 OAuth2 取 token
	ProdOAuthURL = "https://sfapi.sf-express.com/oauth2/accessToken"
)

type oauthTokenEntry struct {
	token     string
	expiresAt time.Time
}

var (
	oauthCacheMu sync.Mutex
	oauthCache   = map[string]oauthTokenEntry{}
)

// OAuthURL 返回当前环境的 OAuth2 accessToken 地址。
func (c *Client) OAuthURL() string {
	if c.sandbox {
		return SandboxOAuthURL
	}
	return ProdOAuthURL
}

// GetAccessToken 获取丰桥 OAuth2 accessToken（约 2 小时有效，进程内缓存）。
// 表单字段：partnerID + secret(=校验码) + grantType=password。
func (c *Client) GetAccessToken(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.partnerID) == "" || strings.TrimSpace(c.checkword) == "" {
		return "", fmt.Errorf("sf oauth: partnerID/checkword is required")
	}
	cacheKey := c.partnerID + "|" + c.OAuthURL()
	oauthCacheMu.Lock()
	if ent, ok := oauthCache[cacheKey]; ok && time.Now().Before(ent.expiresAt) {
		token := ent.token
		oauthCacheMu.Unlock()
		return token, nil
	}
	oauthCacheMu.Unlock()

	token, expiresIn, err := c.fetchAccessToken(ctx)
	if err != nil {
		return "", err
	}
	if expiresIn <= 0 {
		expiresIn = 7200
	}
	// 提前 2 分钟过期，避免边界失败
	ttl := time.Duration(expiresIn)*time.Second - 2*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	oauthCacheMu.Lock()
	oauthCache[cacheKey] = oauthTokenEntry{token: token, expiresAt: time.Now().Add(ttl)}
	oauthCacheMu.Unlock()
	return token, nil
}

func (c *Client) fetchAccessToken(ctx context.Context) (token string, expiresIn int, err error) {
	form := url.Values{}
	form.Set("partnerID", c.partnerID)
	form.Set("secret", c.checkword)
	form.Set("grantType", "password")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.OAuthURL(), strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("sf oauth http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode >= 400 {
		return "", 0, fmt.Errorf("sf oauth http %d: %s", resp.StatusCode, truncate(string(body), 256))
	}

	token, expiresIn, err = parseOAuthAccessTokenResponse(body)
	if err != nil {
		return "", 0, err
	}
	return token, expiresIn, nil
}

type oauthAccessTokenResponse struct {
	APIResultCode string `json:"apiResultCode"`
	APIErrorMsg   string `json:"apiErrorMsg"`
	AccessToken   string `json:"accessToken"`
	ExpiresIn     int    `json:"expiresIn"`
	// 部分环境用 snake / 嵌套
	Access_token string `json:"access_token"`
	Expires_in   int    `json:"expires_in"`
}

// parseOAuthAccessTokenResponse 解析丰桥 oauth2/accessToken 响应。
func parseOAuthAccessTokenResponse(body []byte) (token string, expiresIn int, err error) {
	var resp oauthAccessTokenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", 0, fmt.Errorf("sf oauth decode: %w", err)
	}
	code := strings.TrimSpace(resp.APIResultCode)
	if code != "" && code != "A1000" {
		return "", 0, fmt.Errorf("sf oauth: %s", firstNonEmpty(resp.APIErrorMsg, code))
	}
	token = firstNonEmpty(resp.AccessToken, resp.Access_token)
	if token == "" {
		return "", 0, fmt.Errorf("sf oauth: empty accessToken: %s", truncate(string(body), 256))
	}
	expiresIn = resp.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = resp.Expires_in
	}
	return token, expiresIn, nil
}
