package wechatgateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ilink "github.com/openilink/openilink-sdk-go"

	"nasnotify-go/internal/config"
)

const (
	fixedBaseURL            = "https://ilinkai.weixin.qq.com"
	defaultBotType          = "3"
	activeLoginTTL          = 5 * time.Minute
	qrPollTimeout           = 35 * time.Second
	getUpdatesTimeout       = 35 * time.Second
	sendMessageTimeout      = 15 * time.Second
	sendMediaTimeout        = 90 * time.Second
	upstreamConnectTimeout  = 10 * time.Second
	defaultChannelVersion   = "2.4.3"
	defaultILinkAppID       = "bot"
	defaultILinkClientVer   = "132099"
	defaultBotAgent         = "NasNotify-Go/0.1 OpenClaw/2.4.3"
	defaultGatewayStatePath = "wechat_gateway"

	sessionExpiredErrCode = -14
	sessionPauseDuration  = time.Hour
)

type Status struct {
	GatewayOnline  bool        `json:"gateway_online"`
	SessionActive  bool        `json:"session_active"`
	BoundPeerReady bool        `json:"bound_peer_ready"`
	Login          LoginStatus `json:"login"`
	LastError      string      `json:"last_error"`
}

type LoginStatus struct {
	QRCodeURL      string   `json:"qrcode_url"`
	QRCodeText     string   `json:"qrcode_text"`
	EnteredAt      string   `json:"entered_at"`
	Status         string   `json:"status"`
	NeedVerifyCode bool     `json:"need_verify_code"`
	Tips           []string `json:"tips"`
}

type Message struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	CreatedAt string `json:"created_at"`
}

type serviceState struct {
	Account accountState `json:"account"`
	Runtime runtimeState `json:"runtime"`
}

type accountState struct {
	Token     string `json:"token"`
	AccountID string `json:"account_id"`
	BaseURL   string `json:"base_url"`
	UserID    string `json:"user_id"`
	SavedAt   string `json:"saved_at"`
}

type runtimeState struct {
	SyncBuf             string   `json:"sync_buf"`
	LastError           string   `json:"last_error"`
	UpstreamDiagnostics []string `json:"upstream_diagnostics,omitempty"`
	SessionExpiredAt    string   `json:"session_expired_at,omitempty"`
	SessionPausedUntil  string   `json:"session_paused_until,omitempty"`
	EnteredAt           string   `json:"entered_at"`
	LastInboundAt       string   `json:"last_inbound_at"`
	LastOutboundAt      string   `json:"last_outbound_at"`
	LatestPeerUserID    string   `json:"latest_peer_user_id"`
	LatestContextToken  string   `json:"latest_context_token"`
	BoundPeerUserID     string   `json:"bound_peer_user_id"`
	BoundContextToken   string   `json:"bound_context_token"`
}

type activeLogin struct {
	SessionKey        string
	QRCode            string
	QRCodeURL         string
	StartedAt         time.Time
	Status            string
	PendingVerifyCode string
	CurrentAPIBaseURL string
	LastMessage       string
}

type upstreamEnvelope struct {
	Ret     int             `json:"ret"`
	ErrCode int             `json:"errcode"`
	ErrMsg  string          `json:"errmsg"`
	Data    json.RawMessage `json:"data"`
}

type upstreamBusinessError struct {
	Ret     int
	ErrCode int
	ErrMsg  string
}

func (e *upstreamBusinessError) Error() string {
	message := strings.TrimSpace(e.ErrMsg)
	if message == "" {
		message = "unknown gateway error"
	}
	return fmt.Sprintf("wechat gateway upstream business error: ret=%d errcode=%d errmsg=%s", e.Ret, e.ErrCode, message)
}

type Service struct {
	mu             sync.Mutex
	once           sync.Once
	stateDir       string
	accountFile    string
	runtimeFile    string
	account        accountState
	runtime        runtimeState
	activeLogin    *activeLogin
	inbound        []Message
	outbound       []Message
	monitorRunning bool
}

var defaultService = &Service{}

func Init() {
	defaultService.Init()
}

func GetStatus() Status {
	return defaultService.GetStatus()
}

func StartLogin(force bool) error {
	return defaultService.StartLogin(force)
}

func SubmitVerifyCode(code string) error {
	return defaultService.SubmitVerifyCode(code)
}

