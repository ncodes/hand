package logutils

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"

	"github.com/wandxy/agent/internal/config"
)

func TestSetOutput_SetsCustomWriterAndDefaultsToStderr(t *testing.T) {
	original := loggerOutput
	t.Cleanup(func() {
		loggerOutput = original
	})

	buf := &bytes.Buffer{}
	SetOutput(buf)
	require.Same(t, buf, loggerOutput)

	SetOutput(nil)
	require.Same(t, os.Stderr, loggerOutput)
}

func TestConfigureLogger_DefaultsProgramNameAndWritesLog(t *testing.T) {
	originalOutput := loggerOutput
	originalLogger := log.Logger
	t.Cleanup(func() {
		loggerOutput = originalOutput
		log.Logger = originalLogger
	})

	buf := &bytes.Buffer{}
	SetOutput(buf)

	logger := ConfigureLogger("   ", true)
	require.NotNil(t, logger)

	log.Info().Msg("hello")

	output := buf.String()
	require.Contains(t, output, "hello")
	require.Contains(t, output, "program=agent")
	require.NotContains(t, output, "\x1b[")
}

func TestInitLogger_UsesCurrentNoColorSetting(t *testing.T) {
	originalOutput := loggerOutput
	originalLogger := log.Logger
	originalConfig := config.Get()
	t.Cleanup(func() {
		loggerOutput = originalOutput
		log.Logger = originalLogger
		config.Set(originalConfig)
	})

	config.Set(&config.Config{LogNoColor: true})
	buf := &bytes.Buffer{}
	SetOutput(buf)

	logger := InitLogger("svc")
	require.NotNil(t, logger)

	log.Info().Msg("hello")
	output := buf.String()
	require.Contains(t, output, "program=svc")
	require.NotContains(t, output, "\x1b[")
}

func TestGetLogger_UsesCurrentNoColorSetting(t *testing.T) {
	originalOutput := loggerOutput
	originalLogger := log.Logger
	originalConfig := config.Get()
	t.Cleanup(func() {
		loggerOutput = originalOutput
		log.Logger = originalLogger
		config.Set(originalConfig)
	})

	config.Set(&config.Config{LogNoColor: false})
	buf := &bytes.Buffer{}
	SetOutput(buf)

	logger := GetLogger("svc")
	require.NotNil(t, logger)

	log.Info().Msg("hello")
	output := buf.String()
	require.Contains(t, output, "program=svc")
}

func TestSetLogLevel_MapsLevels(t *testing.T) {
	original := zerolog.GlobalLevel()
	t.Cleanup(func() {
		zerolog.SetGlobalLevel(original)
	})

	tests := []struct {
		name     string
		input    string
		expected zerolog.Level
	}{
		{name: "debug", input: "debug", expected: zerolog.DebugLevel},
		{name: "warn", input: " warn ", expected: zerolog.WarnLevel},
		{name: "error", input: "ERROR", expected: zerolog.ErrorLevel},
		{name: "default", input: "invalid", expected: zerolog.InfoLevel},
		{name: "empty", input: "", expected: zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLogLevel(tt.input)
			require.Equal(t, tt.expected, zerolog.GlobalLevel())
		})
	}
}

func TestNewConsoleWriter_ConfiguresFields(t *testing.T) {
	out := &bytes.Buffer{}
	writer := newConsoleWriter(out, true)

	require.Same(t, out, writer.Out)
	require.Equal(t, time.RFC3339, writer.TimeFormat)
	require.True(t, writer.NoColor)
}

func TestCurrentNoColorSetting_UsesConfig(t *testing.T) {
	original := config.Get()
	t.Cleanup(func() {
		config.Set(original)
	})

	config.Set(nil)
	require.False(t, currentNoColorSetting())

	config.Set(&config.Config{LogNoColor: true})
	require.True(t, currentNoColorSetting())
}

func TestConfigureLogger_UsesConfiguredOutputWriter(t *testing.T) {
	originalOutput := loggerOutput
	originalLogger := log.Logger
	t.Cleanup(func() {
		loggerOutput = originalOutput
		log.Logger = originalLogger
	})

	reader, writer := io.Pipe()
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	SetOutput(writer)
	ConfigureLogger("svc", true)

	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 256)
		n, _ := reader.Read(buf)
		done <- string(buf[:n])
	}()

	log.Info().Msg("pipe-test")
	_ = writer.Close()

	output := <-done
	require.Contains(t, output, "pipe-test")
	require.Contains(t, output, "program=svc")
}
