package splunk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
)

func TestEnvironmentSessionAppearsInAuthInventoryWithoutPersistence(t *testing.T) {
	const sessionKey = "ephemeral-session-key"
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(sessionKey); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	configDir := t.TempDir()
	t.Setenv("SPLSEARCH_SESSION_KEY_FD", strconv.Itoa(int(reader.Fd())))
	t.Setenv("SPLSEARCH_SESSION_TARGET_URL", "https://splunk.example.com:8089/services/server/info")
	store, err := NewAuthStoreFromEnvironment(configDir)
	if err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{Store: store, Client: NewClient(nil, false)})
	statuses, err := service.StatusAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || !statuses[0].LocalValid || statuses[0].URL != "https://splunk.example.com:8089" {
		t.Fatalf("unexpected ephemeral auth inventory: %+v", statuses)
	}

	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Method != MethodSessionKey || records[0].SessionKey != sessionKey {
		t.Fatal("ephemeral session record was not loaded from the inherited descriptor")
	}
	raw, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), sessionKey) || strings.Contains(string(raw), `"SessionKey"`) {
		t.Fatal("ephemeral session secret was exposed by JSON serialization")
	}
	if _, err := os.Stat(filepath.Join(configDir, "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ephemeral authentication must not create auth.json: %v", err)
	}
}

func TestFileStoreRejectsSessionAuthentication(t *testing.T) {
	configDir := t.TempDir()
	store := NewFileStore(configDir)
	target := mustTarget(t, "https://splunk.example.com:8089")
	err := store.Set(target, AuthRecord{Method: MethodSessionKey, SessionKey: "must-not-persist"})
	if err == nil {
		t.Fatal("expected file store to reject ephemeral session authentication")
	}
	if _, statErr := os.Stat(filepath.Join(configDir, "auth.json")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("session authentication created auth.json: %v", statErr)
	}
}

