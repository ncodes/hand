package doctor

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func TestNewCommand_PrintsPassingReport(t *testing.T) {
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"doctor",
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "[\x1b[32mPASS\x1b[0m] config validation: configuration is valid")
	require.Contains(t, output.String(), "doctor checks passed")
}

func TestNewCommand_PrintsFailureReport(t *testing.T) {
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"doctor",
	})
	require.EqualError(t, err, "doctor checks failed: config validation: model key is required; set MODEL_KEY, provide it in config, or use --model.key; model auth: model key is required; set MODEL_KEY, provide it in config, or use --model.key")
	require.Contains(t, output.String(), "[\x1b[31mFAIL\x1b[0m] config validation")
	require.Contains(t, output.String(), "[\x1b[31mFAIL\x1b[0m] model auth")
}

func TestNewCommand_DisablesColorWhenRequested(t *testing.T) {
	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	cmd := newRootCommandForTest()
	err := cmd.Run(context.Background(), []string{
		"hand",
		"--name", "flag-agent",
		"--model", "openai/gpt-4o-mini",
		"--model.provider", "openrouter",
		"--model.key", "flag-key",
		"--log.no-color",
		"doctor",
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "[PASS] config validation: configuration is valid")
	require.NotRegexp(t, regexp.MustCompile(`\x1b\[[0-9;]*m`), output.String())
}

func newRootCommandForTest() *cli.Command {
	envFile := ".env"
	configFile := "config.yaml"

	return &cli.Command{
		Name:  "hand",
		Flags: handcli.RootFlags(&envFile, &configFile),
		Commands: []*cli.Command{
			NewCommand(),
		},
	}
}
