package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hazz-dev/esxport/internal/collector"
	"github.com/hazz-dev/esxport/internal/config"
	"github.com/hazz-dev/esxport/internal/health"
	"github.com/hazz-dev/esxport/internal/version"
	"github.com/hazz-dev/esxport/internal/vsphere"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "esxport",
		Short: "VMware vSphere/ESXi Prometheus exporter",
		Long:  "esxport is a modern Prometheus exporter for VMware vSphere and ESXi hosts.",
	}

	root.AddCommand(serveCmd())
	root.AddCommand(checkCmd())
	root.AddCommand(versionCmd())

	return root
}

func serveCmd() *cobra.Command {
	var (
		configFile string
		port       int
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP metrics server",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

			cfg, err := loadConfig(configFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if port != 0 {
				cfg.Server.Port = port
			}

			timeout, err := cfg.Scrape.TimeoutDuration()
			if err != nil {
				return fmt.Errorf("parsing scrape timeout: %w", err)
			}

			pool := vsphere.NewPool(vsphere.Connect)

			coll := collector.New(cfg.Targets, timeout, logger)
			coll.SetPool(pool)

			healthHandler := health.NewHandler()
			coll.SetOnScrapeComplete(func(target string, success bool) {
				if success {
					healthHandler.RecordSuccess(target)
				} else {
					healthHandler.RecordFailure(target)
				}
			})

			prometheus.MustRegister(coll)

			metricsPath := cfg.Server.MetricsPath
			if metricsPath == "" {
				metricsPath = "/metrics"
			}

			mux := http.NewServeMux()
			mux.Handle(metricsPath, promhttp.Handler())
			mux.HandleFunc("/healthz", healthHandler.Healthz)
			mux.HandleFunc("/readyz", healthHandler.Readyz)
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `<html><head><title>esxport</title></head><body>
<h1>esxport - VMware vSphere Exporter</h1>
<p><a href="%s">Metrics</a></p>
</body></html>`, metricsPath)
			})

			addr := fmt.Sprintf(":%d", cfg.Server.Port)
			logger.Info("starting esxport", "address", addr, "metrics_path", metricsPath)

			server := &http.Server{
				Addr:         addr,
				Handler:      mux,
				ReadTimeout:  10 * time.Second,
				WriteTimeout: timeout + 5*time.Second,
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			return runServer(ctx, server, func() {
				logger.Info("shutting down, closing connection pool")
				pool.CloseAll()
			})
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "HTTP server port (overrides config)")

	return cmd
}

func checkCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "One-shot: connect, collect metrics, print to stdout",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

			cfg, err := loadConfig(configFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			timeout, err := cfg.Scrape.TimeoutDuration()
			if err != nil {
				return fmt.Errorf("parsing scrape timeout: %w", err)
			}

			coll := collector.New(cfg.Targets, timeout, logger)

			// Collect and print metrics
			reg := prometheus.NewRegistry()
			reg.MustRegister(coll)

			mfs, err := reg.Gather()
			if err != nil {
				return fmt.Errorf("gathering metrics: %w", err)
			}

			for _, mf := range mfs {
				fmt.Printf("# HELP %s %s\n", mf.GetName(), mf.GetHelp())
				fmt.Printf("# TYPE %s %s\n", mf.GetName(), mf.GetType().String())
				for _, m := range mf.GetMetric() {
					labels := ""
					for i, l := range m.GetLabel() {
						if i > 0 {
							labels += ","
						}
						labels += fmt.Sprintf(`%s="%s"`, l.GetName(), l.GetValue())
					}
					if labels != "" {
						labels = "{" + labels + "}"
					}
					if m.GetGauge() != nil {
						fmt.Printf("%s%s %g\n", mf.GetName(), labels, m.GetGauge().GetValue())
					} else if m.GetCounter() != nil {
						fmt.Printf("%s%s %g\n", mf.GetName(), labels, m.GetCounter().GetValue())
					}
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configFile, "config", "c", "", "Path to config file")

	return cmd
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("esxport %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.BuildDate)
		},
	}
}

// runServer starts the HTTP server and blocks until the context is cancelled.
// On cancellation it gracefully shuts down the server and calls cleanup.
func runServer(ctx context.Context, server *http.Server, cleanup func()) error {
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", server.Addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down server: %w", err)
	}

	cleanup()
	return nil
}

func loadConfig(configFile string) (*config.Config, error) {
	if configFile != "" {
		return config.LoadFile(configFile)
	}
	// Try default locations
	for _, path := range []string{"config.yml", "config.yaml", "/etc/esxport/config.yml"} {
		if _, err := os.Stat(path); err == nil {
			return config.LoadFile(path)
		}
	}
	// Fall back to env vars only
	cfg := config.LoadFromEnv()
	if len(cfg.Targets) == 0 {
		return nil, fmt.Errorf("no config file found and ESXPORT_HOST not set; use --config or set environment variables")
	}
	return cfg, nil
}
