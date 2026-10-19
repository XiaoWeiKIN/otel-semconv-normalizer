// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv1_25 "go.opentelemetry.io/otel/semconv/v1.25.0"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestHTTPFieldKeysMatchOfficialSemconv(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "http request method", got: keyHTTPRequestMethod, want: string(semconv1_41.HTTPRequestMethodKey)},
		{name: "http request method original", got: keyHTTPRequestMethodOriginal, want: string(semconv1_41.HTTPRequestMethodOriginalKey)},
		{name: "legacy http method", got: keyHTTPMethod, want: string(semconv1_25.HTTPMethodKey)},
		{name: "http response status code", got: keyHTTPResponseStatusCode, want: string(semconv1_41.HTTPResponseStatusCodeKey)},
		{name: "legacy http status code", got: keyHTTPStatusCode, want: string(semconv1_25.HTTPStatusCodeKey)},
		{name: "url full", got: keyURLFull, want: string(semconv1_41.URLFullKey)},
		{name: "legacy http url", got: keyHTTPURL, want: string(semconv1_25.HTTPURLKey)},
		{name: "url scheme", got: keyURLScheme, want: string(semconv1_41.URLSchemeKey)},
		{name: "legacy http scheme", got: keyHTTPScheme, want: string(semconv1_25.HTTPSchemeKey)},
		{name: "url template", got: keyURLTemplate, want: string(semconv1_41.URLTemplateKey)},
		{name: "http route", got: keyHTTPRoute, want: string(semconv1_41.HTTPRouteKey)},
		{name: "user agent original", got: keyUserAgentOriginal, want: string(semconv1_41.UserAgentOriginalKey)},
		{name: "legacy http user agent", got: keyHTTPUserAgent, want: string(semconv1_25.HTTPUserAgentKey)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("key = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestHTTPRequestMethodValuesMatchOfficialSemconv(t *testing.T) {
	for _, method := range []string{
		semconv1_41.HTTPRequestMethodConnect.Value.AsString(),
		semconv1_41.HTTPRequestMethodDelete.Value.AsString(),
		semconv1_41.HTTPRequestMethodGet.Value.AsString(),
		semconv1_41.HTTPRequestMethodHead.Value.AsString(),
		semconv1_41.HTTPRequestMethodOptions.Value.AsString(),
		semconv1_41.HTTPRequestMethodPatch.Value.AsString(),
		semconv1_41.HTTPRequestMethodPost.Value.AsString(),
		semconv1_41.HTTPRequestMethodPut.Value.AsString(),
		semconv1_41.HTTPRequestMethodTrace.Value.AsString(),
		semconv1_41.HTTPRequestMethodQuery.Value.AsString(),
		semconv1_41.HTTPRequestMethodOther.Value.AsString(),
	} {
		if got := canonicalizeHTTPRequestMethod(method); got != method {
			t.Errorf("canonicalizeHTTPRequestMethod(%q) = %q, want unchanged", method, got)
		}
	}
}

func TestGetHTTPStringFields(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr(keyHTTPRequestMethod, "GET")
	attrs.PutStr(keyHTTPMethod, "POST")
	attrs.PutStr(keyHTTPRequestMethodOriginal, "PROPFIND")
	attrs.PutStr(keyURLFull, "https://current.example/orders")
	attrs.PutStr(keyHTTPURL, "https://legacy.example/orders")
	attrs.PutStr(keyURLScheme, "https")
	attrs.PutStr(keyHTTPScheme, "http")
	attrs.PutStr(keyURLTemplate, "/orders/{id}")
	attrs.PutStr(keyHTTPRoute, "/orders/:id")
	attrs.PutStr(keyUserAgentOriginal, "current-agent")
	attrs.PutStr(keyHTTPUserAgent, "legacy-agent")

	tests := []struct {
		name string
		get  func(pcommon.Map) string
		want string
	}{
		{name: "http request method", get: GetHTTPRequestMethod, want: "GET"},
		{name: "http request method original", get: GetHTTPRequestMethodOriginal, want: "PROPFIND"},
		{name: "url full", get: GetURLFull, want: "https://current.example/orders"},
		{name: "url scheme", get: GetURLScheme, want: "https"},
		{name: "url template", get: GetURLTemplate, want: "/orders/{id}"},
		{name: "http route", get: GetHTTPRoute, want: "/orders/:id"},
		{name: "user agent original", get: GetUserAgentOriginal, want: "current-agent"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.get(attrs); got != test.want {
				t.Fatalf("getter() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestGetHTTPResponseStatusCode(t *testing.T) {
	t.Run("current wins", func(t *testing.T) {
		attrs := pcommon.NewMap()
		attrs.PutInt(keyHTTPResponseStatusCode, 201)
		attrs.PutInt(keyHTTPStatusCode, 503)

		got, ok := GetHTTPResponseStatusCode(attrs)
		if !ok || got != 201 {
			t.Fatalf("GetHTTPResponseStatusCode() = (%d, %v), want (201, true)", got, ok)
		}
	})

	t.Run("wrong current type falls back", func(t *testing.T) {
		attrs := pcommon.NewMap()
		attrs.PutStr(keyHTTPResponseStatusCode, "201")
		attrs.PutInt(keyHTTPStatusCode, 503)

		got, ok := GetHTTPResponseStatusCode(attrs)
		if !ok || got != 503 {
			t.Fatalf("GetHTTPResponseStatusCode() = (%d, %v), want (503, true)", got, ok)
		}
	})

	t.Run("missing", func(t *testing.T) {
		got, ok := GetHTTPResponseStatusCode(pcommon.NewMap())
		if ok || got != 0 {
			t.Fatalf("GetHTTPResponseStatusCode() = (%d, %v), want (0, false)", got, ok)
		}
	})
}

func TestGetHTTPRequestMethod(t *testing.T) {
	knownMethods := []string{
		semconv1_41.HTTPRequestMethodConnect.Value.AsString(),
		semconv1_41.HTTPRequestMethodDelete.Value.AsString(),
		semconv1_41.HTTPRequestMethodGet.Value.AsString(),
		semconv1_41.HTTPRequestMethodHead.Value.AsString(),
		semconv1_41.HTTPRequestMethodOptions.Value.AsString(),
		semconv1_41.HTTPRequestMethodPatch.Value.AsString(),
		semconv1_41.HTTPRequestMethodPost.Value.AsString(),
		semconv1_41.HTTPRequestMethodPut.Value.AsString(),
		semconv1_41.HTTPRequestMethodTrace.Value.AsString(),
		semconv1_41.HTTPRequestMethodQuery.Value.AsString(),
	}
	for _, method := range knownMethods {
		t.Run("known "+method, func(t *testing.T) {
			attrs := testAttrs(map[string]string{keyHTTPRequestMethod: method})
			if got := GetHTTPRequestMethod(attrs); got != method {
				t.Fatalf("GetHTTPRequestMethod() = %q, want %q", got, method)
			}
			if got := GetHTTPRequestMethodOriginal(attrs); got != "" {
				t.Fatalf("GetHTTPRequestMethodOriginal() = %q, want empty", got)
			}
		})
	}

	tests := []struct {
		name         string
		attrs        map[string]string
		wantMethod   string
		wantOriginal string
	}{
		{
			name: "current other with explicit original",
			attrs: map[string]string{
				keyHTTPRequestMethod:         "_OTHER",
				keyHTTPRequestMethodOriginal: "PROPFIND",
			},
			wantMethod:   "_OTHER",
			wantOriginal: "PROPFIND",
		},
		{
			name:         "current other without original",
			attrs:        map[string]string{keyHTTPRequestMethod: "_OTHER"},
			wantMethod:   "_OTHER",
			wantOriginal: "",
		},
		{
			name:         "current custom derives original",
			attrs:        map[string]string{keyHTTPRequestMethod: "BREW"},
			wantMethod:   "_OTHER",
			wantOriginal: "BREW",
		},
		{
			name:         "legacy propfind",
			attrs:        map[string]string{keyHTTPMethod: "PROPFIND"},
			wantMethod:   "_OTHER",
			wantOriginal: "PROPFIND",
		},
		{
			name:         "legacy acl",
			attrs:        map[string]string{keyHTTPMethod: "ACL"},
			wantMethod:   "_OTHER",
			wantOriginal: "ACL",
		},
		{
			name: "current conflict wins",
			attrs: map[string]string{
				keyHTTPRequestMethod: "BREW",
				keyHTTPMethod:        "GET",
			},
			wantMethod:   "_OTHER",
			wantOriginal: "BREW",
		},
		{
			name: "current empty falls back",
			attrs: map[string]string{
				keyHTTPRequestMethod: "",
				keyHTTPMethod:        "QUERY",
			},
			wantMethod:   "QUERY",
			wantOriginal: "",
		},
		{
			name: "explicit original wins over derived value",
			attrs: map[string]string{
				keyHTTPRequestMethod:         "CUSTOM",
				keyHTTPRequestMethodOriginal: "WIRE-CUSTOM",
			},
			wantMethod:   "_OTHER",
			wantOriginal: "WIRE-CUSTOM",
		},
		{
			name:         "missing",
			attrs:        map[string]string{},
			wantMethod:   "",
			wantOriginal: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := testAttrs(test.attrs)
			if got := GetHTTPRequestMethod(attrs); got != test.wantMethod {
				t.Errorf("GetHTTPRequestMethod() = %q, want %q", got, test.wantMethod)
			}
			if got := GetHTTPRequestMethodOriginal(attrs); got != test.wantOriginal {
				t.Errorf("GetHTTPRequestMethodOriginal() = %q, want %q", got, test.wantOriginal)
			}
		})
	}
}

func TestHTTPStringFieldCompatibilityMatrix(t *testing.T) {
	fields := []struct {
		name    string
		current string
		legacy  string
		get     func(pcommon.Map) string
	}{
		{name: "url full", current: keyURLFull, legacy: keyHTTPURL, get: GetURLFull},
		{name: "url scheme", current: keyURLScheme, legacy: keyHTTPScheme, get: GetURLScheme},
		{name: "user agent", current: keyUserAgentOriginal, legacy: keyHTTPUserAgent, get: GetUserAgentOriginal},
	}

	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			tests := []struct {
				name  string
				attrs map[string]string
				want  string
			}{
				{name: "new only", attrs: map[string]string{field.current: "current"}, want: "current"},
				{name: "legacy only", attrs: map[string]string{field.legacy: "legacy"}, want: "legacy"},
				{name: "both same", attrs: map[string]string{field.current: "same", field.legacy: "same"}, want: "same"},
				{name: "both conflict", attrs: map[string]string{field.current: "current", field.legacy: "legacy"}, want: "current"},
				{name: "present empty", attrs: map[string]string{field.current: "", field.legacy: "legacy"}, want: "legacy"},
				{name: "unknown value", attrs: map[string]string{field.current: "future-value"}, want: "future-value"},
				{name: "missing", attrs: map[string]string{}, want: ""},
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					if got := field.get(testAttrs(test.attrs)); got != test.want {
						t.Fatalf("getter() = %q, want %q", got, test.want)
					}
				})
			}
		})
	}

	if got := GetURLTemplate(testAttrs(map[string]string{keyURLTemplate: "/orders/{id}"})); got != "/orders/{id}" {
		t.Errorf("GetURLTemplate() = %q, want /orders/{id}", got)
	}
	if got := GetHTTPRoute(testAttrs(map[string]string{keyHTTPRoute: "/orders/{id}"})); got != "/orders/{id}" {
		t.Errorf("GetHTTPRoute() = %q, want /orders/{id}", got)
	}
}

