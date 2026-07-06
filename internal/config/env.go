package config

import (
	"encoding/json"
	"os"
	"strconv"
	"time"

	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/pkg/str"
)

func applyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	stringValue1 := str.String(os.Getenv("MORPH_NAME"))
	if value := stringValue1.Trim(); value != "" {
		cfg.Name = value
	}
	stringValue2 := str.String(os.Getenv("MORPH_MODEL"))
	if value := stringValue2.Trim(); value != "" {
		cfg.Models.Main.Name = value
	}
	stringValue3 := str.String(os.Getenv("MORPH_MODEL_SUMMARY"))
	if value := stringValue3.Trim(); value != "" {
		cfg.Models.Summary.Name = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MODEL_STREAM"); ok {
		cfg.Models.Main.Stream = new(value)
	}
	stringValue4 := str.String(os.Getenv("MORPH_MODEL_CONTEXT_LENGTH"))
	if value := stringValue4.Trim(); value != "" {
		if contextLength, err := strconv.Atoi(value); err == nil {
			cfg.Models.Main.ContextLength = contextLength
		}
	}
	stringValue5 := str.String(os.Getenv("MORPH_MODEL_MAX_RETRIES"))
	if value := stringValue5.Trim(); value != "" {
		if retries, err := strconv.Atoi(value); err == nil {
			cfg.Models.MaxRetries = &retries
		}
	}
	stringValue6 := str.String(os.Getenv("MORPH_MODEL_PROVIDER"))
	if value := stringValue6.Trim(); value != "" {
		cfg.Models.Main.Provider = value
	}
	stringValue7 := str.String(os.Getenv("MORPH_MODEL_EMBEDDING_PROVIDER"))
	if value := stringValue7.Trim(); value != "" {
		cfg.Models.Embedding.Provider = value
	}
	stringValue8 := str.String(os.Getenv("MORPH_MODEL_EMBEDDING_MODEL"))
	if value := stringValue8.Trim(); value != "" {
		cfg.Models.Embedding.Name = value
	}
	stringValue9 := str.String(os.Getenv("MORPH_MODEL_BASE_URL"))
	if value := stringValue9.Trim(); value != "" {
		cfg.Models.Main.BaseURL = value
	}
	stringValue10 := str.String(os.Getenv("MORPH_MODEL_SUMMARY_PROVIDER"))
	if value := stringValue10.Trim(); value != "" {
		cfg.Models.Summary.Provider = value
	}
	stringValue11 := str.String(os.Getenv("MORPH_MODEL_SUMMARY_BASE_URL"))
	if value := stringValue11.Trim(); value != "" {
		cfg.Models.Summary.BaseURL = value
	}
	stringValue12 := str.String(os.Getenv("MORPH_MODEL_API"))
	if value := stringValue12.Trim(); value != "" {
		cfg.Models.Main.API = value
	}
	stringValue13 := str.String(os.Getenv("MORPH_MODEL_SUMMARY_API"))
	if value := stringValue13.Trim(); value != "" {
		cfg.Models.Summary.API = value
	}
	stringValue14 := str.String(os.Getenv("MORPH_RPC_ADDRESS"))
	if value := stringValue14.Trim(); value != "" {
		cfg.RPC.Address = value
	}
	stringValue15 := str.String(os.Getenv("MORPH_RPC_PORT"))
	if value := stringValue15.Trim(); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.RPC.Port = port
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_GATEWAY_ENABLED"); ok {
		cfg.Gateway.Enabled = value
	}
	stringValue16 := str.String(os.Getenv("MORPH_GATEWAY_ADDRESS"))
	if value := stringValue16.Trim(); value != "" {
		cfg.Gateway.Address = value
	}
	stringValue17 := str.String(os.Getenv("MORPH_GATEWAY_PORT"))
	if value := stringValue17.Trim(); value != "" {
		if port, err := strconv.Atoi(value); err == nil {
			cfg.Gateway.Port = port
		}
	}
	stringValue18 := str.String(os.Getenv("MORPH_GATEWAY_AUTH_TOKEN"))
	if value := stringValue18.Trim(); value != "" {
		cfg.Gateway.AuthToken = value
	}
	stringValue19 := str.String(os.Getenv("MORPH_GATEWAY_PAIRING_SECRET"))
	if value := stringValue19.Trim(); value != "" {
		cfg.Gateway.PairingSecret = value
	}
	stringValue20 := str.String(os.Getenv("MORPH_GATEWAY_ALLOWED_USERS"))
	if value := stringValue20.Trim(); value != "" {
		cfg.Gateway.AllowedUsers = splitAndTrimCSV(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_GATEWAY_TELEGRAM_ENABLED"); ok {
		cfg.Gateway.Telegram.Enabled = value
	}
	stringValue21 := str.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_MODE"))
	if value := stringValue21.Trim(); value != "" {
		cfg.Gateway.Telegram.Mode = value
	}
	stringValue22 := str.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_BOT_TOKEN"))
	if value := stringValue22.Trim(); value != "" {
		cfg.Gateway.Telegram.BotToken = value
	}
	stringValue23 := str.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_WEBHOOK_SECRET"))
	if value := stringValue23.Trim(); value != "" {
		cfg.Gateway.Telegram.WebhookSecret = value
	}
	stringValue24 := str.String(os.Getenv("MORPH_GATEWAY_TELEGRAM_ALLOWED_USERS"))
	if value := stringValue24.Trim(); value != "" {
		cfg.Gateway.Telegram.AllowedUsers = splitAndTrimCSV(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_GATEWAY_SLACK_ENABLED"); ok {
		cfg.Gateway.Slack.Enabled = value
	}
	stringValue25 := str.String(os.Getenv("MORPH_GATEWAY_SLACK_MODE"))
	if value := stringValue25.Trim(); value != "" {
		cfg.Gateway.Slack.Mode = value
	}
	stringValue26 := str.String(os.Getenv("MORPH_GATEWAY_SLACK_RESPONSE_MODE"))
	if value := stringValue26.Trim(); value != "" {
		cfg.Gateway.Slack.ResponseMode = value
	}
	stringValue27 := str.String(os.Getenv("MORPH_GATEWAY_SLACK_BOT_TOKEN"))
	if value := stringValue27.Trim(); value != "" {
		cfg.Gateway.Slack.BotToken = value
	}
	stringValue28 := str.String(os.Getenv("MORPH_GATEWAY_SLACK_APP_TOKEN"))
	if value := stringValue28.Trim(); value != "" {
		cfg.Gateway.Slack.AppToken = value
	}
	stringValue29 := str.String(os.Getenv("MORPH_GATEWAY_SLACK_SIGNING_SECRET"))
	if value := stringValue29.Trim(); value != "" {
		cfg.Gateway.Slack.SigningSecret = value
	}
	stringValue30 := str.String(os.Getenv("MORPH_GATEWAY_SLACK_ALLOWED_USERS"))
	if value := stringValue30.Trim(); value != "" {
		cfg.Gateway.Slack.AllowedUsers = splitAndTrimCSV(value)
	}
	stringValue31 := str.String(os.Getenv("MORPH_SESSION_MAX_ITERATIONS"))
	if value := stringValue31.Trim(); value != "" {
		if maxIterations, err := strconv.Atoi(value); err == nil {
			cfg.Session.MaxIterations = maxIterations
		}
	}
	stringValue32 := str.String(os.Getenv("MORPH_LOG_LEVEL"))
	if value := stringValue32.Trim(); value != "" {
		cfg.Log.Level = value
	}
	stringValue33 := str.String(os.Getenv("MORPH_LOG_FILE"))
	if value := stringValue33.Trim(); value != "" {
		cfg.Log.File = value
	}
	stringValue34 := str.String(os.Getenv("MORPH_LOG_MAX_SIZE_MB"))
	if value := stringValue34.Trim(); value != "" {
		if maxSizeMB, err := strconv.Atoi(value); err == nil {
			cfg.Log.MaxSizeMB = maxSizeMB
		}
	}
	stringValue35 := str.String(os.Getenv("MORPH_LOG_MAX_BACKUPS"))
	if value := stringValue35.Trim(); value != "" {
		if maxBackups, err := strconv.Atoi(value); err == nil {
			cfg.Log.MaxBackups = maxBackups
		}
	}
	stringValue36 := str.String(os.Getenv("MORPH_LOG_MAX_AGE_DAYS"))
	if value := stringValue36.Trim(); value != "" {
		if maxAgeDays, err := strconv.Atoi(value); err == nil {
			cfg.Log.MaxAgeDays = maxAgeDays
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_LOG_COMPRESS"); ok {
		cfg.Log.Compress = value
	}
	stringValue37 := str.String(os.Getenv("MORPH_LOG_NO_COLOR"))
	if value := stringValue37.Normalized(); value != "" {
		cfg.Log.NoColor = value == "1" || value == "true" || value == "yes"
	}
	stringValue38 := str.String(os.Getenv("MORPH_DEBUG_REQUESTS"))
	if value := stringValue38.Normalized(); value != "" {
		cfg.Debug.Requests = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SAFETY_INPUT"); ok {
		cfg.Safety.Input = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SAFETY_OUTPUT"); ok {
		cfg.Safety.Output = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SAFETY_PII"); ok {
		cfg.Safety.PII = new(value)
	}
	stringValue39 := str.String(os.Getenv("MORPH_TRACE_ENABLED"))
	if value := stringValue39.Normalized(); value != "" {
		cfg.Trace.Enabled = value == "1" || value == "true" || value == "yes"
	}
	if value, ok := parseOptionalBoolEnv("MORPH_TRACE_DISK_ENABLED"); ok {
		cfg.Trace.Disk.Enabled = new(value)
	}
	stringValue40 := str.String(os.Getenv("MORPH_TRACE_DISK_DIR"))
	if value := stringValue40.Trim(); value != "" {
		cfg.Trace.Disk.Dir = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_TRACE_DATABASE_ENABLED"); ok {
		cfg.Trace.Database.Enabled = new(value)
	}
	stringValue41 := str.String(os.Getenv("MORPH_TRACE_DATABASE_MAX_EVENTS_PER_SESSION"))
	if value := stringValue41.Trim(); value != "" {
		if maxEvents, err := strconv.Atoi(value); err == nil {
			cfg.Trace.Database.MaxEventsPerSession = maxEvents
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_TUI_THINKING_COMPOSER"); ok {
		cfg.TUI.ThinkingComposer = new(value)
	}
	stringValue42 := str.String(os.Getenv("MORPH_WEB_PROVIDER"))
	if value := stringValue42.Trim(); value != "" {
		cfg.Web.Provider = value
	}
	stringValue43 := str.String(os.Getenv("MORPH_WEB_API_KEY"))
	if value := stringValue43.Trim(); value != "" {
		cfg.Web.APIKey = value
	}
	stringValue44 := str.String(os.Getenv("MORPH_WEB_BASE_URL"))
	if value := stringValue44.Trim(); value != "" {
		cfg.Web.BaseURL = value
	}
	stringValue45 := str.String(os.Getenv("MORPH_WEB_MAX_CHAR_PER_RESULT"))
	if value := stringValue45.Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxCharPerResult = chars
		}
	}
	stringValue46 := str.String(os.Getenv("MORPH_WEB_MAX_EXTRACT_CHAR_PER_RESULT"))
	if value := stringValue46.Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractCharPerResult = chars
		}
	}
	stringValue47 := str.String(os.Getenv("MORPH_WEB_MAX_EXTRACT_RESPONSE_BYTES"))
	if value := stringValue47.Trim(); value != "" {
		if bytes, err := strconv.Atoi(value); err == nil {
			cfg.Web.MaxExtractResponseBytes = bytes
		}
	}
	stringValue48 := str.String(os.Getenv("MORPH_WEB_CACHE_TTL"))
	if value := stringValue48.Trim(); value != "" {
		cfg.Web.CacheTTL = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_WEB_BLOCKED_DOMAINS_ENABLED"); ok {
		cfg.Web.BlockedDomainsEnabled = value
	}
	stringValue49 := str.String(os.Getenv("MORPH_WEB_BLOCKED_DOMAINS"))
	if value := stringValue49.Trim(); value != "" {
		cfg.Web.BlockedDomains = splitAndTrimCSV(value)
	}
	stringValue50 := str.String(os.Getenv("MORPH_WEB_BLOCKED_DOMAIN_FILES"))
	if value := stringValue50.Trim(); value != "" {
		cfg.Web.BlockedDomainFiles = splitAndTrimCSV(value)
	}
	stringValue51 := str.String(os.Getenv("MORPH_WEB_NATIVE_ALLOWED_HOSTS"))
	if value := stringValue51.Trim(); value != "" {
		cfg.Web.NativeAllowedHosts = splitAndTrimCSV(value)
	}
	stringValue52 := str.String(os.Getenv("MORPH_WEB_NATIVE_BLOCKED_HOSTS"))
	if value := stringValue52.Trim(); value != "" {
		cfg.Web.NativeBlockedHosts = splitAndTrimCSV(value)
	}
	stringValue53 := str.String(os.Getenv("MORPH_WEB_NATIVE_ALLOWED_HOST_FILES"))
	if value := stringValue53.Trim(); value != "" {
		cfg.Web.NativeAllowedHostFiles = splitAndTrimCSV(value)
	}
	stringValue54 := str.String(os.Getenv("MORPH_WEB_NATIVE_BLOCKED_HOST_FILES"))
	if value := stringValue54.Trim(); value != "" {
		cfg.Web.NativeBlockedHostFiles = splitAndTrimCSV(value)
	}
	stringValue55 := str.String(os.Getenv("MORPH_WEB_EXTRACT_MIN_SUMMARIZE_CHARS"))
	if value := stringValue55.Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMinSummarizeChars = chars
		}
	}
	stringValue56 := str.String(os.Getenv("MORPH_WEB_EXTRACT_MAX_SUMMARY_CHARS"))
	if value := stringValue56.Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChars = chars
		}
	}
	stringValue57 := str.String(os.Getenv("MORPH_WEB_EXTRACT_MAX_SUMMARY_CHUNK_CHARS"))
	if value := stringValue57.Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractMaxSummaryChunkChars = chars
		}
	}
	stringValue58 := str.String(os.Getenv("MORPH_WEB_EXTRACT_REFUSAL_THRESHOLD_CHARS"))
	if value := stringValue58.Trim(); value != "" {
		if chars, err := strconv.Atoi(value); err == nil {
			cfg.Web.ExtractRefusalThresholdChars = chars
		}
	}
	if cfg.Web.Provider == "" {
		firecrawlAPIKey := str.String(os.Getenv("MORPH_FIRECRAWL_API_KEY"))
		firecrawlAPIURL := str.String(os.Getenv("MORPH_FIRECRAWL_API_URL"))
		parallelAPIKey := str.String(os.Getenv("MORPH_PARALLEL_API_KEY"))
		tavilyAPIKey := str.String(os.Getenv("MORPH_TAVILY_API_KEY"))
		exaAPIKey := str.String(os.Getenv("MORPH_EXA_API_KEY"))
		switch {
		case firecrawlAPIKey.Trim() != "" || firecrawlAPIURL.Trim() != "":
			cfg.Web.Provider = constants.WebProviderFirecrawl
		case parallelAPIKey.Trim() != "":
			cfg.Web.Provider = constants.WebProviderParallel
		case tavilyAPIKey.Trim() != "":
			cfg.Web.Provider = constants.WebProviderTavily
		case exaAPIKey.Trim() != "":
			cfg.Web.Provider = constants.WebProviderExa
		}
	}
	if cfg.Web.APIKey == "" {
		stringValue100 := str.String(cfg.Web.Provider)
		switch stringValue100.Normalized() {
		case constants.WebProviderFirecrawl:
			stringValue101 := str.String(os.Getenv("MORPH_FIRECRAWL_API_KEY"))
			cfg.Web.APIKey = stringValue101.Trim()
		case constants.WebProviderParallel:
			stringValue102 := str.String(os.Getenv("MORPH_PARALLEL_API_KEY"))
			cfg.Web.APIKey = stringValue102.Trim()
		case constants.WebProviderTavily:
			stringValue103 := str.String(os.Getenv("MORPH_TAVILY_API_KEY"))
			cfg.Web.APIKey = stringValue103.Trim()
		case constants.WebProviderExa:
			stringValue104 := str.String(os.Getenv("MORPH_EXA_API_KEY"))
			cfg.Web.APIKey = stringValue104.Trim()
		}
	}
	stringValue59 := str.String(cfg.Web.Provider)
	if cfg.Web.BaseURL == "" && stringValue59.Normalized() == constants.WebProviderFirecrawl {
		stringValue105 := str.String(os.Getenv("MORPH_FIRECRAWL_API_URL"))
		cfg.Web.BaseURL = stringValue105.Trim()
	}
	stringValue60 := str.String(os.Getenv("MORPH_RULES_FILES"))
	if value := stringValue60.Trim(); value != "" {
		cfg.Rules.Files = splitAndTrimCSV(value)
	}
	stringValue61 := str.String(os.Getenv("MORPH_SESSION_INSTRUCT"))
	if value := stringValue61.Trim(); value != "" {
		cfg.Session.Instruct = value
	}
	stringValue62 := str.String(os.Getenv("MORPH_PLATFORM"))
	if value := stringValue62.Trim(); value != "" {
		cfg.Platform = value
	}
	stringValue63 := str.String(os.Getenv("MORPH_FS_ROOTS"))
	if value := stringValue63.Trim(); value != "" {
		cfg.FS.Roots = splitAndTrimCSV(value)
	}

	if value, ok := parseOptionalBoolEnv("MORPH_CAP_FS"); ok {
		cfg.Cap.Filesystem = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_NET"); ok {
		cfg.Cap.Network = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_EXEC"); ok {
		cfg.Cap.Exec = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_MEM"); ok {
		cfg.Cap.Memory = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_CAP_BROWSER"); ok {
		cfg.Cap.Browser = new(value)
	}
	stringValue64 := str.String(os.Getenv("MORPH_EXEC_ALLOW"))
	if value := stringValue64.Trim(); value != "" {
		cfg.Exec.Allow = splitAndTrimCSV(value)
	}
	stringValue65 := str.String(os.Getenv("MORPH_EXEC_ASK"))
	if value := stringValue65.Trim(); value != "" {
		cfg.Exec.Ask = splitAndTrimCSV(value)
	}
	stringValue66 := str.String(os.Getenv("MORPH_EXEC_DENY"))
	if value := stringValue66.Trim(); value != "" {
		cfg.Exec.Deny = splitAndTrimCSV(value)
	}
	stringValue67 := str.String(os.Getenv("MORPH_STORAGE_BACKEND"))
	if value := stringValue67.Trim(); value != "" {
		cfg.Storage.Backend = value
	}
	stringValue68 := str.String(os.Getenv("MORPH_SESSION_DEFAULT_IDLE_EXPIRY"))
	if value := stringValue68.Trim(); value != "" {
		cfg.Session.DefaultIdleExpiry = parseDurationOrZero(value)
	}
	stringValue69 := str.String(os.Getenv("MORPH_SESSION_ARCHIVE_RETENTION"))
	if value := stringValue69.Trim(); value != "" {
		cfg.Session.ArchiveRetention = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SEARCH_VECTOR_ENABLED"); ok {
		cfg.Search.Vector.Enabled = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_ENABLED"); ok {
		cfg.Memory.Enabled = new(value)
	}
	stringValue70 := str.String(os.Getenv("MORPH_MEMORY_PROVIDER"))
	if value := stringValue70.Trim(); value != "" {
		cfg.Memory.Provider = value
	}
	stringValue71 := str.String(os.Getenv("MORPH_MEMORY_BACKEND"))
	if value := stringValue71.Trim(); value != "" {
		cfg.Memory.Backend = value
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_PINNED_ENABLED"); ok {
		cfg.Memory.Pinned.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_RETRIEVAL_ENABLED"); ok {
		cfg.Memory.Retrieval.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_FLUSH_ENABLED"); ok {
		cfg.Memory.Flush.Enabled = new(value)
	}
	stringValue72 := str.String(os.Getenv("MORPH_MEMORY_FLUSH_MAX_CALLS"))
	if value := stringValue72.Trim(); value != "" {
		if maxCalls, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Flush.MaxCalls = maxCalls
		}
	}
	stringValue73 := str.String(os.Getenv("MORPH_MEMORY_FLUSH_MAX_OUTPUT_TOKENS"))
	if value := stringValue73.Trim(); value != "" {
		if maxOutputTokens, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.Memory.Flush.MaxOutputTokens = maxOutputTokens
		}
	}
	stringValue74 := str.String(os.Getenv("MORPH_MEMORY_FLUSH_TIMEOUT"))
	if value := stringValue74.Trim(); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil {
			cfg.Memory.Flush.Timeout = timeout
		}
	}
	stringValue75 := str.String(os.Getenv("MORPH_MEMORY_PINNED_MAX_CHARS"))
	if value := stringValue75.Trim(); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxChars = maxChars
		}
	}
	stringValue76 := str.String(os.Getenv("MORPH_MEMORY_PINNED_MAX_ITEM_CHARS"))
	if value := stringValue76.Trim(); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Pinned.MaxItemChars = maxChars
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_EPISODIC_ENABLED"); ok {
		cfg.Memory.Episodic.Enabled = new(value)
	}
	stringValue77 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_INTERVAL"))
	if value := stringValue77.Trim(); value != "" {
		cfg.Memory.Episodic.Interval = parseDurationOrZero(value)
	}
	stringValue78 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_IDLE_AFTER"))
	if value := stringValue78.Trim(); value != "" {
		cfg.Memory.Episodic.IdleAfter = parseDurationOrZero(value)
	}
	stringValue79 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_MIN_MESSAGES"))
	if value := stringValue79.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MinMessages = count
		}
	}
	stringValue80 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_WINDOW_SIZE"))
	if value := stringValue80.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.WindowSize = count
		}
	}
	stringValue81 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_WINDOWS"))
	if value := stringValue81.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindows = count
		}
	}
	stringValue82 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_WINDOW_CHARS"))
	if value := stringValue82.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowChars = count
		}
	}
	stringValue83 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_WINDOW_TOKENS"))
	if value := stringValue83.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxWindowTokens = count
		}
	}
	stringValue84 := str.String(os.Getenv("MORPH_MEMORY_EPISODIC_MAX_RETRIES"))
	if value := stringValue84.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Episodic.MaxRetries = count
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_REFLECTION_ENABLED"); ok {
		cfg.Memory.Reflection.Enabled = new(value)
	}
	stringValue85 := str.String(os.Getenv("MORPH_MEMORY_REFLECTION_INTERVAL"))
	if value := stringValue85.Trim(); value != "" {
		cfg.Memory.Reflection.Interval = parseDurationOrZero(value)
	}
	stringValue86 := str.String(os.Getenv("MORPH_MEMORY_REFLECTION_LIMIT"))
	if value := stringValue86.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.Limit = count
		}
	}
	stringValue87 := str.String(os.Getenv("MORPH_MEMORY_REFLECTION_RELATED_LIMIT"))
	if value := stringValue87.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Reflection.RelatedLimit = count
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_PROMOTION_ENABLED"); ok {
		cfg.Memory.Promotion.Enabled = new(value)
	}
	stringValue88 := str.String(os.Getenv("MORPH_MEMORY_PROMOTION_INTERVAL"))
	if value := stringValue88.Trim(); value != "" {
		cfg.Memory.Promotion.Interval = parseDurationOrZero(value)
	}
	stringValue89 := str.String(os.Getenv("MORPH_MEMORY_PROMOTION_LIMIT"))
	if value := stringValue89.Trim(); value != "" {
		if count, err := strconv.Atoi(value); err == nil {
			cfg.Memory.Promotion.Limit = count
		}
	}
	stringValue90 := str.String(os.Getenv("MORPH_MEMORY_PROMOTION_EVALUATED_RETENTION"))
	if value := stringValue90.Trim(); value != "" {
		cfg.Memory.Promotion.EvaluatedRetention = parseDurationOrZero(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_MEMORY_WRITE_ENABLED"); ok {
		cfg.Memory.Write.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SEARCH_VECTOR_REQUIRED"); ok {
		cfg.Search.Vector.Required = value
	}
	stringValue91 := str.String(os.Getenv("MORPH_SEARCH_VECTOR_REBUILD_BATCH_SIZE"))
	if value := stringValue91.Trim(); value != "" {
		if batchSize, err := strconv.Atoi(value); err == nil {
			cfg.Search.Vector.RebuildBatchSize = batchSize
		}
	}
	if value, ok := parseOptionalBoolEnv("MORPH_RERANKER_ENABLED"); ok {
		cfg.Reranker.Enabled = new(value)
	}
	if value, ok := parseOptionalBoolEnv("MORPH_SEARCH_ENABLE_RERANK"); ok {
		cfg.Search.EnableRerank = new(value)
	}
	stringValue92 := str.String(os.Getenv("MORPH_RERANKER_TYPE"))
	if value := stringValue92.Trim(); value != "" {
		cfg.Reranker.Type = value
	}
	stringValue93 := str.String(os.Getenv("MORPH_RERANKER_MODEL"))
	if value := stringValue93.Trim(); value != "" {
		cfg.Reranker.Model = value
	}
	stringValue94 := str.String(os.Getenv("MORPH_RERANKER_MAX_CANDIDATES"))
	if value := stringValue94.Trim(); value != "" {
		if maxCandidates, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidates = maxCandidates
		}
	}
	stringValue95 := str.String(os.Getenv("MORPH_RERANKER_MAX_CANDIDATE_TEXT_CHARS"))
	if value := stringValue95.Trim(); value != "" {
		if maxChars, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxCandidateTextChars = maxChars
		}
	}
	stringValue96 := str.String(os.Getenv("MORPH_RERANKER_MAX_OUTPUT_TOKENS"))
	if value := stringValue96.Trim(); value != "" {
		if maxTokens, err := strconv.Atoi(value); err == nil {
			cfg.Reranker.MaxOutputTokens = maxTokens
		}
	}
	stringValue97 := str.String(os.Getenv("MORPH_RERANKER_OVERRIDES"))
	if value := stringValue97.Trim(); value != "" {
		var overrides map[string]RerankerOverrideConfig
		if err := json.Unmarshal([]byte(value), &overrides); err == nil {
			cfg.Reranker.Overrides = overrides
		}
	}

	if value, ok := parseOptionalBoolEnv("MORPH_COMPACTION_ENABLED"); ok {
		cfg.Compaction.Enabled = new(value)
	}
	stringValue98 := str.String(os.Getenv("MORPH_COMPACTION_TRIGGER_PERCENT"))
	if value := stringValue98.Trim(); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.TriggerPercent = percent
		}
	}
	stringValue99 := str.String(os.Getenv("MORPH_COMPACTION_WARN_PERCENT"))
	if value := stringValue99.Trim(); value != "" {
		if percent, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Compaction.WarnPercent = percent
		}
	}
}