func PullMessages(limit int) []Message {
	return defaultService.PullMessages(limit)
}

func SendTextMessage(title, text string) error {
	return defaultService.SendTextMessage(title, text)
}

func SendImageMessage(pngData []byte) error {
	return defaultService.SendImageMessage(pngData)
}

func ResetSession() {
	defaultService.ResetSession()
}

func BindLatestPeer() bool {
	return defaultService.BindLatestPeer()
}

func (s *Service) Init() {
	s.once.Do(func() {
		s.stateDir = filepath.Join(config.AppDataDir(), defaultGatewayStatePath)
		s.accountFile = filepath.Join(s.stateDir, "account.json")
		s.runtimeFile = filepath.Join(s.stateDir, "runtime.json")
		if err := os.MkdirAll(s.stateDir, 0o700); err != nil {
			log.Printf("create wechat gateway state dir failed: %v", err)
		}
		s.restore()
		if s.account.Token != "" {
			go s.ensureMonitorLoop()
		}
	})
}

func (s *Service) GetStatus() Status {
	s.Init()

	s.mu.Lock()
	now := time.Now()
	sessionActive := s.sessionActiveLocked(now)

	loginStatus := LoginStatus{
		QRCodeURL:      valueOrEmpty(s.activeLogin, func(v *activeLogin) string { return v.QRCodeURL }),
		QRCodeText:     valueOrEmpty(s.activeLogin, func(v *activeLogin) string { return v.QRCode }),
		EnteredAt:      s.runtime.EnteredAt,
		Status:         "idle",
		NeedVerifyCode: false,
		Tips: []string{
			"默认使用内置 ClawBot 网关，无需额外启动第二个服务。",
			"如果扫码后微信提示数字验证码，请在管理页补充提交。",
			"完成绑定后，可使用菜单、状态、通知、存储、Docker、进程、备份、电源、UPS、测试、风扇2、CPU1等固定指令。",
		},
	}
	if len(s.runtime.UpstreamDiagnostics) > 0 {
		loginStatus.Tips = append(loginStatus.Tips, s.runtime.UpstreamDiagnostics...)
	}

	if sessionActive {
		loginStatus.Status = "connected"
	} else if strings.TrimSpace(s.account.Token) != "" && s.sessionPausedLocked(now) {
		loginStatus.Status = "paused"
	}
	if s.activeLogin != nil {
		loginStatus.Status = s.activeLogin.Status
		loginStatus.NeedVerifyCode = s.activeLogin.Status == "need_verifycode"
	}

	result := Status{
		GatewayOnline:  true,
		SessionActive:  sessionActive,
		BoundPeerReady: s.runtime.BoundPeerUserID != "" || s.runtime.LatestPeerUserID != "",
		Login:          loginStatus,
		LastError:      s.runtime.LastError,
	}
	s.mu.Unlock()

	return result
}
func (s *Service) StartLogin(force bool) error {
	s.Init()

	s.mu.Lock()
	if !force && s.activeLogin != nil && time.Since(s.activeLogin.StartedAt) < activeLoginTTL && s.activeLogin.QRCodeURL != "" {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	var response struct {
		QRCodeImgContent string `json:"qrcode_img_content"`
		QRCode           string `json:"qrcode"`
	}
	if err := s.doJSONRequest(http.MethodPost, fixedBaseURL, "ilink/bot/get_bot_qrcode?bot_type="+url.QueryEscape(defaultBotType), "", map[string]any{
		"local_token_list": s.localTokenList(),
	}, 15*time.Second, &response); err != nil {
		s.setNetworkError(err, fixedBaseURL, "获取微信登录二维码")
		return err
	}

	s.mu.Lock()
	s.activeLogin = &activeLogin{
		SessionKey:        randomID(),
		QRCode:            strings.TrimSpace(response.QRCode),
		QRCodeURL:         strings.TrimSpace(response.QRCodeImgContent),
		StartedAt:         time.Now(),
		Status:            "wait",
		CurrentAPIBaseURL: fixedBaseURL,
		LastMessage:       "请使用手机微信扫描二维码。",
	}
	s.runtime.LastError = ""
	s.runtime.UpstreamDiagnostics = nil
	loginCopy := *s.activeLogin
	s.mu.Unlock()

	go s.qrPollLoop(loginCopy.SessionKey)
	return nil
}

func (s *Service) SubmitVerifyCode(code string) error {
	s.Init()
	code = strings.TrimSpace(code)
	if code == "" {
		return fmt.Errorf("验证码不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeLogin == nil {
		return fmt.Errorf("当前没有等待中的二维码会话")
	}
	s.activeLogin.PendingVerifyCode = code
	return nil
}

func (s *Service) PullMessages(limit int) []Message {
	s.Init()
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if limit > len(s.inbound) {
		limit = len(s.inbound)
	}
	items := make([]Message, limit)
	copy(items, s.inbound[:limit])
	s.inbound = append([]Message{}, s.inbound[limit:]...)
	return items
}

func (s *Service) SendTextMessage(title, text string) error {
	s.Init()
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	s.mu.Lock()
	token := s.account.Token
	baseURL := firstNonEmpty(s.account.BaseURL, fixedBaseURL)
	peerUserID, contextToken := s.targetPeerAndContextLocked()
	sessionPaused := s.sessionPausedLocked(time.Now())
	s.mu.Unlock()

	if token == "" {
		return fmt.Errorf("ClawBot 网关尚未完成登录")
	}
	if sessionPaused {
		return fmt.Errorf("微信 ClawBot 会话被上游临时暂停，请稍后自动重试")
	}
	if peerUserID == "" {
		return fmt.Errorf("还没有可用的 ClawBot 会话，请先向 ClawBot 发送一条消息")
	}

	fullText := text
	if strings.TrimSpace(title) != "" {
		fullText = strings.TrimSpace(title) + "\n\n" + text
	}

	payload := map[string]any{
		"msg": map[string]any{
			"from_user_id":  "",
			"to_user_id":    peerUserID,
			"client_id":     "nasnotify-" + randomID(),
			"message_type":  2,
			"message_state": 2,
			"context_token": emptyToNil(contextToken),
			"item_list": []any{
				map[string]any{
					"type": 1,
					"text_item": map[string]any{
						"text": fullText,
					},
				},
			},
		},
		"base_info": map[string]any{
			"channel_version": defaultChannelVersion,
			"bot_agent":       defaultBotAgent,
		},
	}

	if err := s.doJSONRequest(http.MethodPost, baseURL, "ilink/bot/sendmessage", token, payload, sendMessageTimeout, nil); err != nil {
		if isSessionExpiredError(err) {
			s.handleSessionExpired(err)
			return err
		}
		s.setLastError(err.Error())
		return err
	}

	s.mu.Lock()
	s.runtime.LastOutboundAt = nowString()
	s.outbound = append(s.outbound, Message{
		ID:        "out-" + randomID(),
		Type:      "text",
		Text:      fullText,
		CreatedAt: nowString(),
	})
	s.persistLocked()
	s.mu.Unlock()
	return nil
}

func (s *Service) SendImageMessage(pngData []byte) error {
	s.Init()
	if len(pngData) == 0 {
		return fmt.Errorf("image payload is empty")
	}

	s.mu.Lock()
	token := s.account.Token
	baseURL := firstNonEmpty(s.account.BaseURL, fixedBaseURL)
	peerUserID, contextToken := s.targetPeerAndContextLocked()
	sessionPaused := s.sessionPausedLocked(time.Now())
	s.mu.Unlock()

	if token == "" {
		return fmt.Errorf("ClawBot 网关尚未完成登录")
	}
	if sessionPaused {
		return fmt.Errorf("微信 ClawBot 会话被上游临时暂停，请稍后自动重试")
	}
	if peerUserID == "" {
		return fmt.Errorf("还没有可用的 ClawBot 会话，请先向 ClawBot 发送一条消息")
	}

	client := ilink.NewClient(
		token,
		ilink.WithBaseURL(baseURL),
		ilink.WithVersion(defaultChannelVersion),
		ilink.WithHTTPClient(&http.Client{
			Transport: buildGatewayTransport(),
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), sendMediaTimeout)
	defer cancel()

	uploaded, err := client.UploadFile(ctx, pngData, peerUserID, ilink.MediaImage)
	if err != nil {
		if isSessionExpiredError(err) {
			s.handleSessionExpired(err)
			return err
		}
		s.setLastError(err.Error())
		return err
	}

	clientID, err := client.SendImage(ctx, peerUserID, contextToken, uploaded)
	if err != nil {
		if isSessionExpiredError(err) {
			s.handleSessionExpired(err)
			return err
		}
		s.setLastError(err.Error())
		return err
	}

	s.mu.Lock()
	s.runtime.LastOutboundAt = nowString()
	s.outbound = append(s.outbound, Message{
		ID:        firstNonEmpty(strings.TrimSpace(clientID), "out-"+randomID()),
		Type:      "image",
		Text:      fmt.Sprintf("[image/png] %d bytes", len(pngData)),
		CreatedAt: nowString(),
	})
	s.persistLocked()
	s.mu.Unlock()
	return nil
}

func (s *Service) ResetSession() {
	s.Init()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.account = accountState{}
	s.runtime = runtimeState{}
	s.activeLogin = nil
	s.inbound = nil
	s.outbound = nil
	s.persistLocked()
}

func (s *Service) BindLatestPeer() bool {
	s.Init()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime.LatestPeerUserID == "" {
		return false
	}
	s.runtime.BoundPeerUserID = s.runtime.LatestPeerUserID
	s.runtime.BoundContextToken = s.runtime.LatestContextToken
	s.persistLocked()
	return true
}

func (s *Service) targetPeerAndContextLocked() (string, string) {
	if strings.TrimSpace(s.runtime.BoundPeerUserID) != "" {
		peerUserID := strings.TrimSpace(s.runtime.BoundPeerUserID)
		if strings.TrimSpace(s.runtime.LatestPeerUserID) == peerUserID && strings.TrimSpace(s.runtime.LatestContextToken) != "" {
			return peerUserID, strings.TrimSpace(s.runtime.LatestContextToken)
		}
		return peerUserID, strings.TrimSpace(s.runtime.BoundContextToken)
	}
	return strings.TrimSpace(s.runtime.LatestPeerUserID), strings.TrimSpace(s.runtime.LatestContextToken)
}

func (s *Service) sessionActiveLocked(now time.Time) bool {
	return strings.TrimSpace(s.account.Token) != "" && !s.sessionPausedLocked(now)
}

func (s *Service) sessionPausedLocked(now time.Time) bool {
	return !s.sessionPausedUntilLocked(now).IsZero()
}

func (s *Service) sessionPausedUntilLocked(now time.Time) time.Time {
	if strings.TrimSpace(s.account.Token) == "" {
		return time.Time{}
	}
	pausedUntil := parseStateTime(s.runtime.SessionPausedUntil)
	if pausedUntil.IsZero() || !now.Before(pausedUntil) {
		return time.Time{}
	}
	return pausedUntil
}

func (s *Service) qrPollLoop(sessionKey string) {
	for {
		s.mu.Lock()
		login := s.activeLogin
		if login == nil || login.SessionKey != sessionKey || time.Since(login.StartedAt) >= activeLoginTTL {
			s.mu.Unlock()
			return
		}
		baseURL := firstNonEmpty(login.CurrentAPIBaseURL, fixedBaseURL)
		qrCode := login.QRCode
		verifyCode := login.PendingVerifyCode
		s.mu.Unlock()

		endpoint := "ilink/bot/get_qrcode_status?qrcode=" + url.QueryEscape(qrCode)
		if verifyCode != "" {
			endpoint += "&verify_code=" + url.QueryEscape(verifyCode)
		}

		var response struct {
			Status       string `json:"status"`
			RedirectHost string `json:"redirect_host"`
			BotToken     string `json:"bot_token"`
			ILinkBotID   string `json:"ilink_bot_id"`
			BaseURL      string `json:"baseurl"`
			ILinkUserID  string `json:"ilink_user_id"`
		}
		err := s.doJSONRequest(http.MethodGet, baseURL, endpoint, "", nil, qrPollTimeout, &response)
		if err != nil {
			s.setNetworkError(err, baseURL, "轮询二维码状态")
			time.Sleep(2 * time.Second)
			continue
		}

		s.mu.Lock()
		if s.activeLogin == nil || s.activeLogin.SessionKey != sessionKey {
			s.mu.Unlock()
			return
		}

		s.activeLogin.Status = strings.TrimSpace(response.Status)
		switch s.activeLogin.Status {
		case "scaned_but_redirect":
			if strings.TrimSpace(response.RedirectHost) != "" {
				s.activeLogin.CurrentAPIBaseURL = "https://" + strings.TrimSpace(response.RedirectHost)
			}
		case "binded_redirect":
			if strings.TrimSpace(s.account.Token) != "" {
				s.runtime.LastError = ""
				s.runtime.UpstreamDiagnostics = nil
				s.runtime.SessionExpiredAt = ""
				s.runtime.SessionPausedUntil = ""
				s.activeLogin = nil
				s.persistLocked()
				s.mu.Unlock()
				go s.ensureMonitorLoop()
				return
			}
			s.runtime.LastError = "微信提示此 ClawBot 已绑定，但本地没有可用凭据，请重新扫码登录。"
		case "expired":
			s.runtime.LastError = "二维码已过期，请重新生成。"
			s.activeLogin = nil
		case "confirmed":
			if strings.TrimSpace(response.BotToken) != "" && strings.TrimSpace(response.ILinkBotID) != "" {
				s.account = accountState{
					Token:     strings.TrimSpace(response.BotToken),
					AccountID: strings.TrimSpace(response.ILinkBotID),
					BaseURL:   firstNonEmpty(strings.TrimSpace(response.BaseURL), fixedBaseURL),
					UserID:    strings.TrimSpace(response.ILinkUserID),
					SavedAt:   nowString(),
				}
				s.runtime.EnteredAt = nowString()
				s.runtime.LastError = ""
				s.runtime.UpstreamDiagnostics = nil
				s.runtime.SessionExpiredAt = ""
				s.runtime.SessionPausedUntil = ""
				s.activeLogin = nil
				s.persistLocked()
				s.mu.Unlock()
				go s.ensureMonitorLoop()
				return
			}
		}
		s.persistLocked()
		s.mu.Unlock()
		time.Sleep(1200 * time.Millisecond)
	}
}

func (s *Service) ensureMonitorLoop() {
	s.mu.Lock()
	if s.monitorRunning || s.account.Token == "" {
		s.mu.Unlock()
		return
	}
	s.monitorRunning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.monitorRunning = false
		s.mu.Unlock()
	}()

	for {
		s.mu.Lock()
		pausedUntil := s.sessionPausedUntilLocked(time.Now())
		s.mu.Unlock()
		if !pausedUntil.IsZero() {
			sleepFor := time.Until(pausedUntil)
			if sleepFor > 30*time.Second {
				sleepFor = 30 * time.Second
			}
			if sleepFor > 0 {
				time.Sleep(sleepFor)
			}
			continue
		}

		s.mu.Lock()
		token := s.account.Token
		baseURL := firstNonEmpty(s.account.BaseURL, fixedBaseURL)
		buf := s.runtime.SyncBuf
		s.mu.Unlock()

		if token == "" {
			return
		}

		var response struct {
			GetUpdatesBuf string `json:"get_updates_buf"`
			Msgs          []struct {
				FromUserID   string `json:"from_user_id"`
				ContextToken string `json:"context_token"`
				ItemList     []struct {
					Type     int `json:"type"`
					TextItem struct {
						Text string `json:"text"`
					} `json:"text_item"`
				} `json:"item_list"`
			} `json:"msgs"`
		}

		err := s.doJSONRequest(http.MethodPost, baseURL, "ilink/bot/getupdates", token, map[string]any{
			"get_updates_buf": buf,
			"base_info": map[string]any{
				"channel_version": defaultChannelVersion,
				"bot_agent":       defaultBotAgent,
			},
		}, getUpdatesTimeout, &response)
		if err != nil {
			if isSessionExpiredError(err) {
				s.handleSessionExpired(err)
				return
			}
			s.setNetworkError(err, baseURL, "拉取微信消息")
			time.Sleep(2 * time.Second)
			continue
		}

		s.mu.Lock()
		s.runtime.LastError = ""
		s.runtime.UpstreamDiagnostics = nil
		s.runtime.SessionExpiredAt = ""
		s.runtime.SessionPausedUntil = ""
		if strings.TrimSpace(response.GetUpdatesBuf) != "" {
			s.runtime.SyncBuf = strings.TrimSpace(response.GetUpdatesBuf)
		}
		for _, msg := range response.Msgs {
			fromUserID := strings.TrimSpace(msg.FromUserID)
			contextToken := strings.TrimSpace(msg.ContextToken)
			for _, item := range msg.ItemList {
				if item.Type != 1 || strings.TrimSpace(item.TextItem.Text) == "" {
					continue
				}
				s.runtime.LatestPeerUserID = fromUserID
				s.runtime.LatestContextToken = contextToken
				if s.runtime.BoundPeerUserID != "" && fromUserID == s.runtime.BoundPeerUserID && contextToken != "" {
					s.runtime.BoundContextToken = contextToken
				}
				s.runtime.LastInboundAt = nowString()
				s.inbound = append(s.inbound, Message{
					ID:        "in-" + randomID(),
					Type:      "text",
					Text:      strings.TrimSpace(item.TextItem.Text),
					CreatedAt: nowString(),
				})
			}
		}
		s.persistLocked()
		s.mu.Unlock()
	}
}

func (s *Service) restore() {
	s.mu.Lock()
	defer s.mu.Unlock()

	var account accountState
	if loadJSON(s.accountFile, &account) == nil {
		s.account = account
	}
	var runtime runtimeState
	if loadJSON(s.runtimeFile, &runtime) == nil {
		s.runtime = runtime
	}
}

func (s *Service) persistLocked() {
	if err := os.MkdirAll(s.stateDir, 0o700); err != nil {
		log.Printf("create wechat gateway state dir failed: %v", err)
		return
	}
	if err := saveJSON(s.accountFile, s.account); err != nil {
		log.Printf("save wechat gateway account state failed: %v", err)
	}
	if err := saveJSON(s.runtimeFile, s.runtime); err != nil {
		log.Printf("save wechat gateway runtime state failed: %v", err)
	}
}

func (s *Service) setLastError(message string) {
	s.mu.Lock()
	s.runtime.LastError = message
	s.persistLocked()
	s.mu.Unlock()
}

func (s *Service) setNetworkError(err error, baseURL, action string) {
	message := s.decorateUpstreamError(err, baseURL, action)
	s.mu.Lock()
	s.runtime.LastError = message
	s.persistLocked()
	s.mu.Unlock()
}

func (s *Service) handleSessionExpired(err error) {
	if err != nil {
		log.Printf("wechat gateway session expired: %v", err)
	}
	now := time.Now()
	message := "微信 ClawBot 会话被上游临时暂停（errcode=-14），已按官方逻辑等待 1 小时后自动重试。"

	s.mu.Lock()
	s.runtime.SessionExpiredAt = formatStateTime(now)
	s.runtime.SessionPausedUntil = formatStateTime(now.Add(sessionPauseDuration))
	s.runtime.LastError = message
	s.runtime.UpstreamDiagnostics = nil
	s.persistLocked()
	s.mu.Unlock()
}

func (s *Service) localTokenList() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.account.Token) == "" {
		return []string{}
	}
	return []string{s.account.Token}
}

