package logutils

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/wandxy/agent/internal/config"
)

var loggerMu sync.Mutex
var loggerOutput io.Writer = os.Stderr

func InitLogger(programName string) *zerolog.Logger {
	return ConfigureLogger(programName, currentNoColorSetting())
}

func SetOutput(out io.Writer) {
	loggerMu.Lock()
	defer loggerMu.Unlock()

	if out == nil {
		loggerOutput = os.Stderr
		return
	}

	loggerOutput = out
}

func ConfigureLogger(programName string, noColor bool) *zerolog.Logger {
	if strings.TrimSpace(programName) == "" {
		programName = "agent"
	}

	loggerMu.Lock()
	defer loggerMu.Unlock()

	log.Logger = log.Output(newConsoleWriter(loggerOutput, noColor)).With().
		Str("program", programName).
		Logger()

	return &log.Logger
}

func GetLogger(programName string) *zerolog.Logger {
	return ConfigureLogger(programName, currentNoColorSetting())
}

func SetLogLevel(level string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

func newConsoleWriter(out io.Writer, noColor bool) zerolog.ConsoleWriter {
	return zerolog.ConsoleWriter{
		Out:        out,
		TimeFormat: time.RFC3339,
		NoColor:    noColor,
	}
}

func currentNoColorSetting() bool {
	cfg := config.Get()
	return cfg != nil && cfg.LogNoColor
}
