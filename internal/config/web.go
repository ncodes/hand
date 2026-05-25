package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

// WebConfig controls web provider credentials, limits, cache, and host policies.
type WebConfig struct {
	Provider                     string        `yaml:"provider"`
	APIKey                       string        `yaml:"apiKey"`
	BaseURL                      string        `yaml:"baseUrl"`
	MaxCharPerResult             int           `yaml:"maxCharPerResult"`
	MaxExtractCharPerResult      int           `yaml:"maxExtractCharPerResult"`
	MaxExtractResponseBytes      int           `yaml:"maxExtractResponseBytes"`
	CacheTTL                     time.Duration `yaml:"cacheTTL"`
	BlockedDomainsEnabled        bool          `yaml:"-"`
	BlockedDomains               []string      `yaml:"-"`
	BlockedDomainFiles           []string      `yaml:"-"`
	NativeAllowedHosts           []string      `yaml:"-"`
	NativeBlockedHosts           []string      `yaml:"-"`
	NativeAllowedHostFiles       []string      `yaml:"-"`
	NativeBlockedHostFiles       []string      `yaml:"-"`
	ExtractMinSummarizeChars     int           `yaml:"extractMinSummarizeChars"`
	ExtractMaxSummaryChars       int           `yaml:"extractMaxSummaryChars"`
	ExtractMaxSummaryChunkChars  int           `yaml:"extractMaxSummaryChunkChars"`
	ExtractRefusalThresholdChars int           `yaml:"extractRefusalThresholdChars"`
}

func (c *WebConfig) UnmarshalYAML(value *yaml.Node) error {
	type plain WebConfig
	var raw struct {
		plain          `yaml:",inline"`
		BlockedDomains struct {
			Enabled bool     `yaml:"enabled"`
			Domains []string `yaml:"domains"`
			Files   []string `yaml:"files"`
		} `yaml:"blockedDomains"`
		Native struct {
			AllowedHosts     []string `yaml:"allowedHosts"`
			BlockedHosts     []string `yaml:"blockedHosts"`
			AllowedHostFiles []string `yaml:"allowedHostFiles"`
			BlockedHostFiles []string `yaml:"blockedHostFiles"`
		} `yaml:"native"`
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}

	*c = WebConfig(raw.plain)
	c.BlockedDomainsEnabled = raw.BlockedDomains.Enabled
	c.BlockedDomains = raw.BlockedDomains.Domains
	c.BlockedDomainFiles = raw.BlockedDomains.Files
	c.NativeAllowedHosts = raw.Native.AllowedHosts
	c.NativeBlockedHosts = raw.Native.BlockedHosts
	c.NativeAllowedHostFiles = raw.Native.AllowedHostFiles
	c.NativeBlockedHostFiles = raw.Native.BlockedHostFiles

	return nil
}
