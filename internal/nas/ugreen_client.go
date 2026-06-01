package nas

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nasnotify-go/internal/appenv"
	"nasnotify-go/internal/crypto"
)

func newUGreenHTTPClient(timeout time.Duration, jar http.CookieJar) *http.Client {
	return &http.Client{
		Jar:     jar,
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			Proxy:           http.ProxyFromEnvironment,
		},
	}
}

type UGreenAuthInfo struct {
	TokenID   string `json:"token_id"`
	Token     string `json:"token"`
	PublicKey string `json:"public_key"`
	CookieStr string `json:"cookie_str"`
}

type UGreenLoginResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		PublicKey string `json:"public_key"`
		Token     string `json:"token"`
		TokenID   string `json:"token_id"`
	} `json:"data"`
}

func requestUGreenDeepAPI(authInfo *UGreenAuthInfo, ip string, port int, useSSL bool, method string, apiPath string, params map[string]string, body map[string]interface{}) ([]byte, error) {
	protocol := "http"
	if useSSL {
		protocol = "https"
	}
	query := url.Values{}
	for key, value := range params {
		query.Set(key, value)
	}
	urlStr := fmt.Sprintf("%s://%s:%d%s", protocol, ip, port, apiPath)
	if encoded := query.Encode(); encoded != "" {
		urlStr += "?" + encoded
	}

	aesKey := randomAESKey()
	var bodyReader io.Reader
	if body != nil {
		bodyJSON, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		encryptedBody, err := crypto.AESGCMEncrypt(aesKey, string(bodyJSON))
		if err != nil {
			return nil, err
		}
		encReq := map[string]string{"encrypt_req_body": encryptedBody}
		encReqJSON, err := json.Marshal(encReq)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(encReqJSON)
	}

	req, err := http.NewRequest(method, urlStr, bodyReader)
	if err != nil {
		return nil, err
	}

	securityCode, err := crypto.RsaEncrypt(authInfo.PublicKey, aesKey)
	if err != nil {
		return nil, err
	}
	ugreenToken, err := crypto.RsaEncrypt(authInfo.PublicKey, authInfo.Token)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Client-Id", "cli-go-tool")
	req.Header.Set("Client-Version", "77682")
	req.Header.Set("UG-Agent", "PC/WEB")
	req.Header.Set("X-Specify-Language", "zh-CN")
	req.Header.Set("X-Ugreen-Security-Code", securityCode)
	req.Header.Set("X-Ugreen-Security-Key", crypto.MD5Hex(authInfo.Token))
	req.Header.Set("X-Ugreen-Token", ugreenToken)
	if authInfo.CookieStr != "" {
		req.Header.Set("Cookie", authInfo.CookieStr)
	}

	client := newUGreenHTTPClient(10*time.Second, nil)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("api http status %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	rawStr := string(raw)

	decrypted, err := crypto.AESGCMDecrypt(aesKey, rawStr)
	if err == nil {
		var apiResp struct {
			Code int             `json:"code"`
			Msg  string          `json:"msg"`
			Data json.RawMessage `json:"data"`
		}
		if jsonErr := json.Unmarshal([]byte(decrypted), &apiResp); jsonErr == nil {
			if apiResp.Code != 200 && apiResp.Code != 0 {
				return nil, fmt.Errorf("api error: %v, %s", apiResp.Code, apiResp.Msg)
			}
			if len(apiResp.Data) > 0 {
				return apiResp.Data, nil
			}
		}
		return []byte(decrypted), nil
	}

	var encResp struct {
		EncryptRespBody string `json:"encrypt_resp_body"`
	}
	if json.Unmarshal(raw, &encResp) == nil && encResp.EncryptRespBody != "" {
		dec, decErr := crypto.AESGCMDecrypt(aesKey, encResp.EncryptRespBody)
		if decErr == nil {
			var apiResp struct {
				Code int             `json:"code"`
				Msg  string          `json:"msg"`
				Data json.RawMessage `json:"data"`
			}
			if jsonErr := json.Unmarshal([]byte(dec), &apiResp); jsonErr == nil {
				if apiResp.Code != 200 && apiResp.Code != 0 {
					return nil, fmt.Errorf("api error: %v, %s", apiResp.Code, apiResp.Msg)
				}
				if len(apiResp.Data) > 0 {
					return apiResp.Data, nil
				}
			}
			return []byte(dec), nil
		}
	}

	var apiResp struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &apiResp); err == nil {
		if apiResp.Code != 200 && apiResp.Code != 0 {
			return nil, fmt.Errorf("api error: %v, %s", apiResp.Code, apiResp.Msg)
		}
		var dataFields struct {
			EncryptRespBody string `json:"encrypt_resp_body"`
		}
		if json.Unmarshal(apiResp.Data, &dataFields) == nil && dataFields.EncryptRespBody != "" {
			dec, decErr := crypto.AESGCMDecrypt(aesKey, dataFields.EncryptRespBody)
			if decErr == nil {
				return []byte(dec), nil
			}
		}
		if len(apiResp.Data) > 0 {
			return apiResp.Data, nil
		}
	}

	return nil, fmt.Errorf("failed to parse response")
}

func ensureAuth(username, password, ip string, port int, useSSL bool) *UGreenAuthInfo {
	authInfo, _ := ensureAuthWithError(username, password, ip, port, useSSL)
	return authInfo
}

