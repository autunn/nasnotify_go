package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"nasnotify-go/internal/app"
)

func TestFrontendEntryPathsRoot(t *testing.T) {
	got := app.FrontendEntryPathsForTest("/")
	if len(got) != 1 || got[0] != "/" {
		t.Fatalf("frontendEntryPaths(/) = %#v; want [/] ", got)
	}
}

func TestFrontendEntryPathsPrefixed(t *testing.T) {
	got := app.FrontendEntryPathsForTest("/nasnotify")
	if len(got) != 2 || got[0] != "/nasnotify" || got[1] != "/nasnotify/" {
		t.Fatalf("frontendEntryPaths(/nasnotify) = %#v", got)
	}
}

func TestAppRoutePrefixesIncludeOfficialStylePaths(t *testing.T) {
	got := app.AppRoutePrefixes()
	want := map[string]bool{
		"/":                            true,
		"/nasnotify":                   true,
		"/com.autunn.nasnotifyfresh":   true,
		"/ugreen/:ugVersion/nasnotify": true,
		"/ugreen/:ugVersion/com.autunn.nasnotifyfresh": true,
	}

	if len(got) != len(want) {
		t.Fatalf("AppRoutePrefixes() len = %d; want %d", len(got), len(want))
	}

	for _, item := range got {
		if !want[item] {
			t.Fatalf("unexpected route prefix %q", item)
		}
		delete(want, item)
	}

	if len(want) != 0 {
		t.Fatalf("missing route prefixes: %#v", want)
	}
}

func TestRegisterFrontendEntryRoutesHandlesBothPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	app.RegisterFrontendEntryRoutesForTest(r, "/nasnotify", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/nasnotify?from=mobile", nil)
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("GET /nasnotify status = %d; want %d", w1.Code, http.StatusOK)
	}
	if got := w1.Body.String(); got != "ok" {
		t.Fatalf("GET /nasnotify body = %q; want %q", got, "ok")
	}

	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/nasnotify/", nil)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET /nasnotify/ status = %d; want %d", w2.Code, http.StatusOK)
	}
}

func TestRegisterFrontendIndexRoutesInjectsBaseForCanonicalPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<!doctype html><html><head><meta charset=\"utf-8\"></head><body><script src=\"./src/main.js\"></script></body></html>"), 0o600); err != nil {
		t.Fatalf("write index failed: %v", err)
	}

	r := gin.New()
	app.RegisterFrontendIndexRoutesForTest(r, "/nasnotify", http.Dir(tmpDir))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nasnotify", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /nasnotify status = %d; want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "<base href=\"/nasnotify/\">") {
		t.Fatalf("response body missing base href, body=%q", body)
	}
	if !strings.Contains(body, "./src/main.js") {
		t.Fatalf("response body missing script reference, body=%q", body)
	}
}

func TestRegisterFrontendIndexRoutesInjectsConcreteBaseForUGOSVersionPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(indexPath, []byte("<!doctype html><html><head><meta charset=\"utf-8\"></head><body></body></html>"), 0o600); err != nil {
		t.Fatalf("write index failed: %v", err)
	}

	r := gin.New()
	app.RegisterFrontendIndexRoutesForTest(r, "/ugreen/:ugVersion/nasnotify", http.Dir(tmpDir))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ugreen/v1/nasnotify", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /ugreen/v1/nasnotify status = %d; want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if strings.Contains(body, ":ugVersion") {
		t.Fatalf("response body contains gin param instead of concrete path, body=%q", body)
	}
	if !strings.Contains(body, "<base href=\"/ugreen/v1/nasnotify/\">") {
		t.Fatalf("response body missing concrete base href, body=%q", body)
	}
}

func TestRegisterFrontendRoutesServesVersionJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<!doctype html><html><head></head><body></body></html>"), 0o600); err != nil {
		t.Fatalf("write index failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "version.json"), []byte("{\"windowConfig\":{\"width\":1280}}"), 0o600); err != nil {
		t.Fatalf("write version.json failed: %v", err)
	}

	r := gin.New()
	app.RegisterFrontendRoutesForTest(r, "/nasnotify", tmpDir)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/nasnotify/version.json", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /nasnotify/version.json status = %d; want %d", w.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "{\"windowConfig\":{\"width\":1280}}" {
		t.Fatalf("GET /nasnotify/version.json body = %q", got)
	}
}
