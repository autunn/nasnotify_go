package wechatgateway

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTargetPeerAndContextUsesBoundPair(t *testing.T) {
	s := &Service{
		runtime: runtimeState{
			LatestPeerUserID:   "other@im.wechat",
			LatestContextToken: "other-token",
			BoundPeerUserID:    "bound@im.wechat",
			BoundContextToken:  "bound-token",
		},
	}

	peer, contextToken := s.targetPeerAndContextLocked()
	if peer != "bound@im.wechat" || contextToken != "bound-token" {
		t.Fatalf("targetPeerAndContextLocked() = (%q, %q); want bound peer/context", peer, contextToken)
	}
}

func TestTargetPeerAndContextUsesFreshBoundLatestContext(t *testing.T) {
	s := &Service{
		runtime: runtimeState{
			LatestPeerUserID:   "bound@im.wechat",
			LatestContextToken: "fresh-token",
			BoundPeerUserID:    "bound@im.wechat",
			BoundContextToken:  "old-token",
		},
	}

	peer, contextToken := s.targetPeerAndContextLocked()
	if peer != "bound@im.wechat" || contextToken != "fresh-token" {
		t.Fatalf("targetPeerAndContextLocked() = (%q, %q); want fresh bound context", peer, contextToken)
	}
}

func TestDoJSONRequestReturnsBusinessError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":-14,"errcode":0,"errmsg":"session timeout"}`))
	}))
	defer server.Close()

	s := &Service{}
	err := s.doJSONRequest(http.MethodPost, server.URL, "/ilink/bot/getupdates", "token", map[string]any{}, time.Second, nil)
	if !isSessionExpiredError(err) {
		t.Fatalf("isSessionExpiredError(%v) = false; want true", err)
	}

	var businessErr *upstreamBusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("doJSONRequest error type = %T; want *upstreamBusinessError", err)
	}
	if businessErr.Ret != sessionExpiredErrCode {
		t.Fatalf("businessErr.Ret = %d; want %d", businessErr.Ret, sessionExpiredErrCode)
	}
}

func TestSessionActiveLockedHonorsPauseWindow(t *testing.T) {
	now := time.Date(2026, 6, 3, 10, 0, 0, 0, time.Local)
	s := &Service{account: accountState{Token: "token"}}
	if !s.sessionActiveLocked(now) {
		t.Fatalf("sessionActiveLocked() = false; want true with token and no expired marker")
	}

	s.runtime.SessionExpiredAt = formatStateTime(now.Add(-time.Minute))
	if !s.sessionActiveLocked(now) {
		t.Fatalf("sessionActiveLocked() = false; want true when only diagnostic expired time is set")
	}

	s.runtime.SessionPausedUntil = formatStateTime(now.Add(time.Minute))
	if s.sessionActiveLocked(now) {
		t.Fatalf("sessionActiveLocked() = true; want false while paused")
	}

	s.runtime.SessionPausedUntil = formatStateTime(now.Add(-time.Minute))
	if !s.sessionActiveLocked(now) {
		t.Fatalf("sessionActiveLocked() = false; want true after pause window")
	}
}

func TestBuildHeadersUsesOfficialClientVersion(t *testing.T) {
	headers := buildHeaders("")
	if got := headers["iLink-App-ClientVersion"]; got != defaultILinkClientVer {
		t.Fatalf("iLink-App-ClientVersion = %q; want %q", got, defaultILinkClientVer)
	}
	if defaultILinkClientVer != "132099" {
		t.Fatalf("defaultILinkClientVer = %q; want official 2.4.3 encoded version 132099", defaultILinkClientVer)
	}
}
