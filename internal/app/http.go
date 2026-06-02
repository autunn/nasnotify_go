package app

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"

	"nasnotify-go/internal/api"
	"nasnotify-go/internal/config"
	"nasnotify-go/internal/nas"
	"nasnotify-go/internal/notify"
)

type HTTPGateway struct {
	version string
}

func NewHTTPGateway(version string) *HTTPGateway {
	return &HTTPGateway{version: version}
}

func (h *HTTPGateway) NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	if err := r.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
	}
	for _, prefix := range AppRoutePrefixes() {
		h.registerAppRoutes(r, prefix)
	}
	r.GET("/wx-receive", api.HandleVerify)
	r.POST("/wx-receive", api.HandleMessage)
	if frontendDir := findFrontendDir(); frontendDir != "" {
		for _, prefix := range AppRoutePrefixes() {
			registerFrontendRoutes(r, prefix, frontendDir)
		}
	} else {
		for _, prefix := range AppRoutePrefixes() {
			registerFrontendEntryRoutes(r, prefix, backendRootHandler(h.version))
		}
	}
	return r
}

func (h *HTTPGateway) registerAppRoutes(r *gin.Engine, prefix string) {
	r.GET(routePath(prefix, "/healthz"), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	for _, apiPrefix := range apiRoutePrefixes(prefix) {
		h.registerAPIGroup(r.Group(apiPrefix))
	}
}

func (h *HTTPGateway) registerAPIGroup(apiGroup *gin.RouterGroup) {
	apiGroup.GET("/bootstrap", func(c *gin.Context) {
		c.JSON(http.StatusOK, buildBootstrapResponse(c, h.version))
	})
	apiGroup.POST("/setup", func(c *gin.Context) {
		var req setupRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}
		status, message := performInitialSetup(req)
		if message != "" {
			c.JSON(status, gin.H{"error": message})
			return
		}
		setAuthCookie(c)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	apiGroup.POST("/login", func(c *gin.Context) {
		if !config.IsInitialized() {
			c.JSON(http.StatusForbidden, gin.H{"error": "system not initialized"})
			return
		}
		var req loginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}
		if authenticateAdminPassword(req.Password) {
			setAuthCookie(c)
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
			return
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "password incorrect"})
	})
	apiGroup.POST("/logout", func(c *gin.Context) {
		clearAuthCookie(c)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	apiAuth := apiGroup.Group("")
	apiAuth.Use(apiAuthMiddleware())
	{
		apiAuth.GET("/wechat/status", func(c *gin.Context) { c.JSON(http.StatusOK, notify.GetClawBotStatus()) })
		apiAuth.GET("/wechat/qrcode", proxyWechatQRCode)
		apiAuth.POST("/wechat/unbind", func(c *gin.Context) {
			if err := notify.UnbindClawBot(); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		apiAuth.POST("/wechat/login/start", func(c *gin.Context) {
			if err := notify.StartClawBotLogin(); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		apiAuth.POST("/wechat/login/verify-code", func(c *gin.Context) {
			var req verifyCodeRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
				return
			}
			if err := notify.SubmitClawBotVerifyCode(req.VerifyCode); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		apiAuth.POST("/test-push", func(c *gin.Context) {
			if err := triggerTestPush(); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"success": false, "error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": true, "msg": "test push sent"})
		})
		apiAuth.POST("/health-check", func(c *gin.Context) {
			go nas.PushUGreenHealthCheck()
			c.JSON(http.StatusOK, gin.H{"success": true, "msg": "health check queued"})
		})
		apiAuth.POST("/save", func(c *gin.Context) {
			var req saveRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
				return
			}
			status, message := saveAppConfig(req)
			if message != "" {
				c.JSON(status, gin.H{"error": message})
				return
			}
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
	}
}

func apiRoutePrefixes(prefix string) []string {
	values := []string{routePath(prefix, "/api")}
	alias := strings.TrimSuffix(prefix, "/")
	if alias == "" || alias == "/" {
		alias = ""
	}
	if alias != values[0] {
		values = append(values, alias)
	}
	return values
}

func routePath(prefix, p string) string {
	if prefix == "/" || prefix == "" {
		return p
	}
	if p == "/" {
		return prefix + "/"
	}
	return prefix + p
}

func registerFrontendRoutes(r *gin.Engine, prefix, frontendDir string) {
	indexFS := http.Dir(frontendDir)
	for _, dirName := range []string{"src", "assets", "css", "js", "fonts", "img", "locale", "widgets", "trays"} {
		r.GET(routePath(prefix, "/"+dirName+"/*filepath"), serveFrontendAsset(http.Dir(filepath.Join(frontendDir, dirName))))
	}
	for _, fileName := range []string{"version.json", "favicon.ico", "manifest.json"} {
		r.GET(routePath(prefix, "/"+fileName), serveFrontendFile(indexFS, fileName))
	}
	registerFrontendIndexRoutes(r, prefix, indexFS)
}

func registerFrontendEntryRoutes(r *gin.Engine, prefix string, handler gin.HandlerFunc) {
	if prefix == "/" || prefix == "" {
		r.GET("/", handler)
		return
	}
	canonicalPrefix := strings.TrimSuffix(prefix, "/")
	r.GET(canonicalPrefix, handler)
	r.GET(canonicalPrefix+"/", handler)
}

func registerFrontendIndexRoutes(r *gin.Engine, prefix string, fileSystem http.FileSystem) {
	if prefix == "/" || prefix == "" {
		r.GET("/", serveFrontendIndex(fileSystem))
		return
	}
	canonicalPrefix := strings.TrimSuffix(prefix, "/")
	r.GET(canonicalPrefix, serveFrontendIndexWithRequestBase(fileSystem))
	r.GET(canonicalPrefix+"/", serveFrontendIndex(fileSystem))
}

func frontendEntryPaths(prefix string) []string {
	if prefix == "/" || prefix == "" {
		return []string{"/"}
	}
	canonicalPrefix := strings.TrimSuffix(prefix, "/")
	return []string{canonicalPrefix, canonicalPrefix + "/"}
}

func FrontendEntryPathsForTest(prefix string) []string {
	return frontendEntryPaths(prefix)
}

func RegisterFrontendEntryRoutesForTest(r *gin.Engine, prefix string, handler gin.HandlerFunc) {
	registerFrontendEntryRoutes(r, prefix, handler)
}

func RegisterFrontendIndexRoutesForTest(r *gin.Engine, prefix string, fileSystem http.FileSystem) {
	registerFrontendIndexRoutes(r, prefix, fileSystem)
}

func RegisterFrontendRoutesForTest(r *gin.Engine, prefix, frontendDir string) {
	registerFrontendRoutes(r, prefix, frontendDir)
}

func findFrontendDir() string {
	candidates := []string{strings.TrimSpace(os.Getenv("UGAPP_WEB_DIR"))}
	for _, baseDir := range executableBaseDirs() {
		candidates = append(candidates, filepath.Join(baseDir, "www"), filepath.Join(baseDir, "..", "www"))
	}
	candidates = append(candidates, "www", filepath.Join("packaging", "ugreen-native-app", "rootfs_common", "www"))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		indexPath := filepath.Join(candidate, "index.html")
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func executableBaseDirs() []string {
	seen := map[string]struct{}{}
	var dirs []string
	for _, candidate := range []string{os.Args[0], mustExecutablePath()} {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		dir := filepath.Clean(filepath.Dir(candidate))
		if _, exists := seen[dir]; exists {
			continue
		}
		seen[dir] = struct{}{}
		dirs = append(dirs, dir)
	}
	return dirs
}

func mustExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return path
}

func serveFrontendIndex(fileSystem http.FileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		content, err := readFrontendIndex(fileSystem)
		if err != nil {
			backendRootHandler("")(c)
			return
		}
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Data(http.StatusOK, "text/html; charset=utf-8", content)
	}
}

func serveFrontendIndexWithRequestBase(fileSystem http.FileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		content, err := readFrontendIndex(fileSystem)
		if err != nil {
			backendRootHandler("")(c)
			return
		}
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Data(http.StatusOK, "text/html; charset=utf-8", injectBaseHref(content, requestBaseHref(c)))
	}
}

func requestBaseHref(c *gin.Context) string {
	path := strings.TrimSpace(c.Request.URL.Path)
	if path == "" || path == "/" {
		return "/"
	}
	return strings.TrimRight(path, "/") + "/"
}

func readFrontendIndex(fileSystem http.FileSystem) ([]byte, error) {
	file, err := fileSystem.Open("index.html")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

func injectBaseHref(content []byte, baseHref string) []byte {
	if bytes.Contains(bytes.ToLower(content), []byte("<base ")) {
		return content
	}
	headTag := []byte("<head>")
	lower := bytes.ToLower(content)
	pos := bytes.Index(lower, headTag)
	if pos < 0 {
		return content
	}
	insert := []byte("<base href=\"" + escapeHTMLAttr(baseHref) + "\">")
	pos += len(headTag)
	out := make([]byte, 0, len(content)+len(insert))
	out = append(out, content[:pos]...)
	out = append(out, insert...)
	out = append(out, content[pos:]...)
	return out
}

func escapeHTMLAttr(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "\"", "&quot;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(s)
}

func serveFrontendAsset(fileSystem http.FileSystem) gin.HandlerFunc {
	return func(c *gin.Context) {
		assetPath := strings.TrimPrefix(c.Param("filepath"), "/")
		if assetPath == "" {
			c.Status(http.StatusNotFound)
			return
		}
		file, err := fileSystem.Open(assetPath)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		_ = file.Close()
		c.FileFromFS(assetPath, fileSystem)
	}
}

func serveFrontendFile(fileSystem http.FileSystem, fileName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := fileSystem.Open(fileName)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		_ = file.Close()
		c.FileFromFS(fileName, fileSystem)
	}
}

func apiAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !config.IsInitialized() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "system not initialized"})
			return
		}
		if !isAuthenticatedRequest(c) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func backendRootHandler(version string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":    "NasNotify",
			"version": version,
			"status":  "backend-only",
			"hint":    "Open the packaged native-app frontend entry instead of the backend service root.",
		})
	}
}

func proxyWechatQRCode(c *gin.Context) {
	qr, err := notify.GetClawBotQRCode()
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	content := strings.TrimSpace(qr.URL)
	if content == "" {
		content = strings.TrimSpace(qr.QRCode)
	}
	if content == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "wechat qrcode content unavailable"})
		return
	}
	png, err := qrcode.Encode(content, qrcode.Medium, 320)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Header("Cache-Control", "no-store, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Data(http.StatusOK, "image/png", png)
}
