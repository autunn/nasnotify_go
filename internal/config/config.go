package config

import (
	"encoding/json"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"nasnotify-go/internal/appenv"
)

const (
	DefaultIntervalMinutes             = 5
	DefaultSystemStatusIntervalMinutes = 60
	DefaultWechatGatewayURL            = "http://127.0.0.1:5091"
	DefaultLocalNasHost                = "127.0.0.1"
)

var (
	Config AppConfig
	CfgMu  sync.RWMutex
)

type AppConfig struct {
	AdminPasswordHash string `json:"admin_password_hash,omitempty"`
	AdminPassword     string `json:"admin_password,omitempty"`

	IntervalMinutes             float64 `json:"interval_minutes"`
	SystemStatusIntervalMinutes float64 `json:"system_status_interval_minutes"`

	// Enterprise WeChat keeps compatibility with the original nasnotify_go
	// configuration and callback flow.
	CorpID         string `json:"corpid"`
	AgentID        string `json:"agentid"`
	CorpSecret     string `json:"corpsecret"`
	Token          string `json:"token"`
	EncodingAESKey string `json:"encoding_aes_key"`
	ProxyURL       string `json:"proxy_url"`
	NasURL         string `json:"nas_url"`
	PhotoURL       string `json:"photo_url"`

	// ClawBot is the newer personal WeChat gateway used by the black-gold UI.
	WechatGatewayURL    string `json:"wechat_gateway_url"`
	WechatGatewaySecret string `json:"wechat_gateway_secret"`
	WechatBindingCode   string `json:"wechat_binding_code"`
	WechatBound         bool   `json:"wechat_bound"`
	WechatBoundAt       string `json:"wechat_bound_at"`

	LocalNasName     string `json:"local_nas_name"`
	LocalNasHost     string `json:"local_nas_host"`
	LocalNasPort     int    `json:"local_nas_port"`
	LocalNasUsername string `json:"local_nas_username"`
	LocalNasPassword string `json:"local_nas_password"`

	// Legacy multi-device fields are preserved so existing configs do not lose
	// data when this repo is upgraded to the new single-local-NAS console.
	ZSpace []ZSpaceConfig `json:"zspace,omitempty"`
	UGreen []UGreenConfig `json:"ugreen,omitempty"`
	FnOs   []FnOsConfig   `json:"fnos,omitempty"`
}

type UGreenConfig struct {
	ID             string `json:"id,omitempty"`
	IpPort         string `json:"ip_port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	NotifyTypeName string `json:"notify_type_name"`
	UseSSL         bool   `json:"use_ssl"`
	MacAddress     string `json:"mac_address,omitempty"`
}

type ZSpaceConfig struct {
	ID             string `json:"id,omitempty"`
	IpPort         string `json:"ip_port"`
	Cookie         string `json:"cookie"`
	NotifyTypeName string `json:"notify_type_name"`
	UseSSL         bool   `json:"use_ssl"`
	MacAddress     string `json:"mac_address,omitempty"`
}

type FnOsConfig struct {
	ID             string `json:"id,omitempty"`
	Server         string `json:"server"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	NotifyTypeName string `json:"notify_type_name"`
	UseSSL         bool   `json:"use_ssl"`
	Cookie         string `json:"cookie,omitempty"`
	MacAddress     string `json:"mac_address,omitempty"`
}

func appDataDir() string {
	return appenv.DataDir()
}

func AppDataDir() string {
	return appDataDir()
}

func configPath() string {
	return filepath.Join(appDataDir(), "config", "config.json")
}

func InitConfig() {
	CfgMu.Lock()
	defer CfgMu.Unlock()

	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		log.Printf("create config dir failed, entering first-time setup mode: %v", err)
		Config = AppConfig{
			IntervalMinutes:             DefaultIntervalMinutes,
			SystemStatusIntervalMinutes: DefaultSystemStatusIntervalMinutes,
			WechatGatewayURL:            DefaultWechatGatewayURL,
			LocalNasHost:                DefaultLocalNasHost,
			LocalNasPort:                9999,
		}
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Println("config not found, entering first-time setup mode")
		Config = AppConfig{
			IntervalMinutes:             DefaultIntervalMinutes,
			SystemStatusIntervalMinutes: DefaultSystemStatusIntervalMinutes,
			WechatGatewayURL:            DefaultWechatGatewayURL,
			LocalNasHost:                DefaultLocalNasHost,
			LocalNasPort:                9999,
		}
		return
	}

	if err := json.Unmarshal(data, &Config); err != nil {
		backupPath := path + ".invalid"
		_ = os.Rename(path, backupPath)
		log.Printf("parse config failed, moved invalid config to %s and entered setup mode: %v", backupPath, err)
		Config = AppConfig{
			IntervalMinutes:             DefaultIntervalMinutes,
			SystemStatusIntervalMinutes: DefaultSystemStatusIntervalMinutes,
			WechatGatewayURL:            DefaultWechatGatewayURL,
			LocalNasHost:                DefaultLocalNasHost,
			LocalNasPort:                9999,
		}
		return
	}

	normalizeConfigLocked()
}

