// SPDX-License-Identifier: Apache-2.0

package semconvweave

import (
	"slices"
	"strconv"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

var grpcStatusNames = [...]string{"OK", "CANCELLED", "UNKNOWN", "INVALID_ARGUMENT", "DEADLINE_EXCEEDED", "NOT_FOUND", "ALREADY_EXISTS", "PERMISSION_DENIED", "RESOURCE_EXHAUSTED", "FAILED_PRECONDITION", "ABORTED", "OUT_OF_RANGE", "UNIMPLEMENTED", "INTERNAL", "UNAVAILABLE", "DATA_LOSS", "UNAUTHENTICATED"}

func (m spanMigration) projectRPC() {
	system := m.rpcSystem()
	for _, key := range []string{"rpc.grpc.status_code", "rpc.jsonrpc.error_code"} {
		v, ok := m.attrs.Get(key)
		if !ok {
			continue
		}
		if m.has("rpc.response.status_code") {
			m.out.Attributes.Remove(key)
			continue
		}
		if (m.has("rpc.grpc.status_code") && m.has("rpc.jsonrpc.error_code")) || m.has("rpc.connect_rpc.error_code") {
			m.out.Attributes.Remove("rpc.response.status_code")
			for _, sourceKey := range []string{"rpc.grpc.status_code", "rpc.jsonrpc.error_code", "rpc.connect_rpc.error_code"} {
				if sourceValue, exists := m.attrs.Get(sourceKey); exists {
					sourceValue.CopyTo(m.out.Attributes.PutEmpty(sourceKey))
				}
			}
			m.issue(key, MigrationInvalidValue, "conflicting RPC status code sources")
			continue
		}
		if v.Type() != pcommon.ValueTypeInt {
			m.issue(key, MigrationInvalidValue, "RPC response status code must be an integer")
			continue
		}
		var code string
		if key == "rpc.grpc.status_code" {
			if v.Int() < 0 || v.Int() >= int64(len(grpcStatusNames)) {
				m.issue(key, MigrationInvalidValue, "gRPC status code is outside 0..16")
				continue
			}
			if system != "" && system != "grpc" {
				m.issue(key, MigrationInvalidValue, "gRPC status code conflicts with RPC system")
				continue
			}
			code = grpcStatusNames[v.Int()]
		} else {
			if system != "" && system != "jsonrpc" {
				m.issue(key, MigrationInvalidValue, "JSON-RPC error code conflicts with RPC system")
				continue
			}
			code = strconv.FormatInt(v.Int(), 10)
		}
		m.stringTarget(key, "rpc.response.status_code", code)
	}
	if v, ok := m.attrs.Get("rpc.jsonrpc.error_message"); ok {
		if system != "" && system != "jsonrpc" {
			m.issue("rpc.jsonrpc.error_message", MigrationInvalidValue, "JSON-RPC error message conflicts with RPC system")
		} else if m.source.Status().Code() != ptrace.StatusCodeError {
			m.issue("rpc.jsonrpc.error_message", MigrationMissingContext, "ERROR span status is required for status description")
		} else if v.Type() != pcommon.ValueTypeStr {
			m.issue("rpc.jsonrpc.error_message", MigrationInvalidValue, "JSON-RPC error message must be a string")
		} else {
			if m.out.StatusDescription == "" {
				m.out.StatusDescription = v.Str()
			}
			m.out.Attributes.Remove("rpc.jsonrpc.error_message")
		}
	}
	// Same-key changes require evidence of legacy RPC semantics. Canonical
	// rpc.system.name alone is never proof that a method is still unqualified.
	legacy := m.has("rpc.service") || m.has("rpc.system") || m.has("rpc.grpc.status_code") || m.has("rpc.jsonrpc.error_code")
	if legacy {
		m.rpcMethod(system)
	}
	for _, key := range []string{"message.compressed_size", "message.id", "message.type", "message.uncompressed_size", "rpc.message.compressed_size", "rpc.message.id", "rpc.message.type", "rpc.message.uncompressed_size"} {
		m.remove(key)
	}
	if system != "" {
		// rpc.system.name marks a dual-emitted current endpoint. A genuinely
		// legacy RPC endpoint has the same key but changed meaning in v1.41.0.
		if legacy && !m.has("rpc.system.name") {
			m.rpcExistingEndpoint()
		}
		// Network and client attributes remain valid general attributes; removal from the RPC
		// requirement table does not make them globally invalid.
		for _, key := range []string{"net.peer.name", "net.peer.port"} {
			if !m.has(key) {
				continue
			}
			target := "server.address"
			if key == "net.peer.port" {
				target = "server.port"
			}
			if m.has(target) {
				m.out.Attributes.Remove(key)
				continue
			}
			ctx := m.options.RPCServer
			if ctx == nil || ctx.Address == "" {
				m.issue(key, MigrationMissingContext, "configured logical RPC server is required")
				continue
			}
			if key == "net.peer.name" {
				m.stringTarget(key, target, ctx.Address)
			} else if ctx.Port > 0 && ctx.Port <= 65535 {
				m.intTarget(key, target, ctx.Port)
			} else {
				m.issue(key, MigrationInvalidValue, "configured RPC server port is missing or invalid")
			}
		}
	}
}

func (m spanMigration) rpcMethod(system string) {
	method, ok := m.attrs.Get("rpc.method")
	if !ok {
		if m.has("rpc.service") {
			m.issue("rpc.service", MigrationMissingContext, "RPC method is required to merge service")
		}
		return
	}
	if method.Type() != pcommon.ValueTypeStr || method.Str() == "" {
		m.issue("rpc.method", MigrationInvalidValue, "RPC method must be a nonempty string")
		return
	}
	if method.Str() == "_OTHER" {
		m.out.Attributes.Remove("rpc.service")
		return
	}
	original := method.Str()
	if m.has("rpc.system.name") && strings.Contains(original, "/") {
		// The already-qualified current method wins over a dual-emitted service.
		m.out.Attributes.Remove("rpc.service")
		return
	}
	service := stringAttribute(m.attrs, "rpc.service")
	if m.has("rpc.service") && service == "" {
		m.issue("rpc.service", MigrationInvalidValue, "RPC service must be a nonempty string")
		return
	}
	if system != "jsonrpc" {
		if service != "" && !strings.Contains(original, "/") {
			original = service + "/" + original
		}
		if !strings.Contains(original, "/") {
			m.issue("rpc.method", MigrationMissingContext, "fully qualified RPC service/method is required")
			return
		}
	}
	if system != "jsonrpc" && service != "" && !strings.HasPrefix(original, service+"/") {
		m.issue("rpc.method", MigrationInvalidValue, "RPC service conflicts with qualified method")
		return
	}
	m.out.Attributes.Remove("rpc.service")
	if slices.Contains(m.options.KnownRPCMethods, original) {
		m.out.Attributes.PutStr("rpc.method", original)
		return
	}
	m.out.Attributes.PutStr("rpc.method", "_OTHER")
	if !m.has("rpc.method_original") {
		m.out.Attributes.PutStr("rpc.method_original", original)
	}
}

func (m spanMigration) rpcExistingEndpoint() {
	for _, key := range []string{"server.address", "server.port"} {
		if !m.has(key) {
			continue
		}
		ctx := m.options.RPCServer
		if ctx == nil || ctx.Address == "" {
			m.issue(key, MigrationMissingContext, "legacy RPC endpoint requires configured logical server")
			continue
		}
		if key == "server.address" {
			m.out.Attributes.PutStr(key, ctx.Address)
			continue
		}
		if ctx.Port < 1 || ctx.Port > 65535 {
			m.issue(key, MigrationInvalidValue, "configured RPC server port is missing or invalid")
			continue
		}
		m.out.Attributes.PutInt(key, ctx.Port)
	}
}
