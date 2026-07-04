package config

import (
	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/internal/datadir"
	"github.com/wandxy/morph/pkg/stringx"
)

// normalizeFields applies trimming and defaults except default model base URL resolution.
func (c *Config) normalizeFields() {
	if c == nil {
		return
	}

	c.Name = stringx.String(c.Name).Trim()
	if c.Name == "" {
		c.Name = constants.DefaultName
	}
	c.Models.Main.Name = stringx.String(c.Models.Main.Name).Trim()
	c.Models.Summary.Name = stringx.String(c.Models.Summary.Name).Trim()
	c.Models.Main.Provider = stringx.String(c.Models.Main.Provider).Normalized()
	c.Models.Embedding.Provider = stringx.String(c.Models.Embedding.Provider).Normalized()
	c.Models.Embedding.Name = stringx.String(c.Models.Embedding.Name).Trim()
	c.Models.Embedding.API = stringx.String(c.Models.Embedding.API).Normalized()
	c.Models.Providers = normalizeProviderModelConfigs(c.Models.Providers)
	c.Models.Main.APIKey = stringx.String(c.Models.Main.APIKey).Trim()
	c.Models.Main.BaseURL = stringx.String(c.Models.Main.BaseURL).Trim()
	c.Models.Summary.Provider = stringx.String(c.Models.Summary.Provider).Normalized()
	c.Models.Summary.APIKey = stringx.String(c.Models.Summary.APIKey).Trim()
	c.Models.Summary.BaseURL = stringx.String(c.Models.Summary.BaseURL).Trim()
	c.Models.Embedding.APIKey = stringx.String(c.Models.Embedding.APIKey).Trim()
	c.Models.Main.API = stringx.String(c.Models.Main.API).Normalized()
	c.Models.Summary.API = stringx.String(c.Models.Summary.API).Normalized()
	c.Log.Level = stringx.String(c.Log.Level).Normalized()
	c.Trace.Disk.Dir = stringx.String(c.Trace.Disk.Dir).Trim()
	c.Web.Provider = stringx.String(c.Web.Provider).Normalized()
	c.Web.APIKey = stringx.String(c.Web.APIKey).Trim()
	c.Web.BaseURL = stringx.String(c.Web.BaseURL).Trim()
	c.Web.BlockedDomains = dedupeAndTrim(c.Web.BlockedDomains)
	c.Web.BlockedDomainFiles = dedupeAndTrim(c.Web.BlockedDomainFiles)
	c.Web.NativeAllowedHosts = dedupeAndTrim(c.Web.NativeAllowedHosts)
	c.Web.NativeBlockedHosts = dedupeAndTrim(c.Web.NativeBlockedHosts)
	c.Web.NativeAllowedHostFiles = dedupeAndTrim(c.Web.NativeAllowedHostFiles)
	c.Web.NativeBlockedHostFiles = dedupeAndTrim(c.Web.NativeBlockedHostFiles)
	c.Gateway.Address = stringx.String(c.Gateway.Address).Trim()
	c.Gateway.AuthToken = stringx.String(c.Gateway.AuthToken).Trim()
	c.Gateway.PairingSecret = stringx.String(c.Gateway.PairingSecret).Trim()
	c.Gateway.AllowedUsers = dedupeAndTrim(c.Gateway.AllowedUsers)
	c.Gateway.Telegram.Mode = stringx.String(c.Gateway.Telegram.Mode).Normalized()
	c.Gateway.Telegram.BotToken = stringx.String(c.Gateway.Telegram.BotToken).Trim()
	c.Gateway.Telegram.WebhookSecret = stringx.String(c.Gateway.Telegram.WebhookSecret).Trim()
	c.Gateway.Telegram.AllowedUsers = dedupeAndTrim(c.Gateway.Telegram.AllowedUsers)
	c.Gateway.Slack.Mode = stringx.String(c.Gateway.Slack.Mode).Normalized()
	c.Gateway.Slack.ResponseMode = stringx.String(c.Gateway.Slack.ResponseMode).Normalized()
	c.Gateway.Slack.BotToken = stringx.String(c.Gateway.Slack.BotToken).Trim()
	c.Gateway.Slack.AppToken = stringx.String(c.Gateway.Slack.AppToken).Trim()
	c.Gateway.Slack.SigningSecret = stringx.String(c.Gateway.Slack.SigningSecret).Trim()
	c.Gateway.Slack.AllowedUsers = dedupeAndTrim(c.Gateway.Slack.AllowedUsers)
	c.Rules.Files = normalizeRulePaths(c.Rules.Files)
	c.Session.Instruct = stringx.String(c.Session.Instruct).Trim()
	c.Platform = stringx.String(c.Platform).Normalized()
	c.FS.Roots = normalizeFSRoots(c.FS.Roots)
	c.Exec.Allow = dedupeAndTrim(c.Exec.Allow)
	c.Exec.Ask = dedupeAndTrim(c.Exec.Ask)
	c.Exec.Deny = dedupeAndTrim(c.Exec.Deny)
	c.Storage.Backend = stringx.String(c.Storage.Backend).Normalized()
	c.Memory.Provider = stringx.String(c.Memory.Provider).Normalized()
	c.Memory.Backend = stringx.String(c.Memory.Backend).Normalized()
	c.Reranker.Type = stringx.String(c.Reranker.Type).Normalized()
	c.Reranker.Model = stringx.String(c.Reranker.Model).Trim()
	c.Reranker.Overrides = normalizeRerankerOverrides(c.Reranker.Overrides)
	c.normalizePersonalities()

	if c.Models.Main.Stream == nil {
		c.Models.Main.Stream = new(constants.DefaultProfileModelStream)
	}
	if c.Models.MaxRetries == nil {
		c.Models.MaxRetries = new(constants.DefaultModelMaxRetries)
	}
	if c.Models.Main.ContextLength <= 0 {
		c.Models.Main.ContextLength = constants.DefaultContextLength
	}

	if c.Models.Main.API == "" {
		c.Models.Main.API = c.getProviderAPIConfig(c.Models.Main.Provider)
	}
	if c.Models.Main.API == "" {
		c.Models.Main.API = getDefaultAPIForProvider(c.Models.Main.Provider)
	}

	if c.Log.Level == "" {
		c.Log.Level = constants.DefaultLogLevel
	}
	if c.Log.MaxSizeMB == 0 {
		c.Log.MaxSizeMB = constants.DefaultLogMaxSizeMB
	}
	if c.Log.MaxBackups == 0 {
		c.Log.MaxBackups = constants.DefaultLogMaxBackups
	}
	if c.Log.MaxAgeDays == 0 {
		c.Log.MaxAgeDays = constants.DefaultLogMaxAgeDays
	}
	if c.RPC.Address == "" {
		c.RPC.Address = constants.DefaultRPCAddress
	}

	if c.RPC.Port == 0 {
		c.RPC.Port = constants.DefaultRPCPort
	}
	if c.Gateway.Address == "" {
		c.Gateway.Address = constants.DefaultRPCAddress
	}
	if c.Gateway.Port == 0 {
		c.Gateway.Port = constants.DefaultGatewayPort
	}
	if c.Gateway.Telegram.Mode == "" {
		c.Gateway.Telegram.Mode = GatewayTelegramModePolling
	}
	if c.Gateway.Slack.Mode == "" {
		c.Gateway.Slack.Mode = GatewaySlackModeSocket
	}
	if c.Gateway.Slack.ResponseMode == "" {
		c.Gateway.Slack.ResponseMode = GatewaySlackResponseModeThread
	}
	if c.Session.MaxIterations == 0 {
		c.Session.MaxIterations = constants.DefaultMaxIterations
	}
	if c.Trace.Disk.Dir == "" {
		c.Trace.Disk.Dir = datadir.DebugTraceDir()
	}
	if c.Trace.Disk.Enabled == nil {
		c.Trace.Disk.Enabled = new(constants.DefaultProfileTraceDiskEnabled)
	}
	if c.Trace.Database.Enabled == nil {
		c.Trace.Database.Enabled = new(constants.DefaultProfileTraceDatabaseEnabled)
	}
	if c.Trace.Database.MaxEventsPerSession <= 0 {
		c.Trace.Database.MaxEventsPerSession = constants.DefaultTraceMaxEventsPerSession
	}
	if c.TUI.ThinkingComposer == nil {
		c.TUI.ThinkingComposer = new(constants.DefaultTUIThinkingComposerEnabled)
	}
	if c.Safety.Input == nil {
		c.Safety.Input = new(constants.DefaultSafetyInputEnabled)
	}
	if c.Safety.Output == nil {
		c.Safety.Output = new(constants.DefaultSafetyOutputEnabled)
	}
	if c.Safety.PII == nil {
		c.Safety.PII = new(constants.DefaultSafetyPIIEnabled)
	}
	if c.Platform == "" {
		c.Platform = constants.DefaultPlatform
	}
	if c.Web.MaxCharPerResult <= 0 {
		c.Web.MaxCharPerResult = constants.DefaultWebMaxCharPerResult
	}
	if c.Web.MaxExtractCharPerResult <= 0 {
		c.Web.MaxExtractCharPerResult = constants.DefaultWebMaxExtractCharPerResult
	}
	if c.Web.MaxExtractResponseBytes <= 0 {
		c.Web.MaxExtractResponseBytes = constants.DefaultWebMaxExtractResponseBytes
	}
	if c.Web.CacheTTL < 0 {
		c.Web.CacheTTL = constants.DefaultWebCacheTTL
	}
	if c.Web.ExtractMinSummarizeChars <= 0 {
		c.Web.ExtractMinSummarizeChars = constants.DefaultWebExtractMinSummarizeChars
	}
	if c.Web.ExtractMaxSummaryChars <= 0 {
		c.Web.ExtractMaxSummaryChars = constants.DefaultWebExtractMaxSummaryChars
	}
	if c.Web.ExtractMaxSummaryChunkChars <= 0 {
		c.Web.ExtractMaxSummaryChunkChars = constants.DefaultWebExtractMaxSummaryChunkChars
	}
	if c.Web.ExtractRefusalThresholdChars <= 0 {
		c.Web.ExtractRefusalThresholdChars = constants.DefaultWebExtractRefusalThresholdChars
	}

	if c.Cap.Filesystem == nil {
		c.Cap.Filesystem = new(constants.DefaultProfileCapabilityFilesystem)
	}
	if c.Cap.Network == nil {
		c.Cap.Network = new(constants.DefaultProfileCapabilityNetwork)
	}
	if c.Cap.Exec == nil {
		c.Cap.Exec = new(constants.DefaultProfileCapabilityExec)
	}
	if c.Cap.Memory == nil {
		c.Cap.Memory = new(constants.DefaultProfileCapabilityMemory)
	}
	if c.Cap.Browser == nil {
		c.Cap.Browser = new(constants.DefaultProfileCapabilityBrowser)
	}

	if len(c.FS.Roots) == 0 {
		c.FS.Roots = getDefaultFSRoots()
	}

	if c.Storage.Backend == "" {
		c.Storage.Backend = constants.DefaultStorageBackend
	}

	if c.Session.DefaultIdleExpiry <= 0 {
		c.Session.DefaultIdleExpiry = constants.DefaultSessionIdleExpiry
	}
	if c.Session.ArchiveRetention <= 0 {
		c.Session.ArchiveRetention = constants.DefaultArchiveRetention
	}
	if c.Compaction.Enabled == nil {
		c.Compaction.Enabled = new(constants.DefaultProfileCompactionEnabled)
	}
	if c.Compaction.TriggerPercent <= 0 {
		c.Compaction.TriggerPercent = constants.DefaultCompactionTrigger
	}
	if c.Compaction.WarnPercent <= 0 {
		c.Compaction.WarnPercent = constants.DefaultCompactionWarn
	}
	if c.Compaction.RecentSessionTail == nil {
		c.Compaction.RecentSessionTail = new(constants.RecentSessionTail)
	}
	if c.Memory.Enabled == nil {
		c.Memory.Enabled = new(constants.DefaultProfileMemoryEnabled)
	}
	if c.Memory.Provider == "" {
		c.Memory.Provider = constants.MemoryProviderDefault
	}
	if c.Memory.Pinned.Enabled == nil {
		c.Memory.Pinned.Enabled = new(constants.DefaultProfileMemoryPinnedEnabled)
	}
	if c.Memory.Retrieval.Enabled == nil {
		c.Memory.Retrieval.Enabled = new(constants.DefaultProfileMemoryRetrievalEnabled)
	}
	if c.Memory.Flush.Enabled == nil {
		c.Memory.Flush.Enabled = new(constants.DefaultProfileMemoryFlushEnabled)
	}
	if c.Memory.Flush.MaxCalls <= 0 {
		c.Memory.Flush.MaxCalls = constants.DefaultProfileMemoryFlushMaxCalls
	}
	if c.Memory.Flush.MaxOutputTokens <= 0 {
		c.Memory.Flush.MaxOutputTokens = constants.DefaultProfileMemoryFlushMaxOutputTokens
	}
	if c.Memory.Flush.Timeout <= 0 {
		c.Memory.Flush.Timeout = constants.DefaultProfileMemoryFlushTimeout
	}
	if c.Memory.Episodic.Enabled == nil {
		c.Memory.Episodic.Enabled = new(constants.DefaultMemoryEpisodicEnabled)
	}
	if c.Memory.Reflection.Enabled == nil {
		c.Memory.Reflection.Enabled = new(constants.DefaultMemoryReflectionEnabled)
	}
	if c.Memory.Promotion.Enabled == nil {
		c.Memory.Promotion.Enabled = new(constants.DefaultProfileMemoryPromotionEnabled)
	}
	if c.Memory.Promotion.EvaluatedRetention == 0 {
		c.Memory.Promotion.EvaluatedRetention = constants.DefaultProfileMemoryPromotionRetention
	}
	if c.Memory.Write.Enabled == nil {
		c.Memory.Write.Enabled = new(constants.DefaultProfileMemoryWriteEnabled)
	}

}

