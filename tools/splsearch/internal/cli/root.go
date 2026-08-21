package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type env struct {
	configDir string
	out       io.Writer
	err       io.Writer
	client    *http.Client
	browser   splunk.BrowserAuthenticator
	store     splunk.Store
}

type unavailableStore struct {
	err error
}

func (s unavailableStore) Get(splunk.Target) (*splunk.AuthRecord, error) {
	return nil, s.err
}

func (s unavailableStore) List() ([]splunk.AuthRecord, error) {
	return nil, s.err
}

func (s unavailableStore) Set(splunk.Target, splunk.AuthRecord) error {
	return s.err
}

func (s unavailableStore) Delete(splunk.Target) (bool, error) {
	return false, s.err
}

type exitError struct {
	code   int
	err    error
	silent bool
}

func (e *exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

func Main(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := NewRootCommand(stdout, stderr)
	return executeCommand(ctx, command, args, stderr)
}

func executeCommand(ctx context.Context, command *cobra.Command, args []string, stderr io.Writer) int {
	command.SetContext(ctx)
	command.SetArgs(args)
	if err := command.Execute(); err != nil {
		if ee, ok := err.(*exitError); ok {
			if !ee.silent && err.Error() != "" {
				_, _ = fmt.Fprintln(stderr, err.Error())
			}
			return ee.code
		}
		if err.Error() != "" {
			_, _ = fmt.Fprintln(stderr, err.Error())
		}
		return 1
	}
	return 0
}

func NewRootCommand(stdout, stderr io.Writer) *cobra.Command {
	configDir := splunk.DefaultConfigDir()
	store, err := splunk.NewAuthStoreFromEnvironment(configDir)
	if err != nil {
		store = unavailableStore{err: err}
	}
	e := &env{
		configDir: configDir,
		out:       stdout,
		err:       stderr,
		client:    http.DefaultClient,
		browser:   splunk.NewPlaywrightAuthenticator(),
		store:     store,
	}
	return newRootCommand(e)
}

func authStore(e *env) splunk.Store {
	if e.store != nil {
		return e.store
	}
	return splunk.NewFileStore(e.configDir)
}

func newRootCommand(e *env) *cobra.Command {
	root := &cobra.Command{
		Use:   "splsearch",
		Short: "Search Splunk without flooding AI context",
		Long: `splsearch is a CLI for AI agents that need to work with Splunk.

Use it before running Splunk search workflows:
1. Run ` + "`splsearch auth status`" + ` to list Splunk servers with locally stored credentials, including expired saved targets.
2. Run ` + "`splsearch auth status --url=<splunk-url> --output=json`" + ` when a target needs live validation.
3. If the target is missing or does not validate, run ` + "`splsearch auth login --url=<splunk-url> --output=json`" + `.
4. Run ` + "`splsearch search --query='<SPL>'`" + ` to store results in SQLite and get the table name.
5. Run ` + "`splsearch results-list`" + ` to see saved tables and ready-to-run commands.
6. Run ` + "`splsearch result-info --table=<table>`" + ` to inspect metadata and warnings.
7. Run ` + "`splsearch result-warnings accept --table=<table> --code=full_fetch`" + ` after reviewing accepted warnings.
8. Run ` + "`splsearch result-schema --table=<table>`" + ` to inspect available columns.
9. Run ` + "`splsearch result-text-search --table=<table> --query='<text>'`" + ` to find saved rows by local BM25/FTS text search.
10. Run ` + "`splsearch result-summary --table=<table> --group-by=<field>`" + ` for first-pass aggregates.
11. Run ` + "`splsearch result-events --table=<table> --field=<field> --value=<value>`" + ` for ordered matching events.
12. Run ` + "`splsearch result-search --table=<table> --query='<SQL>'`" + ` for custom saved-result SQL.
13. Run ` + "`splsearch results-drop --table=<table>`" + ` when saved result tables are no longer needed.
14. Use ` + "`--output=json`" + ` on auth commands when another tool needs to parse auth state.

Both Splunk Web URLs and management API URLs are accepted. Do not pass auth
method flags; login uses the supported interactive flow. If search sees missing
or rejected stored auth for a known URL, it opens browser login and retries once.
Use ` + "`splsearch search --immediate`" + `
only when the caller expects very short output.`,
		Example: `  splsearch auth login --url=https://splunk.example.com --output=json
  splsearch auth status
  splsearch auth status --url=https://splunk.example.com --output=json
  splsearch search --query='index=_internal | head 5'
  splsearch search --url=https://splunk.example.com --query='index=_internal' --earliest=-15m --latest=now --result-table=internal_events
  splsearch results-list --limit=20
  splsearch result-info --table=internal_events
  splsearch result-warnings accept --table=internal_events --code=full_fetch
  splsearch result-schema --table=internal_events
  splsearch result-text-search --table=internal_events --query='request_remote_tok 401 Unauthorized'
  splsearch result-summary --table=internal_events --group-by=host --time-from=2026-04-28T10:00:00Z --time-to=2026-04-28T10:10:00Z --limit=20
  splsearch result-summary --table=internal_events --group-by=host --metric=duration_ms --preset=latency --limit=20
  splsearch result-events --table=internal_events --field=host --value=web01
  splsearch result-events --table=internal_events --request-id=abc-123
  splsearch result-search --table=internal_events --query='SELECT _time, host, _json FROM results LIMIT 20' --limit=20
  splsearch results-drop --table=internal_events
  splsearch search --query='index=_internal | head 3' --limit=3 --immediate
  splsearch auth logout --url=https://splunk.example.com

Auth command options:
  --output=json   Return compact JSON for AI/tool parsing.
  --insecure      Allow self-signed TLS for local/dev Splunk during login/status.

Browser auth:
  SPLSEARCH_BROWSER_CHANNEL=chrome   Force a supported installed browser channel.
  SPLSEARCH_BROWSER_CHANNEL=         Force bundled Chromium.
  On macOS, installed Chrome/Edge channels retry through LaunchServices/CDP and known .app bundle paths after Crashpad permission failures.
  Browser launch failures keep stdout compact with error_code, diagnostic_hint, and launch_error_summary; bootstrap_check_in also sets retryable_after_environment_change.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetOut(e.out)
	root.SetErr(e.err)
	root.AddCommand(newAuthCommand(e), newSearchCommand(e), newResultsListCommand(e), newResultInfoCommand(e), newResultWarningsCommand(e), newResultSchemaCommand(e), newResultTextSearchCommand(e), newResultSummaryCommand(e), newResultEventsCommand(e), newResultSearchCommand(e), newResultsDropCommand(e))
	return root
}

func fail(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

func failSilent(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...), silent: true}
}
