package authcmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	modelcredential "github.com/wandxy/hand/internal/model/credential"
	modelprovider "github.com/wandxy/hand/internal/model/provider"
	"github.com/wandxy/hand/internal/profile"
)

var authOutput io.Writer = os.Stdout

func SetOutput(w io.Writer) io.Writer {
	previous := authOutput
	if w == nil {
		authOutput = io.Discard
		return previous
	}
	authOutput = w
	return previous
}

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "auth",
		Usage: "Manage provider credentials",
		Commands: []*cli.Command{
			newLoginCommand(),
			newStatusCommand(),
			newLogoutCommand(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newLoginCommand() *cli.Command {
	return &cli.Command{
		Name:      "login",
		Usage:     "Store credentials for a model provider",
		ArgsUsage: "<provider>",
		Flags: []cli.Flag{
			handcli.ProfileFlag(),
			&cli.StringFlag{Name: "api-key", Usage: "Static API key to store"},
			&cli.StringFlag{Name: "token", Usage: "OAuth or subscription bearer token to store"},
			&cli.StringFlag{Name: "refresh-token", Usage: "OAuth refresh token to store with --token"},
			&cli.StringFlag{Name: "expires-at", Usage: "OAuth token expiry time in RFC3339 format"},
			&cli.StringSliceFlag{Name: "scope", Usage: "OAuth scope to store with --token"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			provider, err := getAuthProviderArg(cmd)
			if err != nil {
				return err
			}
			store, err := getAuthStore(cmd)
			if err != nil {
				return err
			}

			credential, err := getLoginCredential(cmd)
			if err != nil {
				return err
			}
			if err := store.Set(provider, credential); err != nil {
				return err
			}

			_, err = fmt.Fprintf(authOutput, "%s credential stored\n", provider)
			return err
		},
	}
}

func newStatusCommand() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show configured model provider credential sources",
		ArgsUsage: "[provider...]",
		Flags:     []cli.Flag{handcli.ProfileFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			store, err := getAuthStore(cmd)
			if err != nil {
				return err
			}
			cfg, _ := loadStatusConfig(cmd)
			providers, err := getStatusProviders(cmd, store, cfg)
			if err != nil {
				return err
			}

			for _, provider := range providers {
				status, err := getProviderAuthStatus(provider, store, cfg)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(authOutput, "%s: %s\n", provider, formatAuthStatus(status)); err != nil {
					return err
				}
			}

			return nil
		},
	}
}

func newLogoutCommand() *cli.Command {
	return &cli.Command{
		Name:      "logout",
		Usage:     "Remove stored credentials for a model provider",
		ArgsUsage: "<provider>",
		Flags:     []cli.Flag{handcli.ProfileFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			provider, err := getAuthProviderArg(cmd)
			if err != nil {
				return err
			}
			store, err := getAuthStore(cmd)
			if err != nil {
				return err
			}
			if err := store.Remove(provider); err != nil {
				return err
			}

			_, err = fmt.Fprintf(authOutput, "%s credential removed\n", provider)
			return err
		},
	}
}

func getAuthProviderArg(cmd *cli.Command) (string, error) {
	provider := strings.TrimSpace(strings.ToLower(cmd.Args().First()))
	if provider == "" {
		return "", fmt.Errorf("provider is required")
	}
	return provider, nil
}

func getAuthStore(cmd *cli.Command) (*modelcredential.FileStore, error) {
	inputs, err := handcli.ResolveConfigInputs(cmd)
	if err != nil {
		return nil, err
	}

	active := profile.WithMetadataPaths(inputs.Profile)
	profile.SetActive(active)
	return modelcredential.NewFileStore(""), nil
}

func getLoginCredential(cmd *cli.Command) (modelcredential.StoredCredential, error) {
	apiKey := strings.TrimSpace(cmd.String("api-key"))
	token := strings.TrimSpace(cmd.String("token"))
	if apiKey != "" && token != "" {
		return modelcredential.StoredCredential{}, fmt.Errorf("use either --api-key or --token, not both")
	}
	if apiKey != "" {
		return modelcredential.StoredCredential{Type: modelcredential.TypeAPIKey, Key: apiKey}, nil
	}
	if token == "" {
		return modelcredential.StoredCredential{}, fmt.Errorf("credential is required; pass --api-key or --token")
	}

	credential := modelcredential.StoredCredential{
		Type:    modelcredential.TypeOAuth,
		Token:   token,
		Refresh: strings.TrimSpace(cmd.String("refresh-token")),
		Scopes:  cmd.StringSlice("scope"),
	}
	if expiresAt := strings.TrimSpace(cmd.String("expires-at")); expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return modelcredential.StoredCredential{}, fmt.Errorf("parse --expires-at: %w", err)
		}
		credential.ExpiresAt = &parsed
	}

	return credential, nil
}

