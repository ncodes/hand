package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wandxy/hand/internal/config"
	models "github.com/wandxy/hand/internal/model"
	modelclient "github.com/wandxy/hand/internal/model/client"
)

type liveModelClientFactory interface {
	NewClient(modelclient.ClientRequest) (models.Client, error)
}

var liveModelClientFactoryInstance liveModelClientFactory = modelclient.NewDefaultClientFactory()
var loadLiveConfig = config.Load
var newLiveHarness = NewHarness
var newLiveRPCHarness = NewRPCHarness
var writeLiveArtifactFile = os.WriteFile
var mkdirAllLiveArtifacts = os.MkdirAll
var liveNow = time.Now

const (
	LiveClassificationPassed            = "passed"
	LiveClassificationCommandError      = "command_error"
	LiveClassificationExpectationFailed = "expectation_failed"
)

// LiveArtifact describes an artifact written during a live e2e run.
type LiveArtifact struct {
	Scenario       string    `json:"scenario"`
	Prompt         string    `json:"prompt"`
	Output         string    `json:"output,omitempty"`
	Error          string    `json:"error,omitempty"`
	Classification string    `json:"classification"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
}

// NewLiveClients returns live model clients for e2e scenarios.
func NewLiveClients(cfg *config.Config) (models.Client, models.Client, error) {
	if cfg == nil {
		return nil, nil, errors.New("live harness config is required")
	}

	auth, err := cfg.ResolveModelAuth()
	if err != nil {
		return nil, nil, err
	}

	modelClient, err := liveModelClientFactoryInstance.NewClient(liveClientRequest(modelclient.ModelRoleMain, cfg.Models.Main.Name, auth, cfg.ModelMaxRetriesEffective()))
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

	summaryClient, err := liveModelClientFactoryInstance.NewClient(liveClientRequest(modelclient.ModelRoleSummary, cfg.SummaryModelEffective(), summaryAuth, cfg.ModelMaxRetriesEffective()))
	if err != nil {
		return nil, nil, err
	}

	return modelClient, summaryClient, nil
}

// NewLiveHarness returns an e2e harness wired to live model clients.
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

// NewLiveRPCHarness returns an RPC e2e harness wired to live model clients.
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

func liveClientRequest(
	role modelclient.ModelRole,
	model string,
	auth config.ModelAuth,
	maxRetries int,
) modelclient.ClientRequest {
	return modelclient.ClientRequest{
		Role:       role,
		Model:      model,
		Provider:   auth.Provider,
		API:        auth.API,
		APIKey:     auth.APIKey,
		BaseURL:    auth.BaseURL,
		Headers:    auth.Headers,
		MaxRetries: maxRetries,
	}
}

// DefaultLiveArtifactDir returns the directory used for live e2e artifacts.
func DefaultLiveArtifactDir(override string) string {
	if strings.TrimSpace(override) != "" {
		return strings.TrimSpace(override)
	}

	return filepath.Join(os.TempDir(), "hand-live-artifacts")
}

// RunLiveScenario runs live scenario.
func RunLiveScenario(
	name string,
	prompt string,
	artifactDir string,
	run func(string) (string, error),
	check func(string) error,
) (LiveArtifact, error) {
	artifact := LiveArtifact{
		Scenario:  strings.TrimSpace(name),
		Prompt:    strings.TrimSpace(prompt),
		StartedAt: liveNow().UTC(),
	}

	output, runErr := run(prompt)
	artifact.Output = strings.TrimSpace(output)
	artifact.FinishedAt = liveNow().UTC()

	if runErr != nil {
		artifact.Classification = LiveClassificationCommandError
		artifact.Error = runErr.Error()
		writeErr := WriteLiveArtifact(artifactDir, artifact)
		if writeErr != nil {
			return artifact, errors.Join(runErr, writeErr)
		}
		return artifact, runErr
	}

	if checkErr := checkOutput(check, artifact.Output); checkErr != nil {
		artifact.Classification = LiveClassificationExpectationFailed
		artifact.Error = checkErr.Error()
		writeErr := WriteLiveArtifact(artifactDir, artifact)
		if writeErr != nil {
			return artifact, errors.Join(checkErr, writeErr)
		}
		return artifact, checkErr
	}

	artifact.Classification = LiveClassificationPassed
	if err := WriteLiveArtifact(artifactDir, artifact); err != nil {
		return artifact, err
	}

	return artifact, nil
}

// WriteLiveArtifact describes an artifact written during a live e2e run.
func WriteLiveArtifact(dir string, artifact LiveArtifact) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	if err := mkdirAllLiveArtifacts(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}

	filename := sanitizeLiveArtifactName(artifact.Scenario)
	if filename == "" {
		filename = "live-scenario"
	}

	return writeLiveArtifactFile(filepath.Join(dir, filename+".json"), data, 0o600)
}

func sanitizeLiveArtifactName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(builder.String(), "-")
}

func checkOutput(check func(string) error, output string) error {
	if check == nil {
		return nil
	}

	return check(strings.TrimSpace(output))
}
