// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestGetOTelAttrVal(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("current", "")
	attrs.PutStr("current-value", "POST")
	attrs.PutStr("legacy", "GET")
	attrs.PutInt("status", 200)

	tests := []struct {
		name string
		keys []string
		want string
	}{
		{name: "present empty falls back", keys: []string{"current", "legacy"}, want: "GET"},
		{name: "current non-empty wins", keys: []string{"current-value", "legacy"}, want: "POST"},
		{name: "first non-empty wins", keys: []string{"legacy", "status"}, want: "GET"},
		{name: "non-string value is stringified", keys: []string{"status"}, want: "200"},
		{name: "missing value", keys: []string{"missing"}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GetOTelAttrVal(attrs, test.keys...); got != test.want {
				t.Fatalf("GetOTelAttrVal() = %q, want %q", got, test.want)
			}
		})
	}
}

var getOTelAttrValResult string

func TestGetOTelAttrValStringPathDoesNotAllocate(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("legacy", "GET")

	allocs := testing.AllocsPerRun(1000, func() {
		getOTelAttrValResult = GetOTelAttrVal(attrs, "current", "legacy")
	})
	if allocs != 0 {
		t.Fatalf("GetOTelAttrVal string path allocated %v times, want 0", allocs)
	}
}

func BenchmarkGetOTelAttrVal(b *testing.B) {
	attrs := pcommon.NewMap()
	attrs.PutStr("current", "GET")
	attrs.PutStr("legacy", "POST")

	for _, benchmark := range []struct {
		name string
		keys []string
	}{
		{name: "current", keys: []string{"current", "legacy"}},
		{name: "legacy_fallback", keys: []string{"missing", "legacy"}},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				getOTelAttrValResult = GetOTelAttrVal(attrs, benchmark.keys...)
			}
		})
	}
}
