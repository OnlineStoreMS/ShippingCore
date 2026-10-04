package agentscenter

import (
	"bytes"
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
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" || strings.TrimSpace(token) == "" {
		return nil
	}
	return &Client{
		BaseURL: baseURL,
		Token:   strings.TrimSpace(token),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

type Agent struct {
	ID            uint64  `json:"id"`
	MachineID     string  `json:"machineId"`
	Name          string  `json:"name"`
	Status        string  `json:"status"`
	SkillsJSON    string  `json:"skillsJson"`
	LastHeartbeat *string `json:"lastHeartbeat"`
	CreatedAt     string  `json:"createdAt"`
}

type createJobBody struct {
	TenantID         uint64  `json:"tenantId"`
	JobType          string  `json:"jobType"`
	Platform         string  `json:"platform"`
	PlatformShopID   string  `json:"platformShopId"`
	PlatformShopName string  `json:"platformShopName"`
	ParamsJSON       string  `json:"paramsJson"`
	Source           string  `json:"source"`
	Priority         int     `json:"priority"`
	TargetAgentID    *uint64 `json:"targetAgentId"`
}

type CreatedJob struct {
	ID uint64 `json:"id"`
}

type JobStatus struct {
	ID           uint64  `json:"id"`
	Status       string  `json:"status"`
	ErrorMessage string  `json:"errorMessage"`
	ResultJSON   string  `json:"resultJson,omitempty"`
	StartedAt    *string `json:"startedAt,omitempty"`
	FinishedAt   *string `json:"finishedAt,omitempty"`
}

func (c *Client) ListAgents(tenantID uint64, onlineOnly bool, skill string) ([]Agent, error) {
	if c == nil {
		return nil, fmt.Errorf("AgentsCenter 未配置")
	}
	q := url.Values{}
	q.Set("tenantId", fmt.Sprintf("%d", tenantID))
	q.Set("page", "1")
	q.Set("pageSize", "200")
	if onlineOnly {
		q.Set("onlineOnly", "1")
	}
	if strings.TrimSpace(skill) != "" {
		q.Set("skill", strings.TrimSpace(skill))
	}
	u := fmt.Sprintf("%s/api/v1/internal/agents?%s", c.BaseURL, q.Encode())
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Token", c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("AgentsCenter HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var envelope struct {
		Data struct {
			List []Agent `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data.List, nil
}

func (c *Client) GetJobs(tenantID uint64, ids []uint64) ([]JobStatus, error) {
	if c == nil {
		return nil, fmt.Errorf("AgentsCenter 未配置")
	}
	if len(ids) == 0 {
		return nil, nil
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, fmt.Sprintf("%d", id))
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	q := url.Values{}
	q.Set("tenantId", fmt.Sprintf("%d", tenantID))
	q.Set("ids", strings.Join(parts, ","))
	u := fmt.Sprintf("%s/api/v1/internal/jobs?%s", c.BaseURL, q.Encode())
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Token", c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("AgentsCenter HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var envelope struct {
		Data struct {
			List []JobStatus `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	return envelope.Data.List, nil
}

func (c *Client) CreateTargetedJob(tenantID, targetAgentID uint64, jobType, paramsJSON, source string) (*CreatedJob, error) {
	if c == nil {
		return nil, fmt.Errorf("AgentsCenter 未配置")
	}
	if targetAgentID == 0 {
		return nil, fmt.Errorf("targetAgentId 必填")
	}
	if strings.TrimSpace(jobType) == "" {
		jobType = "kdzs.remote.print"
	}
	if strings.TrimSpace(source) == "" {
		source = "shipping"
	}
	aid := targetAgentID
	body := createJobBody{
		TenantID:         tenantID,
		JobType:          jobType,
		Platform:         "shipping",
		PlatformShopID:   "kdzs-print",
		PlatformShopName: "远程打单",
		ParamsJSON:       paramsJSON,
		Source:           source,
		Priority:         50,
		TargetAgentID:    &aid,
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/internal/jobs", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("AgentsCenter HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var envelope struct {
		Data CreatedJob `json:"data"`
	}
	if err := json.Unmarshal(b, &envelope); err != nil {
		return nil, err
	}
	return &envelope.Data, nil
}

// UpdatePendingJobParams 更新仍排队中的执行单参数（合并批量打单用）。
func (c *Client) UpdatePendingJobParams(tenantID, jobID uint64, paramsJSON string) error {
	if c == nil {
		return fmt.Errorf("AgentsCenter 未配置")
	}
	if jobID == 0 {
		return fmt.Errorf("jobId 必填")
	}
	body, _ := json.Marshal(map[string]any{
		"tenantId":   tenantID,
		"paramsJson": paramsJSON,
	})
	u := fmt.Sprintf("%s/api/v1/internal/jobs/%d", c.BaseURL, jobID)
	req, err := http.NewRequest(http.MethodPatch, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("AgentsCenter HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// CancelPendingJob 取消仍排队中的执行单。
func (c *Client) CancelPendingJob(tenantID, jobID uint64, reason string) error {
	if c == nil {
		return fmt.Errorf("AgentsCenter 未配置")
	}
	if jobID == 0 {
		return fmt.Errorf("jobId 必填")
	}
	body, _ := json.Marshal(map[string]any{
		"tenantId": tenantID,
		"reason":   reason,
	})
	u := fmt.Sprintf("%s/api/v1/internal/jobs/%d/cancel", c.BaseURL, jobID)
	req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return fmt.Errorf("AgentsCenter HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}
