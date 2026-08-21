package splunk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	stdurl "net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchCreatesPollsFetchesAndDeletesJob(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/en-US/splunkd/__raw/servicesNS/-/search/search/jobs":
			if r.URL.Query().Get("output_mode") != "json" {
				t.Fatalf("missing output_mode=json: %s", r.URL.RawQuery)
			}
			if !strings.Contains(r.Header.Get("Cookie"), "splunkd=abc") || !strings.Contains(r.Header.Get("Cookie"), "csrf_token=csrf") {
				t.Fatalf("missing auth cookies: %q", r.Header.Get("Cookie"))
			}
			if r.Header.Get("X-Splunk-Form-Key") != "csrf" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				t.Fatalf("missing CSRF headers: %#v", r.Header)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if got := r.Form.Get("search"); got != "search index=test" {
				t.Fatalf("unexpected search body: %q", got)
			}
			if r.Form.Get("earliest_time") != "-5m" || r.Form.Get("latest_time") != "now" {
				t.Fatalf("unexpected time bounds: %#v", r.Form)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-123"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/en-US/splunkd/__raw/servicesNS/-/search/search/jobs/sid-123":
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":4,"runDuration":1.25}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/en-US/splunkd/__raw/servicesNS/-/search/search/jobs/sid-123/results":
			if r.URL.Query().Get("count") != "2" || r.URL.Query().Get("offset") != "1" || r.URL.Query().Get("max_lines") != "0" {
				t.Fatalf("unexpected results query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"results":[{"n":1},{"n":2}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/en-US/splunkd/__raw/servicesNS/-/search/search/jobs/sid-123":
			if r.Header.Get("X-Splunk-Form-Key") != "csrf" {
				t.Fatalf("missing delete CSRF header")
			}
			deleted.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, server.URL, 10)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	result, err := service.Search(context.Background(), SearchRequest{
		URL:      server.URL,
		Query:    "index=test",
		Earliest: "-5m",
		Latest:   "now",
		App:      "search",
		Limit:    2,
		Offset:   1,
		PageSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.Load() {
		t.Fatalf("expected best-effort job cleanup")
	}
	if result.URL != server.URL || result.SID != "sid-123" || result.Query != "search index=test" {
		t.Fatalf("unexpected result metadata: %+v", result)
	}
	if result.ResultCount != 4 || result.ReturnedResults != 2 || !result.HasMore {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if len(result.Results) != 2 || int(result.Results[0]["n"].(float64)) != 1 || int(result.Results[1]["n"].(float64)) != 2 {
		t.Fatalf("unexpected rows: %#v", result.Results)
	}
}

func TestSearchUsesDirectManagementAPIForEnvironmentSession(t *testing.T) {
	const sessionKey = "direct-search-session-key"
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/en-US/splunkd/__raw") {
			http.Error(w, "web proxy is forbidden for session-key auth", http.StatusBadRequest)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/servicesNS/-/search/search/jobs":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-direct"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/servicesNS/-/search/search/jobs/sid-direct":
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1,"runDuration":0.1}}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/servicesNS/-/search/search/jobs/sid-direct/results":
			_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/servicesNS/-/search/search/jobs/sid-direct":
			deleted.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	result, err := service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "| makeresults", PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.SID != "sid-direct" || result.ReturnedResults != 1 || !deleted.Load() {
		t.Fatalf("unexpected direct management API result: %+v, deleted=%v", result, deleted.Load())
	}
}

func TestSearchRejectsUnsuccessfulJobCleanupResponses(t *testing.T) {
	const sessionKey = "cleanup-response-session-key"
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusInternalServerError,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Splunk "+sessionKey {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				switch {
				case r.Method == http.MethodPost:
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"sid":"sid-cleanup-status"}`))
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/results"):
					_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
				case r.Method == http.MethodGet:
					_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1}}]}`))
				case r.Method == http.MethodDelete:
					http.Error(w, "private cleanup response body", status)
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			setEnvironmentSession(t, server.URL, sessionKey)
			store, err := NewAuthStoreFromEnvironment(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
			result, err := service.Search(context.Background(), SearchRequest{
				URL:      server.URL,
				Query:    "| makeresults",
				PageSize: 1,
			})
			if err == nil {
				t.Fatal("expected cleanup response failure")
			}
			if result.OK || result.SID != "" || len(result.Results) != 0 {
				t.Fatalf("cleanup failure returned a successful search result: %+v", result)
			}
			expected := fmt.Sprintf("delete search job: failed to delete search job: HTTP %d", status)
			if err.Error() != expected {
				t.Fatalf("unexpected cleanup response error: got %q want %q", err, expected)
			}
			if strings.Contains(err.Error(), sessionKey) || strings.Contains(err.Error(), "private cleanup response body") {
				t.Fatalf("cleanup response exposed protected details: %v", err)
			}
			details, ok := StructuredError(err)
			if !ok || details.ErrorCode != "splunk_http_error" || details.Operation != "delete_search_job" {
				t.Fatalf("unexpected structured cleanup failure: details=%#v ok=%v", details, ok)
			}
		})
	}
}

