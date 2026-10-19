// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

// GetNetworkProtocolName returns network.protocol.name, falling back to its
// legacy net.protocol.name alias.
func GetNetworkProtocolName(signalAttrs pcommon.Map) string {
	return getCanonicalStringField(signalAttrs, semconvmeta.FieldNetworkProtocolName)
}

// GetNetworkTransport returns network.transport, falling back to its legacy
// net.transport alias. Legacy ip_tcp and ip_udp values are migrated to their
// current spellings; values without a lossless current equivalent pass through.
func GetNetworkTransport(signalAttrs pcommon.Map) string {
	return canonicalizeNetworkTransport(
		getCanonicalStringField(signalAttrs, semconvmeta.FieldNetworkTransport),
	)
}

func canonicalizeNetworkTransport(transport string) string {
	switch transport {
	case "ip_tcp":
		return "tcp"
	case "ip_udp":
		return "udp"
	default:
		return transport
	}
}
