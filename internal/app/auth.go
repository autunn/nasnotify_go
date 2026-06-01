package app

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

var sessionToken = randomHex(32)

const (
	headerUgreenUserID   = "Ugreen-User-ID"
	headerUgreenUserName = "Ugreen-User-Name"
	headerUgreenUserType = "Ugreen-User-Type"
)

type gatewayUser struct {
	ID   string
	Name string
	Type string
}

func checkCookie(c *gin.Context) bool {
	cookie, err := c.Cookie("auth_session")
	if err != nil {
		return false
	}
	return secureCompare(cookie, sessionToken)
}

func currentGatewayUser(c *gin.Context) (gatewayUser, bool) {
	user := gatewayUser{
		ID:   strings.TrimSpace(c.GetHeader(headerUgreenUserID)),
		Name: strings.TrimSpace(c.GetHeader(headerUgreenUserName)),
		Type: strings.ToLower(strings.TrimSpace(c.GetHeader(headerUgreenUserType))),
	}
	if user.Type == "" {
		return gatewayUser{}, false
	}
	if user.ID == "" && user.Name == "" {
		return gatewayUser{}, false
	}
	return user, true
}

func isGatewayAdminRequest(c *gin.Context) bool {
	user, ok := currentGatewayUser(c)
	return ok && user.Type == "admin"
}

func isAuthenticatedRequest(c *gin.Context) bool {
	if isGatewayAdminRequest(c) {
		return true
	}
	return checkCookie(c)
}

func setAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("auth_session", sessionToken, 86400, "/", "", isHTTPSRequest(c), true)
}

func clearAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("auth_session", "", -1, "/", "", isHTTPSRequest(c), true)
}

func isHTTPSRequest(c *gin.Context) bool {
	if c.Request.TLS != nil {
		return true
	}
	proto := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")))
	if proto == "https" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.GetHeader("X-Forwarded-Ssl")), "on")
}

func secureCompare(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func randomHex(byteLen int) string {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
