package e2e

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_ServesConfiguredResponseAndCapturesRequests(t *testing.T) {
	provider := NewProvider(ProviderResponse{
		StatusCode: http.StatusAccepted,
		Headers:    http.Header{"Content-Type": []string{"application/json"}},
		Body:       `{"ok":true}`,
	})
	t.Cleanup(provider.Close)

	resp, err := http.Post(provider.URL()+"/v1/test", "application/json", strings.NewReader(`{"hello":"world"}`))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	assert.Equal(t, `{"ok":true}`, string(body))

	requests := provider.Requests()
	require.Len(t, requests, 1)
	assert.Equal(t, http.MethodPost, requests[0].Method)
	assert.Equal(t, "/v1/test", requests[0].Path)
	assert.Equal(t, `{"hello":"world"}`, requests[0].Body)
}

func TestProvider_NilPaths(t *testing.T) {
	assert.Empty(t, (*Provider)(nil).URL())
	assert.Nil(t, (*Provider)(nil).Requests())
	(*Provider)(nil).Close()

	provider := &Provider{}
	assert.Empty(t, provider.URL())
	provider.Close()
}

func TestProvider_DefaultStatusCode(t *testing.T) {
	provider := NewProvider(ProviderResponse{Body: `ok`})
	t.Cleanup(provider.Close)

	resp, err := http.Get(provider.URL())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ok", string(body))
}
