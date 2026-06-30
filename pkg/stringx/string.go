// Package stringx provides small string wrapper types for common normalization
// operations used across Morph.
package stringx

import (
	"strings"
	"unicode"
)

// String wraps a string so callers can perform common cleanup through methods
// instead of repeating string utility function chains at call sites.
type String string

// Normalized is a string that has been trimmed and lowercased.
type Normalized string

// Trim removes leading and trailing Unicode whitespace.
func (value String) Trim() string {
	return strings.TrimFunc(string(value), unicode.IsSpace)
}

// Normalized returns the string after trimming leading and trailing Unicode
// whitespace and converting it to lower case.
func (value String) Normalized() string {
	return strings.ToLower(value.Trim())
}

// NormalizedValue returns the normalized string as a Normalized value.
func (value String) NormalizedValue() Normalized {
	return Normalized(value.Normalized())
}

// EqualNormalized reports whether value and other are equal after both are
// trimmed and lowercased.
func (value String) EqualNormalized(other string) bool {
	return value.NormalizedValue().Equal(String(other).NormalizedValue())
}

// NewNormalized returns value after trimming and lowercasing it.
func NewNormalized(value string) Normalized {
	return String(value).NormalizedValue()
}

// String returns the underlying normalized string.
func (value Normalized) String() string {
	return string(value)
}

// EqualString reports whether value equals other after normalizing other.
func (value Normalized) EqualString(other string) bool {
	return value == NewNormalized(other)
}

// Equal reports whether value and other are equal normalized strings.
func (value Normalized) Equal(other Normalized) bool {
	return value == other
}
