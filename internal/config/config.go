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
	"unicode"

	"nasnotify-go/internal/appenv"
)

const (
	DefaultIntervalMinutes             = 5
	DefaultSystemStatusIntervalMinutes = 60
	DefaultWechatGatewayURL            = "http://127.0.0.1:5091"
	DefaultLocalNasHost                = "127.0.0.1"
	DefaultLocalNasPort                = 9999
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
	LocalNasMac      string `json:"local_nas_mac"`
	LocalNasPort     int    `json:"local_nas_port"`
	LocalNasUsername string `json:"local_nas_username"`
	LocalNasPassword string `json:"local_nas_password"`

	// Legacy UGreen devices are kept only for compatibility with older config
	// files. New UI flows use the single local NAS fields above.
	UGreen []UGreenConfig `json:"ugreen,omitempty"`
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
		Config = defaultAppConfig()
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Println("config not found, entering first-time setup mode")
		Config = defaultAppConfig()
		return
	}

	if err := json.Unmarshal(data, &Config); err != nil {
		backupPath := path + ".invalid"
		_ = os.Rename(path, backupPath)
		log.Printf("parse config failed, moved invalid config to %s and entered setup mode: %v", backupPath, err)
		Config = defaultAppConfig()
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
	snapshot.UGreen = cloneUGreen(Config.UGreen)
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
	for i := range snapshot.UGreen {
		snapshot.UGreen[i].Password = ""
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
	if incoming.UGreen == nil {
		incoming.UGreen = cloneUGreen(existing.UGreen)
	} else {
		incoming.UGreen = mergeUGreenSensitive(existing.UGreen, incoming.UGreen)
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

func defaultAppConfig() AppConfig {
	return AppConfig{
		IntervalMinutes:             DefaultIntervalMinutes,
		SystemStatusIntervalMinutes: DefaultSystemStatusIntervalMinutes,
		WechatGatewayURL:            DefaultWechatGatewayURL,
		LocalNasName:                "本机绿联 NAS",
		LocalNasHost:                DefaultLocalNasHost,
		LocalNasPort:                DefaultLocalNasPort,
	}
}

func normalizeConfigLocked() {
	if Config.IntervalMinutes <= 0 {
		Config.IntervalMinutes = DefaultIntervalMinutes
	}
	if Config.SystemStatusIntervalMinutes <= 0 {
		Config.SystemStatusIntervalMinutes = DefaultSystemStatusIntervalMinutes
	}
	if Config.LocalNasPort <= 0 {
		Config.LocalNasPort = DefaultLocalNasPort
	}
	Config.LocalNasHost, Config.LocalNasPort = normalizeLocalNasEndpoint(Config.LocalNasHost, Config.LocalNasPort)
	if strings.TrimSpace(Config.LocalNasName) == "" {
		Config.LocalNasName = "本机绿联 NAS"
	}
	Config.LocalNasMac = NormalizeMACAddress(Config.LocalNasMac)
	if strings.TrimSpace(Config.WechatGatewayURL) == "" {
		Config.WechatGatewayURL = DefaultWechatGatewayURL
	}
	if Config.UGreen == nil {
		Config.UGreen = []UGreenConfig{}
	}
}

func NormalizeMACAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var hexDigits []rune
	for _, ch := range value {
		switch {
		case ch == ':' || ch == '-' || ch == '.':
			continue
		case unicode.Is(unicode.ASCII_Hex_Digit, ch):
			hexDigits = append(hexDigits, unicode.ToUpper(ch))
		default:
			return value
		}
	}
	if len(hexDigits) != 12 {
		return value
	}

	parts := make([]string, 0, 6)
	for i := 0; i < len(hexDigits); i += 2 {
		parts = append(parts, string(hexDigits[i:i+2]))
	}
	return strings.Join(parts, ":")
}

func IsMACAddress(value string) bool {
	normalized := NormalizeMACAddress(value)
	if len(normalized) != 17 {
		return false
	}
	for i, ch := range normalized {
		if i%3 == 2 {
			if ch != ':' {
				return false
			}
			continue
		}
		if !unicode.Is(unicode.ASCII_Hex_Digit, ch) {
			return false
		}
	}
	return true
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

func cloneUGreen(in []UGreenConfig) []UGreenConfig {
	if in == nil {
		return nil
	}
	out := make([]UGreenConfig, len(in))
	copy(out, in)
	return out
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
