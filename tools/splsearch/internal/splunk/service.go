package splunk

import (
	"context"
	"fmt"
	"time"
)

const (
	authStatusOperation          = "auth_status"
	credentialsExpiredErrorCode  = "credentials_expired"
	credentialsExpiredStatusText = "stored credentials are expired"
)

type AuthServiceOptions struct {
	Store     Store
	Client    *Client
	Browser   BrowserAuthenticator
	ConfigDir string
}

type AuthService struct {
	store     Store
	client    *Client
	browser   BrowserAuthenticator
	configDir string
}

func NewAuthService(options AuthServiceOptions) *AuthService {
	return &AuthService{
		store:   options.Store,
		client:  options.Client,
		browser: options.Browser,
		configDir: func() string {
			if options.ConfigDir != "" {
				return options.ConfigDir
			}
			return DefaultConfigDir()
		}(),
	}
}

func (s *AuthService) Login(ctx context.Context, request LoginRequest) (LoginResult, error) {
	target, err := NormalizeTarget(request.URL)
	if err != nil {
		return LoginResult{}, err
	}
	existingRecord, err := s.store.Get(target)
	if err != nil {
		return LoginResult{}, err
	}
	if existingRecord != nil && existingRecord.Method == MethodSessionKey {
		if err := ensureUsableSearchAuth(target.Key, existingRecord); err != nil {
			return LoginResult{}, err
		}
		remoteValid, validateErr := s.client.Validate(ctx, target, *existingRecord)
		if validateErr != nil {
			return LoginResult{}, validateErr
		}
		if !remoteValid {
			return LoginResult{}, authRejectedError(target.Key, existingRecord.Method)
		}
		return LoginResult{URL: target.Key, Validated: true, Message: "already authenticated", ExpiresAt: existingRecord.ExpiresAt}, nil
	}
	if existingRecord != nil && ensureUsableSearchAuth(target.Key, existingRecord) == nil {
		remoteValid, _ := s.client.Validate(ctx, target, *existingRecord)
		if remoteValid {
			return LoginResult{URL: target.Key, Validated: true, Message: "already authenticated", ExpiresAt: existingRecord.ExpiresAt}, nil
		}
	}

	now := time.Now().Unix()
	if s.browser == nil {
		return LoginResult{}, fmt.Errorf("web authentication is not available")
	}
	session, err := s.browser.Authenticate(ctx, target, s.configDir, BrowserAuthOptions{Insecure: request.Insecure})
	if err != nil {
		return LoginResult{}, err
	}
	record := AuthRecord{
		APIBaseURL:            target.API,
		WebBaseURL:            target.Web,
		Method:                MethodWeb,
		Cookies:               session.Cookies,
		TLSInsecureSkipVerify: request.Insecure,
		CreatedAt:             now,
		UpdatedAt:             now,
		ExpiresAt:             session.ExpiresAt,
	}
	if err := s.store.Set(target, record); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{URL: target.Key, Validated: true, Message: "authenticated", ExpiresAt: session.ExpiresAt}, nil
}

func (s *AuthService) Status(ctx context.Context, rawURL string) (StatusResult, error) {
	target, err := NormalizeTarget(rawURL)
	if err != nil {
		return StatusResult{}, err
	}
	record, err := s.store.Get(target)
	if err != nil {
		return StatusResult{}, err
	}
	if record == nil {
		return StatusResult{URL: target.Key, Message: "not authenticated"}, fmt.Errorf("not authenticated")
	}
	if record.Method != MethodWeb && !hasSessionKey(*record) {
		return StatusResult{URL: target.Key, Message: "stored credentials use an unsupported authentication method"}, nil
	}
	if record.ExpiresAt != nil && time.Now().Unix() >= *record.ExpiresAt {
		return expiredCredentialsStatus(target.Key, record.ExpiresAt), nil
	}
	remoteValid, validateErr := s.client.Validate(ctx, target, *record)
	if !remoteValid {
		message := "stored credentials were rejected by Splunk"
		var structured StructuredErrorDetails
		var hasStructured bool
		if validateErr != nil {
			message = validateErr.Error()
			structured, hasStructured = StructuredError(validateErr)
		}
		result := StatusResult{URL: target.Key, LocalValid: true, RemoteCheck: true, RemoteValid: false, Message: message, ExpiresAt: record.ExpiresAt}
		if hasStructured {
			result.ErrorCode = structured.ErrorCode
			result.Operation = structured.Operation
			result.Retryable = &structured.Retryable
			result.DiagnosticHint = structured.DiagnosticHint
		}
		return result, nil
	}
	return StatusResult{URL: target.Key, LocalValid: true, RemoteCheck: true, RemoteValid: true, Message: "authenticated", ExpiresAt: record.ExpiresAt}, nil
}

func (s *AuthService) StatusAll() ([]StatusResult, error) {
	records, err := s.store.List()
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	statuses := make([]StatusResult, 0, len(records))
	for _, record := range records {
		if record.Method != MethodWeb && !hasSessionKey(record) {
			continue
		}
		if record.ExpiresAt != nil && now >= *record.ExpiresAt {
			statuses = append(statuses, expiredCredentialsStatus(record.URL, record.ExpiresAt))
			continue
		}
		message := "authenticated"
		statuses = append(statuses, StatusResult{
			URL:        record.URL,
			LocalValid: true,
			Message:    message,
			ExpiresAt:  record.ExpiresAt,
		})
	}
	return statuses, nil
}

func expiredCredentialsStatus(url string, expiresAt *int64) StatusResult {
	retryable := false
	return StatusResult{
		URL:            url,
		LocalValid:     false,
		ErrorCode:      credentialsExpiredErrorCode,
		Operation:      authStatusOperation,
		Retryable:      &retryable,
		DiagnosticHint: fmt.Sprintf("Stored credentials are expired; run splsearch auth login --url=%s --output=json to refresh the browser session.", url),
		Message:        credentialsExpiredStatusText,
		ExpiresAt:      expiresAt,
	}
}

func (s *AuthService) Logout(rawURL string) (LogoutResult, error) {
	target, err := NormalizeTarget(rawURL)
	if err != nil {
		return LogoutResult{}, err
	}
	removed, err := s.store.Delete(target)
	if err != nil {
		return LogoutResult{}, err
	}
	if !removed {
		return LogoutResult{URL: target.Key, Removed: false, Message: "no stored credentials found"}, nil
	}
	return LogoutResult{URL: target.Key, Removed: true, Message: "logged out"}, nil
}