func TestSearchRegistersCleanupBeforeProgressCallback(t *testing.T) {
	const sessionKey = "panicking-progress-session-key"
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-panicking-progress"}`))
		case http.MethodDelete:
			deleted.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	var recovered any
	func() {
		defer func() {
			recovered = recover()
		}()
		_, _ = service.Search(context.Background(), SearchRequest{
			URL:   server.URL,
			Query: "| makeresults",
			Progress: func(SearchProgressEvent) {
				panic("progress callback panic")
			},
		})
	}()
	if recovered != "progress callback panic" {
		t.Fatalf("unexpected recovered panic: %#v", recovered)
	}
	if !deleted.Load() {
		t.Fatal("search job cleanup was not attempted before progress callback panic")
	}
}

func TestSearchRejectsJobCleanupTransportFailure(t *testing.T) {
	const (
		targetURL  = "https://splunk.example.com:8089"
		sessionKey = "cleanup-transport-session-key"
	)
	setEnvironmentSession(t, targetURL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			return testHTTPResponse(r, http.StatusUnauthorized, "unauthorized"), nil
		}
		switch {
		case r.Method == http.MethodPost:
			return testHTTPResponse(r, http.StatusCreated, `{"sid":"sid-cleanup-transport"}`), nil
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/results"):
			return testHTTPResponse(r, http.StatusOK, `{"results":[{"event":"ok"}]}`), nil
		case r.Method == http.MethodGet:
			return testHTTPResponse(r, http.StatusOK, `{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1}}]}`), nil
		case r.Method == http.MethodDelete:
			return nil, fmt.Errorf("transport echoed %s", r.Header.Get("Authorization"))
		default:
			return testHTTPResponse(r, http.StatusNotFound, "not found"), nil
		}
	})}, false)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: client})
	result, err := service.Search(context.Background(), SearchRequest{
		URL:      targetURL,
		Query:    "| makeresults",
		PageSize: 1,
	})
	if err == nil {
		t.Fatal("expected cleanup transport failure")
	}
	if result.OK || result.SID != "" || len(result.Results) != 0 {
		t.Fatalf("cleanup transport failure returned a successful search result: %+v", result)
	}
	if err.Error() != "delete search job: search job cleanup request failed before a usable response" {
		t.Fatalf("unexpected cleanup transport error: %v", err)
	}
	if strings.Contains(err.Error(), sessionKey) || strings.Contains(err.Error(), "transport echoed") {
		t.Fatalf("cleanup transport error exposed credential material: %v", err)
	}
	details, ok := StructuredError(err)
	if !ok || details.ErrorCode != "splunk_operation_failed" || details.Operation != "delete_search_job" {
		t.Fatalf("unexpected structured cleanup transport failure: details=%#v ok=%v", details, ok)
	}
}

