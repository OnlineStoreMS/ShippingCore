package sf

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ProdURL    = "https://bspgw.sf-express.com/std/service"
	SandboxURL = "https://sfapi-sbox.sf-express.com/std/service"

	ServiceCreateOrder = "EXP_RECE_CREATE_ORDER"
	ServiceUpdateOrder = "EXP_RECE_UPDATE_ORDER"
	ServiceCloudPrint  = "COM_RECE_CLOUD_PRINT_WAYBILLS"
)

type Client struct {
	partnerID string
	checkword string
	baseURL   string
	http      *http.Client
}

func NewClient(partnerID, checkword, env string) *Client {
	baseURL := SandboxURL
	if strings.EqualFold(env, "prod") || strings.EqualFold(env, "production") {
		baseURL = ProdURL
	}
	return &Client{
		partnerID: strings.TrimSpace(partnerID),
		checkword: strings.TrimSpace(checkword),
		baseURL:   baseURL,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type ContactInfo struct {
	ContactType int    `json:"contactType"`
	Contact     string `json:"contact"`
	Mobile      string `json:"mobile"`
	Province    string `json:"province,omitempty"`
	City        string `json:"city,omitempty"`
	County      string `json:"county,omitempty"`
	Address     string `json:"address"`
	Company     string `json:"company,omitempty"`
}

type CargoDetail struct {
	Name  string `json:"name"`
	Count int    `json:"count,omitempty"`
}

type CreateOrderRequest struct {
	OrderID      string
	UseMonthly   bool
	CustID       string
	ExpressType  string
	PayMethod    int
	ParcelQty    int
	CargoName    string
	CargoDetails []CargoDetail
	Remark       string
	TotalWeight  float64 // kg, optional
	Shipper      ContactInfo
	Receiver     ContactInfo
}

type CreateOrderResult struct {
	MailNo    string
	SFOrderID string
	Raw       json.RawMessage
}

type PrintResult struct {
	LabelURL   string
	LabelToken string // 下载 PDF 时请求头 X-Auth-token
	LabelData  string
	Raw        json.RawMessage
}

// ComputeMsgDigest 丰桥签名：msgData+timestamp+checkWord → URLEncode(UTF-8) → MD5 → Base64。
// 缺少 URLEncode 会返回「数字签名无效」。
func ComputeMsgDigest(msgData, timestamp, checkword string) string {
	raw := msgData + timestamp + checkword
	encoded := url.QueryEscape(raw)
	sum := md5.Sum([]byte(encoded))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (c *Client) CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResult, error) {
	if req.OrderID == "" {
		return nil, fmt.Errorf("orderId is required")
	}
	if req.PayMethod == 0 {
		req.PayMethod = 1
	}
	if req.ParcelQty <= 0 {
		req.ParcelQty = 1
	}
	if req.ExpressType == "" {
		req.ExpressType = "2"
	}
	cargos := normalizeCargoDetails(req.CargoDetails, req.CargoName)

	payload := map[string]interface{}{
		"language":        "zh-CN",
		"orderId":         req.OrderID,
		"cargoDetails":    cargos,
		"contactInfoList": []ContactInfo{req.Shipper, req.Receiver},
		"expressTypeId":   mustInt(req.ExpressType, 2),
		"payMethod":       req.PayMethod,
		"parcelQty":       req.ParcelQty,
	}
	if req.UseMonthly && req.CustID != "" {
		payload["monthlyCard"] = req.CustID
	}
	if remark := strings.TrimSpace(req.Remark); remark != "" {
		payload["remark"] = remark
	}
	if req.TotalWeight > 0 {
		payload["totalWeight"] = req.TotalWeight
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceCreateOrder, payload, &apiResp); err != nil {
		return nil, err
	}

	var result createOrderMsgData
	if err := decodeResultData(apiResp, &result); err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, fmt.Errorf("sf create order: %s", firstNonEmpty(result.ErrorMsg, result.ErrorCode, "unknown error"))
	}

	mailNo := firstNonEmpty(firstWaybillNo(result.MsgData.WaybillNoInfoList), result.MsgData.WaybillNo, result.MsgData.MailNo)
	orderID := firstNonEmpty(result.MsgData.OrderID, result.MsgData.OrderId, req.OrderID)
	if mailNo == "" {
		return nil, fmt.Errorf("sf create order: empty waybill number")
	}

	raw, _ := json.Marshal(result.MsgData)
	return &CreateOrderResult{
		MailNo:    mailNo,
		SFOrderID: orderID,
		Raw:       raw,
	}, nil
}

func (c *Client) CancelOrder(ctx context.Context, orderID, mailNo string, dealType int) error {
	if dealType == 0 {
		dealType = 2
	}
	payload := map[string]interface{}{
		"dealType": dealType,
	}
	if orderID != "" {
		payload["orderId"] = orderID
	}
	if mailNo != "" {
		payload["waybillNoInfoList"] = []map[string]string{{"waybillNo": mailNo}}
	}
	if orderID == "" && mailNo == "" {
		return fmt.Errorf("orderId or mailNo is required")
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceUpdateOrder, payload, &apiResp); err != nil {
		return err
	}
	var result genericMsgData
	if err := decodeResultData(apiResp, &result); err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("sf cancel order: %s", firstNonEmpty(result.ErrorMsg, result.ErrorCode, "unknown error"))
	}
	return nil
}

