package splunk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSearchEarliest = "-1h"
	defaultSearchLatest   = "now"
	defaultSearchApp      = "search"
	defaultSearchPageSize = 1000
)

const (
	searchProgressPhaseDispatch = "dispatch"
	searchProgressPhaseRunning  = "running"
	searchProgressPhaseFetch    = "fetch"
	searchProgressPhaseWrite    = "write"
)

type SearchServiceOptions struct {
	Store  Store
	Client *Client
}

type SearchService struct {
	store  Store
	client *Client
}

func NewSearchService(options SearchServiceOptions) *SearchService {
	return &SearchService{
		store:  options.Store,
		client: options.Client,
	}
}

func (s *SearchService) Search(ctx context.Context, request SearchRequest) (result SearchResult, err error) {
	started := time.Now()
	target, record, err := s.resolveAuth(request.URL)
	if err != nil {
		return SearchResult{}, err
	}
	request = normalizeSearchRequest(request)
	if request.Query == "" {
		return SearchResult{}, fmt.Errorf("missing --query=<SPL>")
	}
	if request.Limit < 0 {
		return SearchResult{}, fmt.Errorf("--limit must be >= 0")
	}
	if request.Offset < 0 {
		return SearchResult{}, fmt.Errorf("--offset must be >= 0")
	}
	if request.PageSize < 1 {
		return SearchResult{}, fmt.Errorf("--page-size must be >= 1")
	}

	base := record.WebBaseURL
	if record.Method == MethodSessionKey {
		base = record.APIBaseURL
		if base == "" {
			base = target.API
		}
	} else if base == "" {
		base = target.Web
	}
	sid, err := s.createJob(ctx, base, *record, request)
	if err != nil {
		if closedConnectionError(err) {
			if authErr := s.authValidationError(ctx, target, *record); authErr != nil {
				return SearchResult{}, authErr
			}
			return SearchResult{}, searchJobConnectionClosedError(target.Key)
		}
		return SearchResult{}, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupErr := s.deleteJob(cleanupCtx, base, *record, request.App, sid)
		if cleanupErr == nil {
			return
		}
		if err != nil {
			err = errors.Join(err, cleanupErr)
			return
		}
		result = SearchResult{}
		err = cleanupErr
	}()
	emitSearchProgress(request.Progress, SearchProgressEvent{
		Phase:          searchProgressPhaseDispatch,
		SID:            sid,
		State:          "created",
		ElapsedSeconds: elapsedSeconds(started),
	})

	status, err := s.waitForJob(ctx, base, *record, request.App, sid, started, request.Progress)
	if err != nil {
		return SearchResult{}, err
	}
	rows, err := s.fetchAllResults(ctx, base, *record, request.App, sid, status.ResultCount, request.Limit, request.Offset, request.PageSize, started, request.Progress)
	if err != nil {
		return SearchResult{}, err
	}

	return SearchResult{
		OK:              true,
		URL:             target.Key,
		App:             request.App,
		SID:             sid,
		Query:           request.Query,
		Earliest:        request.Earliest,
		Latest:          request.Latest,
		ResultCount:     status.ResultCount,
		ReturnedResults: len(rows),
		Offset:          request.Offset,
		HasMore:         request.Offset+len(rows) < status.ResultCount,
		RunDuration:     status.RunDuration,
		Results:         rows,
	}, nil
}

