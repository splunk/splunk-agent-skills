package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	stdurl "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestSearchCommandJSONFlow(t *testing.T) {
	server := newCLISearchServer(t, 1)
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
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--earliest=-5m", "--latest=now", "--limit=1", "--result-table=identity_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["url"] != server.URL || result["query"] != "search index=test" || result["table"] != "identity_logs" {
		t.Fatalf("unexpected search output: %#v", result)
	}
	if _, ok := result["results"]; ok {
		t.Fatalf("default search output should not include raw results: %#v", result)
	}
	if int(result["rows"].(float64)) != 1 {
		t.Fatalf("unexpected result count: %#v", result)
	}
	if !strings.Contains(result["text_search_command"].(string), "result-text-search --table='identity_logs' --query='<text>'") {
		t.Fatalf("unexpected text search command: %#v", result["text_search_command"])
	}
	assertTableRowCount(t, result["db"].(string), "identity_logs", 1)
}

func TestSearchCommandImmediateKeepsStdoutOnly(t *testing.T) {
	server := newCLISearchServer(t, 1)
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
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=1", "--immediate"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if _, ok := result["results"]; !ok {
		t.Fatalf("immediate search output should include raw results: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(e.configDir, "results.sqlite")); !os.IsNotExist(err) {
		t.Fatalf("immediate search should not create results DB, stat err=%v", err)
	}
}

func TestSearchCommandWarnsForUnlimitedBroadSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-wide"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-wide"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":10001,"runDuration":0.2}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-wide/results"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-wide"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--result-table=wide_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	warnings := result["warnings"].([]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "--limit=0") || !strings.Contains(warnings[0].(string), "10001") {
		t.Fatalf("unexpected warnings: %#v", result)
	}
	details := result["warning_details"].([]any)
	firstDetail := details[0].(map[string]any)
	if firstDetail["code"] != "full_fetch" || firstDetail["accepted"] != false {
		t.Fatalf("unexpected warning details: %#v", result)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"result-info", "--table=wide_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("result-info output is not JSON: %s", stdout.String())
	}
	if int(info["warning_count"].(float64)) != 1 || int(info["accepted_warning_count"].(float64)) != 0 {
		t.Fatalf("expected persisted warning in result-info, got %#v", info)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"results-list", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var list map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &list); err != nil {
		t.Fatalf("results-list output is not JSON: %s", stdout.String())
	}
	tables := list["tables"].([]any)
	if len(tables) != 1 || int(tables[0].(map[string]any)["warning_count"].(float64)) != 1 {
		t.Fatalf("expected persisted warning in results-list, got %#v", list)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"result-warnings", "accept", "--table=wide_logs", "--code=full_fetch"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var accepted map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &accepted); err != nil {
		t.Fatalf("result-warnings output is not JSON: %s", stdout.String())
	}
	if int(accepted["warning_count"].(float64)) != 0 || int(accepted["accepted_warning_count"].(float64)) != 1 {
		t.Fatalf("expected accepted full-fetch warning, got %#v", accepted)
	}
}

