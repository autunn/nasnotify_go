package app

import (
	"net/http"
	"path/filepath"
	"testing"

	"nasnotify-go/internal/config"
)

func TestSetupTokenIsStableAndResetAfterInitialSetup(t *testing.T) {
	t.Setenv("UGAPP_DATA_DIR", filepath.Join(t.TempDir(), "data"))
	resetSetupToken()
	defer resetSetupToken()

	withConfig(t, config.AppConfig{}, func() {
		token := ensureSetupToken()
		if token == "" {
			t.Fatalf("expected setup token to be generated")
		}
		if got := ensureSetupToken(); got != token {
			t.Fatalf("ensureSetupToken() = %q; want stable token %q", got, token)
		}

		req := setupRequest{
			InitToken:     token,
			AdminPassword: "strong-password",
			Config: config.AppConfig{
				IntervalMinutes:             5,
				SystemStatusIntervalMinutes: 60,
				LocalNasHost:                "192.168.1.9",
				LocalNasPort:                config.DefaultLocalNasPort,
				LocalNasUsername:            "admin",
				LocalNasPassword:            "nas-password",
				WechatGatewayURL:            config.DefaultWechatGatewayURL,
			},
		}

		if status, message := performInitialSetup(req); status != http.StatusOK || message != "" {
			t.Fatalf("performInitialSetup() = (%d, %q); want OK", status, message)
		}

		setupTokenMu.Lock()
		defer setupTokenMu.Unlock()
		if setupToken != "" {
			t.Fatalf("expected setup token to be cleared after setup, got %q", setupToken)
		}
	})
}
