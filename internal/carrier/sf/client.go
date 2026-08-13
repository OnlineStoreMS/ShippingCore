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
	"github.com/tjfoc/gmsm/sm3"
)

const (
	ProdURL    = "https://bspgw.sf-express.com/std/service"
	SandboxURL = "https://sfapi-sbox.sf-express.com/std/service"

	ServiceCreateOrder      = "EXP_RECE_CREATE_ORDER"
	ServiceUpdateOrder      = "EXP_RECE_UPDATE_ORDER"
	ServiceCloudPrint       = "COM_RECE_CLOUD_PRINT_WAYBILLS"   // 云打印转 PDF
	ServiceCloudPrintParsed = "COM_RECE_CLOUD_PRINT_PARSEDDATA" // 云打印面单打印插件接口
	ServiceCheckPickupTime  = "EXP_EXCE_CHECK_PICKUP_TIME"      // 上门揽收时段查询
	ServiceQueryDeliverTm = "EXP_RECE_QUERY_DELIVERTM"  // 时效标准及价格查询（下单前）
	ServiceSearchPromitm  = "EXP_RECE_SEARCH_PROMITM"   // 预计派送时间查询（出单后）

	// SignModeStandard 丰桥「标准MD5」：URLEncode → MD5 → Base64
	SignModeStandard = "standard"
	// SignModeSimple 丰桥「简易MD5」：MD5 → Base64（不做 URLEncode）
	SignModeSimple = "simple"
	// SignModeSM3 丰桥「SM3」：URLEncode → SM3 → Base64
	SignModeSM3 = "sm3"

	// SandboxMonthlyCard 沙箱联调统一月结卡号（丰桥控制台说明）
	SandboxMonthlyCard = "7551234567"
)

type Client struct {
	partnerID string
	checkword string
	baseURL   string
	signMode  string
	sandbox   bool
	http      *http.Client
}

func NewClient(partnerID, checkword, env string) *Client {
	return NewClientWithSignMode(partnerID, checkword, env, SignModeSimple)
}

