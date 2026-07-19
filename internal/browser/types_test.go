package browser

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestError_PreservesStableCodeAndCause(t *testing.T) {
	cause := errors.New("browser failed")
	err := &Error{Code: ErrorStartFailed, Operation: ActionStart, Retryable: true, Err: cause}
	require.EqualError(t, err, "browser failed")
	require.ErrorIs(t, err, cause)
	value, ok := GetError(err)
	require.True(t, ok)
	require.Equal(t, ErrorStartFailed, value.Code)
	require.Empty(t, (*Error)(nil).Error())
	require.NoError(t, (*Error)(nil).Unwrap())
}
