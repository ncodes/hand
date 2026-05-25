package constants

import "time"

const (
	// DefaultModel is the fallback model identifier.
	DefaultModel = "openai/gpt-4o-mini"
	// DefaultProfileModel is the starter profile model identifier.
	DefaultProfileModel = "minimax/minimax-m2.7"
	// DefaultProfileSummaryModel is the starter profile summary model identifier.
	DefaultProfileSummaryModel = DefaultModel
	// DefaultProfileEmbeddingModel is the starter profile embedding model identifier.
	DefaultProfileEmbeddingModel = "openai/text-embedding-3-small"
	// DefaultModelProvider is the fallback model provider.
	DefaultModelProvider = "openrouter"
	// ModelProviderOpenRouter identifies OpenRouter-backed model access.
	ModelProviderOpenRouter = "openrouter"
	// ModelProviderOpenAI identifies OpenAI-backed model access.
	ModelProviderOpenAI = "openai"
	// ModelProviderAnthropic identifies Anthropic-backed model access.
	ModelProviderAnthropic = "anthropic"
	// ModelProviderGitHubCopilot identifies GitHub Copilot-backed model access.
	ModelProviderGitHubCopilot = "github-copilot"
	// DefaultOpenRouterBaseURL is the default OpenRouter API base URL.
	DefaultOpenRouterBaseURL           = "https://openrouter.ai/api/v1"
	DefaultOpenRouterResponsesBaseURL  = "https://openrouter.ai/api/v1/responses"
	DefaultOpenRouterEmbeddingsBaseURL = "https://openrouter.ai/api/v1/embeddings"
	DefaultOpenAIBaseURL               = "https://api.openai.com/v1"
	DefaultOpenAIEmbeddingsBaseURL     = "https://api.openai.com/v1/embeddings"
	DefaultAnthropicBaseURL            = "https://api.anthropic.com"
	// DefaultModelMaxRetries is the fallback retry count for model calls.
	DefaultModelMaxRetries    = 2
	DefaultProfileModelStream = true
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
	// DefaultTraceMaxEventsPerSession is the fallback maximum stored trace events per session.
	DefaultTraceMaxEventsPerSession = 10000
	// DefaultPlatform is the fallback runtime platform identifier.
	DefaultPlatform = "cli"
	// DefaultStorageBackend is the fallback state storage backend.
	DefaultStorageBackend                       = "sqlite"
	DefaultProfileLogLevel                      = "debug"
	DefaultProfileDebugRequests                 = true
	DefaultProfileTraceEnabled                  = true
	DefaultProfileTraceDiskEnabled              = true
	DefaultProfileTraceDatabaseEnabled          = true
	DefaultTUIThinkingComposerEnabled           = true
	DefaultSafetyInputEnabled                   = true
	DefaultSafetyOutputEnabled                  = true
	DefaultSafetyPIIEnabled                     = false
	DefaultProfileCapabilityFilesystem          = true
	DefaultProfileCapabilityNetwork             = true
	DefaultProfileCapabilityExec                = true
	DefaultProfileCapabilityMemory              = true
	DefaultProfileCapabilityBrowser             = false
	DefaultProfileSearchEnableRerank            = true
	DefaultProfileSearchVectorEnabled           = true
	DefaultProfileSearchVectorRequired          = true
	DefaultProfileRerankerEnabled               = true
	DefaultProfileRerankerMaxCandidates         = 20
	DefaultProfileRerankerMaxCandidateTextChars = 500
	DefaultProfileRerankerMaxOutputTokens       = 0
	DefaultProfileCompactionEnabled             = true
	// DefaultSessionIdleExpiry is the fallback duration before idle sessions expire.
	DefaultSessionIdleExpiry = 24 * time.Hour
	// DefaultArchiveRetention is the fallback duration archived sessions are retained.
	DefaultArchiveRetention = 30 * 24 * time.Hour
)
