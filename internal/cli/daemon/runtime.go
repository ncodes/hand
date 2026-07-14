package daemon

import (
	"context"
	"fmt"

	"github.com/wandxy/morph/internal/config"
	"github.com/wandxy/morph/internal/permissions"
	"github.com/wandxy/morph/pkg/logutils"
)

func runDaemonOnce(ctx context.Context, cfg *config.Config) error {
	if err := checkDaemonStartPermission(ctx, cfg); err != nil {
		return err
	}

	config.Set(cfg)
	_ = logutils.ConfigureLogger("morph", cfg.Log.NoColor)
	logutils.SetLogLevel(cfg.Log.Level)

	runtimeCfg := prepareDaemonRuntimeConfig(cfg)
	config.Set(runtimeCfg)

	if _, err := fmt.Fprint(startupOutput, renderStartupPanel(runtimeCfg)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(startupOutput); err != nil {
		return err
	}

	daemonLog.Info().Msg("Configuration loaded")
	if runtimeCfg.Search.Vector.Enabled {
		daemonLog.Info().Msg("Vector retrieval configured")
	}

	daemonLog.Info().Msg("Starting Morph services")

	modelClient, summaryClient, rerankerClient, err := buildDaemonModelClients(runtimeCfg)
	if err != nil {
		return err
	}

	lis, err := openRPCListener(runtimeCfg)
	if err != nil {
		return err
	}
	defer lis.Close()

	agent := newAgentRunner(ctx, runtimeCfg, modelClient, summaryClient, rerankerClient)
	if err := agent.Start(ctx); err != nil {
		_ = lis.Close()
		return err
	}

	err = serveDaemonServices(ctx, runtimeCfg, agent, lis)
	if closer, ok := agent.(closeableAgentRunner); ok {
		if closeErr := closer.Close(); err == nil {
			if isMissingCredentialLockError(closeErr) {
				daemonLog.Debug().Err(closeErr).Msg("Ignoring missing credential lock during shutdown")
			} else {
				err = closeErr
			}
		}
	}

	return err
}

func checkDaemonStartPermission(ctx context.Context, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if _, ok := permissions.FromContext(ctx); !ok {
		ctx = permissions.WithContext(ctx, permissions.AuthorizationContext{
			Actor:   permissions.Actor{Kind: permissions.ActorLocalOwner},
			Surface: permissions.SurfaceCLI,
		})
	}

	engine := permissions.NewEngine(cfg.Permissions)
	_, err := engine.Check(ctx, permissions.EvaluationInput{Operation: permissions.Operation{
		Resource:      permissions.ResourceDaemon,
		Action:        permissions.ActionStart,
		Effects:       []permissions.Effect{permissions.EffectPrivilegeChanging, permissions.EffectWrite},
		Target:        "daemon",
		OwnerRequired: true,
	}})
	return err
}
