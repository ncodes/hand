package config

import (
	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/internal/datadir"
	"github.com/wandxy/morph/pkg/str"
)

// normalizeFields applies trimming and defaults except default model base URL resolution.
func (c *Config) normalizeFields() {
	if c == nil {
		return
	}
	stringValue1 := str.String(c.Name)
	c.Name = stringValue1.Trim()
	if c.Name == "" {
		c.Name = constants.DefaultName
	}
	stringValue2 := str.String(c.Models.Main.Name)
	c.Models.Main.Name = stringValue2.Trim()
	stringValue3 := str.String(c.Models.Summary.Name)
	c.Models.Summary.Name = stringValue3.Trim()
	stringValue4 := str.String(c.Models.Main.Provider)
	c.Models.Main.Provider = stringValue4.Normalized()
	stringValue5 := str.String(c.Models.Embedding.Provider)
	c.Models.Embedding.Provider = stringValue5.Normalized()
	stringValue6 := str.String(c.Models.Embedding.Name)
	c.Models.Embedding.Name = stringValue6.Trim()
	stringValue7 := str.String(c.Models.Embedding.API)
	c.Models.Embedding.API = stringValue7.Normalized()
	c.Models.Providers = normalizeProviderModelConfigs(c.Models.Providers)
	stringValue8 := str.String(c.Models.Main.APIKey)
	c.Models.Main.APIKey = stringValue8.Trim()
	stringValue9 := str.String(c.Models.Main.BaseURL)
	c.Models.Main.BaseURL = stringValue9.Trim()
	stringValue10 := str.String(c.Models.Summary.Provider)
	c.Models.Summary.Provider = stringValue10.Normalized()
	stringValue11 := str.String(c.Models.Summary.APIKey)
	c.Models.Summary.APIKey = stringValue11.Trim()
	stringValue12 := str.String(c.Models.Summary.BaseURL)
	c.Models.Summary.BaseURL = stringValue12.Trim()
	stringValue13 := str.String(c.Models.Embedding.APIKey)
	c.Models.Embedding.APIKey = stringValue13.Trim()
	stringValue14 := str.String(c.Models.Main.API)
	c.Models.Main.API = stringValue14.Normalized()
	stringValue15 := str.String(c.Models.Summary.API)
	c.Models.Summary.API = stringValue15.Normalized()
	stringValue16 := str.String(c.Log.Level)
	c.Log.Level = stringValue16.Normalized()
	stringValue17 := str.String(c.Trace.Disk.Dir)
	c.Trace.Disk.Dir = stringValue17.Trim()
	stringValue18 := str.String(c.Web.Provider)
	c.Web.Provider = stringValue18.Normalized()
	stringValue19 := str.String(c.Web.APIKey)
	c.Web.APIKey = stringValue19.Trim()
	stringValue20 := str.String(c.Web.BaseURL)
	c.Web.BaseURL = stringValue20.Trim()
	c.Web.BlockedDomains = dedupeAndTrim(c.Web.BlockedDomains)
	c.Web.BlockedDomainFiles = dedupeAndTrim(c.Web.BlockedDomainFiles)
	c.Web.NativeAllowedHosts = dedupeAndTrim(c.Web.NativeAllowedHosts)
	c.Web.NativeBlockedHosts = dedupeAndTrim(c.Web.NativeBlockedHosts)
	c.Web.NativeAllowedHostFiles = dedupeAndTrim(c.Web.NativeAllowedHostFiles)
	c.Web.NativeBlockedHostFiles = dedupeAndTrim(c.Web.NativeBlockedHostFiles)
	stringValue21 := str.String(c.Gateway.Address)
	c.Gateway.Address = stringValue21.Trim()
	stringValue22 := str.String(c.Gateway.AuthToken)
	c.Gateway.AuthToken = stringValue22.Trim()
	stringValue23 := str.String(c.Gateway.PairingSecret)
	c.Gateway.PairingSecret = stringValue23.Trim()
	c.Gateway.AllowedUsers = dedupeAndTrim(c.Gateway.AllowedUsers)
	stringValue24 := str.String(c.Gateway.Telegram.Mode)
	c.Gateway.Telegram.Mode = stringValue24.Normalized()
	stringValue25 := str.String(c.Gateway.Telegram.BotToken)
	c.Gateway.Telegram.BotToken = stringValue25.Trim()
	stringValue26 := str.String(c.Gateway.Telegram.WebhookSecret)
	c.Gateway.Telegram.WebhookSecret = stringValue26.Trim()
	c.Gateway.Telegram.AllowedUsers = dedupeAndTrim(c.Gateway.Telegram.AllowedUsers)
	stringValue27 := str.String(c.Gateway.Slack.Mode)
	c.Gateway.Slack.Mode = stringValue27.Normalized()
	stringValue28 := str.String(c.Gateway.Slack.ResponseMode)
	c.Gateway.Slack.ResponseMode = stringValue28.Normalized()
	stringValue29 := str.String(c.Gateway.Slack.BotToken)
	c.Gateway.Slack.BotToken = stringValue29.Trim()
	stringValue30 := str.String(c.Gateway.Slack.AppToken)
	c.Gateway.Slack.AppToken = stringValue30.Trim()
	stringValue31 := str.String(c.Gateway.Slack.SigningSecret)
	c.Gateway.Slack.SigningSecret = stringValue31.Trim()
	c.Gateway.Slack.AllowedUsers = dedupeAndTrim(c.Gateway.Slack.AllowedUsers)
	c.Rules.Files = normalizeRulePaths(c.Rules.Files)
	stringValue32 := str.String(c.Session.Instruct)
	c.Session.Instruct = stringValue32.Trim()
	stringValue33 := str.String(c.Platform)
	c.Platform = stringValue33.Normalized()
	c.FS.Roots = normalizeFSRoots(c.FS.Roots)
	c.Exec.Allow = dedupeAndTrim(c.Exec.Allow)
	c.Exec.Ask = dedupeAndTrim(c.Exec.Ask)
	c.Exec.Deny = dedupeAndTrim(c.Exec.Deny)
	stringValue34 := str.String(c.Storage.Backend)
	c.Storage.Backend = stringValue34.Normalized()
	stringValue35 := str.String(c.Memory.Provider)
	c.Memory.Provider = stringValue35.Normalized()
	stringValue36 := str.String(c.Memory.Backend)
	c.Memory.Backend = stringValue36.Normalized()
	stringValue37 := str.String(c.Reranker.Type)
	c.Reranker.Type = stringValue37.Normalized()
	stringValue38 := str.String(c.Reranker.Model)
	c.Reranker.Model = stringValue38.Trim()
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
		stringValue39 := str.String(provider)
		provider = stringValue39.Normalized()
		if provider == "" {
			continue
		}
		stringValue40 := str.String(value.APIKey)
		value.APIKey = stringValue40.Trim()
		value.APIKeyEnv = dedupeAndTrim(value.APIKeyEnv)
		stringValue41 := str.String(value.API)
		value.API = stringValue41.Normalized()
		stringValue42 := str.String(value.BaseURL)
		value.BaseURL = stringValue42.Trim()
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
		stringValue43 := str.String(model)
		model = stringValue43.Trim()
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
		stringValue44 := str.String(key)
		key = stringValue44.Normalized()
		if key == "" {
			continue
		}
		stringValue45 := str.String(override.Type)
		override.Type = stringValue45.Normalized()
		stringValue46 := str.String(override.Model)
		override.Model = stringValue46.Trim()
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
		stringValue47 := str.String(name)
		name = stringValue47.Normalized()
		stringValue48 := str.String(personality.Soul)
		personality.Soul = stringValue48.Trim()
		stringValue49 := str.String(personality.Instruct)
		personality.Instruct = stringValue49.Trim()
		stringValue50 := str.String(personality.State)
		personality.State = stringValue50.Normalized()
		if personality.State == "" {
			personality.State = personalityStateShared
		}
		stringValue51 := str.String(personality.Tools.Memory)
		personality.Tools.Memory = stringValue51.Normalized()
		stringValue52 := str.String(personality.Model.Name)
		personality.Model.Name = stringValue52.Trim()
		stringValue53 := str.String(personality.Model.Provider)
		personality.Model.Provider = stringValue53.Normalized()
		stringValue54 := str.String(personality.Model.API)
		personality.Model.API = stringValue54.Normalized()
		stringValue55 := str.String(personality.Model.BaseURL)
		personality.Model.BaseURL = stringValue55.Trim()
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
	stringValue56 := str.String(value)
	value = stringValue56.Trim()
	if value == "" {
		return false
	}

	for _, providerID := range modelRegistry.GetProviderIDs() {
		providerDef, ok := modelRegistry.GetProvider(providerID)
		if !ok {
			continue
		}
		for _, baseURL := range providerDef.BaseURLs {
			stringValue57 := str.String(baseURL)
			if value == stringValue57.Trim() {
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