func (s *Service) doJSONRequest(method, baseURL, endpoint, token string, payload any, timeout time.Duration, dst any) error {
	fullURL := strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
	var bodyBytes []byte
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		bodyBytes = raw
	}

	attempts := 2
	var resp *http.Response
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		var body io.Reader
		if len(bodyBytes) > 0 {
			body = bytes.NewBuffer(bodyBytes)
		}

		req, reqErr := http.NewRequest(method, fullURL, body)
		if reqErr != nil {
			return reqErr
		}
		for key, value := range buildHeaders(token) {
			req.Header.Set(key, value)
		}
		if method == http.MethodGet {
			req.Header.Del("Content-Type")
		}

		// 每次重试创建独立 client，避免并发修改共享 client.Timeout
		c := &http.Client{
			Timeout:   timeout + time.Duration(attempt-1)*10*time.Second,
			Transport: buildGatewayTransport(),
		}
		resp, err = c.Do(req)
		if err == nil {
			break
		}
		if attempt < attempts {
			time.Sleep(time.Duration(attempt) * 1200 * time.Millisecond)
		}
	}
	if err != nil {
		return fmt.Errorf("wechat gateway upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("wechat gateway upstream request failed: HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	responsePayload := raw
	var envelope upstreamEnvelope
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) == nil {
		if envelope.Ret != 0 || envelope.ErrCode != 0 {
			message := strings.TrimSpace(envelope.ErrMsg)
			if message == "" {
				message = "unknown gateway error"
			}
			return &upstreamBusinessError{Ret: envelope.Ret, ErrCode: envelope.ErrCode, ErrMsg: message}
		}
		if len(envelope.Data) > 0 {
			responsePayload = envelope.Data
		}
	}

	if dst == nil || len(responsePayload) == 0 {
		return nil
	}
	if err := json.Unmarshal(responsePayload, dst); err != nil {
		return fmt.Errorf("wechat gateway upstream decode failed: %w", err)
	}
	return nil
}

