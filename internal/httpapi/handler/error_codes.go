package handler

type errorCode string

const (
	errorCodeDatabaseUnavailable errorCode = "database_unavailable"
	errorCodeEmailExists         errorCode = "email_exists"
	errorCodeInternal            errorCode = "internal_error"
	errorCodeInvalidAccessToken  errorCode = "invalid_access_token"
	errorCodeInvalidCredentials  errorCode = "invalid_credentials"
	errorCodeInvalidCSRF         errorCode = "invalid_csrf"
	errorCodeInvalidID           errorCode = "invalid_id"
	errorCodeInvalidInput        errorCode = "invalid_input"
	errorCodeInvalidJSON         errorCode = "invalid_json"
	errorCodeInvalidOrigin       errorCode = "invalid_origin"
	errorCodeInvalidParent       errorCode = "invalid_parent"
	errorCodeNotFound            errorCode = "not_found"
	errorCodeRateLimited         errorCode = "rate_limited"
	errorCodeServiceBusy         errorCode = "service_busy"
	errorCodeTokenLimit          errorCode = "token_limit"
	errorCodeUnauthenticated     errorCode = "unauthenticated"
	errorCodeUnsupportedMedia    errorCode = "unsupported_media_type"
)
