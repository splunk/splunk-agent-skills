package splunk

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
)

type BrowserSession struct {
	Cookies   []Cookie
	FinalURL  string
	ExpiresAt *int64
}

type BrowserAuthOptions struct {
	Insecure bool
}

type BrowserAuthenticator interface {
	Authenticate(ctx context.Context, target Target, configDir string, options BrowserAuthOptions) (BrowserSession, error)
}

type PlaywrightAuthenticator struct{}

const defaultBrowserErrorHold = 30 * time.Second

var supportedBrowserChannels = map[string]bool{
	"chrome":        true,
	"chrome-beta":   true,
	"chrome-dev":    true,
	"chrome-canary": true,
	"msedge":        true,
	"msedge-beta":   true,
	"msedge-dev":    true,
	"msedge-canary": true,
}

var supportedBrowserChannelNames = []string{
	"chrome",
	"chrome-beta",
	"chrome-dev",
	"chrome-canary",
	"msedge",
	"msedge-beta",
	"msedge-dev",
	"msedge-canary",
}

var macHomePathPattern = regexp.MustCompile(`/Users/[^/\s]+`)

var browserRuntimeGOOS = runtime.GOOS
var openBrowserApp = runOpenBrowserApp
var runOpenCommand = func(args []string) ([]byte, error) {
	return exec.Command("/usr/bin/open", args...).CombinedOutput()
}
var browserAppBundleCandidates = defaultBrowserAppBundleCandidates

type browserChannelSelection struct {
	Channel  string
	Explicit bool
}

type browserLaunchAttempt struct {
	Channel     string
	Strategy    string
	ProfileDir  string
	CrashpadDir string
	Options     playwright.BrowserTypeLaunchPersistentContextOptions
}

type browserContextLauncher interface {
	LaunchPersistentContext(userDataDir string, options ...playwright.BrowserTypeLaunchPersistentContextOptions) (playwright.BrowserContext, error)
	ConnectOverCDP(endpointURL string, options ...playwright.BrowserTypeConnectOverCDPOptions) (playwright.Browser, error)
}

type cdpBrowserContext struct {
	playwright.BrowserContext
	browser playwright.Browser
}

func (c *cdpBrowserContext) Close(options ...playwright.BrowserContextCloseOptions) error {
	if c.browser != nil {
		return c.browser.Close()
	}
	if c.BrowserContext != nil {
		return c.BrowserContext.Close(options...)
	}
	return nil
}

type BrowserAuthErrorDetails struct {
	ErrorCode                       string
	LaunchErrorSummary              string
	DiagnosticsPath                 string
	RequestedChannel                string
	AttemptedChannel                string
	FallbackUsed                    bool
	RetryableAfterEnvironmentChange bool
	RemediationCode                 string
}

type browserAuthError struct {
	code                            string
	message                         string
	launchErrorSummary              string
	diagnosticsPath                 string
	requestedChannel                string
	attemptedChannel                string
	fallbackUsed                    bool
	retryableAfterEnvironmentChange bool
	remediationCode                 string
	underlying                      error
}

func (e *browserAuthError) Error() string {
	return e.message
}

func (e *browserAuthError) Unwrap() error {
	return e.underlying
}

func BrowserErrorDetails(err error) (BrowserAuthErrorDetails, bool) {
	var browserErr *browserAuthError
	if !errors.As(err, &browserErr) {
		return BrowserAuthErrorDetails{}, false
	}
	return BrowserAuthErrorDetails{
		ErrorCode:                       browserErr.code,
		LaunchErrorSummary:              browserErr.launchErrorSummary,
		DiagnosticsPath:                 browserErr.diagnosticsPath,
		RequestedChannel:                browserErr.requestedChannel,
		AttemptedChannel:                browserErr.attemptedChannel,
		FallbackUsed:                    browserErr.fallbackUsed,
		RetryableAfterEnvironmentChange: browserErr.retryableAfterEnvironmentChange,
		RemediationCode:                 browserErr.remediationCode,
	}, true
}

func NewPlaywrightAuthenticator() *PlaywrightAuthenticator {
	return &PlaywrightAuthenticator{}
}

