package logutils

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/constants"
	"github.com/wandxy/hand/internal/datadir"
)

const defaultLogFilename = "hand.log"

type mkdirAllFunc func(string, os.FileMode) error

type newLogFileWriterFunc func(logFileSettings) (io.WriteCloser, error)

type logFileSettings struct {
	path       string
	maxSizeMB  int
	maxBackups int
	maxAgeDays int
	compress   bool
}

func getFileOutputLocked() io.Writer {
	settings := getLogFileSettings()
	if fileOutput != nil {
		if fileSettings.path == "" || fileSettings == settings {
			return fileOutput
		}
		closeFileLocked()
		fileOutput = nil
		fileSettings = logFileSettings{}
	}
	if !defaultFileEnabled() {
		return nil
	}

	if err := mkdirAll(filepath.Dir(settings.path), 0o700); err != nil {
		return nil
	}

	file, err := newLogFileWriter(settings)
	if err != nil {
		return nil
	}

	fileOutput = file
	fileSettings = settings
	fileCloser = file

	return fileOutput
}

func newLumberjackFileWriter(settings logFileSettings) (io.WriteCloser, error) {
	return &lumberjack.Logger{
		Filename:   settings.path,
		MaxSize:    settings.maxSizeMB,
		MaxBackups: settings.maxBackups,
		MaxAge:     settings.maxAgeDays,
		Compress:   settings.compress,
	}, nil
}

func getLogFileSettings() logFileSettings {
	cfg := config.Get()
	settings := logFileSettings{
		path:       getLogFilePath(),
		maxSizeMB:  constants.DefaultLogMaxSizeMB,
		maxBackups: constants.DefaultLogMaxBackups,
		maxAgeDays: constants.DefaultLogMaxAgeDays,
		compress:   constants.DefaultLogCompress,
	}
	if cfg == nil {
		return settings
	}
	if cfg.Log.MaxSizeMB > 0 {
		settings.maxSizeMB = cfg.Log.MaxSizeMB
	}
	if cfg.Log.MaxBackups > 0 {
		settings.maxBackups = cfg.Log.MaxBackups
	}
	if cfg.Log.MaxAgeDays > 0 {
		settings.maxAgeDays = cfg.Log.MaxAgeDays
	}
	settings.compress = cfg.Log.Compress

	return settings
}

func getLogFilePath() string {
	if cfg := config.Get(); cfg != nil {
		if path := strings.TrimSpace(cfg.Log.File); path != "" {
			return path
		}
	}

	return filepath.Join(datadir.HomeDir(), defaultLogFilename)
}

func closeFileLocked() {
	if fileCloser != nil {
		_ = fileCloser.Close()
	}
	fileCloser = nil
}
