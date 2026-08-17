package handler

import "testing"

func TestAPIErrorCodesRemainStableAndUnique(t *testing.T) {
	codes := map[errorCode]string{
		errorCodeDatabaseUnavailable: "database_unavailable",
		errorCodeEmailExists:         "email_exists",
		errorCodeInternal:            "internal_error",
		errorCodeInvalidAccessToken:  "invalid_access_token",
		errorCodeInvalidCredentials:  "invalid_credentials",
		errorCodeInvalidCSRF:         "invalid_csrf",
		errorCodeInvalidID:           "invalid_id",
		errorCodeInvalidInput:        "invalid_input",
		errorCodeInvalidJSON:         "invalid_json",
		errorCodeInvalidOrigin:       "invalid_origin",
		errorCodeInvalidParent:       "invalid_parent",
		errorCodeNotFound:            "not_found",
		errorCodeRateLimited:         "rate_limited",
		errorCodeServiceBusy:         "service_busy",
		errorCodeTokenLimit:          "token_limit",
		errorCodeUnauthenticated:     "unauthenticated",
		errorCodeUnsupportedMedia:    "unsupported_media_type",
	}

	if len(codes) != 17 {
		t.Fatalf("error code count = %d, want 17", len(codes))
	}
	for code, want := range codes {
		if string(code) != want {
			t.Errorf("error code = %q, want %q", code, want)
		}
	}
}
