package constants

import "time"

const (
	// DefaultModel is the fallback model identifier.
	DefaultModel = "openai/gpt-4o-mini"
	// DefaultModelProvider is the fallback model provider.
	DefaultModelProvider = "openrouter"
	// ModelProviderOpenRouter identifies OpenRouter-backed model access.
	ModelProviderOpenRouter = "openrouter"
	// ModelProviderOpenAI identifies OpenAI-backed model access.
	ModelProviderOpenAI = "openai"
	// DefaultModelAPIMode is the fallback model API mode.
	DefaultModelAPIMode = "completions"
	// DefaultModelMaxRetries is the fallback retry count for model calls.
	DefaultModelMaxRetries = 2
)

const (
	// DefaultMaxIterations is the fallback maximum agent loop iteration count.
	DefaultMaxIterations = 90
	// DefaultRPCAddress is the fallback RPC bind address.
	DefaultRPCAddress = "127.0.0.1"
	// DefaultRPCPort is the fallback RPC bind port.
	DefaultRPCPort = 50051
	// DefaultLogLevel is the fallback application log level.
	DefaultLogLevel = "info"
	// DefaultPlatform is the fallback runtime platform identifier.
	DefaultPlatform = "cli"
	// DefaultStorageBackend is the fallback state storage backend.
	DefaultStorageBackend = "sqlite"
	// DefaultSessionIdleExpiry is the fallback duration before idle sessions expire.
	DefaultSessionIdleExpiry = 24 * time.Hour
	// DefaultArchiveRetention is the fallback duration archived sessions are retained.
	DefaultArchiveRetention = 30 * 24 * time.Hour
)
