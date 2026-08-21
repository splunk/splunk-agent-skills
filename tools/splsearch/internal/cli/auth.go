package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type outputMode string

const (
	outputText outputMode = "text"
	outputJSON outputMode = "json"
)

type authResult struct {
	OK                              bool   `json:"ok"`
	Authenticated                   *bool  `json:"authenticated,omitempty"`
	URL                             string `json:"url,omitempty"`
	RemoteValid                     *bool  `json:"remote_valid,omitempty"`
	Validated                       *bool  `json:"validated,omitempty"`
	Removed                         *bool  `json:"removed,omitempty"`
	ErrorCode                       string `json:"error_code,omitempty"`
	Operation                       string `json:"operation,omitempty"`
	Retryable                       *bool  `json:"retryable,omitempty"`
	RetryableAfterEnvironmentChange *bool  `json:"retryable_after_environment_change,omitempty"`
	RemediationCode                 string `json:"remediation_code,omitempty"`
	DiagnosticHint                  string `json:"diagnostic_hint,omitempty"`
	LaunchErrorSummary              string `json:"launch_error_summary,omitempty"`
	DiagnosticsPath                 string `json:"diagnostics_path,omitempty"`
	RequestedChannel                string `json:"requested_channel,omitempty"`
	AttemptedChannel                string `json:"attempted_channel,omitempty"`
	FallbackUsed                    *bool  `json:"fallback_used,omitempty"`
	Message                         string `json:"message"`
	ExpiresAt                       *int64 `json:"expires_at,omitempty"`
}

type authListResult struct {
	OK            bool         `json:"ok"`
	Authenticated bool         `json:"authenticated"`
	Servers       []authResult `json:"servers"`
	Message       string       `json:"message"`
}

type authFlags struct {
	output   string
	insecure bool
	url      string
}

func newAuthCommand(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Splunk authentication",
		Long: `Manage Splunk authentication for splsearch.

Use ` + "`splsearch auth status`" + ` to list locally stored credentials,
including expired saved targets. Use
` + "`splsearch auth status --url=<splunk-url>`" + ` when a requested Splunk server
needs live validation. If it is not listed or does not validate, run
` + "`splsearch auth login --url=<splunk-url> --output=json`" + `.
Use ` + "`--output=json`" + ` for machine-readable responses.

Browser launch failures return compact JSON with error_code, operation,
retryable, diagnostic_hint, launch_error_summary, and diagnostics_path instead
of raw Playwright logs. Browser bootstrap permission failures that require a
different execution context also set retryable_after_environment_change and
remediation_code. On macOS, splsearch prefers installed stable Chrome when
available and isolates browser home and Crashpad state under the splsearch
config directory. If an installed Chrome or Edge channel fails with macOS
Crashpad permission errors, splsearch retries that same browser through
LaunchServices and local CDP, then retries known .app bundle paths if
LaunchServices cannot resolve the app name. For browser_crashpad_permission,
use launch_error_summary to distinguish macOS bootstrap failures from filesystem
permission failures. Set SPLSEARCH_BROWSER_CHANNEL=chrome or msedge to force a
supported installed browser channel, or set it to an empty value to force bundled
Chromium.`,
		Example: `  splsearch auth login --url=https://splunk.example.com --output=json
  splsearch auth status
  splsearch auth status --url=https://splunk.example.com --output=json
  splsearch auth logout --url=https://splunk.example.com
  splsearch auth status --output=json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newAuthStatusCommand(e), newAuthLoginCommand(e), newAuthLogoutCommand(e))
	return cmd
}

func newAuthStatusCommand(e *env) *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show stored Splunk authentication status",
		Long: `Show Splunk authentication status.

With no URL, status lists Splunk servers with locally stored credentials,
including expired saved targets with per-server state in JSON output.

With --url, status validates one Splunk server remotely. Use JSON output when
another tool needs to choose or verify a server; validation transport failures
include error_code, operation, retryable, and diagnostic_hint. URLs must be
passed with --url=<splunk-url>; positional URLs are not accepted.`,
		Example: `  splsearch auth status
  splsearch auth status --output=json
  splsearch auth status --url=https://splunk.example.com
  splsearch auth status --url=https://splunk.example.com --insecure --output=json`,
		Args: rejectPositionalURL,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseOutputMode(flags.output)
			if err != nil {
				return fail(1, "%w", err)
			}
			service := newAuthService(e, flags.insecure)
			if flags.url == "" {
				statuses, err := service.StatusAll()
				if err != nil {
					result := authListResult{OK: false, Authenticated: false, Message: err.Error()}
					_ = writeAuthListResult(cmd, mode, result)
					return failSilent(1, "%w", err)
				}
				result := authListFromStatuses(statuses)
				if err := writeAuthListResult(cmd, mode, result); err != nil {
					return fail(1, "%w", err)
				}
				if !result.Authenticated {
					return failSilent(1, "%s", result.Message)
				}
				return nil
			}
			status, err := service.Status(cmd.Context(), flags.url)
			if err != nil {
				result := authResult{OK: false, URL: flags.url, Message: err.Error()}
				_ = writeAuthResult(cmd, mode, result)
				return failSilent(1, "%w", err)
			}
			authenticated := status.LocalValid && status.RemoteValid
			result := authResult{
				OK:             authenticated,
				Authenticated:  boolPtr(authenticated),
				URL:            status.URL,
				ErrorCode:      status.ErrorCode,
				Operation:      status.Operation,
				Retryable:      status.Retryable,
				DiagnosticHint: status.DiagnosticHint,
				Message:        status.Message,
				ExpiresAt:      status.ExpiresAt,
			}
			if status.RemoteCheck {
				result.RemoteValid = boolPtr(status.RemoteValid)
			}
			if err := writeAuthResult(cmd, mode, result); err != nil {
				return fail(1, "%w", err)
			}
			if !authenticated {
				return failSilent(1, "%s", status.Message)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.url, "url", "", "Splunk Web or management API URL")
	cmd.Flags().StringVar(&flags.output, "output", string(outputText), "output format: text or json")
	cmd.Flags().BoolVar(&flags.insecure, "insecure", false, "allow insecure TLS for this status check")
	return cmd
}

