package splunk

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type Target struct {
	Input  string
	Key    string
	Web    string
	API    string
	Bases  []string
	Scheme string
	Host   string
}

func NormalizeTarget(raw string) (Target, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Target{}, fmt.Errorf("splunk url is required")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return Target{}, fmt.Errorf("invalid splunk url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Target{}, fmt.Errorf("unsupported splunk url scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return Target{}, fmt.Errorf("splunk url must include a host")
	}

	base := canonicalBase(parsed)
	web := inferredWebBase(parsed)
	api := inferredAPIBase(parsed)
	bases := uniqueStrings(base, web, api)
	return Target{
		Input:  raw,
		Key:    base,
		Web:    web,
		API:    api,
		Bases:  bases,
		Scheme: strings.ToLower(parsed.Scheme),
		Host:   strings.ToLower(parsed.Host),
	}, nil
}

func canonicalBase(parsed *url.URL) string {
	host := strings.ToLower(parsed.Host)
	return (&url.URL{Scheme: strings.ToLower(parsed.Scheme), Host: host}).String()
}

func inferredAPIBase(parsed *url.URL) string {
	copyURL := *parsed
	host := strings.ToLower(copyURL.Hostname())
	port := copyURL.Port()
	if port == "" || port == "8000" {
		copyURL.Host = host + ":8089"
	} else {
		copyURL.Host = strings.ToLower(copyURL.Host)
	}
	copyURL.Path = ""
	copyURL.RawPath = ""
	copyURL.RawQuery = ""
	copyURL.Fragment = ""
	return (&url.URL{Scheme: strings.ToLower(copyURL.Scheme), Host: copyURL.Host}).String()
}

func inferredWebBase(parsed *url.URL) string {
	copyURL := *parsed
	host := strings.ToLower(copyURL.Hostname())
	port := copyURL.Port()
	if port == "8089" {
		copyURL.Host = host + ":8000"
	} else {
		copyURL.Host = strings.ToLower(copyURL.Host)
	}
	copyURL.Path = ""
	copyURL.RawPath = ""
	copyURL.RawQuery = ""
	copyURL.Fragment = ""
	return (&url.URL{Scheme: strings.ToLower(copyURL.Scheme), Host: copyURL.Host}).String()
}

func uniqueStrings(values ...string) []string {
	seen := map[string]bool{}
	var result []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
