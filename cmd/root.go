package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/romitou/previsix/api"
	"github.com/romitou/previsix/config"
	"github.com/romitou/previsix/database"
	"github.com/romitou/previsix/scheduler"
	"github.com/romitou/previsix/scheduler/jobs"
	"github.com/romitou/previsix/web"
	"github.com/spf13/cobra"
)

var (
	configFile string
	debug      bool
)

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "config.yml", "config file, optional when every setting comes from the environment")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logs")
}

var rootCmd = &cobra.Command{
	Use:           "previsix",
	Short:         "Start the Previsix forecasts server",
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		level := slog.LevelInfo
		if debug {
			level = slog.LevelDebug
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

		if err := config.Load(configFile); err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		c := config.Get()

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		store, err := database.Connect(ctx, c.Database.URI, c.Database.Name)
		if err != nil {
			return fmt.Errorf("connecting to database: %w", err)
		}
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = store.Close(closeCtx)
		}()

		jobScheduler, err := scheduler.Start(ctx, c, jobs.NewRunner(c, store))
		if err != nil {
			return fmt.Errorf("starting scheduler: %w", err)
		}
		defer func() {
			if err := jobScheduler.Shutdown(); err != nil {
				slog.Error("cannot stop the scheduler", "error", err)
			}
		}()

		gin.SetMode(c.Server.Mode)
		server := &http.Server{
			Addr:              net.JoinHostPort(c.Server.Host, strconv.Itoa(c.Server.Port)),
			Handler:           api.NewRouter(store, web.Files(), time.Now),
			ReadHeaderTimeout: 10 * time.Second,
		}

		serverErr := make(chan error, 1)
		go func() {
			slog.Info("http server listening", "address", server.Addr)
			serverErr <- server.ListenAndServe()
		}()

		select {
		case err = <-serverErr:
			if !errors.Is(err, http.ErrServerClosed) {
				return fmt.Errorf("http server: %w", err)
			}
		case <-ctx.Done():
			slog.Info("shutting down")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err = server.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("stopping http server: %w", err)
			}
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