func TestSearchCommandPrintsLargeWriteProgressToStderr(t *testing.T) {
	const resultCount = 10000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-progress"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-progress"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":10000,"runDuration":0.2}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-progress/results"):
			count, _ := strconv.Atoi(r.URL.Query().Get("count"))
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			_, _ = w.Write([]byte(`{"results":[`))
			for i := 0; i < count; i++ {
				if i > 0 {
					_, _ = w.Write([]byte(","))
				}
				_, _ = fmt.Fprintf(w, `{"event":"progress","n":%d}`, offset+i)
			}
			_, _ = w.Write([]byte(`]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-progress"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=10000", "--result-table=progress_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if int(result["rows"].(float64)) != resultCount {
		t.Fatalf("unexpected saved row count: %#v", result)
	}
	if _, ok := result["results"]; ok {
		t.Fatalf("table-writing search should not print raw results: %#v", result)
	}
	progress := stderr.String()
	if !strings.Contains(progress, "splsearch: starting table=progress_logs") || !strings.Contains(progress, "splsearch: done table=progress_logs") {
		t.Fatalf("expected progress on stderr, got %q", progress)
	}
}

func TestSearchCommandPrintsJobProgressToStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-job-progress"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-job-progress"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"doneProgress":1,"scanCount":20,"eventCount":10,"resultCount":1,"resultPreviewCount":1,"runDuration":0.2}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-job-progress/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-job-progress"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=1", "--result-table=job_progress_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["table"] != "job_progress_logs" {
		t.Fatalf("unexpected search result: %#v", result)
	}
	progress := stderr.String()
	for _, want := range []string{"splsearch: job phase=dispatch sid=sid-job-progress", "splsearch: job phase=running sid=sid-job-progress state=DONE progress=100.0%", "scanned=20", "splsearch: fetch sid=sid-job-progress"} {
		if !strings.Contains(progress, want) {
			t.Fatalf("expected stderr progress to contain %q, got %q", want, progress)
		}
	}
}

func TestSearchCommandProgressJSONL(t *testing.T) {
	server := newCLISearchServer(t, 1)
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=1", "--immediate", "--progress=jsonl"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if _, ok := result["results"]; !ok {
		t.Fatalf("expected immediate results on stdout, got %#v", result)
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected JSONL progress lines, got %q", stderr.String())
	}
	for _, line := range lines {
		var progress map[string]any
		if err := json.Unmarshal([]byte(line), &progress); err != nil {
			t.Fatalf("progress line is not JSON: %q in %q", line, stderr.String())
		}
		if progress["type"] != "splsearch_progress" || progress["phase"] == "" {
			t.Fatalf("unexpected progress record: %#v", progress)
		}
	}
}

func TestSearchCommandProgressOffSuppressesProgress(t *testing.T) {
	server := newCLISearchServer(t, 1)
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=1", "--result-table=quiet_logs", "--progress=off"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stderr.String() != "" {
		t.Fatalf("expected progress=off to suppress progress stderr, got %q", stderr.String())
	}
}

func TestSearchCommandTimeoutReturnsLastProgressJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-timeout-progress"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-timeout-progress"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"RUNNING","isDone":false,"doneProgress":0.5,"scanCount":200,"eventCount":20,"resultCount":3,"resultPreviewCount":2,"runDuration":3.0}}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-timeout-progress"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	code := executeCommand(context.Background(), newRootCommand(e), []string{"search", "--url=" + server.URL, "--query=index=test", "--timeout=20ms", "--progress=off"}, &stderr)
	if code != 1 {
		t.Fatalf("expected timeout failure code 1, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["error_code"] != "search_timeout" || result["operation"] != "wait_search_job" || result["sid"] != "sid-timeout-progress" || result["table_created"] != false {
		t.Fatalf("unexpected timeout JSON: %#v", result)
	}
	lastProgress, ok := result["last_progress"].(map[string]any)
	if !ok || lastProgress["phase"] != "running" || lastProgress["state"] != "RUNNING" || int(lastProgress["scan_count"].(float64)) != 200 {
		t.Fatalf("expected last_progress with running scan count, got %#v", result)
	}
}

func TestSearchCommandFailedJobReturnsSanitizedClassificationAndDeletesSID(t *testing.T) {
	const serverDetail = "sensitive invalid SPL parser detail"
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-failed-after-dispatch"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-failed-after-dispatch"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"FAILED","messages":[{"type":"ERROR","text":"` + serverDetail + `"}]}}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-failed-after-dispatch"):
			deleted.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	code := executeCommand(context.Background(), newRootCommand(e), []string{"search", "--url=" + server.URL, "--query=invalid SPL", "--progress=off"}, &stderr)
	if code != 1 {
		t.Fatalf("expected failed-job exit code 1, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["error_code"] != "search_failed" || result["operation"] != "wait_search_job" || result["retryable"] != false || result["table_created"] != false {
		t.Fatalf("unexpected failed-job JSON: %#v", result)
	}
	if result["message"] != "search job failed" || strings.Contains(stdout.String()+stderr.String(), serverDetail) {
		t.Fatalf("failed-job output was not fixed and sanitized: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !deleted.Load() {
		t.Fatal("expected failed remote search job to be deleted")
	}
}

func TestSearchCommandJobStatusHTTPErrorIsStructured(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-status-500"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-status-500"):
			http.Error(w, "job status unavailable", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-status-500"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)

	code := executeCommand(context.Background(), newRootCommand(e), []string{"search", "--url=" + server.URL, "--query=index=test", "--progress=off"}, &stderr)
	if code != 1 {
		t.Fatalf("expected status HTTP failure code 1, got %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["error_code"] != "splunk_http_error" || result["operation"] != "read_search_job_status" || result["table_created"] != false {
		t.Fatalf("expected structured job-status error, got %#v", result)
	}
	if result["retryable"] != true || !strings.Contains(result["message"].(string), "HTTP 500") {
		t.Fatalf("expected retryable HTTP 500 job-status message, got %#v", result)
	}
}

func TestSearchCommandAutoLogsInWhenStoredCredentialsRejected(t *testing.T) {
	var postCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/services/server/info"):
			if !strings.Contains(r.Header.Get("Cookie"), "splunkd=good") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			postCount.Add(1)
			if !strings.Contains(r.Header.Get("Cookie"), "splunkd=good") {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-cli"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1,"runDuration":0.2}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{session: browserSessionForServerWithCookie(server.URL, "good")})
	storeCLIAuthCookie(t, e.configDir, server.URL, 10, "bad")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=1", "--immediate"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if postCount.Load() != 2 {
		t.Fatalf("expected one failed post and one retry, got %d", postCount.Load())
	}
	if !strings.Contains(stderr.String(), "opening browser login") || !strings.Contains(stderr.String(), "retrying search") {
		t.Fatalf("expected auto-auth progress on stderr, got %q", stderr.String())
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true {
		t.Fatalf("expected successful retry, got %#v", result)
	}
}

func TestSearchCommandDoesNotAutoLoginWhenStoredAuthStillValid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/services/server/info"):
			if r.Header.Get("Cookie") != "splunkd=valid" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			http.Error(w, "search permission denied", http.StatusForbidden)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	var browserCalls atomic.Int32
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), fakeBrowserAuth{
		session: browserSessionForServerWithCookie(server.URL, "fresh"),
		calls:   &browserCalls,
	})
	storeCLIAuthCookie(t, e.configDir, server.URL, 10, "valid")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--immediate"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected search failure")
	}
	if browserCalls.Load() != 0 {
		t.Fatalf("expected browser auth to be skipped, got %d calls", browserCalls.Load())
	}
	if strings.Contains(stderr.String(), "opening browser login") {
		t.Fatalf("did not expect auto-auth progress, got stderr=%q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"ok":false`) || !strings.Contains(stdout.String(), "stored authentication") || !strings.Contains(stdout.String(), "still validates") {
		t.Fatalf("expected valid-auth rejection guidance, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSearchCommandNetworkFailureReturnsStructuredJSON(t *testing.T) {
	const targetURL = "https://missing.example.test"
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, dnsFailingClient("missing.example.test"), nil)
	storeCLIAuth(t, e.configDir, targetURL, 10)

	code := executeCommand(context.Background(), newRootCommand(e), []string{"search", "--url=" + targetURL, "--query=index=test", "--result-table=network_probe"}, &stderr)
	if code != 1 {
		t.Fatalf("expected search failure exit code 1, got %d", code)
	}
	var result map[string]any
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != false || result["url"] != targetURL {
		t.Fatalf("unexpected search failure shape: %#v", result)
	}
	if result["error_code"] != "dns_lookup_failed" || result["operation"] != "create_search_job" || result["retryable"] != false {
		t.Fatalf("expected structured DNS search failure, got %#v", result)
	}
	if result["table_created"] != false || result["diagnostic_hint"] == "" {
		t.Fatalf("expected no-table diagnostic fields, got %#v", result)
	}
	if !strings.Contains(result["message"].(string), "create search job") {
		t.Fatalf("expected human-readable create-job message, got %#v", result)
	}
}

func TestSearchCommandDefaultsToLatestAuthenticatedURL(t *testing.T) {
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("old server should not be used")
	}))
	defer oldServer.Close()
	newServer := newCLISearchServer(t, 0)
	defer newServer.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, newServer.Client(), nil)
	storeCLIAuth(t, e.configDir, oldServer.URL, 10)
	storeCLIAuth(t, e.configDir, newServer.URL, 20)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--query=| makeresults"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["url"] != newServer.URL {
		t.Fatalf("expected latest URL %s, got %#v", newServer.URL, result)
	}
}