func loadStatusConfig(cmd *cli.Command) (*config.Config, error) {
	cfg, _, err := handcli.LoadConfig(cmd)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func getStatusProviders(
	cmd *cli.Command,
	store modelcredential.Store,
	cfg *config.Config,
) ([]string, error) {
	if args := cmd.Args().Slice(); len(args) > 0 {
		providers := make([]string, 0, len(args))
		for _, provider := range args {
			provider = strings.TrimSpace(strings.ToLower(provider))
			if provider != "" {
				providers = append(providers, provider)
			}
		}
		sort.Strings(providers)
		return providers, nil
	}

	seen := make(map[string]struct{})
	for _, provider := range modelprovider.DefaultRegistry().GetProviderIDs() {
		seen[provider] = struct{}{}
	}
	if cfg != nil {
		for provider := range cfg.Models.Providers {
			seen[strings.TrimSpace(strings.ToLower(provider))] = struct{}{}
		}
	}
	stored, err := store.List()
	if err != nil {
		return nil, err
	}
	for _, provider := range stored {
		seen[provider] = struct{}{}
	}

	providers := make([]string, 0, len(seen))
	for provider := range seen {
		if provider != "" {
			providers = append(providers, provider)
		}
	}
	sort.Strings(providers)
	return providers, nil
}

func getProviderAuthStatus(
	provider string,
	store modelcredential.Store,
	cfg *config.Config,
) (modelcredential.Status, error) {
	status := modelcredential.Status{
		Provider: provider,
		Source:   modelcredential.CredentialSourceMissing,
	}

	credential, ok, err := store.Get(provider)
	if err != nil {
		return modelcredential.Status{}, err
	}
	if ok {
		status.Configured = true
		status.Source = modelcredential.CredentialSourceStored
		status.Type = credential.Type
		status.HasExpiry = credential.ExpiresAt != nil
		status.Expired = credential.ExpiresAt != nil && !time.Now().Before(*credential.ExpiresAt)
		return status, nil
	}

	if _, envName := getProviderEnvCredential(provider, cfg); envName != "" {
		status.Configured = true
		status.Source = modelcredential.CredentialSourceEnvironment
		return status, nil
	}

	if cfg != nil {
		providerConfig := cfg.Models.Providers[provider]
		if strings.TrimSpace(providerConfig.APIKey) != "" {
			status.Configured = true
			status.Source = modelcredential.CredentialSourceConfig
		}
	}

	return status, nil
}

func formatAuthStatus(status modelcredential.Status) string {
	if !status.Configured {
		return "missing"
	}

	switch status.Source {
	case modelcredential.CredentialSourceStored:
		parts := []string{"stored"}
		if status.Type != "" {
			parts = append(parts, status.Type)
		}
		if status.HasExpiry {
			if status.Expired {
				parts = append(parts, "expired")
			} else {
				parts = append(parts, "refreshable")
			}
		}
		return strings.Join(parts, " ")
	case modelcredential.CredentialSourceEnvironment:
		return "environment"
	case modelcredential.CredentialSourceConfig:
		return "provider-config"
	default:
		return string(status.Source)
	}
}

func getProviderEnvCredential(provider string, cfg *config.Config) (string, string) {
	if cfg != nil {
		if value, name := getFirstEnvValue(cfg.Models.Providers[provider].APIKeyEnv); value != "" {
			return value, name
		}
	}

	if providerDef, ok := modelprovider.DefaultRegistry().GetProvider(provider); ok {
		return getFirstEnvValue(providerDef.APIKeyEnv)
	}

	return "", ""
}

func getFirstEnvValue(keys []string) (string, string) {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, key
		}
	}
	return "", ""
}
