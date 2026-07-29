package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

const defaultSearchTimeout = 10 * time.Minute
const defaultSearchTimeoutText = "10m"
const autoAuthTimeout = 5 * time.Minute
const broadSearchWarningRows = 10000
const searchProgressEvery = 10 * time.Second

const (
	searchProgressText  = "text"
	searchProgressJSONL = "jsonl"
	searchProgressOff   = "off"
)

type searchFlags struct {
	url         string
	query       string
	earliest    string
	latest      string
	timeout     string
	app         string
	resultTable string
	limit       int
	offset      int
	pageSize    int
	immediate   bool
	progress    string
}

type searchErrorResult struct {
	OK                              bool                        `json:"ok"`
	URL                             string                      `json:"url,omitempty"`
	ErrorCode                       string                      `json:"error_code,omitempty"`
	Operation                       string                      `json:"operation,omitempty"`
	Retryable                       *bool                       `json:"retryable,omitempty"`
	RetryableAfterEnvironmentChange *bool                       `json:"retryable_after_environment_change,omitempty"`
	RemediationCode                 string                      `json:"remediation_code,omitempty"`
	TableCreated                    *bool                       `json:"table_created,omitempty"`
	DiagnosticHint                  string                      `json:"diagnostic_hint,omitempty"`
	LaunchErrorSummary              string                      `json:"launch_error_summary,omitempty"`
	DiagnosticsPath                 string                      `json:"diagnostics_path,omitempty"`
	RequestedChannel                string                      `json:"requested_channel,omitempty"`
	AttemptedChannel                string                      `json:"attempted_channel,omitempty"`
	FallbackUsed                    *bool                       `json:"fallback_used,omitempty"`
	SID                             string                      `json:"sid,omitempty"`
	LastProgress                    *splunk.SearchProgressEvent `json:"last_progress,omitempty"`
	EstimatedTotalSeconds           *float64                    `json:"estimated_total_seconds,omitempty"`
	ETASeconds                      *float64                    `json:"eta_seconds,omitempty"`
	Message                         string                      `json:"message"`
}

