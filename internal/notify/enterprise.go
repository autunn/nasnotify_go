package notify

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nasnotify-go/internal/config"
)

type WeChatXMLMsg struct {
	XMLName    xml.Name `xml:"xml"`
	ToUserName string   `xml:"ToUserName"`
	AgentID    string   `xml:"AgentID"`
	Encrypt    string   `xml:"Encrypt"`
}

type WeChatPlainMsg struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Event        string   `xml:"Event"`
	EventKey     string   `xml:"EventKey"`
	Content      string   `xml:"Content"`
}

var (
	enterpriseAccessToken          string
	enterpriseAccessTokenExpiresAt int64
	enterpriseAccessTokenMu        sync.Mutex
	enterpriseHTTPClient           = &http.Client{Timeout: 20 * time.Second}
	enterpriseUploadHTTPClient     = &http.Client{Timeout: 45 * time.Second}
)

func EnterpriseWechatConfigured() bool {
	snapshot := config.GetConfigSnapshot()
	return strings.TrimSpace(snapshot.CorpID) != "" &&
		strings.TrimSpace(snapshot.CorpSecret) != "" &&
		strings.TrimSpace(snapshot.AgentID) != ""
}

func EnterpriseWechatPush(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	snapshot := config.GetConfigSnapshot()
	baseURL := enterpriseBaseURL(snapshot.ProxyURL)
	token, err := enterpriseToken(baseURL, snapshot.CorpID, snapshot.CorpSecret)
	if err != nil {
		return err
	}

	agentID, _ := strconv.Atoi(strings.TrimSpace(snapshot.AgentID))
	picURL := enterprisePicURL(snapshot.PhotoURL)
	payload := map[string]any{
		"touser":  "@all",
		"msgtype": "news",
		"agentid": agentID,
		"news": map[string]any{
			"articles": []map[string]any{
				{
					"title":       "NAS 通知中心",
					"description": content,
					"url":         strings.TrimSpace(snapshot.NasURL),
					"picurl":      picURL,
				},
			},
		},
	}
	return enterprisePostJSON(baseURL, "/cgi-bin/message/send?access_token="+token, payload, nil)
}

func EnterpriseWechatPushImage(pngData []byte) error {
	if len(pngData) == 0 {
		return fmt.Errorf("图片内容为空")
	}

	snapshot := config.GetConfigSnapshot()
	baseURL := enterpriseBaseURL(snapshot.ProxyURL)
	token, err := enterpriseToken(baseURL, snapshot.CorpID, snapshot.CorpSecret)
	if err != nil {
		return err
	}

	mediaID, err := enterpriseUploadImage(baseURL, token, pngData)
	if err != nil {
		return err
	}

	agentID, _ := strconv.Atoi(strings.TrimSpace(snapshot.AgentID))
	payload := map[string]any{
		"touser":  "@all",
		"msgtype": "image",
		"agentid": agentID,
		"image": map[string]any{
			"media_id": mediaID,
		},
	}
	return enterprisePostJSON(baseURL, "/cgi-bin/message/send?access_token="+token, payload, nil)
}

func CreateEnterpriseWechatMenu() error {
	snapshot := config.GetConfigSnapshot()
	if strings.TrimSpace(snapshot.AgentID) == "" || strings.TrimSpace(snapshot.CorpID) == "" || strings.TrimSpace(snapshot.CorpSecret) == "" {
		return nil
	}

	baseURL := enterpriseBaseURL(snapshot.ProxyURL)
	token, err := enterpriseToken(baseURL, snapshot.CorpID, snapshot.CorpSecret)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"button": []map[string]any{
			{
				"name": "查询",
				"sub_button": []map[string]any{
					{"type": "click", "name": "一键巡检", "key": "GET_UGREEN_HEALTH"},
					{"type": "click", "name": "系统状态", "key": "GET_UGREEN_INFO"},
					{"type": "click", "name": "系统通知", "key": "GET_UGREEN_NOTIFY"},
					{"type": "click", "name": "存储状态", "key": "GET_UGREEN_STORAGE"},
					{"type": "click", "name": "更多查询", "key": "GET_UGREEN_QUERY_HELP"},
				},
			},
			{
				"name": "服务",
				"sub_button": []map[string]any{
					{"type": "click", "name": "Docker", "key": "GET_UGREEN_DOCKER"},
					{"type": "click", "name": "进程列表", "key": "GET_UGREEN_PS"},
					{"type": "click", "name": "备份任务", "key": "GET_UGREEN_BACKUP"},
					{"type": "click", "name": "电源配置", "key": "GET_UGREEN_POWER"},
					{"type": "click", "name": "UPS 电源", "key": "GET_UGREEN_UPS"},
				},
			},
			{
				"name": "控制",
				"sub_button": []map[string]any{
					{"type": "click", "name": "风扇静音", "key": "SET_UGREEN_FAN_1"},
					{"type": "click", "name": "风扇标准", "key": "SET_UGREEN_FAN_2"},
					{"type": "click", "name": "风扇全速", "key": "SET_UGREEN_FAN_3"},
					{"type": "click", "name": "CPU 性能", "key": "GET_UGREEN_CPU_HELP"},
					{"type": "click", "name": "远程唤醒", "key": "GET_NAS_WOL"},
				},
			},
		},
	}

	agentID := strings.TrimSpace(snapshot.AgentID)
	err = enterprisePostJSON(baseURL, "/cgi-bin/menu/create?access_token="+token+"&agentid="+agentID, payload, nil)
	if err != nil {
		return err
	}
	log.Println("企业微信自定义菜单已同步")
	return nil
}