func TestHTTPResponseStatusCodeCompatibilityMatrix(t *testing.T) {
	type statusValue struct {
		key   string
		value int64
	}
	tests := []struct {
		name        string
		ints        []statusValue
		currentText string
		want        int64
		wantOK      bool
	}{
		{
			name:   "new only",
			ints:   []statusValue{{key: keyHTTPResponseStatusCode, value: 201}},
			want:   201,
			wantOK: true,
		},
		{
			name:   "legacy only",
			ints:   []statusValue{{key: keyHTTPStatusCode, value: 404}},
			want:   404,
			wantOK: true,
		},
		{
			name: "both same",
			ints: []statusValue{
				{key: keyHTTPResponseStatusCode, value: 204},
				{key: keyHTTPStatusCode, value: 204},
			},
			want:   204,
			wantOK: true,
		},
		{
			name: "both conflict current wins",
			ints: []statusValue{
				{key: keyHTTPResponseStatusCode, value: 202},
				{key: keyHTTPStatusCode, value: 500},
			},
			want:   202,
			wantOK: true,
		},
		{
			name:        "wrong current type falls back",
			currentText: "201",
			ints:        []statusValue{{key: keyHTTPStatusCode, value: 503}},
			want:        503,
			wantOK:      true,
		},
		{
			name:   "zero remains present",
			ints:   []statusValue{{key: keyHTTPResponseStatusCode, value: 0}},
			want:   0,
			wantOK: true,
		},
		{name: "missing", want: 0, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attrs := pcommon.NewMap()
			if test.currentText != "" {
				attrs.PutStr(keyHTTPResponseStatusCode, test.currentText)
			}
			for _, value := range test.ints {
				attrs.PutInt(value.key, value.value)
			}

			got, ok := GetHTTPResponseStatusCode(attrs)
			if got != test.want || ok != test.wantOK {
				t.Fatalf("GetHTTPResponseStatusCode() = (%d, %v), want (%d, %v)", got, ok, test.want, test.wantOK)
			}
		})
	}
}
