// SPDX-License-Identifier: Apache-2.0

package otlpmap

import "go.opentelemetry.io/collector/pdata/pcommon"

// GetOTelAttrVal returns the first non-empty attribute value for keys, in
// order. Values of every OTLP attribute type are converted with AsString.
//
// Put the current semantic-convention name first and legacy names after it.
// A present-but-empty current value does not suppress a non-empty legacy value;
// this preserves compatibility during mixed-version rollouts.
func GetOTelAttrVal(attrs pcommon.Map, keys ...string) string {
	for _, key := range keys {
		if value, ok := attrs.Get(key); ok {
			if stringValue := value.AsString(); stringValue != "" {
				return stringValue
			}
		}
	}
	return ""
}
