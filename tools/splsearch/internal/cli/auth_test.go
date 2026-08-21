package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type fakeBrowserAuth struct {
	session splunk.BrowserSession
	err     error
	calls   *atomic.Int32
}

func (f fakeBrowserAuth) Authenticate(context.Context, splunk.Target, string, splunk.BrowserAuthOptions) (splunk.BrowserSession, error) {
	if f.calls != nil {
		f.calls.Add(1)
	}
	return f.session, f.err
}

func TestRootHelpIsVerboseForNoArgsAndHelp(t *testing.T) {
	for _, args := range [][]string{nil, []string{"--help"}, []string{"-h"}} {
		var stdout, stderr bytes.Buffer
		code := Main(args, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("expected code 0 for args %v, got %d", args, code)
		}
		output := stdout.String()
		for _, want := range []string{
			"splsearch is a CLI for AI agents",
			"Run `splsearch auth status`",
			"Do not pass auth",
			"splsearch auth login --url=<splunk-url>",
			"splsearch search --query='<SPL>'",
			"splsearch results-list",
			"splsearch result-info --table=<table>",
			"splsearch result-warnings accept --table=<table> --code=full_fetch",
			"splsearch result-schema --table=<table>",
			"splsearch result-summary --table=<table> --group-by=<field>",
			"splsearch result-events --table=<table> --field=<field> --value=<value>",
			"splsearch result-search --table=<table>",
			"splsearch auth status",
			"--output=json",
			"--insecure",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("help for args %v missing %q:\n%s", args, want, output)
			}
		}
	}
}

func TestAuthLoginMissingURLPrintsClearError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"auth", "login"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	output := stderr.String()
	for _, want := range []string{"missing --url=<splunk-url>", "splsearch auth login --url=<splunk-url>"} {
		if !strings.Contains(output, want) {
			t.Fatalf("missing error text %q in:\n%s", want, output)
		}
	}
}

func TestAuthLoginHelpIsWebOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "Authenticate to Splunk through a browser") {
		t.Fatalf("expected browser help, got:\n%s", output)
	}
	if strings.Contains(output, "cookies") || strings.Contains(output, "~/.config/splsearch") {
		t.Fatalf("login help exposes implementation details:\n%s", output)
	}
	for _, removed := range []string{"--basic", "--token", "--web"} {
		if strings.Contains(output, removed) {
			t.Fatalf("login help still exposes %s:\n%s", removed, output)
		}
	}
}

func TestAuthLoginRejectsRemovedMethods(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--url=https://splunk.example.com", "--token=t"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}
}

func TestAuthLoginBrowserFailureUsesAuthDiagnosticHint(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_CHANNEL", "safari")
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, splunk.NewPlaywrightAuthenticator())
	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--url=https://splunk.example.com", "--output=json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected auth login failure")
	}
	var result map[string]any
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("auth login output is not JSON: %s", stdout.String())
	}
	if result["error_code"] != "unsupported_browser_channel" || result["operation"] != "authenticate" || result["retryable"] != false {
		t.Fatalf("expected browser auth failure envelope, got %#v", result)
	}
	hint, _ := result["diagnostic_hint"].(string)
	if hint == "" || strings.Contains(hint, "result table") {
		t.Fatalf("auth login hint should be operation-specific, got %#v", result)
	}
	if result["requested_channel"] != "safari" || result["fallback_used"] != false {
		t.Fatalf("expected browser-specific fields, got %#v", result)
	}
}

func TestAuthLoginStatusLogoutJSONFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/en-US/splunkd/__raw/services/server/info" {
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

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{session: browserSessionForServer(server.URL)})
	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--url=" + server.URL, "--output=json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), `"method"`) {
		t.Fatalf("login output should not expose auth method: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"validated":true`) {
		t.Fatalf("unexpected login output: %s", stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"auth", "status", "--url=" + server.URL, "--output=json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"remote_valid":true`) {
		t.Fatalf("unexpected status output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"method"`) {
		t.Fatalf("status output should not expose auth method: %s", stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"auth", "status", "--output=json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"servers":[`) || !strings.Contains(stdout.String(), server.URL) {
		t.Fatalf("expected status list output, got: %s", stdout.String())
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"auth", "logout", "--url=" + server.URL, "--output=json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"removed":true`) {
		t.Fatalf("unexpected logout output: %s", stdout.String())
	}
}

func TestAuthStatusInventoryListsExpiredCredentials(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	target, err := splunk.NormalizeTarget("https://splunk.example.com")
	if err != nil {
		t.Fatal(err)
	}
	expiresAt := time.Now().Unix() - 60
	store := splunk.NewFileStore(e.configDir)
	if err := store.Set(target, splunk.AuthRecord{Method: splunk.MethodWeb, ExpiresAt: &expiresAt}); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "status", "--output=json"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected expired-only inventory to fail authentication")
	}

	var result authListResult
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("auth status inventory output is not JSON: %s", stdout.String())
	}
	if result.OK || result.Authenticated || len(result.Servers) != 1 {
		t.Fatalf("expected unauthenticated inventory with one expired server, got %+v", result)
	}
	server := result.Servers[0]
	if server.URL != target.Key || server.Authenticated == nil || *server.Authenticated {
		t.Fatalf("expected expired server identity, got %+v", server)
	}
	if server.ErrorCode != "credentials_expired" || server.Operation != "auth_status" {
		t.Fatalf("expected structured expired server fields, got %+v", server)
	}
	if server.Retryable == nil || *server.Retryable || server.DiagnosticHint == "" {
		t.Fatalf("expected non-retryable expired server hint, got %+v", server)
	}
	if server.ExpiresAt == nil || *server.ExpiresAt != expiresAt {
		t.Fatalf("expected expires_at in inventory, got %+v", server)
	}
}

func TestAuthStatusURLNetworkFailureReturnsStructuredJSON(t *testing.T) {
	const targetURL = "https://missing.example.test"
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, dnsFailingClient("missing.example.test"), nil)
	storeCLIAuth(t, e.configDir, targetURL, 10)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "status", "--url=" + targetURL, "--output=json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected auth status failure")
	}
	var result map[string]any
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("auth status output is not JSON: %s", stdout.String())
	}
	if result["ok"] != false || result["authenticated"] != false || result["remote_valid"] != false {
		t.Fatalf("unexpected auth status failure shape: %#v", result)
	}
	if result["error_code"] != "dns_lookup_failed" || result["operation"] != "validate_auth" || result["retryable"] != false {
		t.Fatalf("expected structured DNS validation failure, got %#v", result)
	}
	if result["diagnostic_hint"] == "" {
		t.Fatalf("expected diagnostic_hint, got %#v", result)
	}
	if strings.Contains(result["message"].(string), "stored credentials were rejected") || strings.Contains(result["message"].(string), "stored credentials were not accepted") {
		t.Fatalf("network validation failure should not be reported as credential rejection: %#v", result)
	}
}

func TestAuthLoginSkipsBrowserWhenAlreadyAuthenticated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/en-US/splunkd/__raw/services/server/info" && r.URL.Path != "/services/server/info" {
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

	var browserCalls atomic.Int32
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{session: browserSessionForServer(server.URL), calls: &browserCalls})

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--url=" + server.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if browserCalls.Load() != 1 {
		t.Fatalf("expected first login to open browser once, got %d", browserCalls.Load())
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--url=" + server.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if browserCalls.Load() != 1 {
		t.Fatalf("expected second login to reuse valid auth, got %d browser calls", browserCalls.Load())
	}
	if !strings.Contains(stdout.String(), "already authenticated") {
		t.Fatalf("expected already-authenticated output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestAuthStatusTextListIsCompact(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{session: browserSessionForServer(server.URL)})
	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"auth", "login", "--url=" + server.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"auth", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	if !strings.Contains(output, "- "+server.URL) {
		t.Fatalf("expected compact URL list, got:\n%s", output)
	}
	if strings.Contains(output, "authenticated servers:") || strings.Contains(output, server.URL+" authenticated") {
		t.Fatalf("status text is redundant:\n%s", output)
	}
}

func TestMainReturnsFailureForInvalidOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"auth", "status", "--url=https://splunk.example.com", "--output=yaml"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
}

func TestAuthCommandsRejectPositionalURL(t *testing.T) {
	for _, args := range [][]string{
		{"auth", "login", "https://splunk.example.com"},
		{"auth", "status", "https://splunk.example.com"},
		{"auth", "logout", "https://splunk.example.com"},
	} {
		var stdout, stderr bytes.Buffer
		code := Main(args, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected code 1 for %v, got %d", args, code)
		}
		output := stderr.String()
		if !strings.Contains(output, "use --url=<splunk-url>") {
			t.Fatalf("expected --url guidance for %v, got:\n%s", args, output)
		}
	}
}

func testEnv(t *testing.T, stdout, stderr *bytes.Buffer, client *http.Client, browser splunk.BrowserAuthenticator) *env {
	t.Helper()
	if client == nil {
		client = http.DefaultClient
	}
	if browser == nil {
		browser = fakeBrowserAuth{}
	}
	return &env{
		configDir: t.TempDir(),
		out:       stdout,
		err:       stderr,
		client:    client,
		browser:   browser,
	}
}

func browserSessionForServer(rawURL string) splunk.BrowserSession {
	return browserSessionForServerWithCookie(rawURL, "abc")
}

func browserSessionForServerWithCookie(rawURL, cookieValue string) splunk.BrowserSession {
	domain := strings.TrimPrefix(rawURL, "http://")
	domain = strings.TrimPrefix(domain, "https://")
	return splunk.BrowserSession{
		Cookies: []splunk.Cookie{{Name: "splunkd", Value: cookieValue, Domain: domain, Path: "/"}},
	}
}

type cliRoundTripFunc func(*http.Request) (*http.Response, error)

func (f cliRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func dnsFailingClient(name string) *http.Client {
	return &http.Client{Transport: cliRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &net.DNSError{Err: "no such host", Name: name, IsNotFound: true}
	})}
}
