package logutils

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrettyJSON_FormatsVariousInputTypes(t *testing.T) {
	require.Contains(t, PrettyJSON([]byte(`{"a":1}`)), "\"a\": 1")
	require.Contains(t, PrettyJSON(`{"b":2}`), "\"b\": 2")
	require.Contains(t, PrettyJSON(map[string]any{"c": 3}), "\"c\": 3")
	require.Equal(t, "not-json", PrettyJSON("not-json"))
	require.Contains(t, PrettyJSON(make(chan int)), "marshal error")
}

func TestPrettyPrint_WritesToStdout(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = old
	})

	PrettyPrint(map[string]any{"x": 1})
	require.NoError(t, w.Close())

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	require.Contains(t, buf.String(), `"x": int(1)`)
}
