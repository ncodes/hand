package config

import (
	"path/filepath"
	"strings"

	"github.com/wandxy/morph/internal/constants"
	"github.com/wandxy/morph/internal/datadir"
	"github.com/wandxy/morph/pkg/logutils"
)

func init() {
	logutils.SetConfigProvider(func() logutils.Config {
		cfg := Get()
		settings := logutils.Config{
			LogFile:    filepath.Join(datadir.HomeDir(), "morph.log"),
			MaxSizeMB:  constants.DefaultLogMaxSizeMB,
			MaxBackups: constants.DefaultLogMaxBackups,
			MaxAgeDays: constants.DefaultLogMaxAgeDays,
			Compress:   constants.DefaultLogCompress,
		}
		if cfg == nil {
			return settings
		}

		settings.NoColor = cfg.Log.NoColor
		if path := strings.TrimSpace(cfg.Log.File); path != "" {
			settings.LogFile = path
		}
		settings.MaxSizeMB = cfg.Log.MaxSizeMB
		settings.MaxBackups = cfg.Log.MaxBackups
		settings.MaxAgeDays = cfg.Log.MaxAgeDays
		settings.Compress = cfg.Log.Compress

		return settings
	})
}
