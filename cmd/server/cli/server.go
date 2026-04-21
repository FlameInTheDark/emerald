package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/FlameInTheDark/emerald/internal/api"
	"github.com/FlameInTheDark/emerald/internal/auth"
	"github.com/urfave/cli/v3"
)

func RunServer(ctx context.Context, cmd *cli.Command) error {
	runtime, err := newCLIRuntime(ctx, cliRuntimeOptions{migrate: true})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			log.Printf("failed to close runtime: %v", closeErr)
		}
	}()

	if port := cmd.String("port"); port != "" {
		runtime.Config.Server.Port = port
	}
	if host := cmd.String("host"); host != "" {
		runtime.Config.Server.Host = host
	}
	if err := runtime.Config.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	if err := runtime.UserStore.EnsureDefaultUser(ctx, runtime.Config.Auth.Username, runtime.Config.Auth.Password); err != nil {
		return fmt.Errorf("ensure default user: %w", err)
	}

	if err := runtime.startServerServices(ctx); err != nil {
		return err
	}

	authService := auth.NewService(runtime.UserStore, auth.Config{
		SessionTTL:   runtime.Config.Auth.SessionTTL,
		CookieName:   runtime.Config.Auth.CookieName,
		SessionStore: runtime.UserSessionStore,
	})

	app := api.New(api.Config{
		DB:              runtime.Database,
		Scheduler:       runtime.CronScheduler,
		ChannelService:  runtime.ChannelService,
		EncryptionKey:   runtime.EncryptionKey,
		ExecutionRunner: runtime.ExecutionRunner,
		WSHub:           runtime.WSHub,
		SkillStore:      runtime.SkillStore,
		ShellRunner:     runtime.ShellRunner,
		AuthService:     authService,
		NodeDefinitions: runtime.NodeDefinitionService,
		SecretStore:     runtime.SecretStore,
		TriggerService:  runtime.TriggerService,
		AuditLogStore:   runtime.AuditLogStore,
		Security:        runtime.Config.Security,
	})

	serverErrCh := make(chan error, 1)
	listenAddr := net.JoinHostPort(runtime.Config.Server.Host, runtime.Config.Server.Port)
	go func() {
		serverErrCh <- app.Listen(listenAddr)
	}()

	log.Printf("server started on %s", listenAddr)

	signalCtx, stopSignals := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		log.Println("shutting down server...")
	case err := <-serverErrCh:
		if err != nil {
			return fmt.Errorf("server failed: %w", err)
		}
		return nil
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("failed to shutdown server cleanly: %v", err)
	}

	select {
	case err := <-serverErrCh:
		if err != nil {
			log.Printf("server stopped with error: %v", err)
		}
	case <-time.After(2 * time.Second):
		log.Printf("timed out waiting for server listener to stop")
	}

	return nil
}