func newSearchCommand(e *env) *cobra.Command {
	var flags searchFlags
	cmd := &cobra.Command{
		Use:   "search --query=<SPL>",
		Short: "Run a Splunk search and store results in SQLite",
		Long: `Run a Splunk search through the authenticated Splunk Web session.

If --url is omitted, splsearch uses the most recently authenticated Splunk
server. Authenticate first with ` + "`splsearch auth login --url=<splunk-url>`" + `.
If a known server needs authentication during search, splsearch opens browser
login automatically and retries the search once.

By default, results are written to a SQLite table in the splsearch config
directory and stdout returns one compact JSON summary with the database path,
table name, row count, and search metadata.

Use --result-table=<name> to choose the table name. If that table already
exists, splsearch replaces it with the new search result.

When --limit=0, splsearch fetches all available results into the local table.
Large searches still work, but JSON output may include warnings when Splunk
reports a broad result set.

Use --immediate only when the caller expects very short output. It prints the
raw search results JSON to stdout and skips SQLite storage.

Progress is written to stderr so stdout stays parseable. The default
--progress=text reports job polling, result fetching, and large SQLite writes.
Use --progress=jsonl when an AI caller wants parseable progress events, or
--progress=off to suppress progress output. If a search job exceeds --timeout,
the failure JSON includes error_code:"search_timeout" and last_progress when
Splunk exposed job status before the timeout.

When a search fails before creating a result table, including during automatic
browser authentication, JSON output keeps the human-readable message and
includes stable fields such as error_code, operation, retryable,
table_created:false, and diagnostic_hint. Browser launch failures also include
launch_error_summary. Browser bootstrap permission failures that require a
different execution context also set retryable_after_environment_change and
remediation_code. A search failure JSON envelope with ok:false exits non-zero.`,
		Example: `  splsearch search --query='index=_internal | head 5'
  splsearch search --url=https://splunk.example.com --query='index=_internal' --earliest=-15m --latest=now
  splsearch search --query='| tstats count where index=*' --timeout=10m --app=search --result-table=summary_counts
  splsearch search --query='index=_internal | stats count by sourcetype' --progress=jsonl
  splsearch search --query='index=_internal | head 3' --limit=3 --immediate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeSearchErrorAndFail(cmd, flags.url, "unexpected positional argument %q; use --query=<SPL> and optional --url=<splunk-url>", args[0])
			}
			timeout, err := parseSearchTimeout(flags.timeout)
			if err != nil {
				return writeSearchErrorAndFail(cmd, flags.url, "%s", err.Error())
			}
			progressMode, err := parseSearchProgressMode(flags.progress)
			if err != nil {
				return writeSearchErrorAndFail(cmd, flags.url, "%s", err.Error())
			}
			if strings.TrimSpace(flags.query) == "" {
				return writeSearchErrorAndFail(cmd, flags.url, "missing --query=<SPL>")
			}
			if flags.limit < 0 {
				return writeSearchErrorAndFail(cmd, flags.url, "--limit must be >= 0")
			}
			if flags.offset < 0 {
				return writeSearchErrorAndFail(cmd, flags.url, "--offset must be >= 0")
			}
			if flags.pageSize < 1 {
				return writeSearchErrorAndFail(cmd, flags.url, "--page-size must be >= 1")
			}
			if flags.immediate && strings.TrimSpace(flags.resultTable) != "" {
				return writeSearchErrorAndFail(cmd, flags.url, "--result-table cannot be used with --immediate")
			}
			if err := splunk.ValidateResultTableName(flags.resultTable); err != nil {
				return writeSearchErrorAndFail(cmd, flags.url, "%s", err.Error())
			}

			progressPrinter := newSearchProgressPrinter(cmd.ErrOrStderr(), progressMode)
			resultStore := splunk.NewResultStore(e.configDir)
			var cleanupDone <-chan struct{}
			if !flags.immediate {
				cleanupDone = cleanupResultTablesAsync(resultStore)
			}
			request := splunk.SearchRequest{
				URL:      flags.url,
				Query:    flags.query,
				Earliest: flags.earliest,
				Latest:   flags.latest,
				App:      flags.app,
				Limit:    flags.limit,
				Offset:   flags.offset,
				PageSize: flags.pageSize,
				Progress: progressPrinter.Report,
			}
			result, err := runSearchWithAutoAuth(cmd, e, timeout, request)
			if err != nil {
				waitForResultCleanup(cleanupDone)
				return writeSearchErrorAndFailErr(cmd, flags.url, err)
			}
			if flags.immediate {
				waitForResultCleanup(cleanupDone)
				if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
					return fail(1, "%w", err)
				}
				return nil
			}

			warnings := searchWarnings(flags, result)
			saveOptions := splunk.SaveOptions{WarningDetails: warnings}
			switch progressMode {
			case searchProgressText:
				saveOptions.Progress = cmd.ErrOrStderr()
			case searchProgressJSONL:
				saveOptions.StructuredProgress = progressPrinter.Report
			}
			stored, err := resultStore.SaveWithOptions(cmd.Context(), result, flags.resultTable, saveOptions)
			if err != nil {
				waitForResultCleanup(cleanupDone)
				return writeSearchErrorAndFailErr(cmd, flags.url, err)
			}
			stored.TextSearchCommand = resultTextSearchCommand(stored.Table)
			waitForResultCleanup(cleanupDone)
			if err := writeSearchJSON(cmd.OutOrStdout(), stored); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.url, "url", "", "Splunk Web or management API URL; defaults to the last authenticated server")
	cmd.Flags().StringVar(&flags.query, "query", "", "SPL query to run")
	cmd.Flags().StringVar(&flags.earliest, "earliest", "-1h", "earliest search time")
	cmd.Flags().StringVar(&flags.latest, "latest", "now", "latest search time")
	cmd.Flags().StringVar(&flags.timeout, "timeout", defaultSearchTimeoutText, "search timeout, e.g. 10m or 600s")
	cmd.Flags().StringVar(&flags.app, "app", "search", "Splunk app context; use '-' for the app-less search endpoint")
	cmd.Flags().StringVar(&flags.resultTable, "result-table", "", "SQLite result table name; replaces an existing table with the same name")
	cmd.Flags().IntVar(&flags.limit, "limit", 0, "maximum results to fetch; 0 means all available results")
	cmd.Flags().IntVar(&flags.offset, "offset", 0, "result offset for pagination")
	cmd.Flags().IntVar(&flags.pageSize, "page-size", 1000, "results to fetch per Splunk request")
	cmd.Flags().BoolVar(&flags.immediate, "immediate", false, "print raw results to stdout and skip SQLite; use only for very short expected output")
	cmd.Flags().StringVar(&flags.progress, "progress", searchProgressText, "progress output on stderr: text, jsonl, or off")
	return cmd
}

func newSearchService(e *env) *splunk.SearchService {
	return splunk.NewSearchService(splunk.SearchServiceOptions{
		Store:  authStore(e),
		Client: splunk.NewClient(e.client, false),
	})
}

func runSearchWithAutoAuth(cmd *cobra.Command, e *env, timeout time.Duration, request splunk.SearchRequest) (splunk.SearchResult, error) {
	result, err := runSearchOnce(cmd.Context(), e, timeout, request)
	if err == nil {
		return result, nil
	}
	authURL, ok := splunk.AuthRequiredURL(err)
	if !ok {
		return result, err
	}
	if strings.TrimSpace(authURL) == "" && strings.TrimSpace(request.URL) != "" {
		target, targetErr := splunk.NormalizeTarget(request.URL)
		if targetErr == nil {
			authURL = target.Key
		} else {
			authURL = request.URL
		}
	}
	if strings.TrimSpace(authURL) == "" || e.browser == nil {
		return result, err
	}
	if storedAuthStillValid(cmd.Context(), e, authURL) {
		return result, fmt.Errorf("search was rejected by Splunk, but stored authentication for %s still validates; not opening browser login. Check search permissions, app context, or the SPL request: %w", authURL, err)
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "authentication required for %s; opening browser login\n", authURL)
	authCtx, cancel := context.WithTimeout(cmd.Context(), autoAuthTimeout)
	defer cancel()
	login, loginErr := newAuthService(e, false).Login(authCtx, splunk.LoginRequest{URL: authURL})
	if loginErr != nil {
		return result, fmt.Errorf("automatic browser authentication failed for %s: %w", authURL, loginErr)
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "authenticated %s; retrying search\n", login.URL)

	retry := request
	if strings.TrimSpace(retry.URL) == "" {
		retry.URL = login.URL
	}
	return runSearchOnce(cmd.Context(), e, timeout, retry)
}

func storedAuthStillValid(ctx context.Context, e *env, rawURL string) bool {
	status, err := newAuthService(e, false).Status(ctx, rawURL)
	return err == nil && status.LocalValid && status.RemoteCheck && status.RemoteValid
}

func runSearchOnce(ctx context.Context, e *env, timeout time.Duration, request splunk.SearchRequest) (splunk.SearchResult, error) {
	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return newSearchService(e).Search(searchCtx, request)
}

func parseSearchTimeout(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultSearchTimeout, nil
	}
	if secondsOnly(value) {
		seconds, err := time.ParseDuration(value + "s")
		if err != nil {
			return 0, fmt.Errorf("invalid --timeout %q", value)
		}
		if seconds <= 0 {
			return 0, fmt.Errorf("--timeout must be > 0")
		}
		return seconds, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid --timeout %q; use values like 10m or 600s", value)
	}
	if duration <= 0 {
		return 0, fmt.Errorf("--timeout must be > 0")
	}
	return duration, nil
}

func parseSearchProgressMode(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return searchProgressText, nil
	}
	switch value {
	case searchProgressText, searchProgressJSONL, searchProgressOff:
		return value, nil
	default:
		return "", fmt.Errorf("invalid --progress %q; use text, jsonl, or off", value)
	}
}

type searchProgressPrinter struct {
	writer      io.Writer
	mode        string
	encoder     *json.Encoder
	printed     bool
	lastPrinted time.Time
	lastPhase   string
	lastState   string
	lastPercent float64
	lastFetched int
	lastWritten int
}

func newSearchProgressPrinter(writer io.Writer, mode string) *searchProgressPrinter {
	printer := &searchProgressPrinter{writer: writer, mode: mode}
	if mode == searchProgressJSONL && writer != nil {
		printer.encoder = json.NewEncoder(writer)
		printer.encoder.SetEscapeHTML(false)
	}
	return printer
}

func (p *searchProgressPrinter) Report(event splunk.SearchProgressEvent) {
	if p == nil || p.writer == nil || p.mode == searchProgressOff || event.Phase == "" {
		return
	}
	if !p.shouldPrint(event) {
		return
	}
	switch p.mode {
	case searchProgressJSONL:
		payload := struct {
			Type string `json:"type"`
			splunk.SearchProgressEvent
		}{
			Type:                "splsearch_progress",
			SearchProgressEvent: event,
		}
		_ = p.encoder.Encode(payload)
	default:
		_, _ = fmt.Fprintln(p.writer, formatSearchProgressText(event))
	}
	p.printed = true
	p.lastPrinted = time.Now()
	p.lastPhase = event.Phase
	p.lastState = event.State
	p.lastPercent = event.Percent
	p.lastFetched = event.FetchedRows
	p.lastWritten = event.WrittenRows
}

func (p *searchProgressPrinter) shouldPrint(event splunk.SearchProgressEvent) bool {
	if !p.printed {
		return true
	}
	if event.Phase != p.lastPhase || event.State != p.lastState {
		return true
	}
	if event.Percent >= 100 && p.lastPercent < 100 {
		return true
	}
	if event.Percent-p.lastPercent >= 10 {
		return true
	}
	if event.Phase == "fetch" && event.TotalRows > 0 && event.FetchedRows != p.lastFetched && (event.FetchedRows == 0 || event.FetchedRows >= event.TotalRows) {
		return true
	}
	if event.Phase == "write" && event.TotalRows > 0 && event.WrittenRows != p.lastWritten && (event.WrittenRows == 0 || event.WrittenRows >= event.TotalRows) {
		return true
	}
	return time.Since(p.lastPrinted) >= searchProgressEvery
}

func formatSearchProgressText(event splunk.SearchProgressEvent) string {
	switch event.Phase {
	case "fetch":
		return formatRowProgressText("fetch", "fetched", event)
	case "write":
		return formatRowProgressText("write", "written", event)
	default:
		parts := []string{"splsearch:", "job", "phase=" + event.Phase}
		if event.SID != "" {
			parts = append(parts, "sid="+event.SID)
		}
		if event.State != "" {
			parts = append(parts, "state="+event.State)
		}
		appendSearchProgressFields(&parts, event)
		return strings.Join(parts, " ")
	}
}

func formatRowProgressText(label, rowField string, event splunk.SearchProgressEvent) string {
	parts := []string{"splsearch:", label}
	if event.SID != "" {
		parts = append(parts, "sid="+event.SID)
	}
	if event.Table != "" {
		parts = append(parts, "table="+event.Table)
	}
	if event.State != "" {
		parts = append(parts, "state="+event.State)
	}
	rows := event.FetchedRows
	if rowField == "written" {
		rows = event.WrittenRows
	}
	if event.TotalRows > 0 {
		parts = append(parts, fmt.Sprintf("%s_rows=%d/%d", rowField, rows, event.TotalRows))
	}
	if event.PageSize > 0 {
		parts = append(parts, fmt.Sprintf("page_size=%d", event.PageSize))
	}
	appendSearchProgressFields(&parts, event)
	return strings.Join(parts, " ")
}

func appendSearchProgressFields(parts *[]string, event splunk.SearchProgressEvent) {
	if event.Percent > 0 {
		*parts = append(*parts, fmt.Sprintf("progress=%.1f%%", event.Percent))
	}
	if event.ScanCount > 0 {
		*parts = append(*parts, fmt.Sprintf("scanned=%d", event.ScanCount))
	}
	if event.EventCount > 0 {
		*parts = append(*parts, fmt.Sprintf("events=%d", event.EventCount))
	}
	if event.ResultCount > 0 {
		*parts = append(*parts, fmt.Sprintf("results=%d", event.ResultCount))
	}
	if event.ResultPreviewCount > 0 {
		*parts = append(*parts, fmt.Sprintf("preview=%d", event.ResultPreviewCount))
	}
	*parts = append(*parts, "elapsed="+formatProgressDuration(event.ElapsedSeconds))
	if event.ETASeconds > 0 {
		*parts = append(*parts, "eta="+formatProgressDuration(event.ETASeconds))
	} else if event.Percent > 0 && event.Percent < 100 {
		*parts = append(*parts, "eta=unknown")
	} else if event.Percent >= 100 {
		*parts = append(*parts, "eta=0s")
	}
}

func formatProgressDuration(seconds float64) string {
	if seconds <= 0 {
		return "0s"
	}
	return time.Duration(seconds * float64(time.Second)).Round(time.Second).String()
}

func searchWarnings(flags searchFlags, result splunk.SearchResult) []splunk.ResultWarning {
	if flags.immediate || flags.limit != 0 || result.ResultCount <= broadSearchWarningRows {
		return nil
	}
	return []splunk.ResultWarning{{
		Code:    splunk.ResultWarningCodeFullFetch,
		Message: fmt.Sprintf("search fetched all available rows because --limit=0 and Splunk reported %d results; scope SPL or set --limit when a complete local table is not needed", result.ResultCount),
	}}
}

func secondsOnly(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return value != ""
}

func cleanupResultTablesAsync(store *splunk.ResultStore) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = store.CleanupOlderThan(ctx, 24*time.Hour)
	}()
	return done
}

func waitForResultCleanup(done <-chan struct{}) {
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
	}
}

func writeSearchErrorAndFail(cmd *cobra.Command, rawURL, format string, args ...any) error {
	return writeSearchErrorAndFailErr(cmd, rawURL, fmt.Errorf(format, args...))
}

func writeSearchErrorAndFailErr(cmd *cobra.Command, rawURL string, err error) error {
	message := err.Error()
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorFromError(rawURL, err))
	return failSilent(1, "%s", message)
}

func searchErrorFromError(rawURL string, err error) searchErrorResult {
	result := searchErrorResult{OK: false, URL: rawURL, Message: err.Error()}
	if details, ok := splunk.BrowserErrorDetails(err); ok {
		result.ErrorCode = details.ErrorCode
		result.Operation = "authenticate"
		result.Retryable = boolPtr(browserErrorRetryable(details.ErrorCode))
		result.TableCreated = boolPtr(false)
		result.RetryableAfterEnvironmentChange, result.RemediationCode = browserEnvironmentRetryFields(details)
		result.DiagnosticHint = browserSearchDiagnosticHint(details.ErrorCode)
		result.LaunchErrorSummary = details.LaunchErrorSummary
		result.DiagnosticsPath = details.DiagnosticsPath
		result.RequestedChannel = details.RequestedChannel
		result.AttemptedChannel = details.AttemptedChannel
		result.FallbackUsed = boolPtr(details.FallbackUsed)
		return result
	}
	if details, ok := splunk.StructuredError(err); ok {
		result.ErrorCode = details.ErrorCode
		result.Operation = details.Operation
		result.Retryable = boolPtr(details.Retryable)
		result.TableCreated = boolPtr(false)
		result.DiagnosticHint = details.DiagnosticHint
		result.SID = details.SID
		result.LastProgress = details.LastProgress
		if details.LastProgress != nil {
			if details.LastProgress.EstimatedTotalSeconds > 0 {
				result.EstimatedTotalSeconds = float64Ptr(details.LastProgress.EstimatedTotalSeconds)
			}
			if details.LastProgress.ETASeconds > 0 {
				result.ETASeconds = float64Ptr(details.LastProgress.ETASeconds)
			}
		}
	}
	return result
}

func browserErrorRetryable(errorCode string) bool {
	switch errorCode {
	case "browser_crashpad_permission", "unsupported_browser_channel", "playwright_install_required", "browser_channel_unavailable":
		return false
	default:
		return true
	}
}

func browserEnvironmentRetryFields(details splunk.BrowserAuthErrorDetails) (*bool, string) {
	if !details.RetryableAfterEnvironmentChange {
		return nil, ""
	}
	return boolPtr(true), details.RemediationCode
}

func browserSearchDiagnosticHint(errorCode string) string {
	return browserDiagnosticHint(errorCode, true)
}

func browserAuthDiagnosticHint(errorCode string) string {
	return browserDiagnosticHint(errorCode, false)
}

func browserDiagnosticHint(errorCode string, searchTableContext bool) string {
	switch errorCode {
	case "browser_crashpad_permission":
		if !searchTableContext {
			return "Browser authentication failed before credentials were saved because browser Crashpad bootstrap or state setup was blocked. Use launch_error_summary to distinguish macOS bootstrap_check_in or child_port_handshake failures from filesystem permission failures; for bootstrap failures, rerun from a normal unsandboxed terminal or IDE session, then retry auth."
		}
		return "Browser authentication failed before creating a result table because browser Crashpad bootstrap or state setup was blocked. Use launch_error_summary to distinguish macOS bootstrap_check_in or child_port_handshake failures from filesystem permission failures; for bootstrap failures, rerun from a normal unsandboxed terminal or IDE session, then retry the search."
	case "unsupported_browser_channel":
		return "SPLSEARCH_BROWSER_CHANNEL names an unsupported browser channel. Use chrome, msedge, another supported channel, or set it to an empty value to force bundled Chromium."
	case "playwright_install_required":
		return "Playwright driver or browser assets are missing. Run make playwright-install from the splsearch repo under the same HOME, or use an installed supported browser channel."
	case "browser_channel_unavailable":
		return "The requested browser channel is not installed. Install it, choose another supported SPLSEARCH_BROWSER_CHANNEL value, or set SPLSEARCH_BROWSER_CHANNEL= to force bundled Chromium."
	default:
		if !searchTableContext {
			return "Browser authentication failed before credentials were saved. Inspect diagnostics_path when present, verify local browser availability, and rerun auth login."
		}
		return "Browser authentication failed before creating a result table. Inspect diagnostics_path when present, verify local browser availability, and rerun the search after authentication succeeds."
	}
}

func writeSearchJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func float64Ptr(value float64) *float64 {
	return &value
}