func ensureAuthWithError(username, password, ip string, port int, useSSL bool) (*UGreenAuthInfo, error) {
	authInfo := loadUGreenAuthInfo(ip, port)
	if authInfo != nil && authInfo.PublicKey != "" && authInfo.CookieStr != "" {
		return authInfo, nil
	}
	newAuth, err := loginUGreen(username, password, ip, port, useSSL)
	if err != nil {
		log.Printf("[绿联] %s:%d 登录失败: %v\n", ip, port, err)
		return nil, err
	}
	return newAuth, nil
}

func refreshUGreenAuth(username, password, ip string, port int, useSSL bool) *UGreenAuthInfo {
	newAuth, err := loginUGreen(username, password, ip, port, useSSL)
	if err != nil {
		log.Printf("[ugreen] %s:%d re-login failed: %v\n", ip, port, err)
		return nil
	}
	return newAuth
}

func loginUGreen(username, password, ip string, port int, useSSL bool) (*UGreenAuthInfo, error) {
	protocol := "http"
	if useSSL {
		protocol = "https"
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	client := newUGreenHTTPClient(10*time.Second, jar)

	encPassword := password
	if username != "admin" {
		checkURL := fmt.Sprintf("%s://%s:%d/ugreen/v1/verify/check", protocol, ip, port)
		checkReqBody, err := json.Marshal(map[string]string{"username": username})
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequest("POST", checkURL, bytes.NewBuffer(checkReqBody))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		if resp, err := client.Do(req); err == nil {
			rsaToken := resp.Header.Get("x-rsa-token")
			resp.Body.Close()
			if rsaToken != "" {
				if pemBytes, err := base64.StdEncoding.DecodeString(rsaToken); err == nil {
					if enc, err := crypto.RsaEncrypt(string(pemBytes), password); err == nil {
						encPassword = enc
					}
				}
			}
		}
	}

	loginURL := fmt.Sprintf("%s://%s:%d/ugreen/v1/verify/login", protocol, ip, port)
	loginPayload := map[string]interface{}{"username": username, "password": encPassword, "keepalive": true, "is_simple": true, "otp": false}
	loginReqBody, err := json.Marshal(loginPayload)
	if err != nil {
		return nil, err
	}
	req2, err := http.NewRequest("POST", loginURL, bytes.NewBuffer(loginReqBody))
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Client-Id", "cli-go-tool")
	req2.Header.Set("Client-Version", "77682")
	req2.Header.Set("UG-Agent", "PC/WEB")
	req2.Header.Set("x-specify-language", "zh-CN")

	resp2, err := client.Do(req2)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, err
	}
	if resp2.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("login http status %d: %s", resp2.StatusCode, strings.TrimSpace(string(body2)))
	}
	var loginResp UGreenLoginResp
	if err := json.Unmarshal(body2, &loginResp); err != nil {
		return nil, err
	}
	if loginResp.Code != 200 {
		msg := strings.TrimSpace(loginResp.Msg)
		if msg == "" {
			msg = "unknown login error"
		}
		return nil, fmt.Errorf("login failed: code %d, msg: %s", loginResp.Code, msg)
	}

	pubKeyBytes, err := base64.StdEncoding.DecodeString(loginResp.Data.PublicKey)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse(fmt.Sprintf("%s://%s:%d/ugreen/", protocol, ip, port))
	var cookiePairs []string
	for _, c := range jar.Cookies(u) {
		cookiePairs = append(cookiePairs, c.Name+"="+c.Value)
	}
	cookieStr := strings.Join(cookiePairs, "; ")

	authInfo := &UGreenAuthInfo{
		TokenID:   loginResp.Data.TokenID,
		Token:     loginResp.Data.Token,
		PublicKey: string(pubKeyBytes),
		CookieStr: cookieStr,
	}
	if err := saveUGreenAuthInfo(ip, port, authInfo); err != nil {
		log.Printf("[ugreen] save auth cache failed for %s:%d: %v", ip, port, err)
	}
	return authInfo, nil
}

func loadUGreenAuthInfo(ip string, port int) *UGreenAuthInfo {
	file := ugreenAuthInfoPath(ip, port)
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	var auth UGreenAuthInfo
	if err := json.Unmarshal(data, &auth); err != nil {
		log.Printf("[ugreen] parse auth cache failed for %s:%d: %v", ip, port, err)
		return nil
	}
	return &auth
}

func saveUGreenAuthInfo(ip string, port int, auth *UGreenAuthInfo) error {
	file := ugreenAuthInfoPath(ip, port)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(auth)
	if err != nil {
		return err
	}
	return os.WriteFile(file, data, 0600)
}

func ugreenAuthInfoPath(ip string, port int) string {
	return filepath.Join(appenv.DataDir(), "token", fmt.Sprintf("%s_%d.config", ip, port))
}

func formatUGreenAuthError(err error) string {
	if err == nil {
		return "未返回具体错误。请确认 NAS 端口、账号、密码和 HTTP/HTTPS 设置。"
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "未返回具体错误。请确认 NAS 端口、账号、密码和 HTTP/HTTPS 设置。"
	}
	return "实际错误：" + message + "\n请确认 NAS 端口、账号、密码和 HTTP/HTTPS 设置。"
}

func randomAESKey() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "0123456789abcdef"
	}
	return hex.EncodeToString(buf)
}
