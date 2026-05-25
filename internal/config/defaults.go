package config

import (
	"maps"
	"slices"

	"github.com/wandxy/hand/internal/constants"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
)

var DefaultConfig = Config{
	Models: ModelsConfig{
		Main: MainModelConfig{
			Name:          constants.DefaultProfileModel,
			Provider:      constants.ModelProviderOpenRouter,
			Stream:        new(constants.DefaultProfileModelStream),
			ContextLength: constants.DefaultContextLength,
			API:           modelprovider.APIOpenAIResponses,
			BaseURL:       constants.DefaultOpenRouterResponsesBaseURL,
		},
		Summary: SummaryModelConfig{
			Name: constants.DefaultProfileSummaryModel,
		},
		Embedding: EmbeddingModelConfig{
			Name: constants.DefaultProfileEmbeddingModel,
		},
		Verify:     new(constants.DefaultProfileModelVerify),
		MaxRetries: new(constants.DefaultModelMaxRetries),
	},
	Session: SessionConfig{
		MaxIterations:     constants.DefaultMaxIterations,
		DefaultIdleExpiry: constants.DefaultSessionIdleExpiry,
		ArchiveRetention:  constants.DefaultArchiveRetention,
	},
	RPC: RPCConfig{
		Address: constants.DefaultRPCAddress,
		Port:    constants.DefaultRPCPort,
	},
	FS: FSConfig{
		NoProfileAccess: true,
	},
	Log: LogConfig{
		Level: constants.DefaultProfileLogLevel,
	},
	Debug: DebugConfig{
		Requests: constants.DefaultProfileDebugRequests,
	},
	Trace: TraceConfig{
		Enabled: constants.DefaultProfileTraceEnabled,
		Disk: TraceDiskConfig{
			Enabled: new(constants.DefaultProfileTraceDiskEnabled),
		},
		Database: TraceDatabaseConfig{
			Enabled:             new(constants.DefaultProfileTraceDatabaseEnabled),
			MaxEventsPerSession: constants.DefaultTraceMaxEventsPerSession,
		},
	},
	TUI: TUIConfig{
		ThinkingComposer: new(constants.DefaultTUIThinkingComposerEnabled),
	},
	Safety: SafetyConfig{
		Input:  new(constants.DefaultSafetyInputEnabled),
		Output: new(constants.DefaultSafetyOutputEnabled),
		PII:    new(constants.DefaultSafetyPIIEnabled),
	},
	Web: WebConfig{
		Provider:                     constants.DefaultProfileWebProvider,
		MaxCharPerResult:             constants.DefaultWebMaxCharPerResult,
		MaxExtractCharPerResult:      constants.DefaultWebMaxExtractCharPerResult,
		MaxExtractResponseBytes:      constants.DefaultWebMaxExtractResponseBytes,
		CacheTTL:                     constants.DefaultProfileWebCacheTTL,
		BlockedDomainsEnabled:        constants.DefaultProfileWebBlockedDomainsEnabled,
		ExtractMinSummarizeChars:     constants.DefaultWebExtractMinSummarizeChars,
		ExtractMaxSummaryChars:       constants.DefaultWebExtractMaxSummaryChars,
		ExtractMaxSummaryChunkChars:  constants.DefaultWebExtractMaxSummaryChunkChars,
		ExtractRefusalThresholdChars: constants.DefaultWebExtractRefusalThresholdChars,
	},
	Platform: constants.DefaultPlatform,
	Cap: CapConfig{
		Filesystem: new(constants.DefaultProfileCapabilityFilesystem),
		Network:    new(constants.DefaultProfileCapabilityNetwork),
		Exec:       new(constants.DefaultProfileCapabilityExec),
		Memory:     new(constants.DefaultProfileCapabilityMemory),
		Browser:    new(constants.DefaultProfileCapabilityBrowser),
	},
	Storage: StorageConfig{
		Backend: constants.DefaultStorageBackend,
	},
	Search: SearchConfig{
		EnableRerank: new(constants.DefaultProfileSearchEnableRerank),
		Vector: SearchVectorConfig{
			Enabled:          constants.DefaultProfileSearchVectorEnabled,
			Required:         constants.DefaultProfileSearchVectorRequired,
			RebuildBatchSize: constants.DefaultVectorStoreRebuildBatchSize,
		},
	},
	Reranker: RerankerConfig{
		Enabled:               new(constants.DefaultProfileRerankerEnabled),
		Type:                  constants.RerankerDeterministic,
		MaxCandidates:         constants.DefaultProfileRerankerMaxCandidates,
		MaxCandidateTextChars: constants.DefaultProfileRerankerMaxCandidateTextChars,
		MaxOutputTokens:       constants.DefaultProfileRerankerMaxOutputTokens,
		Overrides: map[string]RerankerOverrideConfig{
			"memory_episodic_extraction": {Type: constants.RerankerLLM},
			"memory_promotion":           {Type: constants.RerankerLLM},
			"memory_reflection":          {Type: constants.RerankerLLM},
		},
	},
	Compaction: CompactionConfig{
		Enabled:           new(constants.DefaultProfileCompactionEnabled),
		TriggerPercent:    constants.DefaultCompactionTrigger,
		WarnPercent:       constants.DefaultCompactionWarn,
		RecentSessionTail: new(constants.RecentSessionTail),
	},
	Memory: MemoryConfig{
		Enabled:  new(constants.DefaultProfileMemoryEnabled),
		Provider: constants.MemoryProviderDefault,
		Pinned: PinnedMemoryConfig{
			Enabled:      new(constants.DefaultProfileMemoryPinnedEnabled),
			MaxChars:     constants.DefaultProfileMemoryPinnedMaxChars,
			MaxItemChars: constants.DefaultProfileMemoryPinnedMaxItemChars,
		},
		Retrieval: RetrievalMemoryConfig{
			Enabled: new(constants.DefaultProfileMemoryRetrievalEnabled),
		},
		Flush: FlushMemoryConfig{
			Enabled:         new(constants.DefaultProfileMemoryFlushEnabled),
			MaxCalls:        constants.DefaultProfileMemoryFlushMaxCalls,
			MaxOutputTokens: constants.DefaultProfileMemoryFlushMaxOutputTokens,
			Timeout:         constants.DefaultProfileMemoryFlushTimeout,
		},
		Episodic: EpisodicMemoryConfig{
			Enabled:         new(constants.DefaultProfileMemoryEpisodicEnabled),
			Interval:        constants.DefaultProfileMemoryEpisodicInterval,
			IdleAfter:       constants.DefaultProfileMemoryEpisodicIdleAfter,
			MinMessages:     constants.DefaultProfileMemoryEpisodicMinMessages,
			WindowSize:      constants.DefaultProfileMemoryEpisodicWindowSize,
			MaxWindows:      constants.DefaultProfileMemoryEpisodicMaxWindows,
			MaxWindowChars:  constants.DefaultProfileMemoryEpisodicMaxWindowChars,
			MaxWindowTokens: constants.DefaultProfileMemoryEpisodicMaxWindowTokens,
			MaxRetries:      constants.DefaultProfileMemoryEpisodicMaxRetries,
		},
		Reflection: ReflectionMemoryConfig{
			Enabled:      new(constants.DefaultProfileMemoryReflectionEnabled),
			Interval:     constants.DefaultProfileMemoryReflectionInterval,
			Limit:        constants.DefaultProfileMemoryReflectionLimit,
			RelatedLimit: constants.DefaultProfileMemoryReflectionRelatedLimit,
		},
		Promotion: PromotionMemoryConfig{
			Enabled:  new(constants.DefaultProfileMemoryPromotionEnabled),
			Interval: constants.DefaultProfileMemoryPromotionInterval,
			Limit:    constants.DefaultProfileMemoryPromotionLimit,
		},
		Write: WriteMemoryConfig{
			Enabled: new(constants.DefaultProfileMemoryWriteEnabled),
		},
	},
}

