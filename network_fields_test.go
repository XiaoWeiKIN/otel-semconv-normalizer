// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	semconv1_25 "go.opentelemetry.io/otel/semconv/v1.25.0"
	semconv1_41 "go.opentelemetry.io/otel/semconv/v1.41.0"
)

func TestNetworkFieldKeysMatchOfficialSemconv(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "network protocol name", got: keyNetworkProtocolName, want: string(semconv1_41.NetworkProtocolNameKey)},
		{name: "legacy net protocol name", got: keyNetProtocolName, want: string(semconv1_25.NetProtocolNameKey)},
		{name: "network transport", got: keyNetworkTransport, want: string(semconv1_41.NetworkTransportKey)},
		{name: "legacy net transport", got: keyNetTransport, want: string(semconv1_25.NetTransportKey)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("key = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestNetworkTransportValuesMatchOfficialSemconv(t *testing.T) {
	tests := []struct {
		legacy string
		want   string
	}{
		{
			legacy: semconv1_25.NetTransportTCP.Value.AsString(),
			want:   semconv1_41.NetworkTransportTCP.Value.AsString(),
		},
		{
			legacy: semconv1_25.NetTransportUDP.Value.AsString(),
			want:   semconv1_41.NetworkTransportUDP.Value.AsString(),
		},
		{
			legacy: semconv1_25.NetTransportPipe.Value.AsString(),
			want:   semconv1_41.NetworkTransportPipe.Value.AsString(),
		},
	}

	for _, test := range tests {
		if got := canonicalizeNetworkTransport(test.legacy); got != test.want {
			t.Errorf("canonicalizeNetworkTransport(%q) = %q, want %q", test.legacy, got, test.want)
		}
	}
}

func TestGetNetworkStringFields(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr(keyNetworkProtocolName, "http")
	attrs.PutStr(keyNetProtocolName, "legacy-http")
	attrs.PutStr(keyNetworkTransport, "tcp")
	attrs.PutStr(keyNetTransport, "ip_udp")

	if got := GetNetworkProtocolName(attrs); got != "http" {
		t.Fatalf("GetNetworkProtocolName() = %q, want http", got)
	}
	if got := GetNetworkTransport(attrs); got != "tcp" {
		t.Fatalf("GetNetworkTransport() = %q, want tcp", got)
	}
}

func TestGetNetworkTransport(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		want  string
	}{
		{name: "legacy tcp", attrs: map[string]string{keyNetTransport: "ip_tcp"}, want: "tcp"},
		{name: "legacy udp", attrs: map[string]string{keyNetTransport: "ip_udp"}, want: "udp"},
		{name: "legacy pipe", attrs: map[string]string{keyNetTransport: "pipe"}, want: "pipe"},
		{name: "current tcp", attrs: map[string]string{keyNetworkTransport: "tcp"}, want: "tcp"},
		{name: "current udp", attrs: map[string]string{keyNetworkTransport: "udp"}, want: "udp"},
		{name: "current pipe", attrs: map[string]string{keyNetworkTransport: "pipe"}, want: "pipe"},
		{name: "current unix", attrs: map[string]string{keyNetworkTransport: "unix"}, want: "unix"},
		{name: "current quic", attrs: map[string]string{keyNetworkTransport: "quic"}, want: "quic"},
		{name: "legacy inproc preserved", attrs: map[string]string{keyNetTransport: "inproc"}, want: "inproc"},
		{name: "legacy other preserved", attrs: map[string]string{keyNetTransport: "other"}, want: "other"},
		{name: "custom preserved", attrs: map[string]string{keyNetTransport: "future_transport"}, want: "future_transport"},
		{
			name: "current wins conflict",
			attrs: map[string]string{
				keyNetworkTransport: "udp",
				keyNetTransport:     "ip_tcp",
			},
			want: "udp",
		},
		{
			name: "current empty falls back",
			attrs: map[string]string{
				keyNetworkTransport: "",
				keyNetTransport:     "ip_tcp",
			},
			want: "tcp",
		},
		{name: "missing", attrs: map[string]string{}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GetNetworkTransport(testAttrs(test.attrs)); got != test.want {
				t.Fatalf("GetNetworkTransport() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNetworkProtocolNameCompatibilityMatrix(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		want  string
	}{
		{name: "new only", attrs: map[string]string{keyNetworkProtocolName: "current"}, want: "current"},
		{name: "legacy only", attrs: map[string]string{keyNetProtocolName: "legacy"}, want: "legacy"},
		{name: "both same", attrs: map[string]string{keyNetworkProtocolName: "same", keyNetProtocolName: "same"}, want: "same"},
		{name: "both conflict", attrs: map[string]string{keyNetworkProtocolName: "current", keyNetProtocolName: "legacy"}, want: "current"},
		{name: "present empty", attrs: map[string]string{keyNetworkProtocolName: "", keyNetProtocolName: "legacy"}, want: "legacy"},
		{name: "unknown value", attrs: map[string]string{keyNetworkProtocolName: "future-value"}, want: "future-value"},
		{name: "missing", attrs: map[string]string{}, want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := GetNetworkProtocolName(testAttrs(test.attrs)); got != test.want {
				t.Fatalf("GetNetworkProtocolName() = %q, want %q", got, test.want)
			}
		})
	}
}
