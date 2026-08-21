package splunk

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	httpClient *http.Client
	insecure   bool
}

type probeResult struct {
	Base string
	Mode string
}

const defaultAuthClientTimeout = 30 * time.Second

func NewClient(client *http.Client, insecure bool) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	client = cloneHTTPClientWithDefaultTimeout(client)
	if insecure {
		client = cloneHTTPClientWithInsecureTLS(client)
	}
	return &Client{httpClient: client, insecure: insecure}
}

func (c *Client) Validate(ctx context.Context, target Target, record AuthRecord) (bool, error) {
	if record.Method == MethodSessionKey {
		base := record.APIBaseURL
		if base == "" {
			base = target.API
		}
		return c.validateAt(ctx, base, "api", record)
	}
	var firstErr error
	rejected := false
	if record.APIBaseURL != "" {
		if ok, err := c.validateAt(ctx, record.APIBaseURL, "api", record); ok {
			return true, nil
		} else if err != nil && firstErr == nil {
			firstErr = err
		} else if err == nil {
			rejected = true
		}
	}
	if record.WebBaseURL != "" {
		if ok, err := c.validateAt(ctx, record.WebBaseURL, "web", record); ok {
			return true, nil
		} else if err != nil && firstErr == nil {
			firstErr = err
		} else if err == nil {
			rejected = true
		}
	}
	for _, base := range uniqueStrings(append(target.Bases, record.Aliases...)...) {
		if ok, err := c.validateAt(ctx, base, "api", record); ok {
			return true, nil
		} else if err != nil && firstErr == nil {
			firstErr = err
		} else if err == nil {
			rejected = true
		}
		if ok, err := c.validateAt(ctx, base, "web", record); ok {
			return true, nil
		} else if err != nil && firstErr == nil {
			firstErr = err
		} else if err == nil {
			rejected = true
		}
	}
	if rejected {
		return false, nil
	}
	if firstErr != nil {
		return false, firstErr
	}
	return false, fmt.Errorf("stored credentials were not accepted")
}

func (c *Client) validateAt(ctx context.Context, base, mode string, record AuthRecord) (bool, error) {
	path := "/services/server/info?output_mode=json"
	if mode == "web" {
		path = "/en-US/splunkd/__raw" + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinURL(base, path), nil)
	if err != nil {
		return false, err
	}
	applyAuth(req, record)
	var resp *http.Response
	if record.Method == MethodSessionKey {
		resp, err = c.doWithoutRedirects(req, record.TLSInsecureSkipVerify)
	} else {
		resp, err = c.do(req, record.TLSInsecureSkipVerify)
	}
	if err != nil {
		return false, operationError("validate_auth", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, nil
	}
	return false, fmt.Errorf("HTTP %d", resp.StatusCode)
}

func applyAuth(req *http.Request, record AuthRecord) {
	switch record.Method {
	case MethodWeb:
		if header := cookieHeader(record.Cookies, req.URL.Hostname()); header != "" {
			req.Header.Set("Cookie", header)
		}
	case MethodSessionKey:
		if record.SessionKey != "" {
			req.Header.Set("Authorization", "Splunk "+record.SessionKey)
		}
	}
}

func applyWebMutationHeaders(req *http.Request, record AuthRecord) {
	method := strings.ToUpper(req.Method)
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodDelete {
		return
	}
	if token := csrfToken(record.Cookies, req.URL.Hostname()); token != "" {
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("X-Splunk-Form-Key", token)
	}
}

func (c *Client) do(req *http.Request, insecure bool) (*http.Response, error) {
	client := c.clientForTLS(insecure)
	return client.Do(req)
}

func (c *Client) doWithoutRedirects(req *http.Request, insecure bool) (*http.Response, error) {
	client := *c.clientForTLS(insecure)
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Do(req)
	if err == nil {
		return resp, nil
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return nil, &sessionTransportError{err: err}
}

type sessionTransportError struct {
	err error
}

func (e *sessionTransportError) Error() string {
	return "session-authenticated request failed before a usable response"
}

func (e *sessionTransportError) Unwrap() error {
	return e.err
}

func (c *Client) clientForTLS(insecure bool) *http.Client {
	if insecure && !c.insecure {
		return cloneHTTPClientWithInsecureTLS(c.httpClient)
	}
	return c.httpClient
}

func (c *Client) webProxyRequest(ctx context.Context, base string, record AuthRecord, method, path string, query url.Values, form url.Values) (*http.Response, error) {
	targetURL, err := url.Parse(joinURL(base, "/en-US/splunkd/__raw"+path))
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		values := targetURL.Query()
		for key, entries := range query {
			for _, value := range entries {
				values.Add(key, value)
			}
		}
		targetURL.RawQuery = values.Encode()
	}
	var body io.Reader
	if len(form) > 0 {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL.String(), body)
	if err != nil {
		return nil, err
	}
	applyAuth(req, record)
	applyWebMutationHeaders(req, record)
	if len(form) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return c.do(req, record.TLSInsecureSkipVerify)
}

func (c *Client) splunkRequest(ctx context.Context, base string, record AuthRecord, method, path string, query url.Values, form url.Values) (*http.Response, error) {
	if record.Method != MethodSessionKey {
		return c.webProxyRequest(ctx, base, record, method, path, query, form)
	}
	targetURL, err := url.Parse(joinURL(base, path))
	if err != nil {
		return nil, err
	}
	if len(query) > 0 {
		values := targetURL.Query()
		for key, entries := range query {
			for _, value := range entries {
				values.Add(key, value)
			}
		}
		targetURL.RawQuery = values.Encode()
	}
	var body io.Reader
	if len(form) > 0 {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL.String(), body)
	if err != nil {
		return nil, err
	}
	applyAuth(req, record)
	if len(form) > 0 {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	return c.doWithoutRedirects(req, record.TLSInsecureSkipVerify)
}

func cookieHeader(cookies []Cookie, host string) string {
	var parts []string
	host = strings.ToLower(host)
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		domain = strings.Split(domain, ":")[0]
		if domain != "" && domain != host && !strings.HasSuffix(host, "."+domain) {
			continue
		}
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	return strings.Join(parts, "; ")
}

func csrfToken(cookies []Cookie, host string) string {
	host = strings.ToLower(host)
	for _, cookie := range cookies {
		name := strings.ToLower(cookie.Name)
		if !strings.Contains(name, "csrf") && !strings.Contains(name, "form_key") {
			continue
		}
		if !cookieMatchesHost(cookie, host) {
			continue
		}
		return cookie.Value
	}
	return ""
}

func cookieMatchesHost(cookie Cookie, host string) bool {
	domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
	domain = strings.Split(domain, ":")[0]
	return domain == "" || domain == host || strings.HasSuffix(host, "."+domain)
}

func joinURL(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

func cloneHTTPClientWithDefaultTimeout(client *http.Client) *http.Client {
	if client.Timeout != 0 {
		return client
	}
	clone := *client
	clone.Timeout = defaultAuthClientTimeout
	return &clone
}

func cloneHTTPClientWithInsecureTLS(client *http.Client) *http.Client {
	clone := *client
	var transport *http.Transport
	if client.Transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	} else if existing, ok := client.Transport.(*http.Transport); ok {
		transport = existing.Clone()
	} else {
		return &clone
	}
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Explicit --insecure user option for dev Splunk instances.
	clone.Transport = transport
	if clone.Timeout == 0 {
		clone.Timeout = defaultAuthClientTimeout
	}
	return &clone
}
