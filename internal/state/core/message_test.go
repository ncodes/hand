package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionIDAndMessageOrderHelpers(t *testing.T) {
	sessionID, err := NewSessionID()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(sessionID, SessionIDPrefix))

	archiveID, err := NewArchiveID()
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(archiveID, ArchiveIDPrefix))

	order, err := NormalizeMessageQueryOrder("")
	require.NoError(t, err)
	require.Equal(t, MessageOrderAsc, order)

	order, err = NormalizeMessageQueryOrder(" DESC ")
	require.NoError(t, err)
	require.Equal(t, MessageOrderDesc, order)

	_, err = NormalizeMessageQueryOrder("sideways")
	require.EqualError(t, err, "message order must be asc or desc")
}

func TestStringHelpers(t *testing.T) {
	require.Equal(t, []string{"one", "two"}, UniqueStrings([]string{" one ", "", "two", "one"}))
	require.Nil(t, UniqueStrings(nil))
	require.Equal(t, "search files", NormalizeMatchValue(" Search   Files "))
}
