// SPDX-License-Identifier: Apache-2.0
// Frozen BR-001 candidate. Protocol helpers live in projection_baseline_test.go.
package semconvweave

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"strings"
)

func (w WeaveID) priorProjectSpan(source ptrace.Span, options SpanMigrationOptions) SpanProjection {
	attrs := source.Attributes()
	result := SpanProjection{Attributes: pcommon.NewMap(), StatusDescription: source.Status().Message()}
	context := SpanContext{Name: source.Name(), Kind: source.Kind(), KnownHTTPMethods: options.KnownHTTPMethods}
	var work priorMigrationWork
	needsProjection := false
	attrs.Range(func(key string, _ pcommon.Value) bool {
		work |= priorMigrationWorkForKey(key)
		needsProjection = needsProjection || priorMayProjectContextAttribute(key)
		return true
	})
	if !needsProjection {
		// Bulk copy is linear and retains independent ownership, including nested
		// maps/slices/bytes. It avoids repeated destination key searches.
		attrs.CopyTo(result.Attributes)
	} else {
		w.priorProjectAttributeMap(attrs, result.Attributes, context)
	}
	m := baselineSpanMigration{weave: w, source: source, attrs: attrs, out: &result, options: options}
	if work&priorMigrationHTTP != 0 {
		m.baselineProjectHTTP()
	}
	if work&priorMigrationDB != 0 {
		m.baselineProjectDatabase()
	}
	if work&priorMigrationRPC != 0 {
		m.baselineProjectRPC()
	}
	if work&priorMigrationHTTP != 0 {
		m.baselineReportInvalidHTTPMethod()
	}
	return result
}

// priorMigrationWork is a conservative per-span work mask. It only skips protocol
// handlers; the installed generated-rule bitmap separately governs key rules.
// Keep namespace checks broad so malformed and future keys take the slow path.
type priorMigrationWork uint8

const (
	priorMigrationHTTP priorMigrationWork = 1 << iota
	priorMigrationDB
	priorMigrationRPC
)

func priorMigrationWorkForKey(key string) priorMigrationWork {
	switch {
	case strings.HasPrefix(key, "http."), strings.HasPrefix(key, "net."), strings.HasPrefix(key, "network."), strings.HasPrefix(key, "server.socket."), strings.HasPrefix(key, "messaging.protocol"):
		return priorMigrationHTTP
	case strings.HasPrefix(key, "db."):
		return priorMigrationDB
	case strings.HasPrefix(key, "rpc."), strings.HasPrefix(key, "message."):
		return priorMigrationRPC
	default:
		return 0
	}
}

// The bitmap belongs to the installed rules, while contextual peer projection
// is an additional rule of the multi-output API. Neither may hide the other.
func priorMayProjectContextAttribute(key string) bool {
	return MayTranslateSpanAttribute(key) || key == "net.peer.name" || key == "net.peer.port"
}

func (w WeaveID) priorProjectAttributeMap(attrs, output pcommon.Map, context SpanContext) {
	// One reservation instead of growing for every batch of new keys. Unlike
	// copy-then-rewrite, this only materializes the final value for renamed keys.
	output.EnsureCapacity(attrs.Len())
	attrs.Range(func(key string, value pcommon.Value) bool {
		if !priorMayProjectContextAttribute(key) {
			value.CopyTo(output.PutEmpty(key))
			return true
		}
		for _, decision := range w.baselineSpanAttributeWithContext(attrs, context, key, value) {
			switch decision.Action {
			case AttributeKeepValue:
				value.CopyTo(output.PutEmpty(decision.Key))
			case AttributeReplaceString:
				output.PutStr(decision.Key, decision.StringValue)
			}
		}
		return true
	})
}