// NewDefaultConfig returns an independent default config instance.
func NewDefaultConfig() *Config {
	cfg := cloneConfig(DefaultConfig)
	cfg.FS.Roots = getDefaultFSRoots()

	return &cfg
}

func cloneConfig(cfg Config) Config {
	cfg.Models.Verify = cloneBoolPtr(cfg.Models.Verify)
	cfg.Models.MaxRetries = cloneIntPtr(cfg.Models.MaxRetries)
	cfg.Models.Providers = cloneProviderModelConfigs(cfg.Models.Providers)
	cfg.Models.Main.Stream = cloneBoolPtr(cfg.Models.Main.Stream)
	cfg.Search.EnableRerank = cloneBoolPtr(cfg.Search.EnableRerank)
	cfg.Memory.Enabled = cloneBoolPtr(cfg.Memory.Enabled)
	cfg.Memory.Pinned.Enabled = cloneBoolPtr(cfg.Memory.Pinned.Enabled)
	cfg.Memory.Retrieval.Enabled = cloneBoolPtr(cfg.Memory.Retrieval.Enabled)
	cfg.Memory.Flush.Enabled = cloneBoolPtr(cfg.Memory.Flush.Enabled)
	cfg.Memory.Episodic.Enabled = cloneBoolPtr(cfg.Memory.Episodic.Enabled)
	cfg.Memory.Reflection.Enabled = cloneBoolPtr(cfg.Memory.Reflection.Enabled)
	cfg.Memory.Promotion.Enabled = cloneBoolPtr(cfg.Memory.Promotion.Enabled)
	cfg.Memory.Write.Enabled = cloneBoolPtr(cfg.Memory.Write.Enabled)
	cfg.Reranker.Enabled = cloneBoolPtr(cfg.Reranker.Enabled)
	cfg.Reranker.Overrides = cloneRerankerOverrides(cfg.Reranker.Overrides)
	cfg.Compaction.Enabled = cloneBoolPtr(cfg.Compaction.Enabled)
	cfg.Compaction.RecentSessionTail = cloneIntPtr(cfg.Compaction.RecentSessionTail)
	cfg.Cap.Filesystem = cloneBoolPtr(cfg.Cap.Filesystem)
	cfg.Cap.Network = cloneBoolPtr(cfg.Cap.Network)
	cfg.Cap.Exec = cloneBoolPtr(cfg.Cap.Exec)
	cfg.Cap.Memory = cloneBoolPtr(cfg.Cap.Memory)
	cfg.Cap.Browser = cloneBoolPtr(cfg.Cap.Browser)
	cfg.Trace.Disk.Enabled = cloneBoolPtr(cfg.Trace.Disk.Enabled)
	cfg.Trace.Database.Enabled = cloneBoolPtr(cfg.Trace.Database.Enabled)
	cfg.TUI.ThinkingComposer = cloneBoolPtr(cfg.TUI.ThinkingComposer)
	cfg.Safety.Input = cloneBoolPtr(cfg.Safety.Input)
	cfg.Safety.Output = cloneBoolPtr(cfg.Safety.Output)
	cfg.Safety.PII = cloneBoolPtr(cfg.Safety.PII)
	cfg.FS.Roots = slices.Clone(cfg.FS.Roots)
	cfg.Exec.Allow = slices.Clone(cfg.Exec.Allow)
	cfg.Exec.Ask = slices.Clone(cfg.Exec.Ask)
	cfg.Exec.Deny = slices.Clone(cfg.Exec.Deny)
	cfg.Web.BlockedDomains = slices.Clone(cfg.Web.BlockedDomains)
	cfg.Web.BlockedDomainFiles = slices.Clone(cfg.Web.BlockedDomainFiles)
	cfg.Web.NativeAllowedHosts = slices.Clone(cfg.Web.NativeAllowedHosts)
	cfg.Web.NativeBlockedHosts = slices.Clone(cfg.Web.NativeBlockedHosts)
	cfg.Web.NativeAllowedHostFiles = slices.Clone(cfg.Web.NativeAllowedHostFiles)
	cfg.Web.NativeBlockedHostFiles = slices.Clone(cfg.Web.NativeBlockedHostFiles)
	cfg.Rules.Files = slices.Clone(cfg.Rules.Files)
	cfg.Personalities = clonePersonalityConfigs(cfg.Personalities)

	return cfg
}

