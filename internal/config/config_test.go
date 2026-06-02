package config

import (
	"path/filepath"
	"testing"
)

func TestNormalizeConfigLockedDefaults(t *testing.T) {
	previous := Config
	defer func() { Config = previous }()

	Config = AppConfig{}

	normalizeConfigLocked()

	if Config.IntervalMinutes != DefaultIntervalMinutes {
		t.Fatalf("expected notification interval default %v, got %v", DefaultIntervalMinutes, Config.IntervalMinutes)
	}
	if Config.SystemStatusIntervalMinutes != DefaultSystemStatusIntervalMinutes {
		t.Fatalf("expected system status interval default %v, got %v", DefaultSystemStatusIntervalMinutes, Config.SystemStatusIntervalMinutes)
	}
	if Config.LocalNasPort != 9999 {
		t.Fatalf("expected local nas port default 9999, got %d", Config.LocalNasPort)
	}
	if Config.LocalNasHost != DefaultLocalNasHost {
		t.Fatalf("expected local nas host default %q, got %q", DefaultLocalNasHost, Config.LocalNasHost)
	}
	if Config.LocalNasName != "本机绿联 NAS" {
		t.Fatalf("expected local nas name default, got %q", Config.LocalNasName)
	}
	if Config.WechatGatewayURL != DefaultWechatGatewayURL {
		t.Fatalf("expected gateway url default %q, got %q", DefaultWechatGatewayURL, Config.WechatGatewayURL)
	}
}

func TestNormalizeConfigLockedCleansLocalNasEndpoint(t *testing.T) {
	previous := Config
	defer func() { Config = previous }()

	Config = AppConfig{
		LocalNasHost: "http://192.168.1.9:10000/ugreen/",
		LocalNasMac:  "aa-bb-cc-dd-ee-ff",
		LocalNasPort: 9999,
	}

	normalizeConfigLocked()

	if Config.LocalNasHost != "192.168.1.9" {
		t.Fatalf("expected host to be cleaned, got %q", Config.LocalNasHost)
	}
	if Config.LocalNasPort != 10000 {
		t.Fatalf("expected port parsed from host, got %d", Config.LocalNasPort)
	}
	if Config.LocalNasMac != "AA:BB:CC:DD:EE:FF" {
		t.Fatalf("expected normalized MAC, got %q", Config.LocalNasMac)
	}
}

func TestNormalizeMACAddress(t *testing.T) {
	tests := []struct {
		value string
		want  string
		valid bool
	}{
		{value: "aa:bb:cc:dd:ee:ff", want: "AA:BB:CC:DD:EE:FF", valid: true},
		{value: "AABBCCDDEEFF", want: "AA:BB:CC:DD:EE:FF", valid: true},
		{value: "aabb.ccdd.eeff", want: "AA:BB:CC:DD:EE:FF", valid: true},
		{value: "invalid", want: "invalid", valid: false},
	}

	for _, tt := range tests {
		if got := NormalizeMACAddress(tt.value); got != tt.want {
			t.Fatalf("NormalizeMACAddress(%q) = %q, want %q", tt.value, got, tt.want)
		}
		if got := IsMACAddress(tt.value); got != tt.valid {
			t.Fatalf("IsMACAddress(%q) = %v, want %v", tt.value, got, tt.valid)
		}
	}
}

func TestMergeWithExistingSensitiveFields(t *testing.T) {
	existing := AppConfig{
		WechatGatewaySecret: "gateway-secret",
		WechatBindingCode:   "ABC123",
		WechatBound:         true,
		WechatBoundAt:       "2026-05-30",
		LocalNasPassword:    "nas-password",
	}
	incoming := AppConfig{}

	merged := MergeWithExistingSensitiveFields(existing, incoming)

	if got := merged.WechatGatewaySecret; got != "gateway-secret" {
		t.Fatalf("expected gateway secret to be preserved, got %q", got)
	}
	if got := merged.WechatBindingCode; got != "ABC123" {
		t.Fatalf("expected binding code to be preserved, got %q", got)
	}
	if !merged.WechatBound {
		t.Fatalf("expected bound flag to be preserved")
	}
	if got := merged.WechatBoundAt; got != "2026-05-30" {
		t.Fatalf("expected bound time to be preserved, got %q", got)
	}
	if got := merged.LocalNasPassword; got != "nas-password" {
		t.Fatalf("expected NAS password to be preserved, got %q", got)
	}
}

func TestSanitizedConfigForWebClearsSecrets(t *testing.T) {
	previous := Config
	defer func() { Config = previous }()

	Config = AppConfig{
		AdminPasswordHash:   "hash",
		AdminPassword:       "plain",
		WechatGatewaySecret: "gateway-secret",
		WechatBindingCode:   "ABC123",
		WechatBound:         true,
		LocalNasPassword:    "nas-password",
	}

	sanitized := SanitizedConfigForWeb()

	if sanitized.AdminPasswordHash != "" || sanitized.AdminPassword != "" {
		t.Fatalf("expected admin password fields to be cleared")
	}
	if got := sanitized.WechatGatewaySecret; got != "" {
		t.Fatalf("expected gateway secret to be cleared, got %q", got)
	}
	if got := sanitized.LocalNasPassword; got != "" {
		t.Fatalf("expected NAS password to be cleared, got %q", got)
	}
	if got := sanitized.WechatBindingCode; got != "ABC123" {
		t.Fatalf("expected binding code to remain visible, got %q", got)
	}
	if !sanitized.WechatBound {
		t.Fatalf("expected bound flag to remain visible")
	}
}

func TestConfigPathUsesUGAppDataDirWhenAvailable(t *testing.T) {
	t.Setenv("UGAPP_DATA_DIR", filepath.Join("runtime", "data"))
	got := configPath()
	want := filepath.Join("runtime", "data", "config", "config.json")
	if got != want {
		t.Fatalf("configPath() = %q; want %q", got, want)
	}
}