func newAuthLoginCommand(e *env) *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "login --url=<splunk-url>",
		Short: "Authenticate to Splunk through a browser",
		Long: `Authenticate to Splunk through a browser.

Run this when ` + "`splsearch auth status`" + ` does not list the target server or
` + "`splsearch auth status --url=<splunk-url>`" + ` reports that it is not authenticated.

Do not pass token, basic, or auth-method flags. Provide the Splunk URL with
--url=<splunk-url> plus optional output/TLS flags.

Browser launch diagnostics are written under the splsearch config directory and
referenced by diagnostics_path in JSON failures; stdout stays compact for agent
parsing.`,
		Example: `  splsearch auth login --url=https://splunk.example.com
  splsearch auth login --url=https://splunk.example.com --output=json
  splsearch auth login --url=https://localhost:8000 --insecure`,
		Args: rejectPositionalURL,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseOutputMode(flags.output)
			if err != nil {
				return fail(1, "%w", err)
			}
			if err := requireURLFlag(cmd, flags.url); err != nil {
				return err
			}
			service := newAuthService(e, flags.insecure)
			request := splunk.LoginRequest{
				URL:      flags.url,
				Insecure: flags.insecure,
			}
			login, err := service.Login(cmd.Context(), request)
			if err != nil {
				_ = writeAuthResult(cmd, mode, authErrorResult(flags.url, err))
				return failSilent(1, "%w", err)
			}
			result := authResult{
				OK:        true,
				URL:       login.URL,
				Validated: boolPtr(login.Validated),
				Message:   login.Message,
				ExpiresAt: login.ExpiresAt,
			}
			if err := writeAuthResult(cmd, mode, result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.url, "url", "", "Splunk Web or management API URL")
	cmd.Flags().StringVar(&flags.output, "output", string(outputText), "output format: text or json")
	cmd.Flags().BoolVar(&flags.insecure, "insecure", false, "allow insecure TLS and persist this setting for the URL")
	return cmd
}