func TestSearchJoinsCleanupFailureAfterOriginalSearchFailure(t *testing.T) {
	const sessionKey = "joined-cleanup-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-joined-cleanup"}`))
		case http.MethodGet:
			http.Error(w, "private original response body", http.StatusInternalServerError)
		case http.MethodDelete:
			http.Error(w, "private cleanup response body", http.StatusServiceUnavailable)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	result, err := service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "| makeresults"})
	if err == nil {
		t.Fatal("expected search and cleanup failure")
	}
	if result.OK || result.SID != "" || len(result.Results) != 0 {
		t.Fatalf("joined failure returned a successful search result: %+v", result)
	}
	expected := strings.Join([]string{
		"read search job status: failed to read search job status: HTTP 500",
		"delete search job: failed to delete search job: HTTP 503",
	}, "\n")
	if err.Error() != expected {
		t.Fatalf("unexpected joined search and cleanup failure: got %q want %q", err, expected)
	}
	if strings.Contains(err.Error(), sessionKey) || strings.Contains(err.Error(), "private") {
		t.Fatalf("joined failure exposed protected details: %v", err)
	}
	details, ok := StructuredError(err)
	if !ok || details.ErrorCode != "splunk_http_error" || details.Operation != "read_search_job_status" {
		t.Fatalf("original operation was not preserved: details=%#v ok=%v", details, ok)
	}
}

func TestSearchAcceptsSuccessfulJobCleanupResponse(t *testing.T) {
	const sessionKey = "successful-cleanup-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-successful-cleanup"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"ok"}]}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1}}]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	result, err := service.Search(context.Background(), SearchRequest{
		URL:      server.URL,
		Query:    "| makeresults",
		PageSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.SID != "sid-successful-cleanup" || result.ReturnedResults != 1 {
		t.Fatalf("unexpected successful search result: %+v", result)
	}
}

func TestEnvironmentSessionResponseBodiesAreNotRetainedInErrors(t *testing.T) {
	const sessionKey = "response+echo/session=key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "request rejected for response%2Becho%2Fsession%3Dkey", http.StatusInternalServerError)
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	_, err = service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "| makeresults"})
	if err == nil {
		t.Fatal("expected echoed server error")
	}
	if strings.Contains(err.Error(), sessionKey) || strings.Contains(err.Error(), "request rejected") {
		t.Fatalf("session response body appeared in search error: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("expected status-only session error, got %v", err)
	}
}

func TestEnvironmentSessionTransportErrorsCannotEchoAuthorization(t *testing.T) {
	const (
		targetURL  = "https://splunk.example.com:8089"
		sessionKey = "transport-echo-session-key"
	)
	setEnvironmentSession(t, targetURL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("transport echoed %s", r.Header.Get("Authorization"))
	})}, false)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: client})
	_, err = service.Search(context.Background(), SearchRequest{URL: targetURL, Query: "| makeresults"})
	if err == nil {
		t.Fatal("expected transport failure")
	}
	if strings.Contains(err.Error(), sessionKey) || strings.Contains(err.Error(), "transport echoed") {
		t.Fatalf("session transport detail appeared in search error: %v", err)
	}
	if !strings.Contains(err.Error(), "session-authenticated request failed") {
		t.Fatalf("expected fixed session transport error, got %v", err)
	}
}

func TestEnvironmentSessionSuppressesSearchFailureMessages(t *testing.T) {
	const sessionKey = "failed-job-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-failed"}`))
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"FAILED","messages":[{"type":"ERROR","text":"sensitive transformed credential detail"}]}}]}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	_, err = service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "| makeresults"})
	if err == nil {
		t.Fatal("expected failed search job")
	}
	if strings.Contains(err.Error(), "sensitive transformed credential detail") {
		t.Fatalf("session search failure message appeared in error: %v", err)
	}
	if err.Error() != "search job failed" {
		t.Fatalf("expected fixed failed-job message, got %v", err)
	}
}

func TestEnvironmentSessionRejectsCredentialBearingResults(t *testing.T) {
	const sessionKey = "result-echo-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-result-echo"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"` + sessionKey + `"}]}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":1}}]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	_, err = service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "| makeresults", PageSize: 1})
	if err == nil {
		t.Fatal("expected credential-bearing result rejection")
	}
	if strings.Contains(err.Error(), sessionKey) {
		t.Fatalf("session key appeared in result rejection: %v", err)
	}
	if err.Error() != "search results contained credential material" {
		t.Fatalf("unexpected credential-bearing result error: %v", err)
	}
}