func (s *Service) decorateUpstreamError(err error, baseURL, action string) string {
	lines := []string{
		fmt.Sprintf("%s失败：%s", action, friendlyError(err)),
	}
	diagnostics := diagnoseUpstream(baseURL)
	if len(diagnostics) > 0 {
		lines = append(lines, "网络诊断：")
		lines = append(lines, diagnostics...)
		s.mu.Lock()
		s.runtime.UpstreamDiagnostics = diagnostics
		s.mu.Unlock()
	}
	lines = append(lines, "说明：这里不需要公网 IP，但 NAS 机器本身必须能正常访问外部 HTTPS 网络。")
	return strings.Join(lines, "\n")
}

func buildHeaders(token string) map[string]string {
	headers := map[string]string{
		"Content-Type":            "application/json",
		"AuthorizationType":       "ilink_bot_token",
		"X-WECHAT-UIN":            randomWechatUIN(),
		"iLink-App-Id":            defaultILinkAppID,
		"iLink-App-ClientVersion": defaultILinkClientVer,
	}
	if strings.TrimSpace(token) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(token)
	}
	return headers
}

func randomWechatUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte("12345678"))
	}
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", int(b[0])<<24|int(b[1])<<16|int(b[2])<<8|int(b[3]))))
}

func loadJSON(path string, dst any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func saveJSON(path string, payload any) error {
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func valueOrEmpty[T any](in *T, getter func(*T) string) string {
	if in == nil {
		return ""
	}
	return getter(in)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func isSessionExpiredError(err error) bool {
	if err == nil {
		return false
	}
	var businessErr *upstreamBusinessError
	if errors.As(err, &businessErr) {
		return businessErr.Ret == sessionExpiredErrCode || businessErr.ErrCode == sessionExpiredErrCode
	}

	text := strings.ToLower(err.Error())
	return strings.Contains(text, "ret=-14") ||
		strings.Contains(text, "errcode=-14") ||
		strings.Contains(text, `"ret":-14`) ||
		strings.Contains(text, `"errcode":-14`)
}

func formatStateTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func parseStateTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func emptyToNil(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func nowString() string {
	return formatStateTime(time.Now())
}

func buildGatewayTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		Proxy: http.ProxyFromEnvironment,
	}
}

func randomID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func diagnoseUpstream(baseURL string) []string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return []string{"- 无法解析上游地址。"}
	}

	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		if strings.EqualFold(parsed.Scheme, "https") {
			port = "443"
		} else {
			port = "80"
		}
	}

	results := []string{}

	ctx, cancel := context.WithTimeout(context.Background(), upstreamConnectTimeout)
	defer cancel()
	ips, lookupErr := net.DefaultResolver.LookupHost(ctx, host)
	if lookupErr != nil {
		results = append(results, fmt.Sprintf("- DNS 解析失败：%v", lookupErr))
		return results
	}
	if len(ips) > 0 {
		results = append(results, fmt.Sprintf("- DNS 解析成功：%s -> %s", host, strings.Join(ips, ", ")))
	} else {
		results = append(results, fmt.Sprintf("- DNS 解析成功：%s", host))
	}

	address := net.JoinHostPort(host, port)
	conn, dialErr := net.DialTimeout("tcp", address, upstreamConnectTimeout)
	if dialErr != nil {
		results = append(results, fmt.Sprintf("- TCP %s 连接失败：%v", address, dialErr))
		return results
	}
	_ = conn.Close()
	results = append(results, fmt.Sprintf("- TCP %s 连接成功", address))

	dialer := &net.Dialer{Timeout: upstreamConnectTimeout}
	tlsConn, tlsErr := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	if tlsErr != nil {
		results = append(results, fmt.Sprintf("- TLS 握手失败：%v", tlsErr))
		return results
	}
	_ = tlsConn.Close()
	results = append(results, "- TLS 握手成功")
	return results
}

func friendlyError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "请求超时"
	}

	text := err.Error()
	if strings.Contains(text, "Client.Timeout") {
		return "等待上游响应超时"
	}
	if strings.Contains(strings.ToLower(text), "no such host") {
		return "DNS 解析失败"
	}
	return text
}
