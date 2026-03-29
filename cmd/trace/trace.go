package trace

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	cli "github.com/urfave/cli/v3"

	handcli "github.com/wandxy/hand/internal/cli"
	"github.com/wandxy/hand/internal/config"
	"github.com/wandxy/hand/internal/datadir"
	"github.com/wandxy/hand/internal/trace/inspect"
	"github.com/wandxy/hand/pkg/logutils"
)

func NewCommand() *cli.Command {
	return &cli.Command{
		Name:  "trace",
		Usage: "Inspect stored session traces",
		Commands: []*cli.Command{
			newViewCommand(),
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return cli.ShowSubcommandHelp(cmd)
		},
	}
}

func newViewCommand() *cli.Command {
	return &cli.Command{
		Name:  "view",
		Usage: "Serve the local trace session viewer",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "trace-dir",
				Usage: "Directory containing trace session .jsonl files",
			},
			&cli.StringFlag{
				Name:  "listen",
				Usage: "Listener address for the local trace viewer",
				Value: "127.0.0.1:0",
			},
			&cli.StringFlag{
				Name:  "username",
				Usage: "Basic auth username for the trace viewer",
			},
			&cli.StringFlag{
				Name:  "password",
				Usage: "Basic auth password for the trace viewer",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			cfg, err := config.Load(cmd.String("env-file"), cmd.String("config"))
			if err != nil {
				return err
			}
			handcli.ApplyConfigOverrides(cmd, cfg)
			config.Set(cfg)
			_ = logutils.ConfigureLogger("hand", cfg.LogNoColor)
			logutils.SetLogLevel(cfg.LogLevel)

			traceDir := strings.TrimSpace(cmd.String("trace-dir"))
			if traceDir == "" {
				traceDir = strings.TrimSpace(cfg.DebugTraceDir)
			}
			if traceDir == "" {
				traceDir = datadir.DebugTraceDir()
			}

			app := inspect.NewApp(traceDir)
			if err := app.Validate(); err != nil {
				return err
			}
			username := strings.TrimSpace(cmd.String("username"))
			password := cmd.String("password")
			if (username == "") != (password == "") {
				return fmt.Errorf("trace viewer basic auth requires both username and password")
			}
			if username != "" {
				app.SetBasicAuth(username, password)
			}

			listenAddr := strings.TrimSpace(cmd.String("listen"))
			if listenAddr == "" {
				listenAddr = "127.0.0.1:0"
			}

			log.Info().
				Str("traceDir", traceDir).
				Str("listen", listenAddr).
				Msg("Starting trace viewer")

			return serve(ctx, app, traceDir, listenAddr)
		},
	}
}

func serve(ctx context.Context, app *inspect.App, traceDir string, listenAddr string) error {
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	defer listener.Close()

	server := &http.Server{
		Handler: app.Handler(),
	}

	url := fmt.Sprintf("http://%s", listener.Addr().String())
	log.Info().
		Str("traceDir", traceDir).
		Str("listen", listener.Addr().String()).
		Str("url", url).
		Msg("Trace viewer listening")

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.Serve(listener)
	}()

	select {
	case err := <-serverErr:
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		err := <-serverErr
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
