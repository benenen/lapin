package lapincli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benenen/lapin/internal/identifier"
)

func TestImportEndpointRejectsAmbiguousOrInsecureOrigins(t *testing.T) {
	tests := []struct {
		baseURL string
		valid   bool
	}{
		{baseURL: "http://127.0.0.1:8080", valid: true},
		{baseURL: "http://localhost:8080/", valid: true},
		{baseURL: "https://lapin.example.com", valid: true},
		{baseURL: "http://lapin.example.com", valid: false},
		{baseURL: "ftp://127.0.0.1", valid: false},
		{baseURL: "https://user:secret@lapin.example.com", valid: false},
		{baseURL: "https://lapin.example.com/base", valid: false},
		{baseURL: "https://lapin.example.com?target=x", valid: false},
		{baseURL: "https://lapin.example.com#fragment", valid: false},
	}
	for _, test := range tests {
		t.Run(test.baseURL, func(t *testing.T) {
			endpoint, err := importEndpoint(test.baseURL)
			if test.valid && (err != nil || !strings.HasSuffix(endpoint, "/openapi/v1/subjects/import")) {
				t.Fatalf("endpoint = %q, error = %v", endpoint, err)
			}
			if !test.valid && err == nil {
				t.Fatalf("endpoint = %q, want error", endpoint)
			}
		})
	}
}

func TestValidSubjectIDMatchesHashIDWireBounds(t *testing.T) {
	codec, err := identifier.New("lapin-cli-test-salt")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		value string
		valid bool
	}{
		{value: codec.Encode(1), valid: true},
		{value: strings.Repeat("a", 9), valid: false},
		{value: strings.Repeat("a", 10), valid: true},
		{value: strings.Repeat("a", 64), valid: true},
		{value: strings.Repeat("a", 65), valid: false},
		{value: "invalid_id", valid: false},
	}
	for _, test := range tests {
		if got := validSubjectID(test.value); got != test.valid {
			t.Errorf("validSubjectID(%q) = %t, want %t", test.value, got, test.valid)
		}
	}
}

func TestImportCourseDoesNotFollowRedirects(t *testing.T) {
	var redirectedRequests atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectedRequests.Add(1)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	_, err := importCourse(context.Background(), source.URL, "lpn_redirect-test", []byte(`{}`), source.Client())
	if err == nil {
		t.Fatal("expected redirect to fail")
	}
	if redirectedRequests.Load() != 0 {
		t.Fatalf("redirect target received %d requests", redirectedRequests.Load())
	}
}

func TestImportCourseRedactsTokenFromAPIErrors(t *testing.T) {
	const token = "lpn_server-reflected-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"` + token + `","message":"rejected ` + token + `"}}`))
	}))
	defer server.Close()

	_, err := importCourse(context.Background(), server.URL, token, []byte(`{}`), server.Client())
	if err == nil {
		t.Fatal("expected API error")
	}
	if strings.Contains(err.Error(), token) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error did not redact token: %q", err)
	}
}

func TestImportCourseRejectsTokenReflectedAsSuccessfulSubjectID(t *testing.T) {
	const token = "lpn_success-reflected-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"data":{"id":"` + token + `","external_id":"ignored","title":"ignored"}}`))
	}))
	defer server.Close()

	result, err := importCourse(context.Background(), server.URL, token, []byte(`{}`), server.Client())
	if err == nil {
		t.Fatalf("result = %#v, want invalid subject ID error", result)
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked token: %q", err)
	}
}

func TestImportCourseSanitizesUntrustedErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Retry-After", "not-a-delay")
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"error":{"code":"bad\u001bcode","message":"first\n\u001b[31m` + strings.Repeat("x", 2000) + `"}}`))
	}))
	defer server.Close()

	_, err := importCourse(context.Background(), server.URL, "lpn_diagnostic", []byte(`{}`), server.Client())
	if err == nil {
		t.Fatal("expected API error")
	}
	message := err.Error()
	if strings.ContainsAny(message, "\r\n\x1b\x07") {
		t.Fatalf("error contains terminal control characters: %q", message)
	}
	if len(message) > 800 {
		t.Fatalf("error length = %d, want bounded diagnostic", len(message))
	}
	if !strings.Contains(message, "request_failed") {
		t.Fatalf("error code was not normalized: %q", message)
	}
}

func TestImportCourseReportsRateLimitWithoutRetrying(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.Header().Set("Retry-After", "7")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"code":"rate_limited","message":"slow down"}}`))
	}))
	defer server.Close()

	_, err := importCourse(context.Background(), server.URL, "lpn_rate-limit", []byte(`{}`), server.Client())
	if err == nil || !strings.Contains(err.Error(), "rate_limited") || !strings.Contains(err.Error(), "retry after 7") {
		t.Fatalf("error = %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d, want 1", requests.Load())
	}
}

func TestImportCourseRejectsOversizedResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(strings.Repeat("x", maxResponseBody+1)))
	}))
	defer server.Close()

	_, err := importCourse(context.Background(), server.URL, "lpn_large-response", []byte(`{}`), server.Client())
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("error = %v", err)
	}
}

func TestImportCourseHonorsContextCancellation(t *testing.T) {
	started := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		close(started)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()

	client.Timeout = time.Second
	_, err := importCourse(ctx, "http://127.0.0.1:8080", "lpn_cancel", []byte(`{}`), client)
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
