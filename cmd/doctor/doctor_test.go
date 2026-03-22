package doctor

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	"github.com/wandxy/hand/internal/config"
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
		"agent",
		"--name", "flag-agent",
		"--model", "flag-model",
		"--model.router", "openrouter",
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
		"agent",
		"--name", "flag-agent",
		"--model", "flag-model",
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
		"agent",
		"--name", "flag-agent",
		"--model", "flag-model",
		"--model.router", "openrouter",
		"--model.key", "flag-key",
		"--log.no-color",
		"doctor",
	})
	require.NoError(t, err)
	require.Contains(t, output.String(), "[PASS] config validation: configuration is valid")
	require.NotRegexp(t, regexp.MustCompile(`\x1b\[[0-9;]*m`), output.String())
}

func newRootCommandForTest() *cli.Command {
	return &cli.Command{
		Name: "agent",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "env-file", Value: ".env"},
			&cli.StringFlag{Name: "config", Value: "config.yaml"},
			&cli.StringFlag{Name: "name", Value: config.Get().Name},
			&cli.StringFlag{Name: "model.router", Value: config.Get().ModelRouter},
			&cli.StringFlag{Name: "model.key"},
			&cli.StringFlag{Name: "model", Value: config.Get().Model},
			&cli.StringFlag{Name: "model.base-url", Value: config.Get().ModelBaseURL},
			&cli.StringFlag{Name: "log.level", Value: config.Get().LogLevel},
			&cli.BoolFlag{Name: "log.no-color"},
			&cli.BoolFlag{Name: "debug.requests"},
		},
		Commands: []*cli.Command{
			NewCommand(),
		},
	}
}