func TestSearchEmitsJobAndFetchProgress(t *testing.T) {
	var pollCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-progress"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-progress"):
			if pollCount.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"RUNNING","isDone":false,"doneProgress":0.25,"scanCount":40,"eventCount":12,"resultCount":3,"resultPreviewCount":2,"runDuration":1.5}}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"doneProgress":1,"scanCount":80,"eventCount":20,"resultCount":2,"resultPreviewCount":2,"runDuration":2.5}}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-progress/results"):
			_, _ = w.Write([]byte(`{"results":[{"event":"one"},{"event":"two"}]}`))
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/search/jobs/sid-progress"):
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, server.URL, 10)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	var events []SearchProgressEvent
	result, err := service.Search(context.Background(), SearchRequest{
		URL:      server.URL,
		Query:    "index=test",
		PageSize: 2,
		Progress: func(event SearchProgressEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ReturnedResults != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(events) < 5 {
		t.Fatalf("expected dispatch, running, done, and fetch progress events, got %#v", events)
	}
	var running, fetchDone bool
	for _, event := range events {
		if event.Phase == searchProgressPhaseRunning && event.State == "RUNNING" {
			running = true
			if event.Percent != 25 || event.DoneProgress != 0.25 || event.ScanCount != 40 || event.EventCount != 12 || event.ResultCount != 3 || event.ResultPreviewCount != 2 {
				t.Fatalf("unexpected running progress: %#v", event)
			}
		}
		if event.Phase == searchProgressPhaseFetch && event.State == "done" {
			fetchDone = true
			if event.FetchedRows != 2 || event.TotalRows != 2 || event.Percent != 100 {
				t.Fatalf("unexpected fetch progress: %#v", event)
			}
		}
	}
	if !running || !fetchDone {
		t.Fatalf("missing expected progress events: %#v", events)
	}
}

func TestSearchUsesLatestStoredAuthWhenURLMissing(t *testing.T) {
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("old server should not be used: %s %s", r.Method, r.URL.String())
	}))
	defer oldServer.Close()

	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-new"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/sid-new"):
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"DONE","isDone":true,"resultCount":0,"runDuration":0.1}}]}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer newServer.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, oldServer.URL, 10)
	storeSearchAuth(t, store, newServer.URL, 20)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(newServer.Client(), false)})
	result, err := service.Search(context.Background(), SearchRequest{Query: "| makeresults", App: "search"})
	if err != nil {
		t.Fatal(err)
	}
	if result.URL != newServer.URL {
		t.Fatalf("expected latest auth URL %s, got %s", newServer.URL, result.URL)
	}
}

func TestSearchNoStoredAuthRequiresLogin(t *testing.T) {
	service := NewSearchService(SearchServiceOptions{Store: NewFileStore(t.TempDir()), Client: NewClient(nil, false)})
	_, err := service.Search(context.Background(), SearchRequest{Query: "index=test"})
	if err == nil || !strings.Contains(err.Error(), "run splsearch auth login --url=<splunk-url>") {
		t.Fatalf("expected login guidance, got %v", err)
	}
}

func TestSearchFailedJobCleansUp(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-failed"}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"FAILED","messages":[{"type":"ERROR","text":"bad SPL"}]}}]}`))
		case r.Method == http.MethodDelete:
			deleted.Store(true)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, server.URL, 10)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	_, err := service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "bad spl"})
	if err == nil || err.Error() != "search job failed" {
		t.Fatalf("expected fixed failed search message, got %v", err)
	}
	details, ok := StructuredError(err)
	if !ok || details.ErrorCode != "search_failed" || details.Operation != "wait_search_job" || details.Retryable || details.SID != "sid-failed" {
		t.Fatalf("expected structured failed search details, got details=%#v ok=%v err=%v", details, ok, err)
	}
	if !deleted.Load() {
		t.Fatalf("expected failed job cleanup")
	}
}

func TestSearchTimeoutCleansUp(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-timeout"}`))
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"entry":[{"content":{"dispatchState":"RUNNING","isDone":false}}]}`))
		case r.Method == http.MethodDelete:
			deleted.Store(true)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, server.URL, 10)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := service.Search(ctx, SearchRequest{URL: server.URL, Query: "index=test"})
	if err == nil || !strings.Contains(err.Error(), "search timed out waiting for sid=sid-timeout") {
		t.Fatalf("expected timeout message, got %v", err)
	}
	details, ok := StructuredError(err)
	if !ok || details.ErrorCode != "search_timeout" || details.Operation != "wait_search_job" || details.SID != "sid-timeout" || details.LastProgress == nil {
		t.Fatalf("expected structured search timeout, got details=%#v ok=%v err=%v", details, ok, err)
	}
	if !deleted.Load() {
		t.Fatalf("expected timed-out job cleanup")
	}
}

func TestSearchJobStatusHTTPErrorIsReported(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"sid":"sid-status-failed"}`))
		case r.Method == http.MethodGet:
			http.Error(w, "app context missing", http.StatusInternalServerError)
		case r.Method == http.MethodDelete:
			deleted.Store(true)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, server.URL, 10)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	_, err := service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "index=test"})
	if err == nil || !strings.Contains(err.Error(), "failed to read search job status: HTTP 500") {
		t.Fatalf("expected job status HTTP error, got %v", err)
	}
	if !deleted.Load() {
		t.Fatalf("expected failed status job cleanup")
	}
}

