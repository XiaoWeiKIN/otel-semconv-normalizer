package semconvweave

import (
	"reflect"
	"testing"
	"unsafe"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

type runtimeABIV1 interface {
	State() SchemaState
	ResourceAttribute(pcommon.Map, string, pcommon.Value) AttributeDecision
	SpanAttribute(pcommon.Map, string, string, pcommon.Value) AttributeDecision
	EventAttribute(pcommon.Map, string, string, string, pcommon.Value) AttributeDecision
	SpanName(string) string
	EventName(string, string) string
}

var (
	_ runtimeABIV1         = WeaveID(0)
	_ func(string) WeaveID = Resolve

	_ func(string) bool = MayTranslateResourceAttribute
	_ func(string) bool = MayTranslateSpanAttribute
	_ func(string) bool = MayTranslateEventAttribute
)

func TestRuntimeABIShapeAndValues(t *testing.T) {
	if RuntimeABIVersion != 1 {
		t.Fatalf("RuntimeABIVersion = %d", RuntimeABIVersion)
	}
	if unsafe.Sizeof(WeaveID(0)) != 2 || unsafe.Sizeof(SchemaState(0)) != 1 ||
		unsafe.Sizeof(AttributeAction(0)) != 1 {
		t.Fatal("ABI scalar width changed")
	}
	if WeaveUnknown != 0 || WeaveMissing != 1 || WeaveCanonical != 2 {
		t.Fatal("WeaveID reserved values changed")
	}
	if SchemaUnknown != 0 || SchemaMissing != 1 || SchemaCanonical != 2 ||
		SchemaSupportedLegacy != 3 {
		t.Fatal("SchemaState values changed")
	}
	if AttributeSuppress != 0 || AttributeKeepValue != 1 || AttributeReplaceString != 2 {
		t.Fatal("AttributeAction values changed")
	}
	typ := reflect.TypeOf(AttributeDecision{})
	want := []struct {
		name string
		typ  reflect.Type
	}{
		{"Key", reflect.TypeOf("")},
		{"StringValue", reflect.TypeOf("")},
		{"Action", reflect.TypeOf(AttributeAction(0))},
	}
	if typ.NumField() != len(want) {
		t.Fatalf("AttributeDecision has %d fields", typ.NumField())
	}
	for index, expected := range want {
		field := typ.Field(index)
		if field.Name != expected.name || field.Type != expected.typ {
			t.Fatalf("field %d = %s %v", index, field.Name, field.Type)
		}
	}
}

func TestRuntimeABIDoesNotExposeProjectDerivedScalars(t *testing.T) {
	typ := reflect.TypeOf(WeaveID(0))
	for _, method := range []string{"OperationName", "ResourceName"} {
		if _, ok := typ.MethodByName(method); ok {
			t.Errorf("WeaveID unexpectedly exposes Project derived scalar %s", method)
		}
	}
}

func TestResolveAndStateWithGeneratedData(t *testing.T) {
	tests := []struct {
		url   string
		id    WeaveID
		state SchemaState
	}{
		{"", WeaveMissing, SchemaMissing},
		{"https://opentelemetry.io/schemas/1.41.0", WeaveCanonical, SchemaCanonical},
		{"https://opentelemetry.io/schemas/1.7.0", 3, SchemaSupportedLegacy},
		{"https://opentelemetry.io/schemas/1.4.0", 4, SchemaSupportedLegacy},
		{"https://opentelemetry.io/schemas/1.8.0", WeaveUnknown, SchemaUnknown},
		{"garbage", WeaveUnknown, SchemaUnknown},
	}
	for _, test := range tests {
		if got := Resolve(test.url); got != test.id {
			t.Errorf("Resolve(%q) = %d, want %d", test.url, got, test.id)
		}
		if got := test.id.State(); got != test.state {
			t.Errorf("State(%d) = %v, want %v", test.id, got, test.state)
		}
	}
	for _, invalid := range []WeaveID{5, 6, 255, 65535} {
		if got := invalid.State(); got != SchemaUnknown {
			t.Errorf("invalid State(%d) = %v", invalid, got)
		}
	}
}

func TestBaselineProjectionIsTotalAndReadOnly(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("target", "canonical")
	attrs.PutInt("count", 42)
	before := attrs.AsRaw()
	source := pcommon.NewValueInt(7)

	for _, id := range []WeaveID{WeaveUnknown, WeaveMissing, WeaveCanonical, 3, 65535} {
		decisions := []AttributeDecision{
			id.ResourceAttribute(attrs, "legacy", source),
			id.SpanAttribute(attrs, "span", "legacy", source),
			id.EventAttribute(attrs, "span", "event", "legacy", source),
		}
		for _, decision := range decisions {
			if decision != (AttributeDecision{Key: "legacy", Action: AttributeKeepValue}) {
				t.Errorf("id %d decision = %+v", id, decision)
			}
		}
		if got := id.SpanName("span"); got != "span" {
			t.Errorf("SpanName(%d) = %q", id, got)
		}
		if got := id.EventName("span", "event"); got != "event" {
			t.Errorf("EventName(%d) = %q", id, got)
		}
	}
	if !reflect.DeepEqual(before, attrs.AsRaw()) {
		t.Fatal("baseline projection mutated input attributes")
	}
	if source.Int() != 7 {
		t.Fatal("baseline projection mutated source value")
	}
}
