package splunk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeBrowser struct {
	session BrowserSession
	err     error
	calls   *atomic.Int32
	options *BrowserAuthOptions
}

func (b fakeBrowser) Authenticate(_ context.Context, _ Target, _ string, options BrowserAuthOptions) (BrowserSession, error) {
	if b.calls != nil {
		b.calls.Add(1)
	}
	if b.options != nil {
		*b.options = options
	}
	return b.session, b.err
}

func TestNormalizeTargetAcceptsWebAndInfersAPI(t *testing.T) {
	target, err := NormalizeTarget("https://splunk.example.com:8000/en-US/app/search")
	if err != nil {
		t.Fatal(err)
	}
	if target.Key != "https://splunk.example.com:8000" {
		t.Fatalf("unexpected key: %s", target.Key)
	}
	if target.API != "https://splunk.example.com:8089" {
		t.Fatalf("unexpected api base: %s", target.API)
	}
}

func TestNormalizeTargetAcceptsAPIAndInfersWeb(t *testing.T) {
	target, err := NormalizeTarget("https://splunk.example.com:8089/services/server/info")
	if err != nil {
		t.Fatal(err)
	}
	if target.Web != "https://splunk.example.com:8000" {
		t.Fatalf("unexpected web base: %s", target.Web)
	}
	if !containsString(target.Bases, "https://splunk.example.com:8000") {
		t.Fatalf("expected web alias in bases: %#v", target.Bases)
	}
}

func TestStatusReportsRemoteInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	dir := t.TempDir()
	store := NewFileStore(dir)
	target := mustTarget(t, server.URL)
	if err := store.Set(target, AuthRecord{Method: MethodWeb, Cookies: []Cookie{{Name: "splunkd", Value: "bad"}}}); err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{
		Store:  store,
		Client: NewClient(server.Client(), false),
	})
	status, err := service.Status(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status.RemoteValid {
		t.Fatalf("expected remote invalid status: %+v", status)
	}
}

func TestStatusReportsExpiredCredentialsWithStructuredFields(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	target := mustTarget(t, "https://splunk.example.com")
	expiresAt := time.Now().Unix() - 60
	if err := store.Set(target, AuthRecord{Method: MethodWeb, ExpiresAt: &expiresAt}); err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{
		Store:  store,
		Client: NewClient(nil, false),
	})

	status, err := service.Status(context.Background(), target.Key)
	if err != nil {
		t.Fatal(err)
	}
	if status.LocalValid || status.Message != "stored credentials are expired" {
		t.Fatalf("expected expired local status, got %+v", status)
	}
	if status.ErrorCode != "credentials_expired" || status.Operation != "auth_status" {
		t.Fatalf("expected structured expired status, got %+v", status)
	}
	if status.Retryable == nil || *status.Retryable {
		t.Fatalf("expected expired status to be non-retryable without login, got %+v", status)
	}
	if status.DiagnosticHint == "" || !strings.Contains(status.DiagnosticHint, "splsearch auth login --url="+target.Key) {
		t.Fatalf("expected login diagnostic hint, got %+v", status)
	}
}

func TestStatusAllIncludesExpiredCredentials(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	target := mustTarget(t, "https://splunk.example.com")
	expiresAt := time.Now().Unix() - 60
	if err := store.Set(target, AuthRecord{Method: MethodWeb, ExpiresAt: &expiresAt}); err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{
		Store:  store,
		Client: NewClient(nil, false),
	})

	statuses, err := service.StatusAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected expired status in inventory, got %+v", statuses)
	}
	if statuses[0].URL != target.Key || statuses[0].LocalValid || statuses[0].ErrorCode != "credentials_expired" {
		t.Fatalf("expected expired credential inventory entry, got %+v", statuses[0])
	}
}

func TestLogoutRemovesRecordByAlias(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	webTarget := mustTarget(t, "https://splunk.example.com:8000")
	if err := store.Set(webTarget, AuthRecord{Method: MethodWeb, Cookies: []Cookie{{Name: "splunkd", Value: "x"}}}); err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{Store: store, Client: NewClient(nil, false)})
	result, err := service.Logout("https://splunk.example.com:8089")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Removed {
		t.Fatalf("expected record removal by alias")
	}
	record, err := store.Get(webTarget)
	if err != nil {
		t.Fatal(err)
	}
	if record != nil {
		t.Fatalf("expected no record, got %+v", record)
	}
}

