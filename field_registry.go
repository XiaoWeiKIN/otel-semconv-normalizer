// SPDX-License-Identifier: Apache-2.0

package otlpmap

import (
	"github.com/XiaoWeiKIN/otel-semconv-normalizer/internal/semconvmeta"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func getCanonicalStringField(signalAttrs pcommon.Map, id semconvmeta.FieldID) string {
	field := semconvmeta.Field(id)
	for i, count := uint8(0), field.SourceKeyCount(); i < count; i++ {
		if value, ok := signalAttrs.Get(field.SourceKey(i)); ok {
			if stringValue := value.AsString(); stringValue != "" {
				return stringValue
			}
		}
	}
	return ""
}

func getCanonicalInt64Field(signalAttrs pcommon.Map, id semconvmeta.FieldID) (int64, bool) {
	field := semconvmeta.Field(id)
	for i, count := uint8(0), field.SourceKeyCount(); i < count; i++ {
		value, ok := signalAttrs.Get(field.SourceKey(i))
		if ok && value.Type() == pcommon.ValueTypeInt {
			return value.Int(), true
		}
	}
	return 0, false
}
