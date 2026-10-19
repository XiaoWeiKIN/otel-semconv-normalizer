// SPDX-License-Identifier: Apache-2.0

package otlpmap

import "go.opentelemetry.io/collector/pdata/pcommon"

func testAttrs(values map[string]string) pcommon.Map {
	attrs := pcommon.NewMap()
	for key, value := range values {
		attrs.PutStr(key, value)
	}
	return attrs
}