func newAuthLogoutCommand(e *env) *cobra.Command {
	var flags authFlags
	cmd := &cobra.Command{
		Use:   "logout --url=<splunk-url>",
		Short: "Remove stored credentials for a Splunk URL",
		Long: `Remove stored Splunk web authentication for one URL.

The URL can be either a Splunk Web URL or a management API URL; splsearch stores
aliases for both when possible. URLs must be passed with --url=<splunk-url>.`,
		Example: `  splsearch auth logout --url=https://splunk.example.com
  splsearch auth logout --url=https://splunk.example.com --output=json`,
		Args: rejectPositionalURL,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode, err := parseOutputMode(flags.output)
			if err != nil {
				return fail(1, "%w", err)
			}
			if err := requireURLFlag(cmd, flags.url); err != nil {
				return err
			}
			service := newAuthService(e, false)
			logout, err := service.Logout(flags.url)
			if err != nil {
				_ = writeAuthResult(cmd, mode, authResult{OK: false, URL: flags.url, Removed: boolPtr(false), Message: err.Error()})
				return failSilent(1, "%w", err)
			}
			result := authResult{
				OK:      logout.Removed,
				URL:     logout.URL,
				Removed: boolPtr(logout.Removed),
				Message: logout.Message,
			}
			if err := writeAuthResult(cmd, mode, result); err != nil {
				return fail(1, "%w", err)
			}
			if !logout.Removed {
				return failSilent(1, "%s", logout.Message)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.url, "url", "", "Splunk Web or management API URL")
	cmd.Flags().StringVar(&flags.output, "output", string(outputText), "output format: text or json")
	return cmd
}

func newAuthService(e *env, insecure bool) *splunk.AuthService {
	return splunk.NewAuthService(splunk.AuthServiceOptions{
		Store:     authStore(e),
		Client:    splunk.NewClient(e.client, insecure),
		Browser:   e.browser,
		ConfigDir: e.configDir,
	})
}

func parseOutputMode(value string) (outputMode, error) {
	switch outputMode(value) {
	case outputText:
		return outputText, nil
	case outputJSON:
		return outputJSON, nil
	default:
		return "", fmt.Errorf("unsupported output format %q", value)
	}
}

func rejectPositionalURL(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fail(1, "unexpected positional argument %q\nuse --url=<splunk-url>\nusage: %s", args[0], cmd.UseLine())
}

func requireURLFlag(cmd *cobra.Command, value string) error {
	if strings.TrimSpace(value) != "" {
		return nil
	}
	return fail(1, "missing --url=<splunk-url>\nusage: %s", cmd.UseLine())
}

func writeAuthResult(cmd *cobra.Command, mode outputMode, result authResult) error {
	switch mode {
	case outputJSON:
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	default:
		_, err := fmt.Fprintln(cmd.OutOrStdout(), textAuthResult(result))
		return err
	}
}

func writeAuthListResult(cmd *cobra.Command, mode outputMode, result authListResult) error {
	switch mode {
	case outputJSON:
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return err
	default:
		_, err := fmt.Fprintln(cmd.OutOrStdout(), textAuthListResult(result))
		return err
	}
}

func textAuthResult(result authResult) string {
	if result.Message != "" {
		return result.Message
	}
	if result.OK {
		return "ok"
	}
	return "not authenticated"
}

func authListFromStatuses(statuses []splunk.StatusResult) authListResult {
	servers := make([]authResult, 0, len(statuses))
	authenticatedAny := false
	for _, status := range statuses {
		authenticated := status.LocalValid
		if authenticated {
			authenticatedAny = true
		}
		servers = append(servers, authResult{
			OK:             authenticated,
			Authenticated:  boolPtr(authenticated),
			URL:            status.URL,
			ErrorCode:      status.ErrorCode,
			Operation:      status.Operation,
			Retryable:      status.Retryable,
			DiagnosticHint: status.DiagnosticHint,
			Message:        status.Message,
			ExpiresAt:      status.ExpiresAt,
		})
	}
	if len(servers) == 0 {
		return authListResult{OK: false, Authenticated: false, Servers: servers, Message: "not authenticated"}
	}
	if !authenticatedAny {
		return authListResult{OK: false, Authenticated: false, Servers: servers, Message: "not authenticated"}
	}
	return authListResult{OK: true, Authenticated: true, Servers: servers, Message: "authenticated"}
}

func authErrorResult(rawURL string, err error) authResult {
	result := authResult{OK: false, URL: rawURL, Message: err.Error()}
	if details, ok := splunk.BrowserErrorDetails(err); ok {
		result.ErrorCode = details.ErrorCode
		result.Operation = "authenticate"
		result.Retryable = boolPtr(browserErrorRetryable(details.ErrorCode))
		result.RetryableAfterEnvironmentChange, result.RemediationCode = browserEnvironmentRetryFields(details)
		result.DiagnosticHint = browserAuthDiagnosticHint(details.ErrorCode)
		result.LaunchErrorSummary = details.LaunchErrorSummary
		result.DiagnosticsPath = details.DiagnosticsPath
		result.RequestedChannel = details.RequestedChannel
		result.AttemptedChannel = details.AttemptedChannel
		result.FallbackUsed = boolPtr(details.FallbackUsed)
	}
	return result
}

func textAuthListResult(result authListResult) string {
	if len(result.Servers) == 0 {
		return "No authenticated Splunk servers.\nRun `splsearch auth login --url=<splunk-url>` to add one."
	}
	lines := make([]string, 0, len(result.Servers))
	for _, server := range result.Servers {
		line := "- " + server.URL
		if server.Message != "" && server.Message != "authenticated" {
			line += " - " + server.Message
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func boolPtr(value bool) *bool {
	return &value
}