func TestSearchAuthRejectedRequiresLogin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, server.URL, 10)
	service := NewSearchService(SearchServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	_, err := service.Search(context.Background(), SearchRequest{URL: server.URL, Query: "index=test"})
	if err == nil || !strings.Contains(err.Error(), "run splsearch auth login --url="+server.URL) {
		t.Fatalf("expected login guidance, got %v", err)
	}
}

func TestSearchClosedConnectionWithRejectedAuthRequiresLogin(t *testing.T) {
	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, "https://splunk.example.com", 10)
	client := NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs") {
			return nil, io.EOF
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/services/server/info") {
			return testHTTPResponse(r, http.StatusUnauthorized, "unauthorized"), nil
		}
		return nil, fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
	})}, false)

	service := NewSearchService(SearchServiceOptions{Store: store, Client: client})
	_, err := service.Search(context.Background(), SearchRequest{URL: "https://splunk.example.com", Query: "index=test"})
	if err == nil {
		t.Fatal("expected search failure")
	}
	if !strings.Contains(err.Error(), "run splsearch auth login --url=https://splunk.example.com") {
		t.Fatalf("expected login guidance, got %v", err)
	}
	if strings.Contains(err.Error(), `Post "`) {
		t.Fatalf("expected readable error without raw transport dump, got %v", err)
	}
}

func TestSearchClosedConnectionWithValidAuthIsReadable(t *testing.T) {
	store := NewFileStore(t.TempDir())
	storeSearchAuth(t, store, "https://splunk.example.com", 10)
	client := NewClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jobs") {
			return nil, io.EOF
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/services/server/info") {
			return testHTTPResponse(r, http.StatusOK, `{"ok":true}`), nil
		}
		return nil, fmt.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
	})}, false)

	service := NewSearchService(SearchServiceOptions{Store: store, Client: client})
	_, err := service.Search(context.Background(), SearchRequest{URL: "https://splunk.example.com", Query: "index=test"})
	if err == nil {
		t.Fatal("expected search failure")
	}
	if !strings.Contains(err.Error(), "Splunk closed the connection while creating the search job") {
		t.Fatalf("expected closed-connection guidance, got %v", err)
	}
	if strings.Contains(err.Error(), `Post "`) {
		t.Fatalf("expected readable error without raw transport dump, got %v", err)
	}
}

func TestSearchEndpointSupportsAppLessMode(t *testing.T) {
	if got := searchEndpoint("-", "/jobs"); got != "/services/search/jobs" {
		t.Fatalf("unexpected app-less endpoint: %s", got)
	}
	if got := searchEndpoint("custom-app", "/jobs"); got != "/servicesNS/-/custom-app/search/jobs" {
		t.Fatalf("unexpected app endpoint: %s", got)
	}
}

func storeSearchAuth(t *testing.T, store Store, rawURL string, updatedAt int64) {
	t.Helper()
	target := mustTarget(t, rawURL)
	parsed, err := stdurl.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	record := AuthRecord{
		APIBaseURL: target.API,
		WebBaseURL: target.Web,
		Method:     MethodWeb,
		Cookies: []Cookie{
			{Name: "splunkd", Value: "abc", Domain: parsed.Hostname(), Path: "/"},
			{Name: "csrf_token", Value: "csrf", Domain: parsed.Hostname(), Path: "/"},
		},
		CreatedAt: updatedAt - 1,
		UpdatedAt: updatedAt,
	}
	if err := store.Set(target, record); err != nil {
		t.Fatal(err)
	}
}

func TestSearchJSONNumbersAreParsedFromStrings(t *testing.T) {
	payload := map[string]any{
		"resultCount": "12",
		"runDuration": json.Number("1.5"),
	}
	if got := intValue(payload["resultCount"]); got != 12 {
		t.Fatalf("unexpected int value: %d", got)
	}
	if got := strconv.FormatFloat(floatValue(payload["runDuration"]), 'f', 1, 64); got != "1.5" {
		t.Fatalf("unexpected float value: %s", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func testHTTPResponse(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}
