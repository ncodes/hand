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

	morphcli "github.com/wandxy/morph/internal/cli"
	"github.com/wandxy/morph/internal/config"
	appcredential "github.com/wandxy/morph/internal/credential"
	modelprovider "github.com/wandxy/morph/internal/model/provider"
	_ "github.com/wandxy/morph/internal/model/provider_copilot"
	"github.com/wandxy/morph/internal/profile"
	"github.com/wandxy/morph/pkg/stringx"
)

var (
	authOutput              io.Writer = os.Stdout
	authInput               io.Reader = os.Stdin
	getSubscriptionProvider           = appcredential.GetSubscriptionProvider
)

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
		Usage:     "Store credentials for a model or web provider",
		ArgsUsage: "<provider>",
		Flags: []cli.Flag{
			morphcli.ProfileFlag(),
			&cli.StringFlag{Name: "api-key", Usage: "Static API key to store"},
			&cli.StringFlag{Name: "token", Usage: "OAuth or subscription bearer token to store"},
			&cli.StringFlag{Name: "refresh-token", Usage: "OAuth refresh token to store with --token"},
			&cli.StringFlag{Name: "expires-at", Usage: "OAuth token expiry time in RFC3339 format"},
			&cli.StringSliceFlag{Name: "scope", Usage: "OAuth scope to store with --token"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			provider, err := getAuthProviderArg(cmd)
			if err != nil {
				return err
			}
			store, err := getAuthStore(cmd)
			if err != nil {
				return err
			}

			credential, err := getLoginCredential(ctx, provider, cmd)
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
		Usage:     "Show configured provider credential sources",
		ArgsUsage: "[provider...]",
		Flags:     []cli.Flag{morphcli.ProfileFlag()},
		Action: func(_ context.Context, cmd *cli.Command) error {
			store, err := getAuthStore(cmd)
			if err != nil {
				return err
			}
			cfg, _ := loadAuthConfig(cmd)
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
		Usage:     "Remove stored credentials for a model or web provider",
		ArgsUsage: "<provider>",
		Flags:     []cli.Flag{morphcli.ProfileFlag()},
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
	provider := stringx.String(cmd.Args().First()).Normalized()
	if provider == "" {
		return "", fmt.Errorf("provider is required")
	}
	return provider, nil
}

func getAuthStore(cmd *cli.Command) (*appcredential.FileStore, error) {
	inputs, err := morphcli.ResolveConfigInputs(cmd)
	if err != nil {
		return nil, err
	}

	active := profile.WithMetadataPaths(inputs.Profile)
	profile.SetActive(active)
	return appcredential.NewFileStore(""), nil
}

func getLoginCredential(
	ctx context.Context,
	provider string,
	cmd *cli.Command,
) (appcredential.StoredCredential, error) {
	apiKey := stringx.String(cmd.String("api-key")).Trim()
	token := stringx.String(cmd.String("token")).Trim()
	if apiKey != "" && token != "" {
		return appcredential.StoredCredential{}, fmt.Errorf("use either --api-key or --token, not both")
	}
	if apiKey != "" {
		return appcredential.StoredCredential{Type: appcredential.TypeAPIKey, Key: apiKey}, nil
	}
	if token == "" {
		if subscriptionProvider, ok := getSubscriptionProvider(provider); ok {
			return subscriptionProvider.Login(ctx, appcredential.LoginOptions{
				Provider: provider,
				Input:    authInput,
				Output:   authOutput,
			})
		}

		return appcredential.StoredCredential{}, fmt.Errorf(
			"credential is required; pass --api-key or --token, or use a provider with subscription login",
		)
	}

	credential := appcredential.StoredCredential{
		Type:    appcredential.TypeOAuth,
		Token:   token,
		Refresh: stringx.String(cmd.String("refresh-token")).Trim(),
		Scopes:  cmd.StringSlice("scope"),
	}
	if expiresAt := stringx.String(cmd.String("expires-at")).Trim(); expiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return appcredential.StoredCredential{}, fmt.Errorf("parse --expires-at: %w", err)
		}
		credential.ExpiresAt = &parsed
	}

	return credential, nil
}

func loadAuthConfig(cmd *cli.Command) (*config.Config, error) {
	cfg, _, err := morphcli.LoadConfig(cmd)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func getStatusProviders(
	cmd *cli.Command,
	store appcredential.Store,
	cfg *config.Config,
) ([]string, error) {
	if args := cmd.Args().Slice(); len(args) > 0 {
		providers := make([]string, 0, len(args))
		for _, provider := range args {
			provider = stringx.String(provider).Normalized()
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
	for _, provider := range config.WebCredentialProviderIDs() {
		seen[provider] = struct{}{}
	}
	if cfg != nil {
		for provider := range cfg.Models.Providers {
			seen[stringx.String(provider).Normalized()] = struct{}{}
		}
		if provider := stringx.String(cfg.Web.Provider).Normalized(); provider != "" &&
			config.IsWebCredentialProvider(provider) {
			seen[provider] = struct{}{}
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
	store appcredential.Store,
	cfg *config.Config,
) (appcredential.Status, error) {
	status := appcredential.Status{
		Provider: provider,
		Source:   appcredential.CredentialSourceMissing,
	}

	credential, ok, err := store.Get(provider)
	if err != nil {
		return appcredential.Status{}, err
	}
	if ok {
		status.Configured = true
		status.Source = appcredential.CredentialSourceStored
		status.Type = credential.Type
		status.HasExpiry = credential.ExpiresAt != nil
		status.Expired = credential.ExpiresAt != nil && !time.Now().Before(*credential.ExpiresAt)
		return status, nil
	}

	if _, envName := getProviderEnvCredential(provider, cfg); envName != "" {
		status.Configured = true
		status.Source = appcredential.CredentialSourceEnvironment
		return status, nil
	}

	if cfg != nil {
		providerConfig := cfg.Models.Providers[provider]
		if stringx.String(providerConfig.APIKey).Trim() != "" {
			status.Configured = true
			status.Source = appcredential.CredentialSourceConfig
			return status, nil
		}
		if config.GetWebProviderConfigAPIKey(provider, cfg) != "" {
			status.Configured = true
			status.Source = appcredential.CredentialSourceConfig
		}
	}

	return status, nil
}

func formatAuthStatus(status appcredential.Status) string {
	if !status.Configured {
		return "missing"
	}

	switch status.Source {
	case appcredential.CredentialSourceStored:
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
	case appcredential.CredentialSourceEnvironment:
		return "environment"
	case appcredential.CredentialSourceConfig:
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
	if config.IsWebCredentialProvider(provider) {
		return getFirstEnvValue(config.WebProviderAPIKeyEnv(provider))
	}

	return "", ""
}

func getFirstEnvValue(keys []string) (string, string) {
	for _, key := range keys {
		key = stringx.String(key).Trim()
		if key == "" {
			continue
		}
		if value := stringx.String(os.Getenv(key)).Trim(); value != "" {
			return value, key
		}
	}
	return "", ""
}