func (s *SearchService) resolveAuth(rawURL string) (Target, *AuthRecord, error) {
	if strings.TrimSpace(rawURL) != "" {
		target, err := NormalizeTarget(rawURL)
		if err != nil {
			return Target{}, nil, err
		}
		record, err := s.store.Get(target)
		if err != nil {
			return Target{}, nil, err
		}
		if record == nil {
			return Target{}, nil, authRequiredError(target.Key, "not authenticated for "+target.Key)
		}
		if err := ensureUsableSearchAuth(target.Key, record); err != nil {
			return Target{}, nil, err
		}
		return target, record, nil
	}

	records, err := s.store.List()
	if err != nil {
		return Target{}, nil, err
	}
	var selected *AuthRecord
	var selectedTime int64
	for _, record := range records {
		if record.URL == "" {
			continue
		}
		if err := ensureUsableSearchAuth(record.URL, &record); err != nil {
			continue
		}
		authTime := record.UpdatedAt
		if authTime == 0 {
			authTime = record.CreatedAt
		}
		if selected == nil || authTime > selectedTime || (authTime == selectedTime && record.URL > selected.URL) {
			copyRecord := record
			selected = &copyRecord
			selectedTime = authTime
		}
	}
	if selected == nil {
		return Target{}, nil, fmt.Errorf("no authenticated Splunk servers; run splsearch auth login --url=<splunk-url>")
	}
	target, err := NormalizeTarget(selected.URL)
	if err != nil {
		return Target{}, nil, err
	}
	return target, selected, nil
}

func ensureUsableSearchAuth(url string, record *AuthRecord) error {
	if record.Method == MethodSessionKey {
		if hasSessionKey(*record) {
			return nil
		}
		return fmt.Errorf("ephemeral session authentication for %s is missing its session key", url)
	}
	if record.Method != MethodWeb {
		return authRequiredError(url, "stored credentials for "+url+" use an unsupported authentication method")
	}
	if record.ExpiresAt != nil && time.Now().Unix() >= *record.ExpiresAt {
		return authRequiredError(url, "stored credentials for "+url+" are expired")
	}
	if len(record.Cookies) == 0 {
		return authRequiredError(url, "stored credentials for "+url+" do not contain a web session")
	}
	return nil
}

func normalizeSearchRequest(request SearchRequest) SearchRequest {
	request.Query = normalizeSearchQuery(request.Query)
	request.Earliest = strings.TrimSpace(request.Earliest)
	if request.Earliest == "" {
		request.Earliest = defaultSearchEarliest
	}
	request.Latest = strings.TrimSpace(request.Latest)
	if request.Latest == "" {
		request.Latest = defaultSearchLatest
	}
	request.App = strings.TrimSpace(request.App)
	if request.App == "" {
		request.App = defaultSearchApp
	}
	if request.PageSize == 0 {
		request.PageSize = defaultSearchPageSize
	}
	return request
}

func normalizeSearchQuery(query string) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return ""
	}
	lower := strings.ToLower(query)
	if strings.HasPrefix(query, "|") || lower == "search" || strings.HasPrefix(lower, "search ") {
		return query
	}
	return "search " + query
}

