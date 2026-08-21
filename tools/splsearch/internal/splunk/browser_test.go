package splunk

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/playwright-community/playwright-go"
)

type fakeBrowserContextLauncher struct {
	calls        []browserLaunchCall
	errs         []error
	cdpEndpoints []string
	cdpErrs      []error
	cdpBrowsers  []playwright.Browser
}

type browserLaunchCall struct {
	userDataDir string
	options     playwright.BrowserTypeLaunchPersistentContextOptions
}

func (f *fakeBrowserContextLauncher) LaunchPersistentContext(userDataDir string, options ...playwright.BrowserTypeLaunchPersistentContextOptions) (playwright.BrowserContext, error) {
	call := browserLaunchCall{userDataDir: userDataDir}
	if len(options) > 0 {
		call.options = options[0]
	}
	f.calls = append(f.calls, call)
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (f *fakeBrowserContextLauncher) ConnectOverCDP(endpointURL string, options ...playwright.BrowserTypeConnectOverCDPOptions) (playwright.Browser, error) {
	f.cdpEndpoints = append(f.cdpEndpoints, endpointURL)
	if len(f.cdpErrs) > 0 {
		err := f.cdpErrs[0]
		f.cdpErrs = f.cdpErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(f.cdpBrowsers) > 0 {
		browser := f.cdpBrowsers[0]
		f.cdpBrowsers = f.cdpBrowsers[1:]
		return browser, nil
	}
	return &fakePlaywrightBrowser{contexts: []playwright.BrowserContext{&fakePlaywrightBrowserContext{}}}, nil
}

type fakePlaywrightBrowser struct {
	playwright.Browser
	contexts []playwright.BrowserContext
	closed   bool
}

func (f *fakePlaywrightBrowser) Contexts() []playwright.BrowserContext {
	return f.contexts
}

func (f *fakePlaywrightBrowser) Close(options ...playwright.BrowserCloseOptions) error {
	f.closed = true
	return nil
}

type fakePlaywrightBrowserContext struct {
	playwright.BrowserContext
	closed bool
}

func (f *fakePlaywrightBrowserContext) Close(options ...playwright.BrowserContextCloseOptions) error {
	f.closed = true
	return nil
}

func TestPreferredBrowserChannelAllowsEnvOverride(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_CHANNEL", "msedge")
	if got := preferredBrowserChannel(); got != "msedge" {
		t.Fatalf("expected env channel, got %q", got)
	}
}

func TestPreferredBrowserChannelAllowsBundledChromium(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_CHANNEL", "")
	if got := preferredBrowserChannel(); got != "" {
		t.Fatalf("expected bundled Chromium channel override, got %q", got)
	}
}

func TestSelectBrowserChannelRejectsUnsupportedEnvOverride(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_CHANNEL", "safari")
	_, err := selectBrowserChannel()
	if err == nil {
		t.Fatal("expected unsupported channel error")
	}
	details, ok := BrowserErrorDetails(err)
	if !ok {
		t.Fatalf("expected browser error details, got %T", err)
	}
	if details.ErrorCode != "unsupported_browser_channel" || details.RequestedChannel != "safari" {
		t.Fatalf("unexpected details: %+v", details)
	}
}

func TestBrowserProfileDirUsesChannelSpecificProfile(t *testing.T) {
	configDir := t.TempDir()
	if got := browserProfileDir(configDir, "chrome"); got != filepath.Join(configDir, "browser-profile-chrome") {
		t.Fatalf("unexpected chrome profile dir: %s", got)
	}
	if got := browserProfileDir(configDir, ""); got != filepath.Join(configDir, "browser-profile") {
		t.Fatalf("unexpected default profile dir: %s", got)
	}
}

func TestBrowserLaunchServicesProfileDirUsesSeparateProfile(t *testing.T) {
	configDir := t.TempDir()
	if got := browserLaunchServicesProfileDir(configDir, "chrome"); got != filepath.Join(configDir, "browser-profile-chrome-launchservices") {
		t.Fatalf("unexpected LaunchServices profile dir: %s", got)
	}
}

func TestBrowserLaunchAttemptIsolatesBrowserStateAndDisablesCrashReporting(t *testing.T) {
	configDir := t.TempDir()
	attempt, err := newBrowserLaunchAttempt(configDir, "chrome", BrowserAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Options.Channel == nil || *attempt.Options.Channel != "chrome" {
		t.Fatalf("expected chrome channel in launch options: %+v", attempt.Options.Channel)
	}
	if got := attempt.Options.Env["HOME"]; got != filepath.Join(configDir, "browser-home") {
		t.Fatalf("expected isolated HOME, got %q", got)
	}
	if got := attempt.Options.Env["CFFIXED_USER_HOME"]; got != filepath.Join(configDir, "browser-home") {
		t.Fatalf("expected isolated CoreFoundation home, got %q", got)
	}
	if got := attempt.Options.Env["XDG_CACHE_HOME"]; got != filepath.Join(configDir, "browser-cache") {
		t.Fatalf("expected isolated cache, got %q", got)
	}
	for _, wantDir := range []string{
		filepath.Join(configDir, "browser-home", "Library", "Application Support", "Google", "Chrome", "Crashpad"),
		filepath.Join(configDir, "browser-home", "Library", "Application Support", "Chromium", "Crashpad"),
		filepath.Join(configDir, "browser-home", "Library", "Application Support", "Microsoft Edge", "Crashpad"),
	} {
		if info, statErr := os.Stat(wantDir); statErr != nil || !info.IsDir() {
			t.Fatalf("expected browser launch prep to create %s, stat err=%v", wantDir, statErr)
		}
	}
	joinedArgs := strings.Join(attempt.Options.Args, " ")
	for _, want := range []string{
		"--disable-breakpad",
		"--disable-crash-reporter",
		"--crash-dumps-dir=" + filepath.Join(configDir, "browser-crashpad"),
	} {
		if !strings.Contains(joinedArgs, want) {
			t.Fatalf("missing browser arg %q in %q", want, joinedArgs)
		}
	}
	if containsSwitch(attempt.Options.Args, "--disable-crashpad-for-testing") {
		t.Fatalf("testing-only crashpad flag must not be used for browser authentication: %q", joinedArgs)
	}
}

func TestBrowserLaunchAttemptHonorsInsecureTLSOption(t *testing.T) {
	attempt, err := newBrowserLaunchAttempt(t.TempDir(), "", BrowserAuthOptions{Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Options.IgnoreHttpsErrors == nil || !*attempt.Options.IgnoreHttpsErrors {
		t.Fatalf("expected browser context to ignore HTTPS errors for --insecure")
	}
}

func TestFilterBrowserCookiesForTargetDropsUnrelatedSSOCookies(t *testing.T) {
	target := mustTarget(t, "https://customer-stack.splunkcloud.com")
	cookies := []playwright.Cookie{
		{Name: "splunkd_8443", Value: "target", Domain: "customer-stack.splunkcloud.com", Path: "/"},
		{Name: "csrf", Value: "parent", Domain: ".splunkcloud.com", Path: "/"},
		{Name: "idp_session", Value: "sso", Domain: "idp.example.com", Path: "/"},
		{Name: "empty", Domain: "customer-stack.splunkcloud.com", Path: "/"},
	}

	filtered := filterBrowserCookiesForTarget(cookies, target)
	if len(filtered) != 2 {
		t.Fatalf("expected only target-applicable cookies, got %+v", filtered)
	}
	if filtered[0].Name != "splunkd_8443" || filtered[1].Name != "csrf" {
		t.Fatalf("unexpected filtered cookies: %+v", filtered)
	}
}

func TestLaunchBrowserContextHonorsExplicitChannelWithoutFallback(t *testing.T) {
	configDir := t.TempDir()
	launcher := &fakeBrowserContextLauncher{
		errs: []error{errors.New("chrome launch failed")},
	}
	_, err := launchBrowserContext(launcher, configDir, browserChannelSelection{Channel: "chrome", Explicit: true}, BrowserAuthOptions{})
	if err == nil {
		t.Fatal("expected launch error")
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("explicit channel should not fall back, got %d calls", len(launcher.calls))
	}
	if got := launcher.calls[0].userDataDir; got != filepath.Join(configDir, "browser-profile-chrome") {
		t.Fatalf("unexpected profile dir: %s", got)
	}
	details, ok := BrowserErrorDetails(err)
	if !ok {
		t.Fatalf("expected browser error details, got %T", err)
	}
	if details.RequestedChannel != "chrome" || details.AttemptedChannel != "chrome" || details.FallbackUsed {
		t.Fatalf("unexpected browser details: %+v", details)
	}
}

func TestLaunchBrowserContextImplicitChannelCanFallbackToBundled(t *testing.T) {
	configDir := t.TempDir()
	launcher := &fakeBrowserContextLauncher{
		errs: []error{errors.New("chrome launch failed"), nil},
	}
	_, err := launchBrowserContext(launcher, configDir, browserChannelSelection{Channel: "chrome"}, BrowserAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 2 {
		t.Fatalf("expected primary plus fallback launch, got %d calls", len(launcher.calls))
	}
	if got := launcher.calls[1].userDataDir; got != filepath.Join(configDir, "browser-profile") {
		t.Fatalf("unexpected fallback profile dir: %s", got)
	}
	if launcher.calls[1].options.Channel != nil {
		t.Fatalf("bundled fallback should not set channel: %+v", launcher.calls[1].options.Channel)
	}
}

func TestLaunchBrowserContextUsesLaunchServicesFallbackForMacCrashpadPermission(t *testing.T) {
	restoreGOOS := setBrowserRuntimeGOOSForTest(t, "darwin")
	defer restoreGOOS()

	var openCalls []browserOpenCall
	restoreOpen := setOpenBrowserAppForTest(t, func(appName string, args []string, env map[string]string) error {
		openCalls = append(openCalls, browserOpenCall{appName: appName, args: args, env: env})
		profileDir := userDataDirArg(t, args)
		return os.WriteFile(filepath.Join(profileDir, "DevToolsActivePort"), []byte("7777\n/devtools/browser/fake\n"), 0o600)
	})
	defer restoreOpen()

	configDir := t.TempDir()
	launcher := &fakeBrowserContextLauncher{
		errs: []error{
			errors.New("bootstrap_check_in org.chromium.crashpad.child_port_handshake.1: Permission denied (1100)"),
			errors.New("chrome_crashpad_handler: Permission denied (1100)"),
		},
	}
	_, err := launchBrowserContext(launcher, configDir, browserChannelSelection{Channel: "chrome"}, BrowserAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 2 {
		t.Fatalf("expected primary plus bundled persistent launch before LaunchServices fallback, got %d calls", len(launcher.calls))
	}
	if len(openCalls) != 1 {
		t.Fatalf("expected one LaunchServices fallback call, got %d", len(openCalls))
	}
	if openCalls[0].appName != "Google Chrome" {
		t.Fatalf("unexpected app name: %q", openCalls[0].appName)
	}
	if got := userDataDirArg(t, openCalls[0].args); got != filepath.Join(configDir, "browser-profile-chrome-launchservices") {
		t.Fatalf("unexpected LaunchServices profile dir: %s", got)
	}
	joinedArgs := strings.Join(openCalls[0].args, " ")
	for _, want := range []string{
		"--remote-debugging-port=0",
		"--remote-debugging-address=127.0.0.1",
	} {
		if !strings.Contains(joinedArgs, want) {
			t.Fatalf("missing LaunchServices arg %q in %q", want, joinedArgs)
		}
	}
	if containsSwitch(openCalls[0].args, "--disable-crashpad-for-testing") {
		t.Fatalf("testing-only crashpad flag must not be used for LaunchServices authentication: %q", joinedArgs)
	}
	if got := launcher.cdpEndpoints; len(got) != 1 || got[0] != "http://127.0.0.1:7777" {
		t.Fatalf("unexpected CDP endpoints: %+v", got)
	}
	if _, ok := openCalls[0].env["HOME"]; !ok {
		t.Fatalf("expected isolated HOME in LaunchServices env: %+v", openCalls[0].env)
	}
}

func TestLaunchBrowserContextExplicitChannelUsesLaunchServicesWithoutBundledFallback(t *testing.T) {
	restoreGOOS := setBrowserRuntimeGOOSForTest(t, "darwin")
	defer restoreGOOS()

	var openCalls []browserOpenCall
	restoreOpen := setOpenBrowserAppForTest(t, func(appName string, args []string, env map[string]string) error {
		openCalls = append(openCalls, browserOpenCall{appName: appName, args: args, env: env})
		profileDir := userDataDirArg(t, args)
		return os.WriteFile(filepath.Join(profileDir, "DevToolsActivePort"), []byte("9222\n/devtools/browser/fake\n"), 0o600)
	})
	defer restoreOpen()

	configDir := t.TempDir()
	launcher := &fakeBrowserContextLauncher{
		errs: []error{errors.New("bootstrap_check_in org.chromium.crashpad.child_port_handshake.1: Permission denied (1100)")},
	}
	_, err := launchBrowserContext(launcher, configDir, browserChannelSelection{Channel: "chrome", Explicit: true}, BrowserAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(launcher.calls) != 1 {
		t.Fatalf("explicit channel should not try bundled fallback, got %d persistent launches", len(launcher.calls))
	}
	if len(openCalls) != 1 {
		t.Fatalf("expected one LaunchServices fallback call, got %d", len(openCalls))
	}
	if got := launcher.cdpEndpoints; len(got) != 1 || got[0] != "http://127.0.0.1:9222" {
		t.Fatalf("unexpected CDP endpoints: %+v", got)
	}
}

func TestLaunchBrowserContextSkipsLaunchServicesFallbackForNonCrashpadLaunchError(t *testing.T) {
	restoreGOOS := setBrowserRuntimeGOOSForTest(t, "darwin")
	defer restoreGOOS()

	var openCalls []browserOpenCall
	restoreOpen := setOpenBrowserAppForTest(t, func(appName string, args []string, env map[string]string) error {
		openCalls = append(openCalls, browserOpenCall{appName: appName, args: args, env: env})
		return nil
	})
	defer restoreOpen()

	configDir := t.TempDir()
	launcher := &fakeBrowserContextLauncher{
		errs: []error{errors.New("chrome launch failed"), errors.New("bundled launch failed")},
	}
	_, err := launchBrowserContext(launcher, configDir, browserChannelSelection{Channel: "chrome"}, BrowserAuthOptions{})
	if err == nil {
		t.Fatal("expected launch error")
	}
	if len(openCalls) != 0 {
		t.Fatalf("LaunchServices fallback should be scoped to Crashpad permission failures, got %d calls", len(openCalls))
	}
}

func TestRunOpenBrowserAppRetriesKnownBundlePathWhenNameLookupFails(t *testing.T) {
	appPath := filepath.Join(t.TempDir(), "Google Chrome.app")
	if err := os.MkdirAll(appPath, 0o700); err != nil {
		t.Fatal(err)
	}
	restoreCandidates := setBrowserAppBundleCandidatesForTest(t, func(appName string) []string {
		if appName != "Google Chrome" {
			t.Fatalf("unexpected app name: %q", appName)
		}
		return []string{appPath}
	})
	defer restoreCandidates()

	var calls [][]string
	restoreRun := setRunOpenCommandForTest(t, func(args []string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if containsArg(args, "-a") {
			return []byte("Unable to find application named 'Google Chrome'"), errors.New("exit status 1")
		}
		if containsArg(args, appPath) {
			return nil, nil
		}
		return nil, errors.New("unexpected open args")
	})
	defer restoreRun()

	err := runOpenBrowserApp("Google Chrome", []string{"--remote-debugging-port=0"}, map[string]string{"HOME": "/tmp/splsearch-home"})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected name lookup plus bundle-path retry, got %d calls: %+v", len(calls), calls)
	}
	if !containsArg(calls[0], "-a") || !containsArg(calls[0], "Google Chrome") {
		t.Fatalf("first call should use LaunchServices app name lookup: %+v", calls[0])
	}
	if containsArg(calls[1], "-a") || !containsArg(calls[1], appPath) {
		t.Fatalf("second call should open the app bundle path directly: %+v", calls[1])
	}
	if !containsArg(calls[1], "--env") || !containsArg(calls[1], "HOME=/tmp/splsearch-home") {
		t.Fatalf("bundle-path retry should preserve isolated env args: %+v", calls[1])
	}
}

func TestBrowserLaunchErrorIsCompactAndWritesDiagnostics(t *testing.T) {
	configDir := t.TempDir()
	attempt, err := newBrowserLaunchAttempt(configDir, "", BrowserAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rawErr := errors.New("Target page, context or browser has been closed\nBrowser logs:\n<launching> /Users/example/Library/Caches/ms-playwright/chromium-1134/chrome-mac/Chromium.app --many --flags\nopen /Users/example/Library/Application Support/Chromium/Crashpad/settings.dat: Operation not permitted\nstack frame one\nstack frame two")
	launchErr := newBrowserLaunchError(configDir, browserChannelSelection{}, attempt, rawErr, nil, false)
	message := launchErr.Error()
	if len(message) > 220 {
		t.Fatalf("expected compact message, got %d bytes: %q", len(message), message)
	}
	for _, noisy := range []string{"<launching>", "stack frame", "ms-playwright"} {
		if strings.Contains(message, noisy) {
			t.Fatalf("compact message leaked %q: %q", noisy, message)
		}
	}
	details, ok := BrowserErrorDetails(launchErr)
	if !ok {
		t.Fatalf("expected browser details, got %T", launchErr)
	}
	if details.ErrorCode != "browser_crashpad_permission" {
		t.Fatalf("unexpected error code: %+v", details)
	}
	if details.RetryableAfterEnvironmentChange {
		t.Fatalf("filesystem crashpad errors should not be marked retryable after environment change: %+v", details)
	}
	if details.RemediationCode != "" {
		t.Fatalf("filesystem crashpad errors should not get an environment remediation code: %+v", details)
	}
	if !strings.Contains(details.LaunchErrorSummary, "Crashpad/settings.dat") || !strings.Contains(details.LaunchErrorSummary, "Operation not permitted") {
		t.Fatalf("expected compact crashpad summary, got %+v", details)
	}
	if details.DiagnosticsPath == "" {
		t.Fatal("expected diagnostics path")
	}
	data, readErr := os.ReadFile(details.DiagnosticsPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(data), "<launching>") || !strings.Contains(string(data), "Crashpad/settings.dat") {
		t.Fatalf("diagnostics file should contain raw launch details, got:\n%s", string(data))
	}
}

func TestBrowserLaunchErrorDetectsMacCrashpadBootstrapPermission(t *testing.T) {
	configDir := t.TempDir()
	attempt, err := newBrowserLaunchAttempt(configDir, "", BrowserAuthOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rawErr := errors.New("Target page, context or browser has been closed\nBrowser logs:\n<launching> ~/Chromium.app/Contents/MacOS/Chromium --many --flags\n[ERROR:mach_port_rendezvous.cc(384)] bootstrap_check_in org.chromium.Chromium.crashpad: Permission denied (1100)\n[FATAL:child_port_handshake.cc(118)] Check failed: server_port.is_valid().")
	launchErr := newBrowserLaunchError(configDir, browserChannelSelection{}, attempt, rawErr, nil, false)
	details, ok := BrowserErrorDetails(launchErr)
	if !ok {
		t.Fatalf("expected browser details, got %T", launchErr)
	}
	if details.ErrorCode != "browser_crashpad_permission" {
		t.Fatalf("unexpected error code: %+v", details)
	}
	if !details.RetryableAfterEnvironmentChange {
		t.Fatalf("expected bootstrap crashpad error to be retryable after environment change: %+v", details)
	}
	if details.RemediationCode != "retry_from_unsandboxed_environment" {
		t.Fatalf("unexpected remediation code: %+v", details)
	}
	if !strings.Contains(details.LaunchErrorSummary, "bootstrap_check_in") || !strings.Contains(details.LaunchErrorSummary, "Permission denied") {
		t.Fatalf("expected actionable launch summary, got %+v", details)
	}
	for _, noisy := range []string{"<launching>", "--many --flags"} {
		if strings.Contains(details.LaunchErrorSummary, noisy) {
			t.Fatalf("launch summary leaked noisy browser detail %q: %q", noisy, details.LaunchErrorSummary)
		}
	}
}

func TestBrowserErrorHoldDurationAllowsEnvOverride(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_ERROR_HOLD", "2m")
	if got := browserErrorHoldDuration(); got != 2*time.Minute {
		t.Fatalf("expected 2m hold, got %s", got)
	}
}

func TestBrowserErrorHoldDurationFallsBackForInvalidEnv(t *testing.T) {
	t.Setenv("SPLSEARCH_BROWSER_ERROR_HOLD", "not-a-duration")
	if got := browserErrorHoldDuration(); got != defaultBrowserErrorHold {
		t.Fatalf("expected default hold, got %s", got)
	}
}

func TestBrowserNavigationErrorExplainsDNSFailure(t *testing.T) {
	target := mustTarget(t, "customer-stack.splunkcloud.com")
	err := browserNavigationError(target, errors.New("Frame.Goto: playwright: net::ERR_NAME_NOT_RESOLVED at https://customer-stack.splunkcloud.com/"))
	message := err.Error()
	for _, want := range []string{
		"cannot open Splunk URL https://customer-stack.splunkcloud.com",
		`host "customer-stack.splunkcloud.com" could not be resolved`,
		"Check the URL spelling",
		"DNS/VPN access",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in %q", want, message)
		}
	}
	for _, noisy := range []string{"Frame.Goto", "playwright"} {
		if strings.Contains(message, noisy) {
			t.Fatalf("message should hide low-level browser detail %q: %q", noisy, message)
		}
	}
}

func TestBrowserNavigationErrorExplainsConnectionRefused(t *testing.T) {
	target := mustTarget(t, "https://localhost:8000")
	err := browserNavigationError(target, errors.New("Frame.Goto: playwright: net::ERR_CONNECTION_REFUSED at https://localhost:8000/"))
	message := err.Error()
	if !strings.Contains(message, "connection refused") || !strings.Contains(message, "URL and port are correct") {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestBrowserNavigationErrorExplainsConnectionClosed(t *testing.T) {
	target := mustTarget(t, "https://customer-stack.splunkcloud.com")
	err := browserNavigationError(target, errors.New("Frame.Goto: playwright: net::ERR_CONNECTION_CLOSED at https://customer-stack.splunkcloud.com/"))
	message := err.Error()
	for _, want := range []string{
		"connection to \"customer-stack.splunkcloud.com\" was closed",
		"Check VPN/network/proxy access",
		"normal browser",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected %q in %q", want, message)
		}
	}
	for _, noisy := range []string{"Frame.Goto", "playwright"} {
		if strings.Contains(message, noisy) {
			t.Fatalf("message should hide low-level browser detail %q: %q", noisy, message)
		}
	}
}

type browserOpenCall struct {
	appName string
	args    []string
	env     map[string]string
}

func setBrowserRuntimeGOOSForTest(t *testing.T, goos string) func() {
	t.Helper()
	previous := browserRuntimeGOOS
	browserRuntimeGOOS = goos
	return func() {
		browserRuntimeGOOS = previous
	}
}

func setOpenBrowserAppForTest(t *testing.T, fn func(appName string, args []string, env map[string]string) error) func() {
	t.Helper()
	previous := openBrowserApp
	openBrowserApp = fn
	return func() {
		openBrowserApp = previous
	}
}

func setRunOpenCommandForTest(t *testing.T, fn func(args []string) ([]byte, error)) func() {
	t.Helper()
	previous := runOpenCommand
	runOpenCommand = fn
	return func() {
		runOpenCommand = previous
	}
}

func setBrowserAppBundleCandidatesForTest(t *testing.T, fn func(appName string) []string) func() {
	t.Helper()
	previous := browserAppBundleCandidates
	browserAppBundleCandidates = fn
	return func() {
		browserAppBundleCandidates = previous
	}
}

func userDataDirArg(t *testing.T, args []string) string {
	t.Helper()
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--user-data-dir="); ok {
			return value
		}
	}
	t.Fatalf("missing --user-data-dir in args: %+v", args)
	return ""
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func TestContainsSwitchRecognizesBareAndAssignedForms(t *testing.T) {
	name := "--disable-crashpad-for-testing"
	for _, arg := range []string{name, name + "=true"} {
		if !containsSwitch([]string{arg}, name) {
			t.Fatalf("expected switch %q to match %q", name, arg)
		}
	}
	if containsSwitch([]string{name + "-extra"}, name) {
		t.Fatalf("switch %q must not match a longer switch name", name)
	}
}

func containsSwitch(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}
