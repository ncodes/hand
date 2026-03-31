package nanoid

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateDefaultLengthNoPrefix(t *testing.T) {
	id, err := Generate("")
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^[A-Za-z0-9]{21}$`), id)
}

func TestGenerateWithPrefixAndLength(t *testing.T) {
	id, err := Generate("clm_", 10)
	require.NoError(t, err)
	require.Regexp(t, regexp.MustCompile(`^clm_[A-Za-z0-9]{10}$`), id)
}

func TestGenerateInvalidLength(t *testing.T) {
	_, err := Generate("", 0)
	require.EqualError(t, err, "nanoid length must be greater than zero")
}

func TestGenerateTooManyLengthArgs(t *testing.T) {
	_, err := Generate("", 21, 22)
	require.EqualError(t, err, "nanoid length accepts at most one value")
}

func TestGenerateInvalidPrefix(t *testing.T) {
	for _, prefix := range []string{"jti__", "jti", "jti-test_", "jti_foo_", "_", "jti!_"} {
		_, err := Generate(prefix)
		require.EqualError(t, err, "nanoid prefix must be alphanumeric with a single trailing underscore")
	}
}

func TestMustGeneratePanicsOnGeneratorFailure(t *testing.T) {
	original := generate
	t.Cleanup(func() {
		generate = original
	})

	generate = func(string, int) (string, error) {
		return "", errors.New("boom")
	}

	require.Panics(t, func() {
		_ = MustGenerate("clm_")
	})
}

func TestMustGenerateSuccess(t *testing.T) {
	id := MustGenerate("sub_", 8)
	require.Regexp(t, regexp.MustCompile(`^sub_[A-Za-z0-9]{8}$`), id)
}

func TestFromSeedDeterministicOutput(t *testing.T) {
	id, err := FromSeed("act_", "seed-1", "fallback123")
	require.NoError(t, err)
	require.Equal(t, "act_seed1seed1seed1seed1s", id)
}

func TestFromSeedUsesFallback(t *testing.T) {
	id, err := FromSeed("act_", "!!!", "fallback123")
	require.NoError(t, err)
	require.Equal(t, "act_fallback123fallback12", id)

	id2, err := FromSeed("act_", "!!!", "fallback123")
	require.NoError(t, err)
	require.Equal(t, id, id2)
}

func TestFromSeedInvalidFallback(t *testing.T) {
	_, err := FromSeed("act_", "!!!", "___")
	require.EqualError(t, err, "nanoid seed fallback must contain alphanumeric characters")
}

func TestValidateIDValidAndInvalidFormats(t *testing.T) {
	valid := "jti_1234567890ABCDEabcdeF"
	require.NoError(t, ValidateID(valid))
	require.True(t, IsValidID(valid))

	for _, value := range []string{"", "jti", "jti__1234567890ABCDEabcdeF", "jti-1234567890ABCDEabcdeF", "jti_123", "jti_1234567890ABCDEabcde_"} {
		require.Error(t, ValidateID(value))
		require.False(t, IsValidID(value))
	}
}
