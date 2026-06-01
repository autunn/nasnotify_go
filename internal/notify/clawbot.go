package notify

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nasnotify-go/internal/config"
	"nasnotify-go/internal/notifycard"
	"nasnotify-go/internal/wechatgateway"
)

const (
	defaultClawBotPushTitle = "NAS 通知中心"
)

var clawBotBindingMu sync.Mutex

type ClawBotQRCode struct {
	URL    string `json:"url"`
	QRCode string `json:"qrcode"`
}

type ClawBotBindingInfo struct {
	CreateTime       string `json:"createTime"`
	HaveContextToken int    `json:"haveContextToken"`
}

type ClawBotMessage struct {
	ID        string `json:"id"`
	Type      int    `json:"type"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type ClawBotStatus struct {
	Configured       bool           `json:"configured"`
	OpenAPIReady     bool           `json:"open_api_ready"`
	EntryBound       bool           `json:"entry_bound"`
	Bound            bool           `json:"bound"`
	Activated        bool           `json:"activated"`
	NeedVerifyCode   bool           `json:"need_verify_code"`
	BindingCode      string         `json:"binding_code,omitempty"`
	BindTime         string         `json:"bind_time,omitempty"`
	EntryBindTime    string         `json:"entry_bind_time,omitempty"`
	QRCode           *ClawBotQRCode `json:"qrcode,omitempty"`
	Tips             []string       `json:"tips"`
	LastError        string         `json:"last_error,omitempty"`
	HaveContextToken bool           `json:"have_context_token"`
}

type gatewayStatusResponse struct {
	GatewayOnline  bool `json:"gateway_online"`
	SessionActive  bool `json:"session_active"`
	BoundPeerReady bool `json:"bound_peer_ready"`
	Login          struct {
		QRCodeURL      string   `json:"qrcode_url"`
		QRCodeText     string   `json:"qrcode_text"`
		EnteredAt      string   `json:"entered_at"`
		Status         string   `json:"status"`
		NeedVerifyCode bool     `json:"need_verify_code"`
		Tips           []string `json:"tips"`
	} `json:"login"`
	LastError string `json:"last_error"`
}

type gatewayMessage struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}
type gatewayEnvelope struct {
	Ret     int             `json:"ret"`
	ErrCode int             `json:"errcode"`
	ErrMsg  string          `json:"errmsg"`
	Data    json.RawMessage `json:"data"`
}

func ClawBotPush(content string) error {
	if !ClawBotConfigured() {
		return fmt.Errorf("wechat gateway is not configured")
	}

	text := strings.TrimSpace(content)
	if text == "" {
		return nil
	}

	if useEmbeddedGateway() {
		if err := wechatgateway.SendTextMessage(defaultClawBotPushTitle, text); err != nil {
			log.Printf("embedded wechat gateway send failed: %v", err)
			return err
		}
		return nil
	}

	payload := map[string]string{
		"title": defaultClawBotPushTitle,
		"text":  text,
	}
	if err := gatewayJSONRequest(http.MethodPost, "/api/v1/messages/send", nil, payload, nil); err != nil {
		log.Printf("external wechat gateway send failed: %v", err)
		return err
	}
	return nil
}

func ClawBotPushCard(card notifycard.Card, fallbackText string) error {
	if !ClawBotConfigured() {
		return fmt.Errorf("wechat gateway is not configured")
	}

	fallbackText = strings.TrimSpace(fallbackText)
	if fallbackText == "" {
		fallbackText = strings.TrimSpace(card.Title)
		if summary := strings.TrimSpace(card.Summary); summary != "" {
			if fallbackText != "" {
				fallbackText += "\n\n"
			}
			fallbackText += summary
		}
	}

	if !useEmbeddedGateway() {
		if fallbackText == "" {
			return nil
		}
		return ClawBotPush(fallbackText)
	}

	pngData, err := notifycard.RenderPNG(card)
	if err != nil {
		log.Printf("render notify card failed: %v", err)
		if fallbackText != "" {
			return ClawBotPush(fallbackText)
		}
		return err
	}

	if err := wechatgateway.SendImageMessage(pngData); err != nil {
		log.Printf("embedded wechat gateway image send failed: %v", err)
		if fallbackText != "" {
			if fallbackErr := ClawBotPush(fallbackText); fallbackErr != nil {
				return fmt.Errorf("send image message failed: %w; fallback text failed: %v", err, fallbackErr)
			}
			return nil
		}
		return err
	}
	return nil
}

func ClawBotMenuText() string {
	return buildClawBotMenuText()
}

func ClawBotQueryDeckText() string {
	var builder strings.Builder
	builder.WriteString("查询菜单\n")
	builder.WriteString("`查询菜单` / `查询` / `query` / `query deck`\n\n")
	builder.WriteString("`状态` / `system`         CPU / 内存 / 网络概览\n")
	builder.WriteString("`通知` / `notice` / `notify` 系统通知流\n")
	builder.WriteString("`存储` / `storage`        存储卷容量与使用率\n")
	builder.WriteString("`Docker`                 容器运行概览\n")
	builder.WriteString("`进程` / `ps`             资源占用 TOP 5\n")
	builder.WriteString("`备份` / `backup`         备份任务状态\n")
	builder.WriteString("`电源` / `power`          电源与休眠配置\n")
	builder.WriteString("`UPS`                    UPS 供电状态\n")
	builder.WriteString("`测试` / `test`           推送链路测试\n")
	builder.WriteString("`菜单` / `menu`           返回总览菜单")
	return builder.String()
}

func ClawBotControlDeckText() string {
	var builder strings.Builder
	builder.WriteString("控制菜单\n")
	builder.WriteString("`控制菜单` / `控制` / `control` / `control deck`\n\n")
	builder.WriteString("风扇模式：`风扇1` 静音，`风扇2` 标准，`风扇3` 全速\n")
	builder.WriteString("英文别名：`fan1` / `fan2` / `fan3`\n")
	builder.WriteString("CPU 模式：`CPU0` 高性能，`CPU1` 均衡，`CPU2` 节能\n")
	builder.WriteString("英文别名：`cpu0` / `cpu1` / `cpu2`\n")
	builder.WriteString("\n直接发送上面的具体指令即可执行控制，回复 `菜单` 可返回总览。")
	return builder.String()
}

func buildClawBotMenuText() string {
	var builder strings.Builder
	builder.WriteString("NAS 通知中心\n")
	builder.WriteString("模式：微信控制台\n")
	builder.WriteString(fmt.Sprintf("时间：%s\n", time.Now().Format("01-02 15:04")))
	builder.WriteString("────────────────\n")

	builder.WriteString("\n入口命令\n")
	builder.WriteString("`菜单` / `help` / `menu`        返回总览菜单\n")
	builder.WriteString("`查询菜单` / `查询` / `query` / `query deck`\n")
	builder.WriteString("                              返回查询指令表\n")
	builder.WriteString("`控制菜单` / `控制` / `control` / `control deck`\n")
	builder.WriteString("                              返回控制指令表\n")

	builder.WriteString("\n查询菜单\n")
	builder.WriteString("`状态` / `system`           CPU / 内存 / 网络概览\n")
	builder.WriteString("`通知` / `notice` / `notify` 系统通知流\n")
	builder.WriteString("`存储` / `storage`          存储卷容量与使用率\n")
	builder.WriteString("`Docker`                   容器运行概览\n")
	builder.WriteString("`进程` / `ps`               资源占用 TOP 5\n")
	builder.WriteString("`备份` / `backup`           备份任务状态\n")
	builder.WriteString("`电源` / `power`            电源与休眠配置\n")
	builder.WriteString("`UPS`                      UPS 供电状态\n")
	builder.WriteString("`测试` / `test`             推送链路测试\n")

	builder.WriteString("\n控制菜单\n")
	builder.WriteString("`风扇1` / `fan1`            静音\n")
	builder.WriteString("`风扇2` / `fan2`            标准\n")
	builder.WriteString("`风扇3` / `fan3`            全速\n")
	builder.WriteString("`CPU0` / `cpu0`            高性能\n")
	builder.WriteString("`CPU1` / `cpu1`            均衡\n")
	builder.WriteString("`CPU2` / `cpu2`            节能\n")

	builder.WriteString("\n指令示例\n")
	builder.WriteString("查询状态  `状态`\n")
	builder.WriteString("风扇控制  `风扇2`\n")
	builder.WriteString("性能模式  `CPU1`\n")
	return strings.TrimSpace(builder.String())
}

func ClawBotConfigured() bool {
	snapshot := config.GetConfigSnapshot()
	return useEmbeddedGatewayURL(snapshot.WechatGatewayURL) || strings.TrimSpace(snapshot.WechatGatewayURL) != ""
}

func ClawBotBound() bool {
	config.CfgMu.RLock()
	defer config.CfgMu.RUnlock()
	return config.Config.WechatBound
}

func EnsureClawBotBindingCode() (string, error) {
	clawBotBindingMu.Lock()
	defer clawBotBindingMu.Unlock()

	snapshot := config.GetConfigSnapshot()
	if strings.TrimSpace(snapshot.WechatBindingCode) != "" {
		return strings.ToUpper(strings.TrimSpace(snapshot.WechatBindingCode)), nil
	}

	snapshot.WechatBindingCode = generateClawBotBindingCode()
	if err := config.SaveConfig(snapshot); err != nil {
		return "", err
	}
	return snapshot.WechatBindingCode, nil
}

func RotateClawBotBindingCode() (string, error) {
	clawBotBindingMu.Lock()
	defer clawBotBindingMu.Unlock()

	snapshot := config.GetConfigSnapshot()
	snapshot.WechatBindingCode = generateClawBotBindingCode()
	snapshot.WechatBound = false
	snapshot.WechatBoundAt = ""
	if err := config.SaveConfig(snapshot); err != nil {
		return "", err
	}
	return snapshot.WechatBindingCode, nil
}

func MatchClawBotBindingMessage(text string) bool {
	normalizedText := normalizeBindingText(text)
	if normalizedText == "" {
		return false
	}

	// 在单个锁区间内完成“获取/生成绑定码 → 匹配 → 标记已绑定”，
	// 避免先调用 EnsureClawBotBindingCode()（内部加锁）后再次加锁导致死锁。
	clawBotBindingMu.Lock()
	defer clawBotBindingMu.Unlock()

	snapshot := config.GetConfigSnapshot()
	code := normalizeBindingText(snapshot.WechatBindingCode)
	if code == "" {
		// 内联生成逻辑，等价于 EnsureClawBotBindingCode() 但不重复加锁
		snapshot.WechatBindingCode = generateClawBotBindingCode()
		if err := config.SaveConfig(snapshot); err != nil {
			log.Printf("ensure wechat binding code failed: %v", err)
			return false
		}
		code = normalizeBindingText(snapshot.WechatBindingCode)
	}

	if !strings.Contains(normalizedText, code) {
		return false
	}

	if snapshot.WechatBound {
		return true
	}

	snapshot.WechatBound = true
	snapshot.WechatBoundAt = time.Now().Format("2006-01-02 15:04:05")
	if err := config.SaveConfig(snapshot); err != nil {
		log.Printf("save wechat binding state failed: %v", err)
		return false
	}

	if useEmbeddedGateway() {
		wechatgateway.BindLatestPeer()
	}

	ClawBotPushCard(
		ClawBotMenuCard("微信入口绑定成功，现在可以直接在聊天里查看黑金菜单并发送命令。"),
		"微信入口绑定成功。\n\n现在可以发送“菜单”查看可用命令。",
	)
	return true
}

func GetClawBotStatus() ClawBotStatus {
	snapshot := config.GetConfigSnapshot()
	status := ClawBotStatus{
		Configured: true,
		Bound:      snapshot.WechatBound,
		BindTime:   strings.TrimSpace(snapshot.WechatBoundAt),
		Tips: []string{
			"默认使用内置微信网关，不需要单独部署第二个服务。",
			"先扫码登录微信入口，再向该入口发送当前绑定码，NasNotify 才会认定为已绑定。",
			"绑定完成后，可发送 菜单、状态、通知、存储、Docker、进程、备份、电源、UPS、测试、风扇2、CPU1 等命令。",
		},
	}

	code, err := EnsureClawBotBindingCode()
	if err == nil {
		status.BindingCode = code
	} else {
		status.LastError = err.Error()
	}

	if useEmbeddedGateway() {
		return mergeEmbeddedGatewayStatus(status)
	}

	if strings.TrimSpace(snapshot.WechatGatewayURL) == "" {
		if status.LastError == "" {
			status.LastError = "请先填写微信网关地址，或使用默认内置网关。"
		}
		status.Configured = false
		return status
	}

	var remote gatewayStatusResponse
	if err := gatewayJSONRequest(http.MethodGet, "/api/v1/status", nil, nil, &remote); err != nil {
		if status.LastError == "" {
			status.LastError = err.Error()
		}
		return status
	}

	return mergeGatewayStatus(status, remote)
}

func StartClawBotLogin() error {
	if useEmbeddedGateway() {
		return wechatgateway.StartLogin(true)
	}
	return gatewayJSONRequest(http.MethodPost, "/api/v1/login/start", nil, map[string]string{}, nil)
}

func SubmitClawBotVerifyCode(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("验证码不能为空")
	}

	if useEmbeddedGateway() {
		return wechatgateway.SubmitVerifyCode(code)
	}
	return gatewayJSONRequest(http.MethodPost, "/api/v1/login/verify-code", nil, map[string]string{
		"verify_code": code,
	}, nil)
}

func UnbindClawBot() error {
	_, localErr := RotateClawBotBindingCode()
	if useEmbeddedGateway() {
		wechatgateway.ResetSession()
		return localErr
	}
	if err := gatewayJSONRequest(http.MethodPost, "/api/v1/session/reset", nil, map[string]string{"reason": "manual_unbind"}, nil); err != nil {
		log.Printf("external wechat gateway reset failed: %v", err)
	}
	return localErr
}

func GetClawBotQRCode() (*ClawBotQRCode, error) {
	status := GetClawBotStatus()
	if status.QRCode == nil {
		return nil, fmt.Errorf("暂时没有可用的登录二维码")
	}
	return status.QRCode, nil
}

func GetClawBotBindingInfo() (*ClawBotBindingInfo, error) {
	status := GetClawBotStatus()
	if !status.EntryBound && status.EntryBindTime == "" {
		return nil, nil
	}

	info := &ClawBotBindingInfo{
		CreateTime:       status.EntryBindTime,
		HaveContextToken: 0,
	}
	if status.HaveContextToken {
		info.HaveContextToken = 1
	}
	return info, nil
}

func GetClawBotMessages() ([]ClawBotMessage, error) {
	if useEmbeddedGateway() {
		items := wechatgateway.PullMessages(20)
		messages := make([]ClawBotMessage, 0, len(items))
		for _, item := range items {
			text := strings.TrimSpace(item.Text)
			if text == "" {
				continue
			}
			messages = append(messages, ClawBotMessage{
				ID:        strings.TrimSpace(item.ID),
				Type:      1,
				Text:      text,
				CreatedAt: strings.TrimSpace(item.CreatedAt),
			})
		}
		return messages, nil
	}

	var payload struct {
		Messages []gatewayMessage `json:"messages"`
	}
	query := url.Values{}
	query.Set("limit", "20")
	if err := gatewayJSONRequest(http.MethodGet, "/api/v1/messages/pull", query, nil, &payload); err != nil {
		return nil, err
	}

	messages := make([]ClawBotMessage, 0, len(payload.Messages))
	for _, item := range payload.Messages {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		messages = append(messages, ClawBotMessage{
			ID:        strings.TrimSpace(item.ID),
			Type:      1,
			Text:      text,
			CreatedAt: strings.TrimSpace(item.CreatedAt),
		})
	}
	return messages, nil
}

func InvalidateClawBotAccessKey() {}

func generateClawBotBindingCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 6)
	randBytes := make([]byte, 6)
	if _, err := rand.Read(randBytes); err != nil {
		return "NAS888"
	}
	for i, b := range randBytes {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

func normalizeBindingText(text string) string {
	text = strings.ToUpper(strings.TrimSpace(text))
	text = strings.ReplaceAll(text, " ", "")
	text = strings.ReplaceAll(text, "\n", "")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.ReplaceAll(text, "\t", "")
	return text
}

func mergeEmbeddedGatewayStatus(status ClawBotStatus) ClawBotStatus {
	remote := wechatgateway.GetStatus()
	status.OpenAPIReady = remote.GatewayOnline
	status.EntryBound = remote.SessionActive
	status.Activated = remote.SessionActive && remote.BoundPeerReady
	status.HaveContextToken = remote.BoundPeerReady
	status.NeedVerifyCode = remote.Login.NeedVerifyCode
	status.EntryBindTime = strings.TrimSpace(remote.Login.EnteredAt)
	if remote.Login.QRCodeURL != "" || remote.Login.QRCodeText != "" {
		status.QRCode = &ClawBotQRCode{
			URL:    strings.TrimSpace(remote.Login.QRCodeURL),
			QRCode: strings.TrimSpace(remote.Login.QRCodeText),
		}
	}
	if remote.LastError != "" && status.LastError == "" {
		status.LastError = remote.LastError
	}
	if len(remote.Login.Tips) > 0 {
		status.Tips = remote.Login.Tips
	}
	return status
}

func mergeGatewayStatus(status ClawBotStatus, remote gatewayStatusResponse) ClawBotStatus {
	status.OpenAPIReady = remote.GatewayOnline
	status.EntryBound = remote.SessionActive
	status.Activated = remote.SessionActive && remote.BoundPeerReady
	status.HaveContextToken = remote.BoundPeerReady
	status.NeedVerifyCode = remote.Login.NeedVerifyCode
	status.EntryBindTime = strings.TrimSpace(remote.Login.EnteredAt)
	if remote.Login.QRCodeURL != "" || remote.Login.QRCodeText != "" {
		status.QRCode = &ClawBotQRCode{
			URL:    strings.TrimSpace(remote.Login.QRCodeURL),
			QRCode: strings.TrimSpace(remote.Login.QRCodeText),
		}
	}
	if remote.LastError != "" && status.LastError == "" {
		status.LastError = remote.LastError
	}
	if len(remote.Login.Tips) > 0 {
		status.Tips = remote.Login.Tips
	}
	return status
}

func useEmbeddedGateway() bool {
	return useEmbeddedGatewayURL(config.GetConfigSnapshot().WechatGatewayURL)
}

func useEmbeddedGatewayURL(raw string) bool {
	value := strings.TrimSpace(strings.TrimRight(raw, "/"))
	if value == "" {
		return true
	}
	if value == strings.TrimRight(config.DefaultWechatGatewayURL, "/") {
		return true
	}
	return value == "http://127.0.0.1:5091" || value == "http://localhost:5091"
}

func gatewayJSONRequest(method, route string, query url.Values, body any, dst any) error {
	endpoint, secret, err := gatewayEndpoint(route, query)
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewBuffer(raw)
	}

	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if secret != "" {
		req.Header.Set("X-Gateway-Secret", secret)
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接外置微信网关失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("外置微信网关返回错误: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read external wechat gateway response failed: %w", err)
	}

	payload := raw
	var envelope gatewayEnvelope
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) == nil {
		if envelope.Ret != 0 || envelope.ErrCode != 0 {
			message := strings.TrimSpace(envelope.ErrMsg)
			if message == "" {
				message = "unknown gateway error"
			}
			return fmt.Errorf("external wechat gateway business error: ret=%d errcode=%d errmsg=%s", envelope.Ret, envelope.ErrCode, message)
		}
		if len(envelope.Data) > 0 {
			payload = envelope.Data
		}
	}

	if dst == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, dst); err != nil {
		return fmt.Errorf("解析外置微信网关响应失败: %w", err)
	}
	return nil
}

func gatewayEndpoint(route string, query url.Values) (string, string, error) {
	snapshot := config.GetConfigSnapshot()
	base := strings.TrimRight(strings.TrimSpace(snapshot.WechatGatewayURL), "/")
	if base == "" {
		return "", "", fmt.Errorf("微信网关地址未配置")
	}
	if query != nil && len(query) > 0 {
		route += "?" + query.Encode()
	}
	return base + route, strings.TrimSpace(snapshot.WechatGatewaySecret), nil
}
