package doctor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/pkg/logutils"
)

func init() {
	logutils.SetOutput(io.Discard)
}

func Test_E2E_DoctorCommand_ConfigPassAndFail(t *testing.T) {
	t.Run("passes for valid config", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  key: config-key
  verify: false
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
`), 0o600))

		output, err := runDoctorCommand(t, "hand", "--config", configPath, "doctor")
		require.NoError(t, err)
		assert.Contains(t, output, "config validation: configuration is valid")
		assert.Contains(t, output, "doctor checks passed")
	})

	t.Run("fails clearly for invalid config", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(`
name: config-agent
models:
  main:
    name: openai/gpt-4o-mini
    provider: openrouter
search:
  vector:
    enabled: false
`), 0o600))

		output, err := runDoctorCommand(t, "hand", "--config", configPath, "doctor")
		fmt.Println(output)
		require.EqualError(t, err, "doctor checks failed: config validation: model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key; model auth: model key is required; set HAND_MODEL_KEY, provide it in config, or use --model.key")
		assert.Contains(t, output, "config validation")
		assert.Contains(t, output, "model auth")
	})
}

func runDoctorCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	originalOutput := doctorOutput
	t.Cleanup(func() {
		doctorOutput = originalOutput
	})

	var output bytes.Buffer
	doctorOutput = &output

	envFile := ".env"
	configFile := "config.yaml"

	cmd := &cli.Command{
		Name:  "hand",
		Flags: handcli.RootFlags(&envFile, &configFile),
		Commands: []*cli.Command{
			NewCommand(),
		},
	}

	err := cmd.Run(context.Background(), args)
	return output.String(), err
}
