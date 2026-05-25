package client

import (
	"errors"
	"testing"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	modelprovider "github.com/wandxy/hand/internal/model/provider"
	models "github.com/wandxy/hand/pkg/agent/model"
)

func TestClientFactory_ResolveUsesRegistryDefaults(t *testing.T) {
	factory := NewClientFactory(modelprovider.DefaultRegistry())

	resolved, err := factory.Resolve(ClientRequest{
		Role:     ModelRoleMain,
		Model:    "openai/gpt-4o-mini",
		Provider: " openrouter ",
		APIKey:   " key ",
	})

	require.NoError(t, err)
	require.Equal(t, ModelRoleMain, resolved.Role)
	require.True(t, resolved.ModelKnown)
	require.Equal(t, "openai/gpt-4o-mini", resolved.Model.ID)
	require.Equal(t, "openrouter", resolved.Provider.ID)
	require.Equal(t, modelprovider.APIOpenAIResponses, resolved.API.ID)
	require.Equal(t, "key", resolved.APIKey)
	require.Equal(t, "https://openrouter.ai/api/v1/responses", resolved.BaseURL)
}

func TestNewClientFactory_UsesDefaultRegistryWhenNil(t *testing.T) {
	factory := NewClientFactory(nil)

	resolved, err := factory.Resolve(ClientRequest{Provider: "openai"})

	require.NoError(t, err)
	require.Equal(t, "openai", resolved.Provider.ID)
}

func TestNewDefaultClientFactory_UsesBuiltInRegistry(t *testing.T) {
	factory := NewDefaultClientFactory()

	resolved, err := factory.Resolve(ClientRequest{Provider: "openrouter"})

	require.NoError(t, err)
	require.Equal(t, modelprovider.APIOpenAIResponses, resolved.API.ID)
}

func TestClientFactory_ResolveUsesRequestOverrides(t *testing.T) {
	factory := NewClientFactory(modelprovider.DefaultRegistry())

	resolved, err := factory.Resolve(ClientRequest{
		Role:       ModelRoleSummary,
		Model:      "custom-model",
		Provider:   "openai",
		API:        modelprovider.APIOpenAICompletions,
		APIKey:     "key",
		BaseURL:    " https://proxy.example/v1 ",
		Headers:    map[string]string{" x-request ": " custom "},
		MaxRetries: 7,
	})

	require.NoError(t, err)
	require.Equal(t, ModelRoleSummary, resolved.Role)
	require.False(t, resolved.ModelKnown)
	require.Equal(t, "custom-model", resolved.Model.ID)
	require.Equal(t, "openai", resolved.Provider.ID)
	require.Equal(t, modelprovider.APIOpenAICompletions, resolved.API.ID)
	require.Equal(t, "https://proxy.example/v1", resolved.BaseURL)
	require.Equal(t, map[string]string{"x-request": "custom"}, resolved.Headers)
	require.Equal(t, 7, resolved.MaxRetries)
}

func TestClientFactory_ResolveMergesProviderAndRequestHeaders(t *testing.T) {
	registry := modelprovider.NewRegistry(
		[]modelprovider.APIDefinition{{ID: modelprovider.APIOpenAIResponses}},
		[]modelprovider.ProviderDefinition{{
			ID:         "custom",
			DefaultAPI: modelprovider.APIOpenAIResponses,
			BaseURLs: map[string]string{
				modelprovider.APIOpenAIResponses: "https://custom.example/v1",
			},
			Headers: map[string]string{
				"x-provider": "provider",
				"x-override": "provider",
			},
		}},
		nil,
	)
	factory := NewClientFactory(registry)

	resolved, err := factory.Resolve(ClientRequest{
		Provider: "custom",
		Headers: map[string]string{
			"x-request":  "request",
			"x-override": "request",
			" ":          "ignored",
		},
	})

	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"x-provider": "provider",
		"x-request":  "request",
		"x-override": "request",
	}, resolved.Headers)
}

func TestClientFactory_ResolveRejectsInvalidRequests(t *testing.T) {
	cases := []struct {
		name string
		req  ClientRequest
		err  string
	}{
		{
			name: "missing provider",
			req:  ClientRequest{},
			err:  "model provider is required",
		},
		{
			name: "unknown provider",
			req:  ClientRequest{Provider: "missing"},
			err:  `model provider "missing" is not registered`,
		},
		{
			name: "unknown api",
			req:  ClientRequest{Provider: "openai", API: "missing"},
			err:  `model API "missing" is not registered`,
		},
		{
			name: "missing base url",
			req: ClientRequest{
				Provider: "custom",
				API:      modelprovider.APIOpenAICompletions,
			},
			err: `model base URL is required for provider "custom" API "openai-completions"`,
		},
	}

	registry := modelprovider.NewRegistry(
		[]modelprovider.APIDefinition{{ID: modelprovider.APIOpenAICompletions}},
		[]modelprovider.ProviderDefinition{{ID: "custom", DefaultAPI: modelprovider.APIOpenAICompletions}},
		nil,
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factory := NewClientFactory(modelprovider.DefaultRegistry())
			if tc.name == "missing base url" {
				factory = NewClientFactory(registry)
			}

			_, err := factory.Resolve(tc.req)
			require.EqualError(t, err, tc.err)
		})
	}
}