func enterpriseBaseURL(proxyURL string) string {
	proxyURL = strings.TrimRight(strings.TrimSpace(proxyURL), "/")
	if proxyURL != "" {
		return proxyURL
	}
	return "https://qyapi.weixin.qq.com"
}

func enterprisePicURL(photoURL string) string {
	photoURL = strings.TrimSpace(photoURL)
	if photoURL == "" {
		return fmt.Sprintf("https://api.vvhan.com/api/wallpaper/acg?rand=%d", time.Now().UnixNano())
	}
	connector := "?"
	if strings.Contains(photoURL, "?") {
		connector = "&"
	}
	return fmt.Sprintf("%s%sv=%d", photoURL, connector, time.Now().UnixNano())
}

func enterpriseToken(baseURL, corpID, corpSecret string) (string, error) {
	enterpriseAccessTokenMu.Lock()
	defer enterpriseAccessTokenMu.Unlock()

	if enterpriseAccessToken != "" && enterpriseAccessTokenExpiresAt > time.Now().Unix() {
		return enterpriseAccessToken, nil
	}

	endpoint := fmt.Sprintf("%s/cgi-bin/gettoken?corpid=%s&corpsecret=%s", baseURL, corpID, corpSecret)
	resp, err := enterpriseHTTPClient.Get(endpoint)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("企业微信 gettoken HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var payload struct {
		ErrCode int64  `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		Token   string `json:"access_token"`
		Expires int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if payload.ErrCode != 0 {
		return "", fmt.Errorf("企业微信 gettoken 失败: %d %s", payload.ErrCode, payload.ErrMsg)
	}
	if strings.TrimSpace(payload.Token) == "" {
		return "", fmt.Errorf("企业微信 gettoken 未返回 access_token")
	}

	enterpriseAccessToken = payload.Token
	enterpriseAccessTokenExpiresAt = time.Now().Unix() + payload.Expires - 60
	return enterpriseAccessToken, nil
}

func enterpriseUploadImage(baseURL, token string, pngData []byte) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("media", "nasnotify-card.png")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(pngData); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := fmt.Sprintf("%s/cgi-bin/media/upload?access_token=%s&type=image", baseURL, token)
	req, err := http.NewRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := enterpriseUploadHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("企业微信上传图片 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var payload struct {
		ErrCode int64  `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if payload.ErrCode != 0 {
		return "", fmt.Errorf("企业微信上传图片失败: %d %s", payload.ErrCode, payload.ErrMsg)
	}
	if strings.TrimSpace(payload.MediaID) == "" {
		return "", fmt.Errorf("企业微信上传图片未返回 media_id")
	}
	return payload.MediaID, nil
}

func enterprisePostJSON(baseURL, route string, payload any, dst any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+route, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := enterpriseHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	responseRaw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("企业微信 API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseRaw)))
	}
	var result struct {
		ErrCode int64  `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(responseRaw, &result); err == nil && result.ErrCode != 0 {
		return fmt.Errorf("企业微信 API 失败: %d %s", result.ErrCode, result.ErrMsg)
	}
	if dst != nil && len(responseRaw) > 0 {
		if err := json.Unmarshal(responseRaw, dst); err != nil {
			return err
		}
	}
	return nil
}
