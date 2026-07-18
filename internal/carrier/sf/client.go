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
		partnerID: partnerID,
		checkword: checkword,
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
	Name string `json:"name"`
}

type CreateOrderRequest struct {
	OrderID       string
	UseMonthly    bool
	CustID        string
	ExpressType   string
	PayMethod     int
	ParcelQty     int
	CargoName     string
	Shipper       ContactInfo
	Receiver      ContactInfo
}

type CreateOrderResult struct {
	MailNo    string
	SFOrderID string
	Raw       json.RawMessage
}

type PrintResult struct {
	LabelURL  string
	LabelData string
	Raw       json.RawMessage
}

func ComputeMsgDigest(msgData, timestamp, checkword string) string {
	raw := msgData + timestamp + checkword
	sum := md5.Sum([]byte(raw))
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
	if req.CargoName == "" {
		req.CargoName = "商品"
	}

	payload := map[string]interface{}{
		"language":        "zh-CN",
		"orderId":         req.OrderID,
		"cargoDetails":    []CargoDetail{{Name: req.CargoName}},
		"contactInfoList": []ContactInfo{req.Shipper, req.Receiver},
		"expressTypeId":   mustInt(req.ExpressType, 2),
		"payMethod":       req.PayMethod,
		"parcelQty":       req.ParcelQty,
	}
	if req.UseMonthly && req.CustID != "" {
		payload["monthlyCard"] = req.CustID
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

func (c *Client) CloudPrint(ctx context.Context, mailNo, partnerID string) (*PrintResult, error) {
	if mailNo == "" {
		return nil, fmt.Errorf("mailNo is required")
	}
	if partnerID == "" {
		partnerID = c.partnerID
	}

	payload := map[string]interface{}{
		"templateCode": "fm_76130_standard_" + partnerID,
		"documents": []map[string]interface{}{
			{
				"masterWaybillNo": mailNo,
			},
		},
		"version": "2.0",
	}

	var apiResp apiEnvelope
	err := c.call(ctx, ServiceCloudPrint, payload, &apiResp)
	if err != nil {
		log.Printf("sf cloud print failed for %s: %v", mailNo, err)
		return &PrintResult{
			LabelURL:  placeholderLabelURL(mailNo),
			LabelData: fmt.Sprintf(`{"mailNo":"%s","note":"cloud print unavailable, placeholder label"}`, mailNo),
		}, nil
	}

	var result printMsgData
	if err := decodeResultData(apiResp, &result); err != nil {
		log.Printf("sf cloud print decode failed for %s: %v", mailNo, err)
		return &PrintResult{
			LabelURL:  placeholderLabelURL(mailNo),
			LabelData: fmt.Sprintf(`{"mailNo":"%s","note":"cloud print decode failed"}`, mailNo),
		}, nil
	}
	if !result.Success {
		log.Printf("sf cloud print business error for %s: %s", mailNo, result.ErrorMsg)
		return &PrintResult{
			LabelURL:  placeholderLabelURL(mailNo),
			LabelData: fmt.Sprintf(`{"mailNo":"%s","error":"%s"}`, mailNo, result.ErrorMsg),
		}, nil
	}

	labelURL := firstNonEmpty(result.MsgData.URL, result.MsgData.FileURL)
	labelData := firstNonEmpty(result.MsgData.File, result.MsgData.PrintData)
	if labelURL == "" {
		labelURL = placeholderLabelURL(mailNo)
	}
	raw, _ := json.Marshal(result.MsgData)
	return &PrintResult{
		LabelURL:  labelURL,
		LabelData: labelData,
		Raw:       raw,
	}, nil
}

func placeholderLabelURL(mailNo string) string {
	return "sf://waybill/" + mailNo
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

type printMsgData struct {
	Success   bool   `json:"success"`
	ErrorCode string `json:"errorCode"`
	ErrorMsg  string `json:"errorMsg"`
	MsgData   struct {
		URL       string `json:"url"`
		FileURL   string `json:"fileUrl"`
		File      string `json:"file"`
		PrintData string `json:"printData"`
	} `json:"msgData"`
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
