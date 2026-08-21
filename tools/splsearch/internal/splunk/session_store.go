package splunk

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

const (
	SessionKeyFDEnvironment     = "SPLSEARCH_SESSION_KEY_FD"
	SessionTargetURLEnvironment = "SPLSEARCH_SESSION_TARGET_URL"
	maxSessionKeyBytes          = 8192
)

type ephemeralSessionStore struct {
	target Target
	record AuthRecord
}

type SessionTargetMismatchError struct {
	BoundURL     string
	RequestedURL string
}

func (e *SessionTargetMismatchError) Error() string {
	return fmt.Sprintf("ephemeral session authentication is bound to %s and cannot authenticate %s", e.BoundURL, e.RequestedURL)
}

func NewAuthStoreFromEnvironment(configDir string) (Store, error) {
	fdValue, hasFD := os.LookupEnv(SessionKeyFDEnvironment)
	targetValue, hasTarget := os.LookupEnv(SessionTargetURLEnvironment)
	if !hasFD && !hasTarget {
		return NewFileStore(configDir), nil
	}
	if !hasFD {
		return nil, fmt.Errorf("%s and %s must be set together", SessionKeyFDEnvironment, SessionTargetURLEnvironment)
	}
	if !hasTarget {
		if fd, err := parseSessionKeyFD(fdValue); err == nil {
			if file := os.NewFile(uintptr(fd), SessionKeyFDEnvironment); file != nil {
				_ = file.Close()
			}
		}
		return nil, fmt.Errorf("%s and %s must be set together", SessionKeyFDEnvironment, SessionTargetURLEnvironment)
	}

	fd, err := parseSessionKeyFD(fdValue)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), SessionKeyFDEnvironment)
	if file == nil {
		return nil, fmt.Errorf("%s is not a valid inherited file descriptor", SessionKeyFDEnvironment)
	}
	target, err := NormalizeTarget(targetValue)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("invalid %s: %w", SessionTargetURLEnvironment, err)
	}
	if err := validateSessionKeyDescriptor(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxSessionKeyBytes+1))
	defer clear(raw)
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read %s: %w", SessionKeyFDEnvironment, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close %s: %w", SessionKeyFDEnvironment, closeErr)
	}
	if err := validateSessionKey(raw); err != nil {
		return nil, err
	}
	sessionKey := string(raw)

	record := AuthRecord{
		URL:        target.Key,
		Aliases:    append([]string(nil), target.Bases...),
		APIBaseURL: target.API,
		WebBaseURL: target.Web,
		Method:     MethodSessionKey,
		SessionKey: sessionKey,
	}
	return &ephemeralSessionStore{target: target, record: record}, nil
}

func (s *ephemeralSessionStore) Get(target Target) (*AuthRecord, error) {
	if target.Key != s.target.Key && !stringSlicesOverlap(target.Bases, s.record.Aliases) {
		return nil, &SessionTargetMismatchError{BoundURL: s.target.Key, RequestedURL: target.Key}
	}
	record := s.record
	record.Aliases = append([]string(nil), s.record.Aliases...)
	return &record, nil
}

func (s *ephemeralSessionStore) List() ([]AuthRecord, error) {
	record := s.record
	record.Aliases = append([]string(nil), s.record.Aliases...)
	return []AuthRecord{record}, nil
}

func (s *ephemeralSessionStore) Set(Target, AuthRecord) error {
	return fmt.Errorf("ephemeral session authentication is read-only")
}

func (s *ephemeralSessionStore) Delete(Target) (bool, error) {
	return false, fmt.Errorf("ephemeral session authentication is read-only")
}

func hasSessionKey(record AuthRecord) bool {
	if record.Method != MethodSessionKey || len(record.SessionKey) == 0 || len(record.SessionKey) > maxSessionKeyBytes {
		return false
	}
	for index := 0; index < len(record.SessionKey); index++ {
		if record.SessionKey[index] < 0x21 || record.SessionKey[index] > 0x7e {
			return false
		}
	}
	return true
}

func validateSessionKeyDescriptor(file *os.File) error {
	details, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect %s: %w", SessionKeyFDEnvironment, err)
	}
	if details.Mode()&os.ModeNamedPipe == 0 {
		return fmt.Errorf("%s must reference an inherited pipe", SessionKeyFDEnvironment)
	}
	return nil
}

func validateSessionKey(raw []byte) error {
	if len(raw) == 0 {
		return fmt.Errorf("%s contained an empty session key", SessionKeyFDEnvironment)
	}
	if len(raw) > maxSessionKeyBytes {
		return fmt.Errorf("%s exceeded the %d-byte limit", SessionKeyFDEnvironment, maxSessionKeyBytes)
	}
	for _, character := range raw {
		if character < 0x21 || character > 0x7e {
			return fmt.Errorf("%s must contain only printable ASCII bytes", SessionKeyFDEnvironment)
		}
	}
	return nil
}

func parseSessionKeyFD(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("%s must be a numeric inherited file descriptor greater than 2", SessionKeyFDEnvironment)
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("%s must be a numeric inherited file descriptor greater than 2", SessionKeyFDEnvironment)
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil || parsed < 3 {
		return 0, fmt.Errorf("%s must be a numeric inherited file descriptor greater than 2", SessionKeyFDEnvironment)
	}
	return int(parsed), nil
}