func TestSetReplacesExistingAliasRecords(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	webTarget := mustTarget(t, "https://splunk.example.com:8000")
	apiTarget := mustTarget(t, "https://splunk.example.com:8089")
	if err := store.Set(webTarget, AuthRecord{
		Method:  MethodWeb,
		Cookies: []Cookie{{Name: "splunkd", Value: "old"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(apiTarget, AuthRecord{
		Method:  MethodWeb,
		Cookies: []Cookie{{Name: "splunkd", Value: "new"}},
	}); err != nil {
		t.Fatal(err)
	}

	record, err := store.Get(webTarget)
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || len(record.Cookies) != 1 || record.Cookies[0].Value != "new" {
		t.Fatalf("expected refreshed alias record, got %+v", record)
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].URL != apiTarget.Key {
		t.Fatalf("expected stale alias key to be removed, got %+v", records)
	}
}

func TestWebLoginStoresCookiesAfterBrowserSuccess(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	dir := t.TempDir()
	store := NewFileStore(dir)
	service := NewAuthService(AuthServiceOptions{
		Store:  store,
		Client: NewClient(server.Client(), false),
		Browser: fakeBrowser{session: BrowserSession{
			Cookies: []Cookie{{Name: "splunkd", Value: "abc", Domain: strings.TrimPrefix(server.URL, "http://"), Path: "/"}},
		}},
	})
	result, err := service.Login(context.Background(), LoginRequest{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Validated {
		t.Fatalf("unexpected result: %+v", result)
	}
	if hits.Load() != 0 {
		t.Fatalf("login should not run a remote validation probe, got %d requests", hits.Load())
	}
	record, err := store.Get(mustTarget(t, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || len(record.Cookies) != 1 || record.Cookies[0].Value != "abc" {
		t.Fatalf("expected stored browser cookies, got %+v", record)
	}
}

func TestWebLoginPassesInsecureToBrowser(t *testing.T) {
	var captured BrowserAuthOptions
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	service := NewAuthService(AuthServiceOptions{
		Store:  NewFileStore(t.TempDir()),
		Client: NewClient(server.Client(), true),
		Browser: fakeBrowser{
			session: BrowserSession{
				Cookies: []Cookie{{Name: "splunkd", Value: "abc", Domain: strings.TrimPrefix(server.URL, "http://"), Path: "/"}},
			},
			options: &captured,
		},
	})
	if _, err := service.Login(context.Background(), LoginRequest{URL: server.URL, Insecure: true}); err != nil {
		t.Fatal(err)
	}
	if !captured.Insecure {
		t.Fatalf("expected --insecure to be passed to browser auth")
	}
}

func TestWebLoginSkipsBrowserWhenStoredSessionStillValid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/server/info" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Cookie") != "splunkd=abc" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	store := NewFileStore(dir)
	target := mustTarget(t, server.URL)
	if err := store.Set(target, AuthRecord{
		APIBaseURL: target.API,
		WebBaseURL: target.Web,
		Method:     MethodWeb,
		Cookies:    []Cookie{{Name: "splunkd", Value: "abc", Path: "/"}},
		CreatedAt:  1,
		UpdatedAt:  2,
	}); err != nil {
		t.Fatal(err)
	}

	var browserCalls atomic.Int32
	service := NewAuthService(AuthServiceOptions{
		Store:   store,
		Client:  NewClient(server.Client(), false),
		Browser: fakeBrowser{calls: &browserCalls},
	})
	result, err := service.Login(context.Background(), LoginRequest{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "already authenticated" || !result.Validated {
		t.Fatalf("unexpected result: %+v", result)
	}
	if browserCalls.Load() != 0 {
		t.Fatalf("expected browser auth to be skipped, got %d calls", browserCalls.Load())
	}
}

func TestPersistedInsecureTLSIsUsedByStatus(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/server/info" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Cookie") != "splunkd=abc" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	dir := t.TempDir()
	store := NewFileStore(dir)
	loginService := NewAuthService(AuthServiceOptions{
		Store:  store,
		Client: NewClient(nil, true),
		Browser: fakeBrowser{session: BrowserSession{
			Cookies: []Cookie{{Name: "splunkd", Value: "abc", Domain: strings.TrimPrefix(server.URL, "https://"), Path: "/"}},
		}},
	})
	if _, err := loginService.Login(context.Background(), LoginRequest{URL: server.URL, Insecure: true}); err != nil {
		t.Fatal(err)
	}

	statusService := NewAuthService(AuthServiceOptions{
		Store:  store,
		Client: NewClient(nil, false),
	})
	status, err := statusService.Status(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !status.RemoteValid {
		t.Fatalf("expected stored insecure setting to validate status: %+v", status)
	}
}

func TestFileStorePermissions(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(filepath.Join(dir, "config"))
	target := mustTarget(t, "https://splunk.example.com")
	if err := store.Set(target, AuthRecord{Method: MethodWeb, Cookies: []Cookie{{Name: "splunkd", Value: "x"}}}); err != nil {
		t.Fatal(err)
	}
	stat, err := os.Stat(filepath.Join(dir, "config", "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if mode := stat.Mode().Perm(); mode != 0o600 {
		t.Fatalf("expected 0600 auth store, got %o", mode)
	}
}

func TestAuthRecordJSONDoesNotExposeEmptySecrets(t *testing.T) {
	raw, err := json.Marshal(AuthRecord{Method: MethodWeb})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "session_key") {
		t.Fatalf("empty session key should be omitted: %s", raw)
	}
}

func mustTarget(t *testing.T, raw string) Target {
	t.Helper()
	target, err := NormalizeTarget(raw)
	if err != nil {
		t.Fatal(err)
	}
	return target
}
