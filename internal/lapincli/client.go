package lapincli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const (
	defaultBaseURL  = "http://127.0.0.1:8080"
	maxResponseBody = 2 << 20
)

type importResult struct {
	SubjectID    string `json:"subject_id"`
	ExternalID   string `json:"external_id"`
	Title        string `json:"title"`
	ChapterCount int    `json:"chapter_count"`
}

type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func importCourse(ctx context.Context, baseURL, token string, body []byte, dependencyClient *http.Client) (importResult, error) {
	endpoint, err := importEndpoint(baseURL)
	if err != nil {
		return importResult{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return importResult{}, fmt.Errorf("create import request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 30 * time.Second}
	if dependencyClient != nil {
		client = *dependencyClient
		if client.Timeout == 0 {
			client.Timeout = 30 * time.Second
		}
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	response, err := client.Do(request)
	if err != nil {
		return importResult{}, fmt.Errorf("send import request: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return importResult{}, fmt.Errorf("read import response: %w", err)
	}
	if len(responseBody) > maxResponseBody {
		return importResult{}, fmt.Errorf("import response exceeds %d bytes", maxResponseBody)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return importResult{}, fmt.Errorf("decode import response (HTTP %d): %w", response.StatusCode, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Error != nil {
		code := "request_failed"
		message := http.StatusText(response.StatusCode)
		if envelope.Error != nil {
			if envelope.Error.Code != "" {
				code = normalizeAPIErrorCode(sanitizeDiagnostic(envelope.Error.Code, token))
			}
			if envelope.Error.Message != "" {
				message = sanitizeDiagnostic(envelope.Error.Message, token)
			}
		}
		if retryAfter, ok := normalizeRetryAfter(response.Header.Get("Retry-After")); ok {
			return importResult{}, fmt.Errorf("API error %s (HTTP %d, retry after %s): %s", code, response.StatusCode, retryAfter, message)
		}
		return importResult{}, fmt.Errorf("API error %s (HTTP %d): %s", code, response.StatusCode, message)
	}
	var subject struct {
		ID string `json:"id"`
	}
	if len(envelope.Data) == 0 || json.Unmarshal(envelope.Data, &subject) != nil || !validSubjectID(subject.ID) {
		return importResult{}, fmt.Errorf("import response does not contain a valid subject")
	}
	return importResult{SubjectID: subject.ID}, nil
}

func redactToken(value, token string) string {
	if token == "" {
		return value
	}
	return strings.ReplaceAll(value, token, "[REDACTED]")
}

func sanitizeDiagnostic(value, token string) string {
	const maxRunes = 512
	value = redactToken(value, token)
	var builder strings.Builder
	count := 0
	for _, character := range value {
		if count == maxRunes {
			builder.WriteString("...")
			break
		}
		if unicode.IsControl(character) || unicode.In(character, unicode.Cf) {
			builder.WriteByte(' ')
		} else {
			builder.WriteRune(character)
		}
		count++
	}
	return strings.TrimSpace(builder.String())
}

func normalizeAPIErrorCode(value string) string {
	if value == "" || len(value) > 64 {
		return "request_failed"
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return "request_failed"
		}
	}
	return value
}

func normalizeRetryAfter(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		return strconv.FormatUint(seconds, 10), true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return "", false
	}
	return when.UTC().Format(http.TimeFormat), true
}

func validSubjectID(value string) bool {
	if len(value) < 10 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func importEndpoint(rawBaseURL string) (string, error) {
	return apiEndpoint(rawBaseURL, "/openapi/v1/subjects/import")
}

func apiEndpoint(rawBaseURL, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("LAPIN_BASE_URL must be an absolute HTTP(S) origin")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("LAPIN_BASE_URL must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("LAPIN_BASE_URL must not contain credentials, a path, query, or fragment")
	}
	if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
		return "", fmt.Errorf("LAPIN_BASE_URL must use HTTPS for non-loopback hosts")
	}
	parsed.Path = path
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
