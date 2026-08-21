package cli

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestAuthStatusInventoriesEnvironmentSessionWithoutPersistingSecret(t *testing.T) {
	const (
		targetURL  = "https://splunk.example.com:8089"
		sessionKey = "cli-inventory-session-key"
	)
	setCLIEnvironmentSession(t, targetURL, sessionKey)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	var stdout, stderr bytes.Buffer
	command := NewRootCommand(&stdout, &stderr)
	command.SetArgs([]string{"auth", "status", "--output=json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"url":"`+targetURL+`"`) || !strings.Contains(stdout.String(), `"authenticated":true`) {
		t.Fatalf("unexpected environment auth inventory: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) {
		t.Fatal("session secret appeared in command output")
	}
	if _, err := os.Stat(filepath.Join(configHome, "splsearch", "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("environment auth must not create auth.json: %v", err)
	}
}

func TestEnvironmentSessionAuthStatusRemotelyValidates(t *testing.T) {
	const sessionKey = "cli-status-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/server/info" || r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"entry":[]}`))
	}))
	defer server.Close()
	setCLIEnvironmentSession(t, server.URL, sessionKey)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	command := NewRootCommand(&stdout, &stderr)
	command.SetArgs([]string{"auth", "status", "--url=" + server.URL, "--output=json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("remote status failed: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"authenticated":true`) || !strings.Contains(stdout.String(), `"remote_valid":true`) {
		t.Fatalf("unexpected remote status output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) {
		t.Fatal("session key appeared in remote status output")
	}
}

func TestEnvironmentSessionTargetMismatchDoesNotOpenBrowser(t *testing.T) {
	const sessionKey = "target-bound-session-key"
	setCLIEnvironmentSession(t, "https://bound.example.com:8089", sessionKey)
	store, err := splunk.NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	var browserCalls atomic.Int32
	e := testEnv(t, &stdout, &stderr, nil, fakeBrowserAuth{calls: &browserCalls})
	e.store = store
	command := newRootCommand(e)
	command.SetArgs([]string{"search", "--url=https://other.example.com:8089", "--query=| makeresults", "--immediate"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected target-bound environment session to reject another target")
	}
	if browserCalls.Load() != 0 {
		t.Fatalf("environment session target mismatch opened the browser %d times", browserCalls.Load())
	}
	if !strings.Contains(stdout.String(), "bound to https://bound.example.com:8089") {
		t.Fatalf("expected target-binding error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) {
		t.Fatal("session secret appeared in target-binding failure output")
	}
}

func TestRejectedEnvironmentSessionDoesNotOpenBrowser(t *testing.T) {
	const sessionKey = "rejected-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	setCLIEnvironmentSession(t, server.URL, sessionKey)
	store, err := splunk.NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	var browserCalls atomic.Int32
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{calls: &browserCalls})
	e.store = store
	command := newRootCommand(e)
	command.SetArgs([]string{"search", "--url=" + server.URL, "--query=| makeresults", "--immediate"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected rejected environment session to fail")
	}
	if browserCalls.Load() != 0 {
		t.Fatalf("rejected environment session opened the browser %d times", browserCalls.Load())
	}
	if !strings.Contains(stdout.String(), "ephemeral session authentication was rejected") {
		t.Fatalf("expected environment-session rejection, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) {
		t.Fatal("rejected session secret appeared in command output")
	}
}

func TestRejectedEnvironmentSessionLoginDoesNotOpenBrowser(t *testing.T) {
	const sessionKey = "rejected-login-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	setCLIEnvironmentSession(t, server.URL, sessionKey)
	store, err := splunk.NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	var browserCalls atomic.Int32
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{calls: &browserCalls})
	e.store = store
	command := newRootCommand(e)
	command.SetArgs([]string{"auth", "login", "--url=" + server.URL})
	if err := command.Execute(); err == nil {
		t.Fatal("expected rejected environment session login to fail")
	}
	if browserCalls.Load() != 0 {
		t.Fatalf("rejected environment session login opened the browser %d times", browserCalls.Load())
	}
	if !strings.Contains(stdout.String(), "ephemeral session authentication was rejected") {
		t.Fatalf("expected environment-session rejection, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) {
		t.Fatal("rejected login session secret appeared in command output")
	}
}

func TestEnvironmentSessionRunsCLISearchAgainstDirectManagementAPI(t *testing.T) {
	const sessionKey = "cli-direct-search-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/en-US/splunkd/__raw") {
			http.Error(w, "web proxy forbidden", http.StatusBadRequest)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-cli-direct"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli-direct"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1,"runDuration":0.1}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli-direct/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli-direct"):
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	setCLIEnvironmentSession(t, server.URL, sessionKey)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var stdout, stderr bytes.Buffer
	command := NewRootCommand(&stdout, &stderr)
	command.SetArgs([]string{"search", "--url=" + server.URL, "--query=| makeresults", "--immediate", "--progress=off"})
	if err := command.Execute(); err != nil {
		t.Fatalf("direct CLI search failed: %v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `"sid":"sid-cli-direct"`) || !strings.Contains(stdout.String(), `"event":"ok"`) {
		t.Fatalf("unexpected direct CLI search output: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) {
		t.Fatal("session key appeared in direct CLI search output")
	}
}

func TestEnvironmentSessionCreateFailureReturnsStableSanitizedClassification(t *testing.T) {
	const sessionKey = "classified-create-failure-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "request rejected for "+sessionKey, http.StatusBadRequest)
	}))
	defer server.Close()
	setCLIEnvironmentSession(t, server.URL, sessionKey)
	store, err := splunk.NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	var browserCalls atomic.Int32
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{calls: &browserCalls})
	e.store = store
	command := newRootCommand(e)
	command.SetArgs([]string{"search", "--url=" + server.URL, "--query=| invalid-command", "--progress=off"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected rejected create-search request")
	}
	if browserCalls.Load() != 0 {
		t.Fatalf("create-search failure opened the browser %d times", browserCalls.Load())
	}
	for _, field := range []string{
		`"error_code":"splunk_http_error"`,
		`"operation":"create_search_job"`,
		`"retryable":false`,
		`"table_created":false`,
	} {
		if !strings.Contains(stdout.String(), field) {
			t.Fatalf("missing %s in stable create failure: %s", field, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), sessionKey) || strings.Contains(stderr.String(), sessionKey) || strings.Contains(stdout.String(), "request rejected") {
		t.Fatalf("create-search failure retained session response data: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func setCLIEnvironmentSession(t *testing.T, targetURL, sessionKey string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(sessionKey); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(reader.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		_ = syscall.Close(fd)
		t.Fatal(err)
	}
	t.Setenv(splunk.SessionKeyFDEnvironment, strconv.Itoa(fd))
	t.Setenv(splunk.SessionTargetURLEnvironment, targetURL)
}