func TestSearchCommandDropsOldResultTablesAsync(t *testing.T) {
	server := newCLISearchServer(t, 1)
	defer server.Close()

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, server.Client(), nil)
	storeCLIAuth(t, e.configDir, server.URL, 20)
	seedOldResultTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + server.URL, "--query=index=test", "--limit=1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	db := openCLIResultDB(t, filepath.Join(e.configDir, "results.sqlite"))
	defer db.Close()
	deadline := time.Now().Add(time.Second)
	for cliTableExists(t, db, "old_results") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if cliTableExists(t, db, "old_results") {
		t.Fatal("expected old result table to be dropped")
	}
}

func TestSearchCommandMissingQueryPrintsJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"search"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"ok":false`) || !strings.Contains(stdout.String(), "missing --query=<SPL>") {
		t.Fatalf("expected JSON query error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSearchCommandNoAuthPrintsJSONError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--query=index=test"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected search failure")
	}
	if !strings.Contains(stdout.String(), `"ok":false`) || !strings.Contains(stdout.String(), "run splsearch auth login --url=<splunk-url>") {
		t.Fatalf("expected no-auth JSON error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSearchCommandAutoAuthBrowserFailureUsesSearchEnvelope(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_CHANNEL", "safari")
	targetURL := "https://splunk.example.com"
	target, err := splunk.NormalizeTarget(targetURL)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, splunk.NewPlaywrightAuthenticator())
	expiredAt := time.Now().Add(-time.Minute).Unix()
	if err := splunk.NewFileStore(e.configDir).Set(target, splunk.AuthRecord{
		APIBaseURL: target.API,
		WebBaseURL: target.Web,
		Method:     splunk.MethodWeb,
		Cookies:    []splunk.Cookie{{Name: "splunkd", Value: "expired", Domain: "splunk.example.com", Path: "/"}},
		ExpiresAt:  &expiredAt,
		CreatedAt:  expiredAt - 1,
		UpdatedAt:  expiredAt,
	}); err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"search", "--url=" + targetURL, "--query=index=test", "--result-table=auth_probe"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected search failure")
	}
	var result map[string]any
	if unmarshalErr := json.Unmarshal(stdout.Bytes(), &result); unmarshalErr != nil {
		t.Fatalf("search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != false || result["url"] != targetURL {
		t.Fatalf("unexpected browser auth failure shape: %#v", result)
	}
	if result["error_code"] != "unsupported_browser_channel" || result["operation"] != "authenticate" || result["retryable"] != false {
		t.Fatalf("expected browser auth search envelope, got %#v", result)
	}
	if result["table_created"] != false || result["diagnostic_hint"] == "" {
		t.Fatalf("expected no-table diagnostic fields, got %#v", result)
	}
	if result["requested_channel"] != "safari" || result["fallback_used"] != false {
		t.Fatalf("expected browser-specific fields to be preserved, got %#v", result)
	}
	if strings.Contains(stdout.String(), "opening browser login") {
		t.Fatalf("search progress should not be written to stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "opening browser login") {
		t.Fatalf("expected auth progress on stderr, got %q", stderr.String())
	}
}

func TestCrashpadDiagnosticHintDoesNotPromiseDeniedPath(t *testing.T) {
	for _, hint := range []string{
		browserAuthDiagnosticHint("browser_crashpad_permission"),
		browserSearchDiagnosticHint("browser_crashpad_permission"),
	} {
		for _, want := range []string{"launch_error_summary", "bootstrap_check_in", "unsandboxed"} {
			if !strings.Contains(hint, want) {
				t.Fatalf("expected crashpad hint to include %q, got %q", want, hint)
			}
		}
		if strings.Contains(hint, "denied path") {
			t.Fatalf("crashpad hint should not promise a denied filesystem path: %q", hint)
		}
	}
}

func TestBrowserEnvironmentRetryFields(t *testing.T) {
	retryable, code := browserEnvironmentRetryFields(splunk.BrowserAuthErrorDetails{
		RetryableAfterEnvironmentChange: true,
		RemediationCode:                 "retry_from_unsandboxed_environment",
	})
	if retryable == nil || *retryable != true || code != "retry_from_unsandboxed_environment" {
		t.Fatalf("expected environment retry fields, got retryable=%v code=%q", retryable, code)
	}

	retryable, code = browserEnvironmentRetryFields(splunk.BrowserAuthErrorDetails{
		RetryableAfterEnvironmentChange: false,
		RemediationCode:                 "retry_from_unsandboxed_environment",
	})
	if retryable != nil || code != "" {
		t.Fatalf("expected environment retry fields to be omitted, got retryable=%v code=%q", retryable, code)
	}
}

func TestSearchCommandRejectsPositionalArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"search", "index=test"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "unexpected positional argument") || !strings.Contains(stdout.String(), "--query=<SPL>") {
		t.Fatalf("expected positional-arg guidance, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSearchCommandRejectsSplFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"search", "--spl=index=test"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stderr.String(), "unknown flag: --spl") {
		t.Fatalf("expected unknown --spl flag, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSearchHelpExplainsDBAndImmediateMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"search", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"SQLite table",
		"--result-table=<name>",
		"Use --immediate only when the caller expects very short output",
		"warnings",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("search help missing %q:\n%s", want, output)
		}
	}
}

func TestSearchCommandRejectsInvalidResultTable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"search", "--query=index=test", "--result-table=bad-name"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"ok":false`) || !strings.Contains(stdout.String(), "invalid --result-table") {
		t.Fatalf("expected invalid table JSON error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSearchCommandRejectsResultTableWithImmediate(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"search", "--query=index=test", "--result-table=identity_logs", "--immediate"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), "--result-table cannot be used with --immediate") {
		t.Fatalf("expected immediate conflict error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestParseSearchTimeoutAcceptsBareSeconds(t *testing.T) {
	duration, err := parseSearchTimeout("600")
	if err != nil {
		t.Fatal(err)
	}
	if duration != 10*time.Minute {
		t.Fatalf("expected 10m, got %s", duration)
	}
}

func newCLISearchServer(t *testing.T, resultCount int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-cli"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":` + strconv.Itoa(resultCount) + `,"runDuration":0.2}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-cli"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected search request: %s %s", r.Method, r.URL.String())
		}
	}))
}

