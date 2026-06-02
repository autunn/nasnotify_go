package app

import (
	"net/http"
	"testing"

	"nasnotify-go/internal/config"
)

func TestCommandMatchesAliases(t *testing.T) {
	tests := []struct {
		text    string
		aliases []string
		want    bool
	}{
		{text: "帮助", aliases: []string{"菜单", "帮助", "help"}, want: true},
		{text: "query deck", aliases: []string{"querydeck", "查询"}, want: true},
		{text: "巡检", aliases: []string{"巡检", "health"}, want: true},
		{text: "硬盘", aliases: []string{"存储", "硬盘", "storage"}, want: true},
		{text: "unknown", aliases: []string{"状态"}, want: false},
	}

	for _, tt := range tests {
		normalized := normalizeClawBotCommand(tt.text)
		compact := removeCommandSpaces(normalized)
		if got := commandMatches(normalized, compact, tt.aliases...); got != tt.want {
			t.Fatalf("commandMatches(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestWakeCommandParsing(t *testing.T) {
	tests := []struct {
		text       string
		wantWake   bool
		wantTarget string
	}{
		{text: "唤醒", wantWake: true, wantTarget: ""},
		{text: "唤醒 书房", wantWake: true, wantTarget: "书房"},
		{text: "wol 书房", wantWake: true, wantTarget: "书房"},
		{text: "wake lounge", wantWake: true, wantTarget: "lounge"},
		{text: "状态", wantWake: false, wantTarget: ""},
	}

	for _, tt := range tests {
		normalized := normalizeClawBotCommand(tt.text)
		compact := removeCommandSpaces(normalized)
		if got := isWakeCommand(normalized, compact); got != tt.wantWake {
			t.Fatalf("isWakeCommand(%q) = %v, want %v", tt.text, got, tt.wantWake)
		}
		if !tt.wantWake {
			continue
		}
		if got := wakeTargetFromCommand(tt.text, normalized); got != tt.wantTarget {
			t.Fatalf("wakeTargetFromCommand(%q) = %q, want %q", tt.text, got, tt.wantTarget)
		}
	}
}

func TestValidateAppConfig(t *testing.T) {
	base := config.AppConfig{
		IntervalMinutes:             5,
		SystemStatusIntervalMinutes: 60,
		LocalNasHost:                "192.168.1.9",
		LocalNasPort:                9999,
		LocalNasUsername:            "admin",
		LocalNasPassword:            "password",
		WechatGatewayURL:            config.DefaultWechatGatewayURL,
	}

	if status, message := validateAppConfig(base, true); status != http.StatusOK || message != "" {
		t.Fatalf("validateAppConfig(valid) = (%d, %q), want OK", status, message)
	}

	invalidPort := base
	invalidPort.LocalNasPort = 70000
	if status, message := validateAppConfig(invalidPort, true); status != http.StatusBadRequest || message == "" {
		t.Fatalf("validateAppConfig(invalid port) = (%d, %q), want bad request", status, message)
	}

	invalidMAC := base
	invalidMAC.LocalNasMac = "not-a-mac"
	if status, message := validateAppConfig(invalidMAC, true); status != http.StatusBadRequest || message == "" {
		t.Fatalf("validateAppConfig(invalid mac) = (%d, %q), want bad request", status, message)
	}

	missingPassword := base
	missingPassword.LocalNasPassword = ""
	if status, message := validateAppConfig(missingPassword, true); status != http.StatusBadRequest || message == "" {
		t.Fatalf("validateAppConfig(missing password) = (%d, %q), want bad request", status, message)
	}
	if status, message := validateAppConfig(missingPassword, false); status != http.StatusOK || message != "" {
		t.Fatalf("validateAppConfig(existing password preserved) = (%d, %q), want OK", status, message)
	}
}