func NewClientWithSignMode(partnerID, checkword, env, signMode string) *Client {
	sandbox := true
	baseURL := SandboxURL
	if strings.EqualFold(env, "prod") || strings.EqualFold(env, "production") {
		baseURL = ProdURL
		sandbox = false
	}
	return &Client{
		partnerID: strings.TrimSpace(partnerID),
		checkword: strings.TrimSpace(checkword),
		baseURL:   baseURL,
		signMode:  NormalizeSignMode(signMode),
		sandbox:   sandbox,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NormalizeSignMode 归一化丰桥数字签名方式。
func NormalizeSignMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case SignModeStandard, "标准md5", "std", "md5":
		return SignModeStandard
	case SignModeSM3, "国密", "sm3-hmac":
		return SignModeSM3
	case SignModeSimple, "简易md5", "easy", "":
		return SignModeSimple
	default:
		return SignModeSimple
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
	TotalVolume  float64 // m³, optional
	LengthCM     float64
	WidthCM      float64
	HeightCM     float64
	IsDoCall     bool   // 预约上门揽收 isDocall=1
	SendStartTm  string // 要求上门取件开始时间 YYYY-MM-DD HH:mm:ss
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

// ParsedPrintResult 云打印插件接口（COM_RECE_CLOUD_PRINT_PARSEDDATA）返回。
type ParsedPrintResult struct {
	RequestID    string
	TemplateCode string
	FileType     string
	ClientCode   string
	FilesJSON    json.RawMessage // obj.files
	ObjJSON      json.RawMessage // 完整 obj，供 SCPPrint / 本地插件消费
	Raw          json.RawMessage
}

// ComputeMsgDigest 按 mode 计算 msgDigest；mode 为空时用简易 MD5。
func ComputeMsgDigest(msgData, timestamp, checkword, mode string) string {
	switch NormalizeSignMode(mode) {
	case SignModeStandard:
		return ComputeMsgDigestStandard(msgData, timestamp, checkword)
	case SignModeSM3:
		return ComputeMsgDigestSM3(msgData, timestamp, checkword)
	default:
		return ComputeMsgDigestSimple(msgData, timestamp, checkword)
	}
}

// ComputeMsgDigestSimple 简易MD5：msgData+timestamp+checkWord → MD5 → Base64。
func ComputeMsgDigestSimple(msgData, timestamp, checkword string) string {
	raw := msgData + timestamp + checkword
	sum := md5.Sum([]byte(raw))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ComputeMsgDigestStandard 标准MD5：msgData+timestamp+checkWord → URLEncode → MD5 → Base64。
func ComputeMsgDigestStandard(msgData, timestamp, checkword string) string {
	raw := msgData + timestamp + checkword
	encoded := url.QueryEscape(raw)
	sum := md5.Sum([]byte(encoded))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ComputeMsgDigestSM3 SM3：msgData+timestamp+checkWord → URLEncode → SM3 → Base64。
func ComputeMsgDigestSM3(msgData, timestamp, checkword string) string {
	raw := msgData + timestamp + checkword
	encoded := url.QueryEscape(raw)
	sum := sm3.Sm3Sum([]byte(encoded))
	return base64.StdEncoding.EncodeToString(sum)
}

func (c *Client) computeDigest(msgData, timestamp string) string {
	return ComputeMsgDigest(msgData, timestamp, c.checkword, c.signMode)
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
	if req.UseMonthly {
		custID := strings.TrimSpace(req.CustID)
		if c.sandbox {
			// 沙箱必须用统一测试月结卡，生产月结卡会在沙箱失败
			custID = SandboxMonthlyCard
		}
		if custID != "" {
			payload["monthlyCard"] = custID
		}
	}
	// 保留内部换行（订单备注多行）；只去掉首尾空白
	if remark := strings.Trim(req.Remark, " \t\r"); remark != "" {
		payload["remark"] = remark
		log.Printf("sf create order payload remark=%q", truncate(remark, 120))
	}
	if req.TotalWeight > 0 {
		payload["totalWeight"] = req.TotalWeight
	}
	if req.TotalVolume > 0 {
		payload["totalVolume"] = req.TotalVolume
	} else if req.LengthCM > 0 && req.WidthCM > 0 && req.HeightCM > 0 {
		// 长宽高(cm) → m³
		payload["totalVolume"] = req.LengthCM * req.WidthCM * req.HeightCM / 1_000_000
	}
	if req.IsDoCall {
		payload["isDocall"] = 1
	}
	if tm := strings.TrimSpace(req.SendStartTm); tm != "" {
		payload["sendStartTm"] = tm
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

// CheckPickupTimeRequest 上门揽收时段查询（EXP_EXCE_CHECK_PICKUP_TIME）。
type CheckPickupTimeRequest struct {
	Address     string // 寄件详细地址
	CityCode    string // 顺丰城市码，如 755/571
	AddressType int    // 1=寄件地址
	SendTime    string // YYYY-MM-DD HH:mm:ss，可选
}

// CheckPickupTimeResult 返回当地可揽收时间窗（HHMM）。
type CheckPickupTimeResult struct {
	Status          bool            `json:"status"`
	StartTm         string          `json:"startTm"` // 如 0800
	EndTm           string          `json:"endTm"`   // 如 2200
	ExceptionReason string          `json:"exceptionReason,omitempty"`
	Raw             json.RawMessage `json:"-"`
}

func (c *Client) CheckPickupTime(ctx context.Context, req CheckPickupTimeRequest) (*CheckPickupTimeResult, error) {
	addr := strings.TrimSpace(req.Address)
	if addr == "" {
		return nil, fmt.Errorf("address is required")
	}
	addrType := req.AddressType
	if addrType == 0 {
		addrType = 1
	}
	sendTime := strings.TrimSpace(req.SendTime)
	if sendTime == "" {
		sendTime = time.Now().Format("2006-01-02 15:04:05")
	}
	payload := map[string]interface{}{
		"address":     addr,
		"addressType": addrType,
		"sendTime":    sendTime,
		"sysCode":     "bsp",
		"version":     "V1.1",
	}
	if code := strings.TrimSpace(req.CityCode); code != "" {
		payload["cityCode"] = code
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceCheckPickupTime, payload, &apiResp); err != nil {
		return nil, err
	}
	var wrap genericMsgData
	if err := decodeResultData(apiResp, &wrap); err != nil {
		return nil, err
	}
	if !wrap.Success {
		return nil, fmt.Errorf("sf check pickup time: %s", firstNonEmpty(wrap.ErrorMsg, wrap.ErrorCode, "unknown error"))
	}
	out := &CheckPickupTimeResult{Raw: wrap.MsgData}
	if len(wrap.MsgData) == 0 || string(wrap.MsgData) == "false" || string(wrap.MsgData) == "null" {
		return out, nil
	}
	var win struct {
		Status          bool   `json:"status"`
		StartTm         string `json:"startTm"`
		EndTm           string `json:"endTm"`
		ExceptionReason string `json:"exceptionReason"`
	}
	if err := json.Unmarshal(wrap.MsgData, &win); err != nil {
		return out, nil
	}
	out.Status = win.Status
	out.StartTm = strings.TrimSpace(win.StartTm)
	out.EndTm = strings.TrimSpace(win.EndTm)
	out.ExceptionReason = strings.TrimSpace(win.ExceptionReason)
	return out, nil
}

// QueryDeliverTmRequest 时效标准及价格查询（EXP_RECE_QUERY_DELIVERTM）。
type QueryDeliverTmRequest struct {
	SrcProvince  string
	SrcCity      string
	SrcDistrict  string
	SrcAddress   string
	DestProvince string
	DestCity     string
	DestDistrict string
	DestAddress  string
	WeightKG     float64
	ConsignedTime string // YYYY-MM-DD HH:mm:ss，可选
	MonthlyCard  string  // 月结卡号，可选
	BusinessType string  // 产品编码过滤，可选；空则尽量返回可售产品
}

// DeliverTmItem 单个物流产品的时效/预估价格。
type DeliverTmItem struct {
	BusinessType     string  `json:"businessType"`
	BusinessTypeDesc string  `json:"businessTypeDesc"`
	DeliverTime      string  `json:"deliverTime"` // 可能为 "开始,结束"
	Fee              float64 `json:"fee"`
	CloseTime        string  `json:"closeTime,omitempty"`
}

type QueryDeliverTmResult struct {
	Items []DeliverTmItem  `json:"items"`
	Raw   json.RawMessage  `json:"-"`
}

func (c *Client) QueryDeliverTm(ctx context.Context, req QueryDeliverTmRequest) (*QueryDeliverTmResult, error) {
	srcAddr := strings.TrimSpace(req.SrcAddress)
	destAddr := strings.TrimSpace(req.DestAddress)
	if strings.TrimSpace(req.SrcProvince) == "" || strings.TrimSpace(req.SrcCity) == "" || srcAddr == "" {
		return nil, fmt.Errorf("src address is required")
	}
	if strings.TrimSpace(req.DestProvince) == "" || strings.TrimSpace(req.DestCity) == "" || destAddr == "" {
		return nil, fmt.Errorf("dest address is required")
	}
	weight := req.WeightKG
	if weight <= 0 {
		weight = 1
	}
	consigned := strings.TrimSpace(req.ConsignedTime)
	if consigned == "" {
		consigned = time.Now().Format("2006-01-02 15:04:05")
	}
	payload := map[string]interface{}{
		"language":      "zh-CN",
		"searchPrice":   "1",
		"weight":        weight,
		"consignedTime": consigned,
		"srcAddress": map[string]string{
			"province": strings.TrimSpace(req.SrcProvince),
			"city":     strings.TrimSpace(req.SrcCity),
			"district": strings.TrimSpace(req.SrcDistrict),
			"address":  srcAddr,
		},
		"destAddress": map[string]string{
			"province": strings.TrimSpace(req.DestProvince),
			"city":     strings.TrimSpace(req.DestCity),
			"district": strings.TrimSpace(req.DestDistrict),
			"address":  destAddr,
		},
	}
	if bt := strings.TrimSpace(req.BusinessType); bt != "" {
		payload["businessType"] = bt
	}
	if card := strings.TrimSpace(req.MonthlyCard); card != "" {
		if c.sandbox {
			card = SandboxMonthlyCard
		}
		payload["monthlyCard"] = card
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceQueryDeliverTm, payload, &apiResp); err != nil {
		return nil, err
	}
	var wrap genericMsgData
	if err := decodeResultData(apiResp, &wrap); err != nil {
		return nil, err
	}
	if !wrap.Success {
		return nil, fmt.Errorf("sf query deliver tm: %s", firstNonEmpty(wrap.ErrorMsg, wrap.ErrorCode, "unknown error"))
	}
	out := &QueryDeliverTmResult{Raw: wrap.MsgData, Items: nil}
	if len(wrap.MsgData) == 0 || string(wrap.MsgData) == "null" {
		return out, nil
	}
	var msg struct {
		DeliverTmDto []struct {
			BusinessType     interface{} `json:"businessType"`
			BusinessTypeDesc string      `json:"businessTypeDesc"`
			DeliverTime      string      `json:"deliverTime"`
			Fee              interface{} `json:"fee"`
			CloseTime        interface{} `json:"closeTime"`
			SearchPrice      interface{} `json:"searchPrice"`
		} `json:"deliverTmDto"`
	}
	if err := json.Unmarshal(wrap.MsgData, &msg); err != nil {
		return out, nil
	}
	for _, d := range msg.DeliverTmDto {
		item := DeliverTmItem{
			BusinessType:     fmt.Sprint(d.BusinessType),
			BusinessTypeDesc: strings.TrimSpace(d.BusinessTypeDesc),
			DeliverTime:      strings.TrimSpace(d.DeliverTime),
			Fee:              toFloat64(d.Fee),
			CloseTime:        strings.TrimSpace(fmt.Sprint(d.CloseTime)),
		}
		if item.CloseTime == "<nil>" || item.CloseTime == "null" {
			item.CloseTime = ""
		}
		if item.BusinessType == "" || item.BusinessType == "<nil>" {
			continue
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func toFloat64(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	default:
		return 0
	}
}

// SearchPromitmRequest 预计派送时间查询（EXP_RECE_SEARCH_PROMITM）。
// checkType=1：电话号码校验，checkNos 传电话；checkType=2：月结卡号校验，checkNos 传月结卡号。
type SearchPromitmRequest struct {
	SearchNo  string   // 运单号
	CheckType int      // 1=电话号码 2=月结卡号
	CheckNos  []string // 校验值列表
}

type SearchPromitmResult struct {
	PromiseTm string          `json:"promiseTm"`
	SearchNo  string          `json:"searchNo,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

func (c *Client) SearchPromitm(ctx context.Context, req SearchPromitmRequest) (*SearchPromitmResult, error) {
	searchNo := strings.TrimSpace(req.SearchNo)
	if searchNo == "" {
		return nil, fmt.Errorf("searchNo is required")
	}
	checkType := req.CheckType
	if checkType == 0 {
		checkType = 1
	}
	checkNos := make([]string, 0, len(req.CheckNos))
	for _, n := range req.CheckNos {
		n = strings.TrimSpace(n)
		if n != "" {
			checkNos = append(checkNos, n)
		}
	}
	if len(checkNos) == 0 {
		return nil, fmt.Errorf("checkNos is required")
	}
	payload := map[string]interface{}{
		"searchNo":  searchNo,
		"checkType": checkType,
		"checkNos":  checkNos,
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceSearchPromitm, payload, &apiResp); err != nil {
		return nil, err
	}
	var wrap genericMsgData
	if err := decodeResultData(apiResp, &wrap); err != nil {
		return nil, err
	}
	if !wrap.Success {
		return nil, fmt.Errorf("sf search promitm: %s", firstNonEmpty(wrap.ErrorMsg, wrap.ErrorCode, "unknown error"))
	}
	out := &SearchPromitmResult{SearchNo: searchNo, Raw: wrap.MsgData}
	if len(wrap.MsgData) == 0 || string(wrap.MsgData) == "null" {
		return out, nil
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(wrap.MsgData, &msg); err != nil {
		return out, nil
	}
	out.PromiseTm = firstMapStr(msg, "promiseTm", "promiseTime", "deliverTm", "promisedTm", "promise_tm")
	if sn := firstMapStr(msg, "searchNo", "waybillNo", "mailNo"); sn != "" {
		out.SearchNo = sn
	}
	return out, nil
}

func firstMapStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			s := strings.TrimSpace(fmt.Sprint(v))
			if s != "" && s != "<nil>" && s != "null" {
				return s
			}
		}
	}
	return ""
}

// PrintDocOptions 云打印 documents 扩展（自定义区备注等）。
type PrintDocOptions struct {
	Remark             string // 映射模板自定义区「备注」
	CustomTemplateCode string // 已发布的自定义模板编码（与标准 templateCode 规格一致）
}

func buildPrintDocument(mailNo string, opt *PrintDocOptions) map[string]interface{} {
	doc := map[string]interface{}{
		"masterWaybillNo": mailNo,
	}
	if opt == nil {
		return doc
	}
	if r := strings.TrimSpace(opt.Remark); r != "" {
		// 自定义区「变量字段」字段名须与下列 key 之一一致（推荐 remark）
		doc["remark"] = r
		doc["cargoDesc"] = r
		doc["goods"] = r
		doc["product"] = r
		doc["备注"] = r
	}
	return doc
}

func buildPrintExtJSON(opt *PrintDocOptions) map[string]interface{} {
	if opt == nil {
		return nil
	}
	r := strings.TrimSpace(opt.Remark)
	if r == "" {
		return nil
	}
	return map[string]interface{}{
		"remark":    r,
		"cargoDesc": r,
		"goods":     r,
		"product":   r,
		"备注":       r,
	}
}

// CloudPrintParsedData 调用云打印面单打印插件接口 COM_RECE_CLOUD_PRINT_PARSEDDATA。
// 返回供顺丰云打印组件（SCPPrint）消费的 JSON 排版数据（非 PDF）。
func (c *Client) CloudPrintParsedData(ctx context.Context, mailNo, templateCode string, opt *PrintDocOptions) (*ParsedPrintResult, error) {
	if mailNo == "" {
		return nil, fmt.Errorf("mailNo is required")
	}
	templateCode = strings.TrimSpace(templateCode)
	if templateCode == "" {
		return nil, fmt.Errorf("templateCode is required: 请在物流账号配置丰桥云打印模板编码")
	}

	doc := buildPrintDocument(mailNo, opt)
	payload := map[string]interface{}{
		"templateCode": templateCode,
		"documents":    []map[string]interface{}{doc},
		"version":      "2.0",
	}
	customTpl := ""
	if opt != nil {
		customTpl = strings.TrimSpace(opt.CustomTemplateCode)
		if customTpl != "" {
			payload["customTemplateCode"] = customTpl
		}
	}
	if ext := buildPrintExtJSON(opt); ext != nil {
		// 丰桥部分网关要求 extJson 为 JSON 字符串
		if b, err := json.Marshal(ext); err == nil {
			payload["extJson"] = string(b)
		} else {
			payload["extJson"] = ext
		}
	}
	if r, _ := doc["remark"].(string); r != "" {
		log.Printf("sf cloud print parsed %s template=%s custom=%s remark=%q", mailNo, templateCode, customTpl, truncate(r, 120))
	} else {
		log.Printf("sf cloud print parsed %s template=%s custom=%s remark=EMPTY", mailNo, templateCode, customTpl)
	}

	var apiResp apiEnvelope
	if err := c.call(ctx, ServiceCloudPrintParsed, payload, &apiResp); err != nil {
		log.Printf("sf cloud print parsed failed for %s template=%s: %v", mailNo, templateCode, err)
		return nil, err
	}

	var result printMsgData
	if err := decodeResultData(apiResp, &result); err != nil {
		return nil, err
	}
	if !result.Success {
		msg := firstNonEmpty(result.ErrorMsg, result.ErrorMessage, result.ErrorCode, "unknown")
		return nil, fmt.Errorf("sf cloud print parsed: %s (templateCode=%s)", msg, templateCode)
	}
	body := result.printPayload()
	if body == nil || len(body.Files) == 0 {
		// files 可能是 contents 结构，用通用解析兜底
		var envelope struct {
			Success   bool            `json:"success"`
			RequestID string          `json:"requestId"`
			Obj       json.RawMessage `json:"obj"`
		}
		rawAll, _ := json.Marshal(result)
		_ = json.Unmarshal(rawAll, &envelope)
		if len(envelope.Obj) == 0 {
			return nil, fmt.Errorf("sf cloud print parsed: 未返回插件面单数据（templateCode=%s）", templateCode)
		}
		var objMeta struct {
			ClientCode   string          `json:"clientCode"`
			FileType     string          `json:"fileType"`
			TemplateCode string          `json:"templateCode"`
			Files        json.RawMessage `json:"files"`
		}
		_ = json.Unmarshal(envelope.Obj, &objMeta)
		if len(objMeta.Files) == 0 || string(objMeta.Files) == "null" {
			return nil, fmt.Errorf("sf cloud print parsed: 未返回 files（templateCode=%s）", templateCode)
		}
		return &ParsedPrintResult{
			RequestID:    envelope.RequestID,
			TemplateCode: firstNonEmpty(objMeta.TemplateCode, templateCode),
			FileType:     firstNonEmpty(objMeta.FileType, "json"),
			ClientCode:   objMeta.ClientCode,
			FilesJSON:    objMeta.Files,
			ObjJSON:      envelope.Obj,
			Raw:          rawAll,
		}, nil
	}

	raw, _ := json.Marshal(result)
	objJSON, _ := json.Marshal(body)
	filesJSON, _ := json.Marshal(body.Files)
	return &ParsedPrintResult{
		RequestID:    result.RequestID,
		TemplateCode: templateCode,
		FileType:     "json",
		FilesJSON:    filesJSON,
		ObjJSON:      objJSON,
		Raw:          raw,
	}, nil
}

// CloudPrint 调用丰桥云打印转 PDF。
// templateCode 必须是丰桥控制台分配的完整模板编码（如 fm_76130_standard_XXXX），不是 partnerId。
func (c *Client) CloudPrint(ctx context.Context, mailNo, templateCode string, opt *PrintDocOptions) (*PrintResult, error) {
	if mailNo == "" {
		return nil, fmt.Errorf("mailNo is required")
	}
	templateCode = strings.TrimSpace(templateCode)
	if templateCode == "" {
		return nil, fmt.Errorf("templateCode is required: 请在物流账号配置丰桥云打印模板编码")
	}

	doc := buildPrintDocument(mailNo, opt)
	// sync=true 直接返回 PDF url+token，便于本机打开/打印（含顺丰打印组件或系统打印机）
	payload := map[string]interface{}{
		"templateCode": templateCode,
		"documents":    []map[string]interface{}{doc},
		"version":      "2.0",
		"fileType":     "pdf",
		"sync":         true,
	}
	customTpl := ""
	if opt != nil {
		customTpl = strings.TrimSpace(opt.CustomTemplateCode)
		if customTpl != "" {
			payload["customTemplateCode"] = customTpl
		}
	}
	if ext := buildPrintExtJSON(opt); ext != nil {
		if b, err := json.Marshal(ext); err == nil {
			payload["extJson"] = string(b)
		} else {
			payload["extJson"] = ext
		}
	}
	if r, _ := doc["remark"].(string); r != "" {
		log.Printf("sf cloud print %s template=%s custom=%s remark=%q", mailNo, templateCode, customTpl, truncate(r, 120))
	} else {
		log.Printf("sf cloud print %s template=%s custom=%s remark=EMPTY", mailNo, templateCode, customTpl)
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
		msg := firstNonEmpty(result.ErrorMsg, result.ErrorMessage, result.ErrorCode, "unknown")
		log.Printf("sf cloud print business error for %s template=%s: %s", mailNo, templateCode, msg)
		return nil, fmt.Errorf("sf cloud print: %s (templateCode=%s)", msg, templateCode)
	}

	labelURL, labelToken := extractPrintFile(result)
	if strings.TrimSpace(labelURL) == "" {
		rawFail, _ := json.Marshal(result)
		log.Printf("sf cloud print empty url for %s template=%s raw=%s", mailNo, templateCode, truncate(string(rawFail), 512))
		return nil, fmt.Errorf("sf cloud print: 未返回 PDF url（templateCode=%s，请确认模板权限与规格）", templateCode)
	}
	body := result.printPayload()
	labelData := ""
	if body != nil {
		labelData = firstNonEmpty(body.File, body.PrintData)
	}
	raw, _ := json.Marshal(result)
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
		OrderID           string `json:"orderId"`
		OrderId           string `json:"orderID"`
		MailNo            string `json:"mailNo"`
		WaybillNo         string `json:"waybillNo"`
		WaybillNoInfoList []struct {
			WaybillNo string `json:"waybillNo"`
		} `json:"waybillNoInfoList"`
	} `json:"msgData"`
}

type printFileItem struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// printPayload 丰桥云打印业务体：同步成功时 files 多在顶层 obj 下（非 msgData）。
type printPayload struct {
	URL       string          `json:"url"`
	FileURL   string          `json:"fileUrl"`
	File      string          `json:"file"`
	PrintData string          `json:"printData"`
	Files     []printFileItem `json:"files"`
}

type printMsgData struct {
	Success      bool          `json:"success"`
	ErrorCode    string        `json:"errorCode"`
	ErrorMsg     string        `json:"errorMsg"`
	ErrorMessage string        `json:"errorMessage"`
	RequestID    string        `json:"requestId"`
	Obj          *printPayload `json:"obj"`
	MsgData      *printPayload `json:"msgData"`
}

func (r printMsgData) printPayload() *printPayload {
	if r.Obj != nil && (len(r.Obj.Files) > 0 || strings.TrimSpace(r.Obj.URL) != "" || strings.TrimSpace(r.Obj.FileURL) != "") {
		return r.Obj
	}
	if r.MsgData != nil {
		return r.MsgData
	}
	return r.Obj
}

func extractPrintFile(result printMsgData) (fileURL, token string) {
	p := result.printPayload()
	if p == nil {
		return "", ""
	}
	fileURL = firstNonEmpty(p.URL, p.FileURL)
	if len(p.Files) > 0 {
		f := p.Files[0]
		fileURL = firstNonEmpty(f.URL, fileURL)
		token = strings.TrimSpace(f.Token)
	}
	return fileURL, token
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
		if strings.Contains(msg, "数字签名") {
			return fmt.Errorf("sf api: %s（请核对顾客编码/校验码/环境；丰桥若为「简易MD5」勿用标准 URLEncode 签名）", msg)
		}
		return fmt.Errorf("sf api: %s", msg)
	}
	if len(apiResp.APIResultData) == 0 || string(apiResp.APIResultData) == "null" || string(apiResp.APIResultData) == `""` {
		return fmt.Errorf("sf api: empty result data (apiResultData=%s)", truncate(string(apiResp.APIResultData), 64))
	}
	data := apiResp.APIResultData
	// 丰桥偶发多层 JSON 字符串包裹
	for i := 0; i < 5; i++ {
		var asStr string
		if err := json.Unmarshal(data, &asStr); err == nil {
			asStr = strings.TrimSpace(asStr)
			if asStr == "" {
				return fmt.Errorf("sf api: empty result data after unwrap")
			}
			data = json.RawMessage(asStr)
			continue
		}
		break
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("sf result decode: %w (raw=%s)", err, truncate(string(data), 256))
	}
	return nil
}

func (c *Client) call(ctx context.Context, serviceCode string, payload interface{}, out *apiEnvelope) error {
	msgDataBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	msgData := string(msgDataBytes)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	requestID := uuid.NewString()
	msgDigest := c.computeDigest(msgData, timestamp)

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