// CloudPrint 调用丰桥云打印转 PDF。
// templateCode 必须是丰桥控制台分配的完整模板编码（如 fm_76130_standard_XXXX），不是 partnerId。
func (c *Client) CloudPrint(ctx context.Context, mailNo, templateCode string) (*PrintResult, error) {
	if mailNo == "" {
		return nil, fmt.Errorf("mailNo is required")
	}
	templateCode = strings.TrimSpace(templateCode)
	if templateCode == "" {
		return nil, fmt.Errorf("templateCode is required: 请在物流账号配置丰桥云打印模板编码")
	}

	// sync=true 直接返回 PDF url+token，便于本机打开/打印（含顺丰打印组件或系统打印机）
	payload := map[string]interface{}{
		"templateCode": templateCode,
		"documents": []map[string]interface{}{
			{
				"masterWaybillNo": mailNo,
			},
		},
		"version":  "2.0",
		"fileType": "pdf",
		"sync":     true,
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceCloudPrint, payload, &apiResp); err != nil {
		log.Printf("sf cloud print failed for %s template=%s: %v", mailNo, templateCode, err)
		return nil, err
	}

	var result printMsgData
	if err := decodeResultData(apiResp, &result); err != nil {
		log.Printf("sf cloud print decode failed for %s template=%s: %v", mailNo, templateCode, err)
		return nil, err
	}
	if !result.Success {
		msg := firstNonEmpty(result.ErrorMsg, result.ErrorCode, "unknown")
		log.Printf("sf cloud print business error for %s template=%s: %s", mailNo, templateCode, msg)
		return nil, fmt.Errorf("sf cloud print: %s (templateCode=%s)", msg, templateCode)
	}

	labelURL, labelToken := extractPrintFile(result)
	if strings.TrimSpace(labelURL) == "" {
		return nil, fmt.Errorf("sf cloud print: 未返回 PDF url（templateCode=%s，请确认模板权限与规格）", templateCode)
	}
	labelData := firstNonEmpty(result.MsgData.File, result.MsgData.PrintData)
	raw, _ := json.Marshal(result.MsgData)
	return &PrintResult{
		LabelURL:   labelURL,
		LabelToken: labelToken,
		LabelData:  labelData,
		Raw:        raw,
	}, nil
}