func normalizeProviderModelConfigs(values map[string]ProviderModelConfig) map[string]ProviderModelConfig {
	if len(values) == 0 {
		return nil
	}

	normalized := make(map[string]ProviderModelConfig, len(values))
	for provider, value := range values {
		provider = stringx.String(provider).Normalized()
		if provider == "" {
			continue
		}

		value.APIKey = stringx.String(value.APIKey).Trim()
		value.APIKeyEnv = dedupeAndTrim(value.APIKeyEnv)
		value.API = stringx.String(value.API).Normalized()
		value.BaseURL = stringx.String(value.BaseURL).Trim()
		value.Headers = normalizeStringMap(value.Headers)
		value.Models = normalizeProviderModelMetadata(value.Models)
		normalized[provider] = value
	}
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func normalizeProviderModelMetadata(values map[string]ProviderModelMetadata) map[string]ProviderModelMetadata {
	if len(values) == 0 {
		return nil
	}

	normalized := make(map[string]ProviderModelMetadata, len(values))
	for model, value := range values {
		model = stringx.String(model).Trim()
		if model == "" {
			continue
		}

		normalized[model] = value
	}
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func normalizeRerankerOverrides(overrides map[string]RerankerOverrideConfig) map[string]RerankerOverrideConfig {
	if len(overrides) == 0 {
		return nil
	}

	normalized := make(map[string]RerankerOverrideConfig, len(overrides))
	for key, override := range overrides {
		key = stringx.String(key).Normalized()
		if key == "" {
			continue
		}

		override.Type = stringx.String(override.Type).Normalized()
		override.Model = stringx.String(override.Model).Trim()
		normalized[key] = override
	}
	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func (c *Config) normalizePersonalities() {
	if c == nil || len(c.Personalities) == 0 {
		return
	}

	normalized := make(map[string]PersonalityConfig, len(c.Personalities))
	for name, personality := range c.Personalities {
		name = stringx.String(name).Normalized()
		personality.Soul = stringx.String(personality.Soul).Trim()
		personality.Instruct = stringx.String(personality.Instruct).Trim()
		personality.State = stringx.String(personality.State).Normalized()
		if personality.State == "" {
			personality.State = personalityStateShared
		}
		personality.Tools.Memory = stringx.String(personality.Tools.Memory).Normalized()
		personality.Model.Name = stringx.String(personality.Model.Name).Trim()
		personality.Model.Provider = stringx.String(personality.Model.Provider).Normalized()
		personality.Model.API = stringx.String(personality.Model.API).Normalized()
		personality.Model.BaseURL = stringx.String(personality.Model.BaseURL).Trim()
		normalized[name] = personality
	}
	c.Personalities = normalized
}

func (c *Config) applyDefaultModelBaseURL() {
	if c == nil {
		return
	}

	mapped := getDefaultBaseURLForProvider(c.Models.Main.Provider, c.Models.Main.API)
	configured := c.getProviderBaseURLConfig(c.Models.Main.Provider)
	if configured != "" && (c.Models.Main.BaseURL == "" || isProviderDefaultBaseURL(c.Models.Main.BaseURL)) {
		c.Models.Main.BaseURL = configured
		return
	}
	if mapped == "" {
		return
	}
	if c.Models.Main.BaseURL == "" || isProviderDefaultBaseURL(c.Models.Main.BaseURL) {
		c.Models.Main.BaseURL = mapped
	}
}

func isProviderDefaultBaseURL(value string) bool {
	value = stringx.String(value).Trim()
	if value == "" {
		return false
	}

	for _, providerID := range modelRegistry.GetProviderIDs() {
		providerDef, ok := modelRegistry.GetProvider(providerID)
		if !ok {
			continue
		}
		for _, baseURL := range providerDef.BaseURLs {
			if value == stringx.String(baseURL).Trim() {
				return true
			}
		}
	}

	return false
}

// Normalize trims fields, applies defaults, and resolves default model base URL when unset.
func (c *Config) Normalize() {
	if c == nil {
		return
	}

	c.normalizeFields()
	c.applyDefaultModelBaseURL()
}
