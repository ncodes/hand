package e2e

import (
	"context"
	"errors"
	"strings"

	"github.com/openai/openai-go/v3/option"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/models"
)

var newLiveModelClient = models.NewOpenAIClient
var loadLiveConfig = config.Load
var newLiveHarness = NewHarness
var newLiveRPCHarness = NewRPCHarness

func NewLiveClients(cfg *config.Config) (models.Client, models.Client, error) {
	if cfg == nil {
		return nil, nil, errors.New("live harness config is required")
	}

	auth, err := cfg.ResolveModelAuth()
	if err != nil {
		return nil, nil, err
	}

	modelClient, err := newLiveModelClient(
		auth.APIKey,
		liveClientOptions(cfg.ModelBaseURL, cfg.ModelMaxRetriesEffective())...,
	)
	if err != nil {
		return nil, nil, err
	}

	summaryAuth, err := cfg.ResolveSummaryModelAuth()
	if err != nil {
		return nil, nil, err
	}
	if config.ModelAuthEqual(auth, summaryAuth) {
		return modelClient, modelClient, nil
	}

	summaryClient, err := newLiveModelClient(
		summaryAuth.APIKey,
		liveClientOptions(summaryAuth.BaseURL, cfg.ModelMaxRetriesEffective())...,
	)
	if err != nil {
		return nil, nil, err
	}

	return modelClient, summaryClient, nil
}

func NewLiveHarness(ctx context.Context, home, envFile, configFile string) (*Harness, error) {
	cfg, err := loadLiveConfig(strings.TrimSpace(envFile), strings.TrimSpace(configFile))
	if err != nil {
		return nil, err
	}

	modelClient, summaryClient, err := NewLiveClients(cfg)
	if err != nil {
		return nil, err
	}

	return newLiveHarness(ctx, HarnessOptions{
		Spec:          DefaultSpec(home),
		Config:        cfg,
		ModelClient:   modelClient,
		SummaryClient: summaryClient,
	})
}

func NewLiveRPCHarness(ctx context.Context, home, envFile, configFile string) (*RPCHarness, error) {
	cfg, err := loadLiveConfig(strings.TrimSpace(envFile), strings.TrimSpace(configFile))
	if err != nil {
		return nil, err
	}

	modelClient, summaryClient, err := NewLiveClients(cfg)
	if err != nil {
		return nil, err
	}

	return newLiveRPCHarness(ctx, HarnessOptions{
		Spec:          DefaultSpec(home),
		Config:        cfg,
		ModelClient:   modelClient,
		SummaryClient: summaryClient,
	})
}

func liveClientOptions(baseURL string, maxRetries int) []option.RequestOption {
	opts := make([]option.RequestOption, 0, 2)
	if strings.TrimSpace(baseURL) != "" {
		opts = append(opts, option.WithBaseURL(strings.TrimSpace(baseURL)))
	}
	opts = append(opts, option.WithMaxRetries(maxRetries))
	return opts
}