func (s *SearchService) createJob(ctx context.Context, base string, record AuthRecord, request SearchRequest) (string, error) {
	form := url.Values{}
	form.Set("search", request.Query)
	form.Set("earliest_time", request.Earliest)
	form.Set("latest_time", request.Latest)
	resp, err := s.client.splunkRequest(ctx, base, record, http.MethodPost, searchEndpoint(request.App, "/jobs"), url.Values{"output_mode": {"json"}}, form)
	if err != nil {
		return "", operationError("create_search_job", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", authRejectedError(record.URL, record.Method)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", operationError(
			"create_search_job",
			&HTTPStatusError{
				StatusCode: resp.StatusCode,
				Summary:    fmt.Sprintf("failed to create search job: %s", responseSummary(resp, record.Method)),
			},
		)
	}
	var payload struct {
		SID string `json:"sid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("parse search job response: %w", err)
	}
	if payload.SID == "" {
		return "", fmt.Errorf("search job response did not include a sid")
	}
	if containsSessionKey(payload.SID, record.SessionKey) {
		return "", fmt.Errorf("search job response contained credential material")
	}
	return payload.SID, nil
}

type jobStatus struct {
	State              string
	DoneProgress       float64
	ResultCount        int
	ResultPreviewCount int
	ScanCount          int
	EventCount         int
	RunDuration        float64
}

func (s *SearchService) waitForJob(ctx context.Context, base string, record AuthRecord, app, sid string, started time.Time, progress SearchProgressFunc) (jobStatus, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var lastProgress *SearchProgressEvent
	for {
		status, done, err := s.readJobStatus(ctx, base, record, app, sid)
		if err != nil {
			if ctx.Err() != nil {
				return jobStatus{}, &SearchTimeoutError{SID: sid, LastProgress: lastProgress, Err: ctx.Err()}
			}
			return jobStatus{}, err
		}
		if status.State != "" || done {
			event := searchProgressEventFromStatus(sid, status, done, started)
			emitSearchProgress(progress, event)
			lastProgress = &event
		}
		if done {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return jobStatus{}, &SearchTimeoutError{SID: sid, LastProgress: lastProgress, Err: ctx.Err()}
		case <-ticker.C:
		}
	}
}

func (s *SearchService) readJobStatus(ctx context.Context, base string, record AuthRecord, app, sid string) (jobStatus, bool, error) {
	resp, err := s.client.splunkRequest(ctx, base, record, http.MethodGet, searchEndpoint(app, "/jobs/"+url.PathEscape(sid)), url.Values{"output_mode": {"json"}}, nil)
	if err != nil {
		return jobStatus{}, false, operationError("read_search_job_status", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return jobStatus{}, false, authRejectedError(record.URL, record.Method)
	}
	if resp.StatusCode != http.StatusOK {
		summary := responseSummary(resp, record.Method)
		return jobStatus{}, false, operationError(
			"read_search_job_status",
			&HTTPStatusError{
				StatusCode: resp.StatusCode,
				Summary:    fmt.Sprintf("failed to read search job status: %s", summary),
			},
		)
	}
	var payload struct {
		Entry []struct {
			Content map[string]any `json:"content"`
		} `json:"entry"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return jobStatus{}, false, fmt.Errorf("parse search job status: %w", err)
	}
	if len(payload.Entry) == 0 {
		return jobStatus{}, false, nil
	}
	content := payload.Entry[0].Content
	if containsSessionKey(content, record.SessionKey) {
		return jobStatus{}, false, fmt.Errorf("search job status contained credential material")
	}
	state := strings.ToUpper(stringValue(content["dispatchState"]))
	status := jobStatus{
		State:              state,
		DoneProgress:       progressValue(content["doneProgress"]),
		ResultCount:        intValue(content["resultCount"]),
		ResultPreviewCount: intValue(content["resultPreviewCount"]),
		ScanCount:          intValue(content["scanCount"]),
		EventCount:         intValue(content["eventCount"]),
		RunDuration:        floatValue(content["runDuration"]),
	}
	if state == "FAILED" || state == "CANCELED" || state == "CANCELLED" {
		return jobStatus{}, false, &searchJobFailedError{SID: sid}
	}
	if boolValue(content["isDone"]) || state == "DONE" {
		return status, true, nil
	}
	return status, false, nil
}

func (s *SearchService) fetchAllResults(ctx context.Context, base string, record AuthRecord, app, sid string, totalResults, limit, offset, pageSize int, started time.Time, progress SearchProgressFunc) ([]map[string]any, error) {
	if offset >= totalResults {
		return []map[string]any{}, nil
	}
	maxResults := totalResults - offset
	if limit > 0 && limit < maxResults {
		maxResults = limit
	}
	rows := make([]map[string]any, 0, maxResults)
	emitFetchProgress(progress, sid, rows, maxResults, pageSize, started, "starting")
	for len(rows) < maxResults {
		count := pageSize
		remaining := maxResults - len(rows)
		if remaining < count {
			count = remaining
		}
		page, err := s.fetchResultsPage(ctx, base, record, app, sid, count, offset+len(rows))
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		if len(page) > remaining {
			page = page[:remaining]
		}
		rows = append(rows, page...)
		emitFetchProgress(progress, sid, rows, maxResults, pageSize, started, "fetching")
	}
	emitFetchProgress(progress, sid, rows, maxResults, pageSize, started, "done")
	return rows, nil
}

func (s *SearchService) fetchResultsPage(ctx context.Context, base string, record AuthRecord, app, sid string, count, offset int) ([]map[string]any, error) {
	query := url.Values{
		"output_mode": {"json"},
		"count":       {strconv.Itoa(count)},
		"offset":      {strconv.Itoa(offset)},
		"max_lines":   {"0"},
	}
	resp, err := s.client.splunkRequest(ctx, base, record, http.MethodGet, searchEndpoint(app, "/jobs/"+url.PathEscape(sid)+"/results"), query, nil)
	if err != nil {
		return nil, operationError("read_search_results", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, authRejectedError(record.URL, record.Method)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to read search results: %s", responseSummary(resp, record.Method))
	}
	var payload struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("parse search results: %w", err)
	}
	if containsSessionKey(payload.Results, record.SessionKey) {
		return nil, fmt.Errorf("search results contained credential material")
	}
	return payload.Results, nil
}

func (s *SearchService) deleteJob(ctx context.Context, base string, record AuthRecord, app, sid string) error {
	resp, err := s.client.splunkRequest(ctx, base, record, http.MethodDelete, searchEndpoint(app, "/jobs/"+url.PathEscape(sid)), nil, nil)
	if err != nil {
		return operationError(
			"delete_search_job",
			errors.New("search job cleanup request failed before a usable response"),
		)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return operationError(
			"delete_search_job",
			&HTTPStatusError{
				StatusCode: resp.StatusCode,
				Summary:    fmt.Sprintf("failed to delete search job: HTTP %d", resp.StatusCode),
			},
		)
	}
	return nil
}

func searchProgressEventFromStatus(sid string, status jobStatus, done bool, started time.Time) SearchProgressEvent {
	progress := status.DoneProgress
	percent := 0.0
	if progress > 0 {
		percent = progress * 100
	}
	if done && percent == 0 {
		percent = 100
		progress = 1
	}
	event := SearchProgressEvent{
		Phase:                 searchProgressPhaseRunning,
		SID:                   sid,
		State:                 status.State,
		Percent:               percent,
		DoneProgress:          progress,
		ScanCount:             status.ScanCount,
		EventCount:            status.EventCount,
		ResultCount:           status.ResultCount,
		ResultPreviewCount:    status.ResultPreviewCount,
		ElapsedSeconds:        elapsedSeconds(started),
		EstimatedTotalSeconds: estimatedTotalSeconds(started, progress),
	}
	if event.EstimatedTotalSeconds > event.ElapsedSeconds {
		event.ETASeconds = event.EstimatedTotalSeconds - event.ElapsedSeconds
	}
	return event
}

func emitFetchProgress(progress SearchProgressFunc, sid string, rows []map[string]any, totalRows, pageSize int, started time.Time, state string) {
	if progress == nil || totalRows <= 0 {
		return
	}
	event := SearchProgressEvent{
		Phase:          searchProgressPhaseFetch,
		SID:            sid,
		State:          state,
		FetchedRows:    len(rows),
		TotalRows:      totalRows,
		PageSize:       pageSize,
		Percent:        float64(len(rows)) * 100 / float64(totalRows),
		ElapsedSeconds: elapsedSeconds(started),
	}
	if len(rows) > 0 && len(rows) < totalRows && event.ElapsedSeconds > 0 {
		rate := float64(len(rows)) / event.ElapsedSeconds
		if rate > 0 {
			event.ETASeconds = float64(totalRows-len(rows)) / rate
			event.EstimatedTotalSeconds = event.ElapsedSeconds + event.ETASeconds
		}
	}
	if len(rows) >= totalRows {
		event.ETASeconds = 0
		event.EstimatedTotalSeconds = event.ElapsedSeconds
	}
	emitSearchProgress(progress, event)
}

func emitSearchProgress(progress SearchProgressFunc, event SearchProgressEvent) {
	if progress != nil {
		progress(event)
	}
}

func progressValue(value any) float64 {
	progress := floatValue(value)
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}

func elapsedSeconds(started time.Time) float64 {
	elapsed := time.Since(started).Seconds()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func estimatedTotalSeconds(started time.Time, progress float64) float64 {
	if progress <= 0 || progress >= 1 {
		return 0
	}
	return elapsedSeconds(started) / progress
}

func searchEndpoint(app, path string) string {
	if strings.TrimSpace(app) == "" || app == "-" {
		return "/services/search" + path
	}
	return "/servicesNS/-/" + url.PathEscape(app) + "/search" + path
}

func authRejectedError(rawURL string, method AuthMethod) error {
	if method == MethodSessionKey {
		return &SessionAuthRejectedError{URL: rawURL}
	}
	return authRequiredError(rawURL, "stored credentials were rejected by Splunk")
}

type SessionAuthRejectedError struct {
	URL string
}

func (e *SessionAuthRejectedError) Error() string {
	if e.URL == "" {
		return "ephemeral session authentication was rejected by Splunk; provide a fresh session key through the inherited descriptor"
	}
	return fmt.Sprintf("ephemeral session authentication was rejected by Splunk for %s; provide a fresh session key through the inherited descriptor", e.URL)
}

func authRequiredError(rawURL, reason string) error {
	return &AuthRequiredError{URL: rawURL, Reason: reason}
}

type AuthRequiredError struct {
	URL    string
	Reason string
}

func (e *AuthRequiredError) Error() string {
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "stored credentials were rejected by Splunk"
	}
	if e.URL == "" {
		return reason + "; run splsearch auth login --url=<splunk-url>"
	}
	return fmt.Sprintf("%s; run splsearch auth login --url=%s", reason, e.URL)
}

func AuthRequiredURL(err error) (string, bool) {
	var authErr *AuthRequiredError
	if !errors.As(err, &authErr) {
		return "", false
	}
	return authErr.URL, true
}

func (s *SearchService) authValidationError(ctx context.Context, target Target, record AuthRecord) error {
	remoteValid, validateErr := s.client.Validate(ctx, target, record)
	if remoteValid {
		return nil
	}
	if validateErr != nil {
		return validateErr
	}
	return authRejectedError(target.Key, record.Method)
}

func closedConnectionError(err error) bool {
	return errors.Is(err, io.EOF)
}

func searchJobConnectionClosedError(rawURL string) error {
	return fmt.Errorf("Splunk closed the connection while creating the search job for %s; check VPN/network access and run splsearch auth status --url=%s --output=json. If it is not authenticated, run splsearch auth login --url=%s", rawURL, rawURL, rawURL)
}

func responseSummary(resp *http.Response, method AuthMethod) string {
	if method == MethodSessionKey {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, text)
}

func containsSessionKey(value any, sessionKey string) bool {
	if sessionKey == "" {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.Contains(typed, sessionKey)
	case []map[string]any:
		for _, entry := range typed {
			if containsSessionKey(entry, sessionKey) {
				return true
			}
		}
	case []any:
		for _, entry := range typed {
			if containsSessionKey(entry, sessionKey) {
				return true
			}
		}
	case map[string]any:
		for key, entry := range typed {
			if strings.Contains(key, sessionKey) || containsSessionKey(entry, sessionKey) {
				return true
			}
		}
	}
	return false
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(typed)
		return parsed
	default:
		return false
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	case string:
		parsed, _ := strconv.ParseFloat(typed, 64)
		return parsed
	default:
		return 0
	}
}
