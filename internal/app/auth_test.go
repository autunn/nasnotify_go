package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"nasnotify-go/internal/config"
)

func withConfig(t *testing.T, cfg config.AppConfig, fn func()) {
	t.Helper()

	config.CfgMu.Lock()
	original := config.Config
	config.Config = cfg
	config.CfgMu.Unlock()

	defer func() {
		config.CfgMu.Lock()
		config.Config = original
		config.CfgMu.Unlock()
	}()

	fn()
}

func TestCurrentGatewayUserParsesOfficialHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(headerUgreenUserID, "1001")
	req.Header.Set(headerUgreenUserName, "alice")
	req.Header.Set(headerUgreenUserType, "admin")
	c.Request = req

	user, ok := currentGatewayUser(c)
	if !ok {
		t.Fatalf("currentGatewayUser() ok = false; want true")
	}
	if user.ID != "1001" || user.Name != "alice" || user.Type != "admin" {
		t.Fatalf("currentGatewayUser() = %#v", user)
	}
}

func TestAPIAuthMiddlewareAllowsGatewayAdmin(t *testing.T) {
	withConfig(t, config.AppConfig{AdminPasswordHash: "configured"}, func() {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(apiAuthMiddleware())
		r.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set(headerUgreenUserID, "1001")
		req.Header.Set(headerUgreenUserType, "admin")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Fatalf("GET /protected status = %d; want %d", w.Code, http.StatusNoContent)
		}
	})
}

func TestAPIAuthMiddlewareRejectsNonAdminGatewayUser(t *testing.T) {
	withConfig(t, config.AppConfig{AdminPasswordHash: "configured"}, func() {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(apiAuthMiddleware())
		r.GET("/protected", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set(headerUgreenUserID, "1002")
		req.Header.Set(headerUgreenUserType, "users")
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("GET /protected status = %d; want %d", w.Code, http.StatusUnauthorized)
		}
	})
}

func TestRegisterAppRoutesSupportsProxyAndAliasAPIPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewHTTPGateway("test-version").registerAppRoutes(r, "/nasnotify")

	for _, path := range []string{"/nasnotify/api/bootstrap", "/nasnotify/bootstrap"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d; want %d", path, w.Code, http.StatusOK)
		}
	}
}
