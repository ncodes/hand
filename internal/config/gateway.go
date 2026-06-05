package config

const (
	GatewayTelegramModePolling = "polling"
	GatewayTelegramModeWebhook = "webhook"
	GatewaySlackModeSocket     = "socket"
	GatewaySlackModeHTTP       = "http"
)

type GatewayConfig struct {
	Enabled   bool                  `yaml:"enabled"`
	Address   string                `yaml:"address"`
	Port      int                   `yaml:"port"`
	AuthToken string                `yaml:"authToken"`
	Telegram  GatewayTelegramConfig `yaml:"telegram"`
	Slack     GatewaySlackConfig    `yaml:"slack"`
}

type GatewayTelegramConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Mode          string `yaml:"mode"`
	BotToken      string `yaml:"botToken"`
	WebhookSecret string `yaml:"webhookSecret"`
}

type GatewaySlackConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Mode          string `yaml:"mode"`
	BotToken      string `yaml:"botToken"`
	AppToken      string `yaml:"appToken"`
	SigningSecret string `yaml:"signingSecret"`
}
