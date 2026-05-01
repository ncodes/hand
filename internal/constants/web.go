package constants

import "time"

const (
	// DefaultWebMaxCharPerResult is the fallback character budget per web search result.
	DefaultWebMaxCharPerResult = 1200
	// DefaultWebMaxExtractCharPerResult is the fallback character budget per extracted web result.
	DefaultWebMaxExtractCharPerResult = 50000
	// DefaultWebMaxExtractResponseBytes is the fallback byte budget for extracted web responses.
	DefaultWebMaxExtractResponseBytes = 2 * 1024 * 1024
	// DefaultWebCacheTTL is the fallback TTL for cached web provider responses.
	DefaultWebCacheTTL = 0 * time.Second
	// DefaultWebExtractMinSummarizeChars is the extracted content size that triggers summarization.
	DefaultWebExtractMinSummarizeChars = 12000
	// DefaultWebExtractMaxSummaryChars is the fallback character budget for web extraction summaries.
	DefaultWebExtractMaxSummaryChars = 4000
	// DefaultWebExtractMaxSummaryChunkChars is the fallback chunk size for web extraction summarization.
	DefaultWebExtractMaxSummaryChunkChars = 25000
	// DefaultWebExtractRefusalThresholdChars is the extracted content size that blocks direct return.
	DefaultWebExtractRefusalThresholdChars = 200000
)

const (
	// WebSearchToolDefaultCount is the fallback number of web search results requested by the tool.
	WebSearchToolDefaultCount = 5
	// WebSearchToolMaxCount is the hard maximum number of web search results requested by the tool.
	WebSearchToolMaxCount = 10
	// WebExtractToolMaxURLs is the hard maximum number of URLs accepted by the web extract tool.
	WebExtractToolMaxURLs = 5
)

const (
	// NativeWebDefaultTimeout is the fallback timeout for native web requests.
	NativeWebDefaultTimeout = 15 * time.Second
	// NativeWebDialTimeout is the fallback dial timeout for native web requests.
	NativeWebDialTimeout = 10 * time.Second
	// NativeWebUserAgent is the fallback user agent for native web requests.
	NativeWebUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
)
