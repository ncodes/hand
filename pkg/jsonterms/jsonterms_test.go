package jsonterms

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTerms_NormalizesJSON(t *testing.T) {
	value := Terms(`{"process":{"id":"proc_1","status":"running"}}`)

	require.Contains(t, value, "process.id proc_1")
	require.Contains(t, value, "proc_1")
	require.Contains(t, value, "process.status running")
	require.Contains(t, value, "running")
}

func TestTerms_LeavesMalformedTextEmpty(t *testing.T) {
	require.Empty(t, Terms("{bad json"))
}

func TestTerms_IsDeterministic(t *testing.T) {
	first := Terms(`{"pattern":"hello","path":"internal"}`)
	second := Terms(`{"pattern":"hello","path":"internal"}`)

	require.Equal(t, first, second)
}

func TestTerms_HandlesNestedJSONStringValues(t *testing.T) {
	value := Terms(`{"input":"{\"action\":\"start\",\"command\":\"python3\"}"}`)
	require.Contains(t, value, "input.action start")
	require.Contains(t, value, "start")
	require.Contains(t, value, "input.command python3")
	require.Contains(t, value, "python3")
}

func TestTerms_WithPrefix_PrefixesStructuredTerms(t *testing.T) {
	value := Terms(`{"action":"start","command":"python3"}`, "input")

	require.Contains(t, value, "input.action start")
	require.Contains(t, value, "start")
	require.Contains(t, value, "input.command python3")
	require.Contains(t, value, "python3")
}

func TestTerms_HandlesArraysBooleansNumbersAndPlainStrings(t *testing.T) {
	value := Terms(`{
		"items":[true,12,"hello"],
		"text":"plain text",
		"nested_text":"not-json"
	}`)

	require.Contains(t, value, "items true")
	require.Contains(t, value, "items 12")
	require.Contains(t, value, "items hello")
	require.Contains(t, value, "true")
	require.Contains(t, value, "12")
	require.Contains(t, value, "hello")
	require.Contains(t, value, "text plain text")
	require.Contains(t, value, "plain text")
	require.Contains(t, value, "nested_text not-json")
	require.Contains(t, value, "not-json")
}

func TestTerms_LeavesEmptyInputsEmpty(t *testing.T) {
	require.Empty(t, Terms(""))
	require.Empty(t, Terms("   "))
}

func TestTerms_DedupesRepeatedTerms(t *testing.T) {
	value := Terms(`{"first":"hello","second":"hello"}`)

	require.Equal(t, "first hello\nhello\nsecond hello", value)
}

func TestNormalizeScalar_HandlesEmptyAndWhitespace(t *testing.T) {
	require.Empty(t, normalizeScalar(""))
	require.Empty(t, normalizeScalar("   "))
	require.Equal(t, "hello world", normalizeScalar("  Hello   World  "))
}

func TestLooksLikeJSON_HandlesEmptyAndPlainText(t *testing.T) {
	require.False(t, looksLikeJSON(""))
	require.False(t, looksLikeJSON("plain"))
	require.True(t, looksLikeJSON("{\"k\":1}"))
	require.True(t, looksLikeJSON("[1,2]"))
	require.True(t, looksLikeJSON(`"hello"`))
}

func TestTermBuilderAdd_HandlesEmptyAndDuplicateParts(t *testing.T) {
	builder := newTermBuilder()

	builder.add("", "   ")
	require.Empty(t, builder.String())

	builder.add(" Hello ", " world ")
	builder.add("hello", "world")

	require.Equal(t, "hello world", builder.String())
}

func TestAddValueTerms_HandlesNilAndDefaultTypes(t *testing.T) {
	builder := newTermBuilder()

	addValueTerms(builder, "prefix", nil)
	addValueTerms(builder, "prefix", int64(7))

	require.Equal(t, "prefix 7\n7", builder.String())
}

func TestAddValueTerms_HandlesFloat64(t *testing.T) {
	builder := newTermBuilder()

	addValueTerms(builder, "prefix", float64(12))

	require.Equal(t, "prefix 12\n12", builder.String())
}

func TestAddValueTerms_FallsBackWhenJSONStringParsingFails(t *testing.T) {
	builder := newTermBuilder()

	addValueTerms(builder, "prefix", "{bad json")

	require.Equal(t, "prefix {bad json\n{bad json", builder.String())
}

func TestAddValueTerms_IgnoresEmptyStrings(t *testing.T) {
	builder := newTermBuilder()

	addValueTerms(builder, "prefix", "   ")

	require.Empty(t, builder.String())
}
