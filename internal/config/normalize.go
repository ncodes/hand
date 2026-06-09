package config

import (
	"strings"

	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/datadir"
)

// normalizeFields applies trimming and defaults except default model base URL resolution.
func (c *Config) normalizeFields() {
	if c == nil {
		return
	}

	c.Name = strings.TrimSpace(c.Name)
	c.Models.Main.Name = strings.TrimSpace(c.Models.Main.Name)
	c.Models.Summary.Name = strings.TrimSpace(c.Models.Summary.Name)
	c.Models.Main.Provider = strings.TrimSpace(strings.ToLower(c.Models.Main.Provider))
	c.Models.Embedding.Provider = strings.TrimSpace(strings.ToLower(c.Models.Embedding.Provider))
	c.Models.Embedding.Name = strings.TrimSpace(c.Models.Embedding.Name)
	c.Models.Embedding.API = strings.TrimSpace(strings.ToLower(c.Models.Embedding.API))
	c.Models.Providers = normalizeProviderModelConfigs(c.Models.Providers)
	c.Models.Main.APIKey = strings.TrimSpace(c.Models.Main.APIKey)
	c.Models.Main.BaseURL = strings.TrimSpace(c.Models.Main.BaseURL)
	c.Models.Summary.Provider = strings.TrimSpace(strings.ToLower(c.Models.Summary.Provider))
	c.Models.Summary.APIKey = strings.TrimSpace(c.Models.Summary.APIKey)
	c.Models.Summary.BaseURL = strings.TrimSpace(c.Models.Summary.BaseURL)
	c.Models.Embedding.APIKey = strings.TrimSpace(c.Models.Embedding.APIKey)
	c.Models.Main.API = strings.TrimSpace(strings.ToLower(c.Models.Main.API))
	c.Models.Summary.API = strings.TrimSpace(strings.ToLower(c.Models.Summary.API))
	c.Log.Level = strings.TrimSpace(strings.ToLower(c.Log.Level))
	c.Trace.Disk.Dir = strings.TrimSpace(c.Trace.Disk.Dir)
	c.Web.Provider = strings.TrimSpace(strings.ToLower(c.Web.Provider))
	c.Web.APIKey = strings.TrimSpace(c.Web.APIKey)
	c.Web.BaseURL = strings.TrimSpace(c.Web.BaseURL)
	c.Web.BlockedDomains = dedupeAndTrim(c.Web.BlockedDomains)
	c.Web.BlockedDomainFiles = dedupeAndTrim(c.Web.BlockedDomainFiles)
	c.Web.NativeAllowedHosts = dedupeAndTrim(c.Web.NativeAllowedHosts)
	c.Web.NativeBlockedHosts = dedupeAndTrim(c.Web.NativeBlockedHosts)
	c.Web.NativeAllowedHostFiles = dedupeAndTrim(c.Web.NativeAllowedHostFiles)
	c.Web.NativeBlockedHostFiles = dedupeAndTrim(c.Web.NativeBlockedHostFiles)
	c.Gateway.Address = strings.TrimSpace(c.Gateway.Address)
	c.Gateway.AuthToken = strings.TrimSpace(c.Gateway.AuthToken)
	c.Gateway.PairingSecret = strings.TrimSpace(c.Gateway.PairingSecret)
	c.Gateway.AllowedUsers = dedupeAndTrim(c.Gateway.AllowedUsers)
	c.Gateway.Telegram.Mode = strings.TrimSpace(strings.ToLower(c.Gateway.Telegram.Mode))
	c.Gateway.Telegram.BotToken = strings.TrimSpace(c.Gateway.Telegram.BotToken)
	c.Gateway.Telegram.WebhookSecret = strings.TrimSpace(c.Gateway.Telegram.WebhookSecret)
	c.Gateway.Telegram.AllowedUsers = dedupeAndTrim(c.Gateway.Telegram.AllowedUsers)
	c.Gateway.Slack.Mode = strings.TrimSpace(strings.ToLower(c.Gateway.Slack.Mode))
	c.Gateway.Slack.BotToken = strings.TrimSpace(c.Gateway.Slack.BotToken)
	c.Gateway.Slack.AppToken = strings.TrimSpace(c.Gateway.Slack.AppToken)
	c.Gateway.Slack.SigningSecret = strings.TrimSpace(c.Gateway.Slack.SigningSecret)
	c.Rules.Files = normalizeRulePaths(c.Rules.Files)
	c.Session.Instruct = strings.TrimSpace(c.Session.Instruct)
	c.Platform = strings.TrimSpace(strings.ToLower(c.Platform))
	c.FS.Roots = normalizeFSRoots(c.FS.Roots)
	c.Exec.Allow = dedupeAndTrim(c.Exec.Allow)
	c.Exec.Ask = dedupeAndTrim(c.Exec.Ask)
	c.Exec.Deny = dedupeAndTrim(c.Exec.Deny)
	c.Storage.Backend = strings.TrimSpace(strings.ToLower(c.Storage.Backend))
	c.Memory.Provider = strings.TrimSpace(strings.ToLower(c.Memory.Provider))
	c.Memory.Backend = strings.TrimSpace(strings.ToLower(c.Memory.Backend))
	c.Reranker.Type = strings.TrimSpace(strings.ToLower(c.Reranker.Type))
	c.Reranker.Model = strings.TrimSpace(c.Reranker.Model)
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
		c.Models.Main.API = getDefaultAPIForProvider(c.Models.Main.Provider)
	}

	if c.Log.Level == "" {
		c.Log.Level = constants.DefaultLogLevel
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
		provider = strings.TrimSpace(strings.ToLower(provider))
		if provider == "" {
			continue
		}

		value.APIKey = strings.TrimSpace(value.APIKey)
		value.APIKeyEnv = dedupeAndTrim(value.APIKeyEnv)
		normalized[provider] = value
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
		key = strings.TrimSpace(strings.ToLower(key))
		if key == "" {
			continue
		}

		override.Type = strings.TrimSpace(strings.ToLower(override.Type))
		override.Model = strings.TrimSpace(override.Model)
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
		name = strings.ToLower(strings.TrimSpace(name))
		personality.Soul = strings.TrimSpace(personality.Soul)
		personality.Instruct = strings.TrimSpace(personality.Instruct)
		personality.State = strings.TrimSpace(strings.ToLower(personality.State))
		if personality.State == "" {
			personality.State = personalityStateShared
		}
		personality.Tools.Memory = strings.TrimSpace(strings.ToLower(personality.Tools.Memory))
		personality.Model.Name = strings.TrimSpace(personality.Model.Name)
		personality.Model.Provider = strings.TrimSpace(strings.ToLower(personality.Model.Provider))
		personality.Model.API = strings.TrimSpace(strings.ToLower(personality.Model.API))
		personality.Model.BaseURL = strings.TrimSpace(personality.Model.BaseURL)
		normalized[name] = personality
	}
	c.Personalities = normalized
}

func (c *Config) applyDefaultModelBaseURL() {
	if c == nil {
		return
	}

	mapped := getDefaultBaseURLForProvider(c.Models.Main.Provider, c.Models.Main.API)
	if mapped == "" {
		return
	}
	if c.Models.Main.BaseURL == "" || isProviderDefaultBaseURL(c.Models.Main.BaseURL) {
		c.Models.Main.BaseURL = mapped
	}
}

func isProviderDefaultBaseURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}

	for _, providerID := range modelRegistry.GetProviderIDs() {
		providerDef, ok := modelRegistry.GetProvider(providerID)
		if !ok {
			continue
		}
		for _, baseURL := range providerDef.BaseURLs {
			if value == strings.TrimSpace(baseURL) {
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
