// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// GetOperationName derives the Datadog operation name (span.name) from a span's
// kind and attributes. Ported from DD's GetOperationName, made version-safe:
// db.system.name, rpc.system.name, messaging.operation.type and HTTP status
// markers are read alongside their older names. Messaging spans without an
// operation type fall back to their span kind.
//
// An explicit "operation.name" attribute always wins (matches DD).
func GetOperationName(signalAttrs pcommon.Map, spanKind ptrace.SpanKind) string {
	if v := GetOTelAttrVal(signalAttrs, keyDDOperationName); v != "" {
		return v
	}

	isClient := spanKind == ptrace.SpanKindClient
	isServer := spanKind == ptrace.SpanKindServer

	// HTTP.
	if GetOTelAttrVal(signalAttrs, keyHTTPRequestMethod, keyHTTPMethod) != "" ||
		GetOTelAttrVal(signalAttrs, keyHTTPResponseStatusCode, keyHTTPStatusCode) != "" {
		if isServer {
			return "http.server.request"
		}
		if isClient {
			return "http.client.request"
		}
	}

	// Database.
	if isClient {
		if dbSystem := GetOTelAttrVal(signalAttrs, keyDBSystemName, keyDBSystem, keyDBType); dbSystem != "" {
			dbSystem = semconvmeta.CanonicalizeTechnology(dbSystem)
			if operation := GetOTelAttrVal(signalAttrs, keyDBOperationName, keyDBOperation); operation != "" {
				return dbSystem + "." + operation
			}
			return dbSystem + ".query"
		}
	}

	// Messaging.
	if system := GetOTelAttrVal(signalAttrs, keyMessagingSystem); system != "" {
		if op := GetOTelAttrVal(signalAttrs, keyMessagingOperationType, keyMessagingOperation); op != "" {
			switch spanKind {
			case ptrace.SpanKindClient, ptrace.SpanKindServer, ptrace.SpanKindConsumer, ptrace.SpanKindProducer:
				return system + "." + op
			}
		}

		switch spanKind {
		case ptrace.SpanKindProducer:
			return system + ".send"
		case ptrace.SpanKindConsumer:
			return system + ".process"
		case ptrace.SpanKindClient:
			return system + ".client"
		case ptrace.SpanKindServer:
			return system + ".server"
		}
	}

	// RPC, with the AWS-API special case.
	rpcSystem := GetOTelAttrVal(signalAttrs, keyRPCSystemName, keyRPCSystem)
	isRPC := rpcSystem != ""
	if isRPC && rpcSystem == "aws-api" && isClient {
		if service := GetOTelAttrVal(signalAttrs, keyRPCService); service != "" {
			return "aws." + service + ".request"
		}
		return "aws.client.request"
	}
	switch rpcSystem {
	case "apache_dubbo", "connect_rpc":
		rpcSystem = semconvmeta.CanonicalizeTechnology(rpcSystem)
	}
	if isRPC && isClient {
		return rpcSystem + ".client.request"
	}
	if isRPC && isServer {
		return rpcSystem + ".server.request"
	}

	// FaaS.
	if provider := GetOTelAttrVal(signalAttrs, keyFaaSInvokedProvider); provider != "" {
		if name := GetOTelAttrVal(signalAttrs, keyFaaSInvokedName); name != "" && isClient {
			return provider + "." + name + ".invoke"
		}
	}
	if trigger := GetOTelAttrVal(signalAttrs, keyFaaSTrigger); trigger != "" && isServer {
		return trigger + ".invoke"
	}

	// GraphQL.
	if GetOTelAttrVal(signalAttrs, keyGraphQLOperationType) != "" {
		return "graphql.server.request"
	}

	// Generic, by protocol + role.
	protocol := GetOTelAttrVal(signalAttrs, keyNetworkProtocolName, keyNetProtocolName)
	if isServer {
		if protocol != "" {
			return protocol + ".server.request"
		}
		return "server.request"
	}
	if isClient {
		if protocol != "" {
			return protocol + ".client.request"
		}
		return "client.request"
	}
	if spanKind != ptrace.SpanKindUnspecified {
		return spanKindOperationName(spanKind)
	}
	return "internal"
}

// spanKindOperationName preserves the lowercase operation-name contract without
// allocating through strings.ToLower(ptrace.SpanKind.String()).
func spanKindOperationName(spanKind ptrace.SpanKind) string {
	switch spanKind {
	case ptrace.SpanKindInternal:
		return "internal"
	case ptrace.SpanKindServer:
		return "server"
	case ptrace.SpanKindClient:
		return "client"
	case ptrace.SpanKindProducer:
		return "producer"
	case ptrace.SpanKindConsumer:
		return "consumer"
	default:
		return "unspecified"
	}
}