func cloneRerankerOverrides(overrides map[string]RerankerOverrideConfig) map[string]RerankerOverrideConfig {
	if len(overrides) == 0 {
		return nil
	}

	cloned := make(map[string]RerankerOverrideConfig, len(overrides))
	maps.Copy(cloned, overrides)
	for useCase, override := range cloned {
		override.MaxCandidates = cloneIntPtr(override.MaxCandidates)
		override.MaxCandidateTextChars = cloneIntPtr(override.MaxCandidateTextChars)
		override.MaxOutputTokens = cloneIntPtr(override.MaxOutputTokens)
		cloned[useCase] = override
	}

	return cloned
}

func cloneProviderModelConfigs(values map[string]ProviderModelConfig) map[string]ProviderModelConfig {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]ProviderModelConfig, len(values))
	for provider, value := range values {
		value.APIKeyEnv = slices.Clone(value.APIKeyEnv)
		cloned[provider] = value
	}

	return cloned
}

func clonePersonalityConfigs(values map[string]PersonalityConfig) map[string]PersonalityConfig {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]PersonalityConfig, len(values))
	for name, personality := range values {
		personality.Memory.Pinned = cloneBoolPtr(personality.Memory.Pinned)
		personality.Memory.Retrieval = cloneBoolPtr(personality.Memory.Retrieval)
		personality.Memory.Write = cloneBoolPtr(personality.Memory.Write)
		personality.Memory.Episodic = cloneBoolPtr(personality.Memory.Episodic)
		personality.Memory.Reflection = cloneBoolPtr(personality.Memory.Reflection)
		personality.Memory.Promotion = cloneBoolPtr(personality.Memory.Promotion)
		personality.Memory.Flush = cloneBoolPtr(personality.Memory.Flush)
		personality.Tools.Filesystem = cloneBoolPtr(personality.Tools.Filesystem)
		personality.Tools.Network = cloneBoolPtr(personality.Tools.Network)
		personality.Tools.Exec = cloneBoolPtr(personality.Tools.Exec)
		personality.Model.Stream = cloneBoolPtr(personality.Model.Stream)
		cloned[name] = personality
	}

	return cloned
}

func cloneBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}

	return new(*value)
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}

	return new(*value)
}