func IsInitialized() bool {
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	return Config.AdminPasswordHash != ""
}

func GetAdminPasswordHash() string {
	CfgMu.RLock()
	defer CfgMu.RUnlock()
	return Config.AdminPasswordHash
}

func GetConfigSnapshot() AppConfig {
	CfgMu.RLock()
	defer CfgMu.RUnlock()

	snapshot := Config
	snapshot.ZSpace = cloneZSpace(Config.ZSpace)
	snapshot.UGreen = cloneUGreen(Config.UGreen)
	snapshot.FnOs = cloneFnOs(Config.FnOs)
	return snapshot
}

func SanitizedConfigForWeb() AppConfig {
	snapshot := GetConfigSnapshot()
	snapshot.AdminPasswordHash = ""
	snapshot.AdminPassword = ""
	snapshot.CorpSecret = ""
	snapshot.Token = ""
	snapshot.EncodingAESKey = ""
	snapshot.WechatGatewaySecret = ""
	snapshot.LocalNasPassword = ""
	for i := range snapshot.ZSpace {
		snapshot.ZSpace[i].Cookie = ""
	}
	for i := range snapshot.UGreen {
		snapshot.UGreen[i].Password = ""
	}
	for i := range snapshot.FnOs {
		snapshot.FnOs[i].Password = ""
		snapshot.FnOs[i].Cookie = ""
	}
	return snapshot
}

func MergeWithExistingSensitiveFields(existing, incoming AppConfig) AppConfig {
	if incoming.CorpSecret == "" {
		incoming.CorpSecret = existing.CorpSecret
	}
	if incoming.Token == "" {
		incoming.Token = existing.Token
	}
	if incoming.EncodingAESKey == "" {
		incoming.EncodingAESKey = existing.EncodingAESKey
	}
	incoming.WechatBindingCode = strings.TrimSpace(incoming.WechatBindingCode)
	if incoming.WechatGatewaySecret == "" {
		incoming.WechatGatewaySecret = existing.WechatGatewaySecret
	}
	if incoming.WechatBindingCode == "" {
		incoming.WechatBindingCode = existing.WechatBindingCode
	}
	if existing.WechatBound {
		incoming.WechatBound = true
	}
	if incoming.WechatBoundAt == "" {
		incoming.WechatBoundAt = existing.WechatBoundAt
	}
	if incoming.LocalNasPassword == "" {
		incoming.LocalNasPassword = existing.LocalNasPassword
	}
	if strings.TrimSpace(incoming.LocalNasHost) == "" {
		incoming.LocalNasHost = existing.LocalNasHost
	}
	if incoming.ZSpace == nil {
		incoming.ZSpace = cloneZSpace(existing.ZSpace)
	} else {
		incoming.ZSpace = mergeZSpaceSensitive(existing.ZSpace, incoming.ZSpace)
	}
	if incoming.UGreen == nil {
		incoming.UGreen = cloneUGreen(existing.UGreen)
	} else {
		incoming.UGreen = mergeUGreenSensitive(existing.UGreen, incoming.UGreen)
	}
	if incoming.FnOs == nil {
		incoming.FnOs = cloneFnOs(existing.FnOs)
	} else {
		incoming.FnOs = mergeFnOsSensitive(existing.FnOs, incoming.FnOs)
	}
	return incoming
}

