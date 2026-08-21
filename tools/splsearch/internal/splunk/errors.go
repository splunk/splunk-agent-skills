package splunk

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/url"
	"strings"
	"syscall"
)

type StructuredErrorDetails struct {
	ErrorCode      string
	Operation      string
	Retryable      bool
	DiagnosticHint string
	SID            string
	LastProgress   *SearchProgressEvent
}

type OperationError struct {
	Operation string
	Err       error
}

func (e *OperationError) Error() string {
	if e.Err == nil {
		return operationLabel(e.Operation)
	}
	return operationLabel(e.Operation) + ": " + e.Err.Error()
}

func (e *OperationError) Unwrap() error {
	return e.Err
}

func operationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &OperationError{Operation: operation, Err: err}
}

type HTTPStatusError struct {
	StatusCode int
	Summary    string
}

func (e *HTTPStatusError) Error() string {
	return e.Summary
}

type SearchTimeoutError struct {
	SID          string
	LastProgress *SearchProgressEvent
	Err          error
}

func (e *SearchTimeoutError) Error() string {
	if e.Err == nil {
		return "search timed out waiting for sid=" + e.SID
	}
	return "search timed out waiting for sid=" + e.SID + ": " + e.Err.Error()
}

func (e *SearchTimeoutError) Unwrap() error {
	return e.Err
}

type searchJobFailedError struct {
	SID string
}

func (e *searchJobFailedError) Error() string {
	return "search job failed"
}

func StructuredError(err error) (StructuredErrorDetails, bool) {
	if err == nil {
		return StructuredErrorDetails{}, false
	}
	details := StructuredErrorDetails{Operation: errorOperation(err)}

	var searchTimeout *SearchTimeoutError
	if errors.As(err, &searchTimeout) {
		details.ErrorCode = "search_timeout"
		details.Operation = "wait_search_job"
		details.Retryable = true
		details.SID = searchTimeout.SID
		details.LastProgress = searchTimeout.LastProgress
		details.DiagnosticHint = "The Splunk search job did not finish before --timeout; inspect last_progress and stderr progress before retrying, and narrow the SPL or time window if progress is stalled or scanning too much data."
		return details, true
	}

	var searchFailed *searchJobFailedError
	if errors.As(err, &searchFailed) {
		details.ErrorCode = "search_failed"
		details.Operation = "wait_search_job"
		details.Retryable = false
		details.SID = searchFailed.SID
		details.DiagnosticHint = "Splunk marked the dispatched search job as failed; check the SPL syntax, app context, permissions, and referenced fields or indexes before retrying."
		return details, true
	}

	var httpStatus *HTTPStatusError
	if errors.As(err, &httpStatus) {
		details.ErrorCode = "splunk_http_error"
		details.Retryable = httpStatus.StatusCode >= 500 && httpStatus.StatusCode < 600
		details.DiagnosticHint = "Splunk returned an HTTP error for the operation; check the target URL, auth state, app context, SPL permissions, and Splunk server health."
		return details, true
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		details.ErrorCode = "dns_lookup_failed"
		details.Retryable = dnsErr.IsTimeout || dnsErr.IsTemporary
		if dnsErr.IsNotFound {
			details.Retryable = false
		}
		details.DiagnosticHint = "DNS lookup failed; check the Splunk URL spelling, DNS, VPN, and network access."
		return details, true
	}

	if errors.Is(err, context.DeadlineExceeded) {
		details.ErrorCode = "network_timeout"
		details.Retryable = true
		details.DiagnosticHint = "The network operation timed out; check VPN/proxy reachability or retry after connectivity recovers."
		return details, true
	}
	if errors.Is(err, context.Canceled) {
		details.ErrorCode = "operation_canceled"
		details.Retryable = false
		details.DiagnosticHint = "The operation was canceled before Splunk returned a response."
		return details, true
	}

	var certUnknown x509.UnknownAuthorityError
	var certHostname x509.HostnameError
	var certInvalid x509.CertificateInvalidError
	if errors.As(err, &certUnknown) || errors.As(err, &certHostname) || errors.As(err, &certInvalid) {
		details.ErrorCode = "tls_certificate_error"
		details.Retryable = false
		details.DiagnosticHint = "TLS certificate validation failed; check the Splunk URL and certificate trust, or use --insecure only for local/dev targets."
		return details, true
	}

	switch {
	case errors.Is(err, syscall.ECONNREFUSED):
		details.ErrorCode = "connection_refused"
		details.Retryable = false
		details.DiagnosticHint = "The host refused the connection; check the Splunk URL, port, and whether the service is running."
		return details, true
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		details.ErrorCode = "network_unreachable"
		details.Retryable = true
		details.DiagnosticHint = "The network path is unreachable; check VPN, proxy, routing, and firewall access to the Splunk target."
		return details, true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		details.ErrorCode = "network_timeout"
		details.Retryable = true
		details.DiagnosticHint = "The network operation timed out; check VPN/proxy reachability or retry after connectivity recovers."
		return details, true
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		details.ErrorCode = "network_error"
		details.Retryable = false
		details.DiagnosticHint = "The Splunk HTTP request failed before a usable response; check URL, DNS, VPN, proxy, and network access."
		return details, true
	}

	if details.Operation != "" {
		details.ErrorCode = "splunk_operation_failed"
		details.Retryable = false
		details.DiagnosticHint = "The Splunk operation failed before a complete response could be processed; check the target URL, auth state, app context, SPL permissions, and retry with a narrower request if needed."
		return details, true
	}

	return details, false
}

func errorOperation(err error) string {
	var operationErr *OperationError
	if errors.As(err, &operationErr) {
		return operationErr.Operation
	}
	return ""
}

func operationLabel(operation string) string {
	switch operation {
	case "create_search_job":
		return "create search job"
	case "read_search_job_status":
		return "read search job status"
	case "read_search_results":
		return "read search results"
	case "wait_search_job":
		return "wait search job"
	case "validate_auth":
		return "validate authentication"
	default:
		return strings.ReplaceAll(operation, "_", " ")
	}
}