func TestEnvironmentSessionRejectsNonPipeDescriptorAndClosesIt(t *testing.T) {
	keyFile, err := os.CreateTemp(t.TempDir(), "session-key-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = keyFile.Close() })
	if _, err := keyFile.WriteString("must-not-read-from-disk"); err != nil {
		t.Fatal(err)
	}
	if _, err := keyFile.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	t.Setenv(SessionKeyFDEnvironment, strconv.Itoa(int(keyFile.Fd())))
	t.Setenv(SessionTargetURLEnvironment, "https://splunk.example.com:8089")
	if _, err := NewAuthStoreFromEnvironment(t.TempDir()); err == nil || !strings.Contains(err.Error(), "inherited pipe") {
		t.Fatalf("expected regular descriptor rejection, got %v", err)
	}
	if _, err := keyFile.Stat(); err == nil {
		t.Fatal("rejected session descriptor remained open")
	}
}

func TestEnvironmentSessionStoreReturnsIndependentAliasCopies(t *testing.T) {
	setEnvironmentSession(t, "https://bound.example.com:8089", "target-bound-copy-key")
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bound := mustTarget(t, "https://bound.example.com:8089")
	other := mustTarget(t, "https://other.example.com:8089")

	record, err := store.Get(bound)
	if err != nil {
		t.Fatal(err)
	}
	for index := range record.Aliases {
		record.Aliases[index] = other.Key
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	for index := range records[0].Aliases {
		records[0].Aliases[index] = other.Key
	}

	if _, err := store.Get(other); err == nil {
		t.Fatal("caller mutation broadened the environment session target binding")
	} else {
		var mismatch *SessionTargetMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("expected target mismatch after caller mutation, got %v", err)
		}
	}
}

func TestEnvironmentSessionDescriptorClosesWhenTargetIsInvalid(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("close-on-invalid-target"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(SessionKeyFDEnvironment, strconv.Itoa(int(reader.Fd())))
	t.Setenv(SessionTargetURLEnvironment, "://invalid")
	if _, err := NewAuthStoreFromEnvironment(t.TempDir()); err == nil {
		t.Fatal("expected invalid target URL")
	}
	buffer := make([]byte, 1)
	if _, err := reader.Read(buffer); err == nil {
		_ = reader.Close()
		t.Fatal("session descriptor remained open after target validation failure")
	}
	_ = reader.Close()
}

func TestEnvironmentSessionDescriptorClosesWhenTargetIsMissing(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("close-on-missing-target"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(SessionKeyFDEnvironment, strconv.Itoa(int(reader.Fd())))
	unsetEnvironment(t, SessionTargetURLEnvironment)
	if _, err := NewAuthStoreFromEnvironment(t.TempDir()); err == nil {
		t.Fatal("expected paired environment variable error")
	}
	buffer := make([]byte, 1)
	if _, err := reader.Read(buffer); err == nil {
		_ = reader.Close()
		t.Fatal("session descriptor remained open after missing target failure")
	}
	_ = reader.Close()
}

func TestEnvironmentSessionRemotelyValidatesWithAuthorizationHeader(t *testing.T) {
	const sessionKey = "remote-validation-session-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/server/info" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Splunk "+sessionKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"entry":[]}`))
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	status, err := service.Status(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !status.LocalValid || !status.RemoteCheck || !status.RemoteValid {
		t.Fatalf("expected remotely valid ephemeral session status, got %+v", status)
	}
}

func TestEnvironmentSessionValidationUsesOnlyBoundManagementEndpoint(t *testing.T) {
	const sessionKey = "single-validation-request-key"
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/services/server/info" {
			http.Error(w, "unexpected path", http.StatusBadRequest)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	setEnvironmentSession(t, server.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{Store: store, Client: NewClient(server.Client(), false)})
	status, err := service.Status(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status.RemoteValid {
		t.Fatal("expected rejected session status")
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one direct management validation request, got %d", requests.Load())
	}
}

func TestEnvironmentSessionDoesNotFollowRedirects(t *testing.T) {
	const sessionKey = "redirect-bound-session-key"
	var redirectedRequests atomic.Int32
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectedRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()
	boundTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL+"/capture", http.StatusFound)
	}))
	defer boundTarget.Close()

	setEnvironmentSession(t, boundTarget.URL, sessionKey)
	store, err := NewAuthStoreFromEnvironment(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewAuthService(AuthServiceOptions{Store: store, Client: NewClient(boundTarget.Client(), false)})
	status, err := service.Status(context.Background(), boundTarget.URL)
	if err != nil {
		t.Fatal(err)
	}
	if status.RemoteValid {
		t.Fatal("redirect response must not validate an environment session")
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("environment session validation followed %d redirect requests", redirectedRequests.Load())
	}
}

func TestValidateSessionKeyRejectsEmptyOversizedAndControlCharacters(t *testing.T) {
	tests := map[string][]byte{
		"empty":        nil,
		"oversized":    []byte(strings.Repeat("a", maxSessionKeyBytes+1)),
		"newline":      []byte("key\n"),
		"null":         []byte{'k', 'e', 'y', 0},
		"unicode_ctrl": []byte("key\u0085"),
		"non_ascii":    []byte("k\u00e9y"),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if err := validateSessionKey(raw); err == nil {
				t.Fatal("expected invalid session key to be rejected")
			}
		})
	}
	if err := validateSessionKey([]byte("valid+/=_-.session:key")); err != nil {
		t.Fatalf("expected visible session key characters to be accepted: %v", err)
	}
}

func TestParseSessionKeyFDRejectsNonCanonicalAndStandardDescriptors(t *testing.T) {
	for _, value := range []string{"", "+3", " 3", "3 ", "-1", "0", "1", "2", "2147483648"} {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			if _, err := parseSessionKeyFD(value); err == nil {
				t.Fatalf("expected descriptor value %q to be rejected", value)
			}
		})
	}
	fd, err := parseSessionKeyFD("3")
	if err != nil || fd != 3 {
		t.Fatalf("expected canonical inherited descriptor, fd=%d err=%v", fd, err)
	}
}

func setEnvironmentSession(t *testing.T, targetURL, sessionKey string) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(sessionKey); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Dup(int(reader.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		_ = syscall.Close(fd)
		t.Fatal(err)
	}
	t.Setenv(SessionKeyFDEnvironment, strconv.Itoa(fd))
	t.Setenv(SessionTargetURLEnvironment, targetURL)
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()
	value, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