func SaveConfig(newConfig AppConfig) error {
	CfgMu.Lock()
	defer CfgMu.Unlock()

	Config = newConfig
	normalizeConfigLocked()
	Config.AdminPassword = ""

	data, err := json.MarshalIndent(Config, "", "  ")
	if err != nil {
		return err
	}

	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func normalizeConfigLocked() {
	if Config.IntervalMinutes <= 0 {
		Config.IntervalMinutes = DefaultIntervalMinutes
	}
	if Config.SystemStatusIntervalMinutes <= 0 {
		Config.SystemStatusIntervalMinutes = DefaultSystemStatusIntervalMinutes
	}
	if Config.LocalNasPort <= 0 {
		Config.LocalNasPort = 9999
	}
	Config.LocalNasHost, Config.LocalNasPort = normalizeLocalNasEndpoint(Config.LocalNasHost, Config.LocalNasPort)
	if strings.TrimSpace(Config.LocalNasName) == "" {
		Config.LocalNasName = "本机绿联 NAS"
	}
	if strings.TrimSpace(Config.WechatGatewayURL) == "" {
		Config.WechatGatewayURL = DefaultWechatGatewayURL
	}
	if Config.ZSpace == nil {
		Config.ZSpace = []ZSpaceConfig{}
	}
	if Config.UGreen == nil {
		Config.UGreen = []UGreenConfig{}
	}
	if Config.FnOs == nil {
		Config.FnOs = []FnOsConfig{}
	}
}

func normalizeLocalNasEndpoint(host string, port int) (string, int) {
	if port <= 0 {
		port = 9999
	}

	host = strings.TrimSpace(host)
	if host != "" && strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil {
			if parsed.Host != "" {
				host = parsed.Host
			} else if parsed.Path != "" {
				host = parsed.Path
			}
		}
	}

	host = strings.TrimSpace(host)
	if cut := strings.IndexAny(host, "/?#"); cut >= 0 {
		host = host[:cut]
	}
	host = strings.TrimSpace(host)

	if parsedHost, parsedPort, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
		if parsedPortNumber, parseErr := strconv.Atoi(parsedPort); parseErr == nil && parsedPortNumber > 0 {
			port = parsedPortNumber
		}
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}

	host = strings.TrimSpace(host)
	if host == "" {
		host = DefaultLocalNasHost
	}
	return host, port
}

func cloneZSpace(in []ZSpaceConfig) []ZSpaceConfig {
	if in == nil {
		return nil
	}
	out := make([]ZSpaceConfig, len(in))
	copy(out, in)
	return out
}

func cloneUGreen(in []UGreenConfig) []UGreenConfig {
	if in == nil {
		return nil
	}
	out := make([]UGreenConfig, len(in))
	copy(out, in)
	return out
}

func cloneFnOs(in []FnOsConfig) []FnOsConfig {
	if in == nil {
		return nil
	}
	out := make([]FnOsConfig, len(in))
	copy(out, in)
	return out
}

func mergeZSpaceSensitive(existing, incoming []ZSpaceConfig) []ZSpaceConfig {
	if len(existing) == 0 || len(incoming) == 0 {
		return incoming
	}
	existingByID := make(map[string]ZSpaceConfig, len(existing))
	for _, item := range existing {
		if id := strings.TrimSpace(item.ID); id != "" {
			existingByID[id] = item
		}
	}
	for i := range incoming {
		id := strings.TrimSpace(incoming[i].ID)
		if id == "" {
			continue
		}
		if old, ok := existingByID[id]; ok && incoming[i].Cookie == "" {
			incoming[i].Cookie = old.Cookie
		}
	}
	return incoming
}

func mergeUGreenSensitive(existing, incoming []UGreenConfig) []UGreenConfig {
	if len(existing) == 0 || len(incoming) == 0 {
		return incoming
	}
	existingByID := make(map[string]UGreenConfig, len(existing))
	for _, item := range existing {
		if id := strings.TrimSpace(item.ID); id != "" {
			existingByID[id] = item
		}
	}
	for i := range incoming {
		id := strings.TrimSpace(incoming[i].ID)
		if id == "" {
			continue
		}
		if old, ok := existingByID[id]; ok && incoming[i].Password == "" {
			incoming[i].Password = old.Password
		}
	}
	return incoming
}

func mergeFnOsSensitive(existing, incoming []FnOsConfig) []FnOsConfig {
	if len(existing) == 0 || len(incoming) == 0 {
		return incoming
	}
	existingByID := make(map[string]FnOsConfig, len(existing))
	for _, item := range existing {
		if id := strings.TrimSpace(item.ID); id != "" {
			existingByID[id] = item
		}
	}
	for i := range incoming {
		id := strings.TrimSpace(incoming[i].ID)
		if id == "" {
			continue
		}
		if old, ok := existingByID[id]; ok {
			if incoming[i].Password == "" {
				incoming[i].Password = old.Password
			}
			if incoming[i].Cookie == "" {
				incoming[i].Cookie = old.Cookie
			}
		}
	}
	return incoming
}
