package splunk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateFallsBackToWebWhenAPIRejectsSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/services/server/info":
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "/en-US/splunkd/__raw/services/server/info":
			if r.Header.Get("Cookie") != "splunkd=abc" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	record := AuthRecord{
		APIBaseURL: server.URL,
		WebBaseURL: server.URL,
		Method:     MethodWeb,
		Cookies: []Cookie{{
			Name:   "splunkd",
			Value:  "abc",
			Domain: strings.TrimPrefix(server.URL, "http://"),
			Path:   "/",
		}},
	}
	ok, err := NewClient(server.Client(), false).Validate(context.Background(), mustTarget(t, server.URL), record)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected web validation to succeed after API rejection")
	}
}

func TestNewClientAddsDefaultTimeout(t *testing.T) {
	client := NewClient(nil, false)
	if client.httpClient.Timeout != defaultAuthClientTimeout {
		t.Fatalf("expected default timeout %s, got %s", defaultAuthClientTimeout, client.httpClient.Timeout)
	}
	if http.DefaultClient.Timeout != 0 {
		t.Fatalf("NewClient must not mutate http.DefaultClient, got %s", http.DefaultClient.Timeout)
	}
}

func TestNewClientPreservesExplicitTimeout(t *testing.T) {
	base := &http.Client{Timeout: 7 * time.Second}
	client := NewClient(base, false)
	if client.httpClient.Timeout != base.Timeout {
		t.Fatalf("expected explicit timeout %s, got %s", base.Timeout, client.httpClient.Timeout)
	}
}

func TestValidateAuthRequestHonorsHTTPTimeout(t *testing.T) {
	client := NewClient(&http.Client{
		Transport: blockingAuthTransport{},
		Timeout:   20 * time.Millisecond,
	}, false)
	started := time.Now()
	ok, err := client.Validate(
		context.Background(),
		mustTarget(t, "https://customer-stack.splunkcloud.com"),
		AuthRecord{
			APIBaseURL: "https://customer-stack.splunkcloud.com",
			Method:     MethodWeb,
			Cookies: []Cookie{{
				Name:   "splunkd",
				Value:  "abc",
				Domain: "customer-stack.splunkcloud.com",
				Path:   "/",
			}},
		},
	)
	if ok || err == nil {
		t.Fatalf("expected timed-out validation failure, ok=%v err=%v", ok, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("auth validation did not honor HTTP timeout, elapsed=%s err=%v", elapsed, err)
	}
	if !strings.Contains(err.Error(), "validate authentication") {
		t.Fatalf("expected validation operation in error, got %v", err)
	}
}

type blockingAuthTransport struct{}

func (blockingAuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}