func (a *PlaywrightAuthenticator) Authenticate(ctx context.Context, target Target, configDir string, options BrowserAuthOptions) (BrowserSession, error) {
	selection, err := selectBrowserChannel()
	if err != nil {
		return BrowserSession{}, err
	}
	pw, err := playwright.Run()
	if err != nil {
		return BrowserSession{}, playwrightStartError(err)
	}
	defer func() {
		_ = pw.Stop()
	}()

	context, err := launchBrowserContext(pw.Chromium, configDir, selection, options)
	if err != nil {
		return BrowserSession{}, err
	}
	defer func() {
		_ = context.Close()
	}()

	page, err := context.NewPage()
	if err != nil {
		return BrowserSession{}, fmt.Errorf("open page: %w", err)
	}
	if _, err := page.Goto(target.Web, playwright.PageGotoOptions{Timeout: playwright.Float(60_000)}); err != nil {
		readableErr := browserNavigationError(target, err)
		hold := browserErrorHoldDuration()
		showNavigationFailurePage(page, target.Web, readableErr, hold)
		holdBrowserAfterNavigationFailure(ctx, hold)
		return BrowserSession{}, readableErr
	}

	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return BrowserSession{}, ctx.Err()
		default:
		}
		current := page.URL()
		cookies, cookieErr := context.Cookies(current)
		if cookieErr != nil {
			return BrowserSession{}, fmt.Errorf("read browser cookies: %w", cookieErr)
		}
		cookies = filterBrowserCookiesForTarget(cookies, target)
		if isWebAuthenticated(target, current, cookies) {
			converted := convertCookies(cookies)
			expiresAt := deriveCookieExpiry(converted)
			return BrowserSession{Cookies: converted, FinalURL: current, ExpiresAt: expiresAt}, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return BrowserSession{}, fmt.Errorf("timed out waiting for browser authentication")
}

func browserNavigationError(target Target, err error) error {
	host := target.Host
	if parsed, parseErr := url.Parse(target.Web); parseErr == nil && parsed.Hostname() != "" {
		host = parsed.Hostname()
	}
	raw := err.Error()
	switch {
	case strings.Contains(raw, "ERR_NAME_NOT_RESOLVED"):
		return fmt.Errorf("cannot open Splunk URL %s: host %q could not be resolved. Check the URL spelling, DNS/VPN access, or use the correct Splunk Cloud stack URL", target.Web, host)
	case strings.Contains(raw, "ERR_CONNECTION_REFUSED"):
		return fmt.Errorf("cannot open Splunk URL %s: connection refused by %q. Check that Splunk is running and that the URL and port are correct", target.Web, host)
	case strings.Contains(raw, "ERR_CONNECTION_CLOSED"):
		return fmt.Errorf("cannot open Splunk URL %s: the connection to %q was closed before login could start. Check VPN/network/proxy access and verify the Splunk URL opens in a normal browser from this machine", target.Web, host)
	case strings.Contains(raw, "ERR_TIMED_OUT") || strings.Contains(strings.ToLower(raw), "timeout"):
		return fmt.Errorf("cannot open Splunk URL %s: browser navigation timed out. Check network access, VPN, proxy settings, and the Splunk URL", target.Web)
	case strings.Contains(raw, "ERR_CERT_AUTHORITY_INVALID") || strings.Contains(raw, "ERR_CERT_COMMON_NAME_INVALID") || strings.Contains(raw, "ERR_CERT_DATE_INVALID"):
		return fmt.Errorf("cannot open Splunk URL %s: TLS certificate was rejected by the browser. Check the certificate or use a trusted Splunk URL", target.Web)
	default:
		return fmt.Errorf("cannot open Splunk URL %s: browser navigation failed: %s", target.Web, conciseBrowserError(raw))
	}
}

func browserErrorHoldDuration() time.Duration {
	value := strings.TrimSpace(os.Getenv("SPLSEARCH_BROWSER_ERROR_HOLD"))
	if value == "" {
		return defaultBrowserErrorHold
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return defaultBrowserErrorHold
	}
	return duration
}

