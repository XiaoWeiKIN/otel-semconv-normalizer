// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

const (
	// DefaultServiceName is returned by GetServiceName when service.name is absent.
	DefaultServiceName = "unknown_service"
	// DefaultDeploymentEnvironmentName is returned by GetDeploymentEnvironmentName when no
	// deployment environment is present.
	DefaultDeploymentEnvironmentName = "default"

	maxResourceNameLen = 5000
)

// GetResourceName derives the Datadog resource name from a span's kind and
// attributes. It follows Datadog's decision tree while preferring current
// semantic-convention keys over legacy aliases. Callers should pass
// ptrace.Span.Name() as fallbackName.
//
// Database query text participates in resource naming to match Datadog. This
// package does not sanitize it: the ingestion contract must guarantee that the
// producing agent sanitized db.query.text/db.statement before export.
// fallbackName remains caller-controlled and must not be treated as sanitized.
func GetResourceName(
	signalAttrs pcommon.Map,
	spanKind ptrace.SpanKind,
	fallbackName string,
) (resourceName string) {
	defer func() {
		if len(resourceName) > maxResourceNameLen {
			resourceName = resourceName[:maxResourceNameLen]
		}
	}()

	if v := GetOTelAttrVal(signalAttrs, keyDDResourceName); v != "" {
		resourceName = v
		return
	}

	if m := GetOTelAttrVal(signalAttrs, keyHTTPRequestMethod, keyHTTPMethod); m != "" {
		if m == httpRequestMethodOther {
			m = "HTTP"
		}
		resourceName = m
		switch spanKind {
		case ptrace.SpanKindServer:
			if route := GetOTelAttrVal(signalAttrs, keyHTTPRoute); route != "" {
				resourceName += " " + route
			}
		case ptrace.SpanKindClient:
			if tmpl := GetOTelAttrVal(signalAttrs, keyURLTemplate); tmpl != "" {
				resourceName += " " + tmpl
			}
		}
		return
	}

	if op := GetOTelAttrVal(signalAttrs, keyMessagingOperationName, keyMessagingOperation); op != "" {
		resourceName = op
		if dest := GetOTelAttrVal(signalAttrs, keyMessagingDestinationName, keyMessagingDestination); dest != "" {
			resourceName += " " + dest
		}
		return
	}

	if m := GetOTelAttrVal(signalAttrs, keyRPCMethod); m != "" {
		resourceName = m
		if svc := GetOTelAttrVal(signalAttrs, keyRPCService); svc != "" {
			resourceName += " " + svc
		}
		return
	}

	if t := GetOTelAttrVal(signalAttrs, keyGraphQLOperationType); t != "" {
		resourceName = t
		if name := GetOTelAttrVal(signalAttrs, keyGraphQLOperationName); name != "" {
			resourceName += " " + name
		}
		return
	}

	if GetOTelAttrVal(signalAttrs, keyDBSystemName, keyDBSystem, keyDBType) != "" {
		if query := GetOTelAttrVal(signalAttrs, keyDBQueryText, keyDBStatement); query != "" {
			resourceName = query
			return
		}
	}

	resourceName = fallbackName
	return
}

// GetServiceName returns service.name from resource attributes, or
// DefaultServiceName when it is absent.
func GetServiceName(resourceAttrs pcommon.Map) string {
	if v := GetOTelAttrVal(resourceAttrs, keyServiceName); v != "" {
		return v
	}
	return DefaultServiceName
}

// GetDeploymentEnvironmentName returns the deployment environment from resource
// attributes, preferring deployment.environment.name over its legacy key.
func GetDeploymentEnvironmentName(resourceAttrs pcommon.Map) string {
	if v := GetOTelAttrVal(resourceAttrs, keyDeploymentEnvironmentName, keyDeploymentEnvironment); v != "" {
		return v
	}
	return DefaultDeploymentEnvironmentName
}

// GetServiceVersion returns service.version from resource attributes, or an empty
// string when it is absent.
func GetServiceVersion(resourceAttrs pcommon.Map) string {
	return GetOTelAttrVal(resourceAttrs, keyServiceVersion)
}