// DownloadLabelPDF 使用云打印返回的 url+token 拉取 PDF 字节（本机打印用）。
func (c *Client) DownloadLabelPDF(ctx context.Context, fileURL, token string) ([]byte, error) {
	fileURL = strings.TrimSpace(fileURL)
	if fileURL == "" {
		return nil, fmt.Errorf("label url is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	if t := strings.TrimSpace(token); t != "" {
		req.Header.Set("X-Auth-token", t)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download label: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("download label http %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	return body, nil
}

type apiEnvelope struct {
	APIResultCode string          `json:"apiResultCode"`
	APIErrorMsg   string          `json:"apiErrorMsg"`
	APIResponseID string          `json:"apiResponseID"`
	APIResultData json.RawMessage `json:"apiResultData"`
}

type genericMsgData struct {
	Success   bool            `json:"success"`
	ErrorCode string          `json:"errorCode"`
	ErrorMsg  string          `json:"errorMsg"`
	MsgData   json.RawMessage `json:"msgData"`
}

type createOrderMsgData struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
	MsgData   struct {
		OrderID   string `json:"orderId"`
		OrderId   string `json:"orderID"`
		MailNo    string `json:"mailNo"`
		WaybillNo string `json:"waybillNo"`
		WaybillNoInfoList []struct {
			WaybillNo string `json:"waybillNo"`
		} `json:"waybillNoInfoList"`
	} `json:"msgData"`
}

type printFileItem struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type printMsgData struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
	MsgData   struct {
		URL       string          `json:"url"`
		FileURL   string          `json:"fileUrl"`
		File      string          `json:"file"`
		PrintData string          `json:"printData"`
		Files     []printFileItem `json:"files"`
		Obj       *struct {
			Files []printFileItem `json:"files"`
		} `json:"obj"`
	} `json:"msgData"`
}

func extractPrintFile(result printMsgData) (url, token string) {
	url = firstNonEmpty(result.MsgData.URL, result.MsgData.FileURL)
	if len(result.MsgData.Files) > 0 {
		f := result.MsgData.Files[0]
		url = firstNonEmpty(f.URL, url)
		token = strings.TrimSpace(f.Token)
	}
	if result.MsgData.Obj != nil && len(result.MsgData.Obj.Files) > 0 {
		f := result.MsgData.Obj.Files[0]
		url = firstNonEmpty(f.URL, url)
		if token == "" {
			token = strings.TrimSpace(f.Token)
		}
	}
	return url, token
}

func firstWaybillNo(items []struct {
	WaybillNo string `json:"waybillNo"`
}) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].WaybillNo
}

func decodeResultData(apiResp apiEnvelope, out interface{}) error {
	if apiResp.APIResultCode != "" && apiResp.APIResultCode != "A1000" {
		msg := firstNonEmpty(apiResp.APIErrorMsg, apiResp.APIResultCode)
		return fmt.Errorf("sf api: %s", msg)
	}
	if len(apiResp.APIResultData) == 0 {
		return fmt.Errorf("sf api: empty result data")
	}
	var raw string
	if err := json.Unmarshal(apiResp.APIResultData, &raw); err == nil && raw != "" {
		return json.Unmarshal([]byte(raw), out)
	}
	return json.Unmarshal(apiResp.APIResultData, out)
}

func (c *Client) call(ctx context.Context, serviceCode string, payload interface{}, out *apiEnvelope) error {
	msgDataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	msgData := string(msgDataBytes)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	requestID := uuid.NewString()
	msgDigest := ComputeMsgDigest(msgData, timestamp, c.checkword)

	form := url.Values{}
	form.Set("partnerID", c.partnerID)
	form.Set("requestID", requestID)
	form.Set("serviceCode", serviceCode)
	form.Set("timestamp", timestamp)
	form.Set("msgDigest", msgDigest)
	form.Set("msgData", msgData)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sf http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("sf http %d: %s", resp.StatusCode, truncate(string(body), 512))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("sf response decode: %w", err)
	}
	return nil
}

func normalizeCargoDetails(details []CargoDetail, cargoName string) []CargoDetail {
	out := make([]CargoDetail, 0, len(details))
	for _, d := range details {
		name := strings.TrimSpace(d.Name)
		if name == "" {
			continue
		}
		count := d.Count
		if count <= 0 {
			count = 1
		}
		out = append(out, CargoDetail{Name: name, Count: count})
	}
	if len(out) == 0 {
		name := strings.TrimSpace(cargoName)
		if name == "" {
			name = "商品"
		}
		out = append(out, CargoDetail{Name: name, Count: 1})
	}
	return out
}

func mustInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