func showNavigationFailurePage(page playwright.Page, targetURL string, err error, hold time.Duration) {
	message := err.Error()
	holdText := hold.String()
	if hold == 0 {
		holdText = "0s"
	}
	content := fmt.Sprintf(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <title>splsearch browser authentication</title>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 40px; line-height: 1.4; color: #1f2933; }
    h1 { font-size: 24px; margin-bottom: 16px; }
    code { background: #eef2f7; padding: 2px 5px; border-radius: 4px; }
    .message { margin-top: 16px; max-width: 900px; }
  </style>
</head>
<body>
  <h1>splsearch could not open Splunk</h1>
  <p>Target: <code>%s</code></p>
  <p class="message">%s</p>
  <p>This window is kept open for %s so the failure is visible. Set <code>SPLSEARCH_BROWSER_ERROR_HOLD</code> to a duration like <code>2m</code> to keep it open longer.</p>
</body>
</html>`, html.EscapeString(targetURL), html.EscapeString(message), html.EscapeString(holdText))
	_ = page.SetContent(content, playwright.PageSetContentOptions{Timeout: playwright.Float(3000)})
}

func holdBrowserAfterNavigationFailure(ctx context.Context, hold time.Duration) {
	if hold <= 0 {
		return
	}
	timer := time.NewTimer(hold)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func conciseBrowserError(raw string) string {
	summary := strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
	summary = strings.TrimPrefix(summary, "Frame.Goto: ")
	summary = strings.ReplaceAll(summary, "%!w(<nil>)", "")
	summary = strings.Join(strings.Fields(summary), " ")
	if len(summary) > 180 {
		summary = summary[:177] + "..."
	}
	return summary
}

func preferredBrowserChannel() string {
	selection, err := selectBrowserChannel()
	if err != nil {
		return ""
	}
	return selection.Channel
}

func selectBrowserChannel() (browserChannelSelection, error) {
	if channel, ok := os.LookupEnv("SPLSEARCH_BROWSER_CHANNEL"); ok {
		channel = strings.TrimSpace(channel)
		if channel != "" && !supportedBrowserChannels[channel] {
			return browserChannelSelection{}, &browserAuthError{
				code:             "unsupported_browser_channel",
				message:          fmt.Sprintf("unsupported SPLSEARCH_BROWSER_CHANNEL %q; use one of %s, or set it to an empty value to force bundled Chromium", channel, strings.Join(supportedBrowserChannelNames, ", ")),
				requestedChannel: channel,
			}
		}
		return browserChannelSelection{Channel: channel, Explicit: true}, nil
	}
	if browserRuntimeGOOS == "darwin" {
		if exists("/Applications/Google Chrome.app") || exists(filepath.Join(os.Getenv("HOME"), "Applications", "Google Chrome.app")) {
			return browserChannelSelection{Channel: "chrome"}, nil
		}
		if exists("/Applications/Microsoft Edge.app") || exists(filepath.Join(os.Getenv("HOME"), "Applications", "Microsoft Edge.app")) {
			return browserChannelSelection{Channel: "msedge"}, nil
		}
	}
	return browserChannelSelection{}, nil
}

func launchBrowserContext(launcher browserContextLauncher, configDir string, selection browserChannelSelection, options BrowserAuthOptions) (playwright.BrowserContext, error) {
	primary, err := newBrowserLaunchAttempt(configDir, selection.Channel, options)
	if err != nil {
		return nil, err
	}
	context, err := launcher.LaunchPersistentContext(primary.ProfileDir, primary.Options)
	if err == nil {
		return context, nil
	}

	var fallback browserLaunchAttempt
	var fallbackErr error
	if !selection.Explicit && selection.Channel != "" {
		fallback, fallbackPlanErr := newBrowserLaunchAttempt(configDir, "", options)
		if fallbackPlanErr != nil {
			return nil, fallbackPlanErr
		}
		context, fallbackErr = launcher.LaunchPersistentContext(fallback.ProfileDir, fallback.Options)
		if fallbackErr == nil {
			return context, nil
		}
	}

	if shouldTryLaunchServicesFallback(selection, err, fallbackErr) {
		launchServices, launchServicesPlanErr := newBrowserLaunchServicesAttempt(configDir, selection.Channel, options)
		if launchServicesPlanErr != nil {
			return nil, newBrowserLaunchError(configDir, selection, primary, err, fmt.Errorf("prepare LaunchServices fallback: %w", launchServicesPlanErr), true)
		}
		context, launchServicesErr := launchBrowserContextWithLaunchServices(launcher, launchServices)
		if launchServicesErr == nil {
			return context, nil
		}
		if fallbackErr != nil {
			launchServicesErr = errors.Join(fallbackErr, launchServicesErr)
		}
		return nil, newBrowserLaunchError(configDir, selection, launchServices, err, launchServicesErr, true)
	}

	if selection.Explicit || selection.Channel == "" {
		return nil, newBrowserLaunchError(configDir, selection, primary, err, nil, false)
	}

	return nil, newBrowserLaunchError(configDir, selection, fallback, err, fallbackErr, true)
}

func newBrowserLaunchAttempt(configDir, channel string, options BrowserAuthOptions) (browserLaunchAttempt, error) {
	return newBrowserLaunchAttemptWithProfile(configDir, channel, browserProfileDir(configDir, channel), "playwright", options)
}

func newBrowserLaunchServicesAttempt(configDir, channel string, options BrowserAuthOptions) (browserLaunchAttempt, error) {
	return newBrowserLaunchAttemptWithProfile(configDir, channel, browserLaunchServicesProfileDir(configDir, channel), "launchservices", options)
}

func newBrowserLaunchAttemptWithProfile(configDir, channel, profileDir, strategy string, authOptions BrowserAuthOptions) (browserLaunchAttempt, error) {
	crashpadDir := filepath.Join(configDir, "browser-crashpad")
	browserHomeDir := filepath.Join(configDir, "browser-home")
	browserCacheDir := filepath.Join(configDir, "browser-cache")
	browserTempDir := filepath.Join(configDir, "browser-tmp")
	dirs := []string{
		configDir,
		profileDir,
		crashpadDir,
		browserHomeDir,
		browserCacheDir,
		browserTempDir,
		filepath.Join(browserHomeDir, "Library", "Application Support"),
		filepath.Join(browserHomeDir, "Library", "Application Support", "Google", "Chrome", "Crashpad"),
		filepath.Join(browserHomeDir, "Library", "Application Support", "Chromium", "Crashpad"),
		filepath.Join(browserHomeDir, "Library", "Application Support", "Microsoft Edge", "Crashpad"),
		filepath.Join(browserHomeDir, "Library", "Caches"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return browserLaunchAttempt{}, fmt.Errorf("create browser state dir: %w", err)
		}
	}
	options := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless:          playwright.Bool(false),
		Args:              browserLaunchArgs(crashpadDir),
		Env:               browserLaunchEnv(browserHomeDir, browserCacheDir, browserTempDir),
		IgnoreHttpsErrors: playwright.Bool(authOptions.Insecure),
	}
	if channel != "" {
		options.Channel = playwright.String(channel)
	}
	return browserLaunchAttempt{
		Channel:     channel,
		Strategy:    strategy,
		ProfileDir:  profileDir,
		CrashpadDir: crashpadDir,
		Options:     options,
	}, nil
}

func shouldTryLaunchServicesFallback(selection browserChannelSelection, primaryErr, fallbackErr error) bool {
	if browserRuntimeGOOS != "darwin" || selection.Channel == "" {
		return false
	}
	if _, ok := browserChannelAppName(selection.Channel); !ok {
		return false
	}
	return isCrashpadPermissionFailure(strings.ToLower(browserLaunchRawError(primaryErr, fallbackErr)))
}

func launchBrowserContextWithLaunchServices(launcher browserContextLauncher, attempt browserLaunchAttempt) (playwright.BrowserContext, error) {
	appName, ok := browserChannelAppName(attempt.Channel)
	if !ok {
		return nil, fmt.Errorf("LaunchServices fallback is not supported for browser channel %q", attempt.Channel)
	}
	devToolsPath := filepath.Join(attempt.ProfileDir, "DevToolsActivePort")
	_ = os.Remove(devToolsPath)
	if err := openBrowserApp(appName, browserLaunchServicesArgs(attempt), browserLaunchServicesEnv(attempt.Options.Env)); err != nil {
		return nil, fmt.Errorf("launch browser through LaunchServices: %w", err)
	}
	endpoint, err := waitForDevToolsEndpoint(devToolsPath, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("connect to LaunchServices browser: %w", err)
	}
	browser, err := launcher.ConnectOverCDP(endpoint, playwright.BrowserTypeConnectOverCDPOptions{Timeout: playwright.Float(15_000)})
	if err != nil {
		return nil, fmt.Errorf("connect over CDP to LaunchServices browser at %s: %w", endpoint, err)
	}
	contexts := browser.Contexts()
	if len(contexts) == 0 {
		_ = browser.Close()
		return nil, fmt.Errorf("connect over CDP to LaunchServices browser at %s: no browser contexts", endpoint)
	}
	return &cdpBrowserContext{BrowserContext: contexts[0], browser: browser}, nil
}

func browserLaunchServicesArgs(attempt browserLaunchAttempt) []string {
	args := []string{
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
		"--disable-component-update",
		"--password-store=basic",
		"--use-mock-keychain",
	}
	args = append(args, browserLaunchArgs(attempt.CrashpadDir)...)
	args = append(args,
		"--user-data-dir="+attempt.ProfileDir,
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
		"--remote-allow-origins=*",
		"about:blank",
	)
	return args
}

func browserLaunchServicesEnv(env map[string]string) map[string]string {
	selected := make(map[string]string)
	for _, key := range []string{"HOME", "CFFIXED_USER_HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "TMPDIR"} {
		if value := env[key]; value != "" {
			selected[key] = value
		}
	}
	return selected
}

func runOpenBrowserApp(appName string, browserArgs []string, env map[string]string) error {
	args := openBrowserAppNameArgs(appName, browserArgs, env)
	output, err := runOpenCommand(args)
	if err == nil {
		return nil
	}
	primaryErr := openBrowserAppNameError(appName, err, output)
	if !isOpenAppNameUnavailable(err, output) {
		return primaryErr
	}
	var pathErrs []error
	for _, appPath := range browserAppBundleCandidates(appName) {
		if !exists(appPath) {
			continue
		}
		pathArgs := openBrowserAppPathArgs(appPath, browserArgs, env)
		pathOutput, pathErr := runOpenCommand(pathArgs)
		if pathErr == nil {
			return nil
		}
		pathErrs = append(pathErrs, openBrowserAppPathError(appPath, pathErr, pathOutput))
	}
	if len(pathErrs) > 0 {
		return errors.Join(append([]error{primaryErr}, pathErrs...)...)
	}
	return primaryErr
}

func openBrowserAppNameArgs(appName string, browserArgs []string, env map[string]string) []string {
	args := []string{"-n", "-F", "-a", appName}
	args = appendOpenBrowserEnvArgs(args, env)
	args = append(args, "--args")
	args = append(args, browserArgs...)
	return args
}

func openBrowserAppPathArgs(appPath string, browserArgs []string, env map[string]string) []string {
	args := []string{"-n", "-F"}
	args = appendOpenBrowserEnvArgs(args, env)
	args = append(args, appPath, "--args")
	args = append(args, browserArgs...)
	return args
}

func appendOpenBrowserEnvArgs(args []string, env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--env", key+"="+env[key])
	}
	return args
}

func openBrowserAppNameError(appName string, err error, output []byte) error {
	summary := conciseBrowserError(strings.TrimSpace(string(output)))
	if summary != "" {
		return fmt.Errorf("/usr/bin/open -a %q failed: %w: %s", appName, err, summary)
	}
	return fmt.Errorf("/usr/bin/open -a %q failed: %w", appName, err)
}

func openBrowserAppPathError(appPath string, err error, output []byte) error {
	summary := conciseBrowserError(strings.TrimSpace(string(output)))
	if summary != "" {
		return fmt.Errorf("/usr/bin/open %q failed: %w: %s", appPath, err, summary)
	}
	return fmt.Errorf("/usr/bin/open %q failed: %w", appPath, err)
}

func isOpenAppNameUnavailable(err error, output []byte) bool {
	lower := strings.ToLower(err.Error() + "\n" + string(output))
	return strings.Contains(lower, "unable to find application named") ||
		(strings.Contains(lower, "application named") && strings.Contains(lower, "not found"))
}

func defaultBrowserAppBundleCandidates(appName string) []string {
	homeApplications := filepath.Join(os.Getenv("HOME"), "Applications")
	switch appName {
	case "Google Chrome":
		return []string{"/Applications/Google Chrome.app", filepath.Join(homeApplications, "Google Chrome.app")}
	case "Google Chrome Beta":
		return []string{"/Applications/Google Chrome Beta.app", filepath.Join(homeApplications, "Google Chrome Beta.app")}
	case "Google Chrome Dev":
		return []string{"/Applications/Google Chrome Dev.app", filepath.Join(homeApplications, "Google Chrome Dev.app")}
	case "Google Chrome Canary":
		return []string{"/Applications/Google Chrome Canary.app", filepath.Join(homeApplications, "Google Chrome Canary.app")}
	case "Microsoft Edge":
		return []string{"/Applications/Microsoft Edge.app", filepath.Join(homeApplications, "Microsoft Edge.app")}
	case "Microsoft Edge Beta":
		return []string{"/Applications/Microsoft Edge Beta.app", filepath.Join(homeApplications, "Microsoft Edge Beta.app")}
	case "Microsoft Edge Dev":
		return []string{"/Applications/Microsoft Edge Dev.app", filepath.Join(homeApplications, "Microsoft Edge Dev.app")}
	case "Microsoft Edge Canary":
		return []string{"/Applications/Microsoft Edge Canary.app", filepath.Join(homeApplications, "Microsoft Edge Canary.app")}
	default:
		return nil
	}
}

func waitForDevToolsEndpoint(devToolsPath string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		data, err := os.ReadFile(devToolsPath)
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) > 0 {
				port := strings.TrimSpace(lines[0])
				if port != "" && !strings.ContainsAny(port, "/ \t\r") {
					return "http://127.0.0.1:" + port, nil
				}
			}
			lastErr = fmt.Errorf("DevToolsActivePort at %s did not contain a local port", devToolsPath)
		} else if !os.IsNotExist(err) {
			lastErr = err
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return "", fmt.Errorf("browser did not write DevToolsActivePort at %s within %s: %w", devToolsPath, timeout, lastErr)
	}
	return "", fmt.Errorf("browser did not write DevToolsActivePort at %s within %s", devToolsPath, timeout)
}

func browserChannelAppName(channel string) (string, bool) {
	switch channel {
	case "chrome":
		return "Google Chrome", true
	case "chrome-beta":
		return "Google Chrome Beta", true
	case "chrome-dev":
		return "Google Chrome Dev", true
	case "chrome-canary":
		return "Google Chrome Canary", true
	case "msedge":
		return "Microsoft Edge", true
	case "msedge-beta":
		return "Microsoft Edge Beta", true
	case "msedge-dev":
		return "Microsoft Edge Dev", true
	case "msedge-canary":
		return "Microsoft Edge Canary", true
	default:
		return "", false
	}
}

func browserLaunchArgs(crashpadDir string) []string {
	return []string{
		"--disable-breakpad",
		"--disable-crash-reporter",
		"--crash-dumps-dir=" + crashpadDir,
	}
}

func browserLaunchEnv(browserHomeDir, browserCacheDir, browserTempDir string) map[string]string {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	env["HOME"] = browserHomeDir
	env["CFFIXED_USER_HOME"] = browserHomeDir
	env["XDG_CONFIG_HOME"] = browserHomeDir
	env["XDG_CACHE_HOME"] = browserCacheDir
	env["TMPDIR"] = browserTempDir
	return env
}

func playwrightStartError(err error) error {
	raw := conciseBrowserError(err.Error())
	code := "playwright_start_failed"
	message := "start playwright: " + raw
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "please install the driver") || strings.Contains(lower, "executable doesn't exist") || strings.Contains(lower, "playwright install") {
		code = "playwright_install_required"
		message = "start playwright: Playwright driver or browser assets are missing; run `make playwright-install` from the splsearch repo under the current HOME"
	}
	return &browserAuthError{code: code, message: message, underlying: err}
}

func newBrowserLaunchError(configDir string, selection browserChannelSelection, attempt browserLaunchAttempt, primaryErr, fallbackErr error, fallbackUsed bool) error {
	raw := browserLaunchRawError(primaryErr, fallbackErr)
	code := browserLaunchErrorCode(raw)
	launchErrorSummary := browserLaunchErrorSummary(raw)
	requestedChannel := displayBrowserChannel(selection.Channel)
	attemptedChannel := displayBrowserChannel(attempt.Channel)
	diagnosticsPath := writeBrowserLaunchDiagnostics(configDir, selection, attempt, primaryErr, fallbackErr, fallbackUsed)
	message := browserLaunchMessage(code, selection, fallbackUsed)
	retryAfterEnvChange := code == "browser_crashpad_permission" && isMacCrashpadBootstrapFailure(strings.ToLower(raw))
	remediationCode := ""
	if retryAfterEnvChange {
		remediationCode = "retry_from_unsandboxed_environment"
	}
	return &browserAuthError{
		code:                            code,
		message:                         message,
		launchErrorSummary:              launchErrorSummary,
		diagnosticsPath:                 diagnosticsPath,
		requestedChannel:                requestedChannel,
		attemptedChannel:                attemptedChannel,
		fallbackUsed:                    fallbackUsed,
		retryableAfterEnvironmentChange: retryAfterEnvChange,
		remediationCode:                 remediationCode,
		underlying:                      errors.Join(primaryErr, fallbackErr),
	}
}

func browserLaunchRawError(primaryErr, fallbackErr error) string {
	var parts []string
	if primaryErr != nil {
		parts = append(parts, primaryErr.Error())
	}
	if fallbackErr != nil {
		parts = append(parts, fallbackErr.Error())
	}
	return strings.Join(parts, "\n")
}

func browserLaunchErrorCode(raw string) string {
	lower := strings.ToLower(raw)
	switch {
	case isCrashpadPermissionFailure(lower):
		return "browser_crashpad_permission"
	case strings.Contains(lower, "please install the driver") || strings.Contains(lower, "executable doesn't exist") || strings.Contains(lower, "playwright install"):
		return "playwright_install_required"
	case strings.Contains(lower, "channel") && (strings.Contains(lower, "not found") || strings.Contains(lower, "not installed")):
		return "browser_channel_unavailable"
	default:
		return "browser_launch_failed"
	}
}

func isCrashpadPermissionFailure(lower string) bool {
	if !strings.Contains(lower, "crashpad") && !strings.Contains(lower, "chrome_crashpad_handler") {
		return false
	}
	if strings.Contains(lower, "operation not permitted") || strings.Contains(lower, "permission denied") {
		return true
	}
	if strings.Contains(lower, "bootstrap_check_in") && strings.Contains(lower, "1100") {
		return true
	}
	return strings.Contains(lower, "child_port_handshake") && strings.Contains(lower, "server_port.is_valid")
}

func isMacCrashpadBootstrapFailure(lower string) bool {
	return (strings.Contains(lower, "bootstrap_check_in") && strings.Contains(lower, "1100")) ||
		(strings.Contains(lower, "child_port_handshake") && strings.Contains(lower, "server_port.is_valid"))
}

func browserLaunchErrorSummary(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		summary := sanitizeBrowserLaunchSummaryLine(line)
		if summary == "" {
			continue
		}
		if browserLaunchSummaryLineIsActionable(strings.ToLower(summary)) {
			return summary
		}
	}
	for _, line := range strings.Split(raw, "\n") {
		summary := sanitizeBrowserLaunchSummaryLine(line)
		if summary != "" {
			return summary
		}
	}
	return conciseBrowserError(raw)
}

func browserLaunchSummaryLineIsActionable(lower string) bool {
	return strings.Contains(lower, "bootstrap_check_in") ||
		strings.Contains(lower, "child_port_handshake") ||
		strings.Contains(lower, "server_port.is_valid") ||
		(strings.Contains(lower, "crashpad") && (strings.Contains(lower, "permission denied") || strings.Contains(lower, "operation not permitted"))) ||
		strings.Contains(lower, "playwright install") ||
		strings.Contains(lower, "executable doesn't exist")
}

func sanitizeBrowserLaunchSummaryLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "<launching>") {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		line = strings.ReplaceAll(line, home, "~")
	}
	line = macHomePathPattern.ReplaceAllString(line, "~")
	line = strings.Join(strings.Fields(line), " ")
	const maxSummaryLen = 220
	if len(line) > maxSummaryLen {
		line = line[:maxSummaryLen-3] + "..."
	}
	return line
}

func browserLaunchMessage(code string, selection browserChannelSelection, fallbackUsed bool) string {
	if code == "browser_crashpad_permission" {
		return "open browser: browser failed before login because Crashpad bootstrap or state setup was blocked; use launch_error_summary and diagnostics_path for details"
	}
	if selection.Explicit && selection.Channel != "" {
		return fmt.Sprintf("open browser: failed to launch requested browser channel %q; see diagnostics_path", selection.Channel)
	}
	if fallbackUsed {
		return fmt.Sprintf("open browser: failed to launch browser channel %q and bundled Chromium fallback; see diagnostics_path", selection.Channel)
	}
	if selection.Channel == "" {
		return "open browser: failed to launch bundled Chromium; see diagnostics_path"
	}
	return fmt.Sprintf("open browser: failed to launch browser channel %q; see diagnostics_path", selection.Channel)
}

func writeBrowserLaunchDiagnostics(configDir string, selection browserChannelSelection, attempt browserLaunchAttempt, primaryErr, fallbackErr error, fallbackUsed bool) string {
	dir := filepath.Join(configDir, "diagnostics")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	name := fmt.Sprintf("browser-auth-%s-%d.log", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	path := filepath.Join(dir, name)
	var b strings.Builder
	fmt.Fprintf(&b, "splsearch browser authentication launch failure\n")
	fmt.Fprintf(&b, "time: %s\n", time.Now().UTC().Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "requested_channel: %s\n", displayBrowserChannel(selection.Channel))
	fmt.Fprintf(&b, "explicit_channel: %t\n", selection.Explicit)
	fmt.Fprintf(&b, "attempted_channel: %s\n", displayBrowserChannel(attempt.Channel))
	fmt.Fprintf(&b, "launch_strategy: %s\n", attempt.Strategy)
	fmt.Fprintf(&b, "fallback_used: %t\n", fallbackUsed)
	fmt.Fprintf(&b, "profile_dir: %s\n", attempt.ProfileDir)
	fmt.Fprintf(&b, "crashpad_dir: %s\n", attempt.CrashpadDir)
	if primaryErr != nil {
		fmt.Fprintf(&b, "\nprimary_error:\n%s\n", primaryErr.Error())
	}
	if fallbackErr != nil {
		fmt.Fprintf(&b, "\nfallback_error:\n%s\n", fallbackErr.Error())
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return ""
	}
	return path
}

func displayBrowserChannel(channel string) string {
	if channel == "" {
		return "bundled"
	}
	return channel
}

func browserProfileDir(configDir, channel string) string {
	if channel == "" {
		return filepath.Join(configDir, "browser-profile")
	}
	return filepath.Join(configDir, "browser-profile-"+channel)
}

func browserLaunchServicesProfileDir(configDir, channel string) string {
	return browserProfileDir(configDir, channel) + "-launchservices"
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isWebAuthenticated(target Target, current string, cookies []playwright.Cookie) bool {
	parsed, err := url.Parse(current)
	if err != nil {
		return false
	}
	targetURL, err := url.Parse(target.Web)
	if err != nil {
		return false
	}
	if !strings.EqualFold(parsed.Host, targetURL.Host) {
		return false
	}
	path := strings.ToLower(parsed.Path)
	if !strings.Contains(path, "/app/") && !strings.Contains(path, "/en-us/app/") {
		return false
	}
	return len(cookies) > 0
}

func filterBrowserCookiesForTarget(cookies []playwright.Cookie, target Target) []playwright.Cookie {
	targetURL, err := url.Parse(target.Web)
	if err != nil {
		return cookies
	}
	host := strings.ToLower(targetURL.Hostname())
	result := make([]playwright.Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		if cookie.Name == "" || cookie.Value == "" {
			continue
		}
		domain := strings.TrimPrefix(strings.ToLower(cookie.Domain), ".")
		domain = strings.Split(domain, ":")[0]
		if domain == "" || domain == host || strings.HasSuffix(host, "."+domain) {
			result = append(result, cookie)
		}
	}
	return result
}

func convertCookies(cookies []playwright.Cookie) []Cookie {
	result := make([]Cookie, 0, len(cookies))
	for _, cookie := range cookies {
		sameSite := ""
		if cookie.SameSite != nil {
			sameSite = string(*cookie.SameSite)
		}
		result = append(result, Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Domain:   cookie.Domain,
			Path:     cookie.Path,
			Expires:  cookie.Expires,
			HTTPOnly: cookie.HttpOnly,
			Secure:   cookie.Secure,
			SameSite: sameSite,
		})
	}
	return result
}

func deriveCookieExpiry(cookies []Cookie) *int64 {
	var min int64
	for _, cookie := range cookies {
		if cookie.Expires <= 0 {
			continue
		}
		expires := int64(cookie.Expires)
		if min == 0 || expires < min {
			min = expires
		}
	}
	if min == 0 {
		fallback := time.Now().Add(8 * time.Hour).Unix()
		return &fallback
	}
	return &min
}