func storeCLIAuth(t *testing.T, configDir, rawURL string, updatedAt int64) {
	t.Helper()
	storeCLIAuthCookie(t, configDir, rawURL, updatedAt, "abc")
}

func storeCLIAuthCookie(t *testing.T, configDir, rawURL string, updatedAt int64, cookieValue string) {
	t.Helper()
	target, err := splunk.NormalizeTarget(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := stdurl.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	store := splunk.NewFileStore(configDir)
	err = store.Set(target, splunk.AuthRecord{
		APIBaseURL: target.API,
		WebBaseURL: target.Web,
		Method:     splunk.MethodWeb,
		Cookies:    []splunk.Cookie{{Name: "splunkd", Value: cookieValue, Domain: parsed.Hostname(), Path: "/"}},
		CreatedAt:  updatedAt - 1,
		UpdatedAt:  updatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertTableRowCount(t *testing.T, dbPath, tableName string, want int) {
	t.Helper()
	db := openCLIResultDB(t, dbPath)
	defer db.Close()

	var got int
	if err := db.QueryRow(`SELECT COUNT(*) FROM "` + tableName + `"`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("expected %s row count %d, got %d", tableName, want, got)
	}
}

func seedOldResultTable(t *testing.T, configDir string) {
	t.Helper()
	store := splunk.NewResultStore(configDir)
	_, err := store.Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "old-sid",
		Query:           "search index=old",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     1,
		ReturnedResults: 1,
		Results:         []map[string]any{{"event": "old"}},
	}, "old_results")
	if err != nil {
		t.Fatal(err)
	}

	db := openCLIResultDB(t, store.Path())
	defer db.Close()
	cutoff := time.Now().Add(-25 * time.Hour).Unix()
	if _, err := db.Exec(`UPDATE searches SET created_at = ? WHERE table_name = ?`, cutoff, "old_results"); err != nil {
		t.Fatal(err)
	}
}

func openCLIResultDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func cliTableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count > 0
}
