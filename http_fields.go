// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

const httpRequestMethodOther = "_OTHER"

// GetHTTPRequestMethod returns the canonical HTTP request method, preferring
// http.request.method over its legacy http.method alias. Unknown methods are
// represented by _OTHER; retrieve their lossless value with
// GetHTTPRequestMethodOriginal.
func GetHTTPRequestMethod(signalAttrs pcommon.Map) string {
	return canonicalizeHTTPRequestMethod(getHTTPRequestMethodRaw(signalAttrs))
}

// GetHTTPRequestMethodOriginal returns an explicitly recorded current original
// method, or derives it from an unknown current/legacy method that
// GetHTTPRequestMethod canonicalizes to _OTHER.
func GetHTTPRequestMethodOriginal(signalAttrs pcommon.Map) string {
	if original := getCanonicalStringField(signalAttrs, semconvmeta.FieldHTTPRequestMethodOriginal); original != "" {
		return original
	}

	method := getHTTPRequestMethodRaw(signalAttrs)
	if method == "" || method == httpRequestMethodOther || isKnownHTTPRequestMethod(method) {
		return ""
	}
	return method
}

func getHTTPRequestMethodRaw(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldHTTPRequestMethod)
}

func canonicalizeHTTPRequestMethod(method string) string {
	if method == "" || method == httpRequestMethodOther || isKnownHTTPRequestMethod(method) {
		return method
	}
	return httpRequestMethodOther
}

func isKnownHTTPRequestMethod(method string) bool {
	switch method {
	case "CONNECT", "DELETE", "GET", "HEAD", "OPTIONS",
		"PATCH", "POST", "PUT", "TRACE", "QUERY":
		return true
	default:
		return false
	}
}

// GetHTTPResponseStatusCode returns the first correctly typed current or legacy
// HTTP response status code. A present value with the wrong OTLP type is ignored.
func GetHTTPResponseStatusCode(signalAttrs pcommon.Map) (int64, bool) {
	return getCanonicalInt64Field(signalAttrs, semconvmeta.FieldHTTPResponseStatusCode)
}

func hasHTTPResponseStatusCode(signalAttrs pcommon.Map) bool {
	if _, ok := GetHTTPResponseStatusCode(signalAttrs); ok {
		return true
	}
	// Preserve compatibility with pre-existing callers that supplied a
	// stringified status marker, while the public getter remains type-safe.
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldHTTPResponseStatusCode) != ""
}

// GetURLFull returns url.full, falling back to its legacy http.url alias.
// It returns the raw value; callers remain responsible for sensitive-data
// redaction before persistence.
func GetURLFull(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldURLFull)
}

// GetURLScheme returns url.scheme, falling back to its legacy http.scheme alias.
func GetURLScheme(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldURLScheme)
}

// GetURLTemplate returns the current url.template value.
func GetURLTemplate(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldURLTemplate)
}

// GetHTTPRoute returns the current http.route value.
func GetHTTPRoute(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldHTTPRoute)
}

// GetUserAgentOriginal returns user_agent.original, falling back to the legacy
// http.user_agent alias. It returns the raw, potentially high-cardinality value.
func GetUserAgentOriginal(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldUserAgentOriginal)
}