func TestClientFactory_NewClientBuildsOpenAICompatibleClient(t *testing.T) {
	var capturedKey string
	var capturedOptions int
	factory := NewClientFactory(modelprovider.DefaultRegistry())
	factory.OpenAIClient = func(apiKey string, opts ...option.RequestOption) (models.Client, error) {
		capturedKey = apiKey
		capturedOptions = len(opts)
		return &models.OpenAIClient{}, nil
	}

	client, err := factory.NewClient(ClientRequest{
		Provider:   "openai",
		API:        modelprovider.APIOpenAIResponses,
		APIKey:     "key",
		BaseURL:    "https://api.example/v1",
		Headers:    map[string]string{"x-test": "value"},
		MaxRetries: 3,
	})

	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, "key", capturedKey)
	require.Equal(t, 3, capturedOptions)
}

func TestClientFactory_NewClientReturnsResolveError(t *testing.T) {
	factory := NewClientFactory(modelprovider.DefaultRegistry())

	_, err := factory.NewClient(ClientRequest{})

	require.EqualError(t, err, "model provider is required")
}

func TestClientFactory_NewClientUsesDefaultBuilder(t *testing.T) {
	var factory *ClientFactory

	client, err := factory.NewClient(ClientRequest{
		Provider: "openai",
		API:      modelprovider.APIOpenAIResponses,
		BaseURL:  "https://api.example/v1",
	})

	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestClientFactory_NewClientReusesEquivalentClients(t *testing.T) {
	var builds int
	factory := NewClientFactory(modelprovider.DefaultRegistry())
	factory.OpenAIClient = func(string, ...option.RequestOption) (models.Client, error) {
		builds++
		return &models.OpenAIClient{}, nil
	}

	first, err := factory.NewClient(ClientRequest{
		Provider:   "openai",
		API:        modelprovider.APIOpenAIResponses,
		BaseURL:    "https://api.example/v1",
		Headers:    map[string]string{"x-b": "b", "x-a": "a"},
		MaxRetries: 3,
	})
	require.NoError(t, err)

	second, err := factory.NewClient(ClientRequest{
		Provider:   "OPENAI",
		API:        modelprovider.APIOpenAIResponses,
		BaseURL:    "https://api.example/v1",
		Headers:    map[string]string{"x-a": "a", "x-b": "b"},
		MaxRetries: 3,
	})
	require.NoError(t, err)

	third, err := factory.NewClient(ClientRequest{
		Provider:   "openai",
		API:        modelprovider.APIOpenAIResponses,
		BaseURL:    "https://api.example/v1",
		Headers:    map[string]string{"x-a": "changed", "x-b": "b"},
		MaxRetries: 3,
	})
	require.NoError(t, err)

	require.Same(t, first, second)
	require.NotSame(t, first, third)
	require.Equal(t, 2, builds)
}

func TestClientFactory_NewClientInitializesNilCache(t *testing.T) {
	var builds int
	factory := NewClientFactory(modelprovider.DefaultRegistry())
	factory.clients = nil
	factory.OpenAIClient = func(string, ...option.RequestOption) (models.Client, error) {
		builds++
		return &models.OpenAIClient{}, nil
	}

	client, err := factory.NewClient(ClientRequest{
		Provider: "openai",
		API:      modelprovider.APIOpenAIResponses,
		BaseURL:  "https://api.example/v1",
	})

	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, 1, builds)
	require.NotNil(t, factory.clients)
}

func TestClientFactory_NewClientReturnsBuilderError(t *testing.T) {
	factory := NewClientFactory(modelprovider.DefaultRegistry())
	factory.OpenAIClient = func(string, ...option.RequestOption) (models.Client, error) {
		return nil, errors.New("build failed")
	}

	_, err := factory.NewClient(ClientRequest{
		Provider: "openai",
		API:      modelprovider.APIOpenAIResponses,
		BaseURL:  "https://api.example/v1",
	})

	require.EqualError(t, err, "build failed")
}

func TestClientFactory_NewClientRejectsUnsupportedChatAPI(t *testing.T) {
	factory := NewClientFactory(modelprovider.DefaultRegistry())

	_, err := factory.NewClient(ClientRequest{
		Provider: "openai",
		API:      modelprovider.APIOpenAIEmbeddings,
	})

	require.EqualError(t, err, `model API "openai-embeddings" is not supported for chat clients`)
}

func TestClientFactory_NilFactoryUsesDefaults(t *testing.T) {
	var factory *ClientFactory

	resolved, err := factory.Resolve(ClientRequest{Provider: "openai"})
	require.NoError(t, err)
	require.Equal(t, "openai", resolved.Provider.ID)
	require.Equal(t, modelprovider.APIOpenAIResponses, resolved.API.ID)
}
