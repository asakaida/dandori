package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	apiv1 "github.com/asakaida/dandori/api/v1"
	grpcadapter "github.com/asakaida/dandori/internal/adapter/grpc"
	httpadapter "github.com/asakaida/dandori/internal/adapter/http"
	"github.com/asakaida/dandori/internal/adapter/postgres"
	"github.com/asakaida/dandori/internal/adapter/telemetry"
	"github.com/asakaida/dandori/internal/engine"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	databaseURL := envOrDefault("DATABASE_URL", "postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable")
	grpcPort := envOrDefault("GRPC_PORT", "7233")
	httpPort := envOrDefault("HTTP_PORT", "8080")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// OpenTelemetry
	tracer, tracerShutdown, err := telemetry.InitTracer(ctx)
	if err != nil {
		slog.Error("failed to init tracer", "error", err)
		os.Exit(1)
	}
	defer tracerShutdown(context.Background())

	// Prometheus
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	reg.MustRegister(collectors.NewGoCollector())
	metrics := telemetry.NewMetrics(reg)

	// Database
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		slog.Error("failed to ping database", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")

	if err := postgres.RunMigrations(context.Background(), db); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations complete")

	store := postgres.New(db)
	eng := engine.New(
		store.Workflows(),
		store.Events(),
		store.WorkflowTasks(),
		store.ActivityTasks(),
		store.Timers(),
		store.Queries(),
		store.Namespaces(),
		store,
	)
	bgWorker := engine.NewBackgroundWorker(
		store.Workflows(),
		store.Events(),
		store.WorkflowTasks(),
		store.ActivityTasks(),
		store.Timers(),
		store,
	)

	// Decorators: engine -> tracing -> metrics -> handler
	tracedClient := telemetry.NewTracingClientService(eng, tracer)
	tracedWFTask := telemetry.NewTracingWorkflowTaskService(eng, tracer)
	tracedActTask := telemetry.NewTracingActivityTaskService(eng, tracer)

	metricsClient := telemetry.NewMetricsClientService(tracedClient, metrics)
	metricsWFTask := telemetry.NewMetricsWorkflowTaskService(tracedWFTask, metrics)
	metricsActTask := telemetry.NewMetricsActivityTaskService(tracedActTask, metrics)

	handler := grpcadapter.NewHandler(metricsClient, metricsWFTask, metricsActTask)

	// Background workers
	go func() {
		if err := bgWorker.RunActivityTimeoutChecker(ctx, 5*time.Second); err != nil && ctx.Err() == nil {
			slog.Error("activity timeout checker stopped", "error", err)
		}
	}()
	go func() {
		if err := bgWorker.RunHeartbeatTimeoutChecker(ctx, 5*time.Second); err != nil && ctx.Err() == nil {
			slog.Error("heartbeat timeout checker stopped", "error", err)
		}
	}()
	go func() {
		if err := bgWorker.RunTimerPoller(ctx, 1*time.Second); err != nil && ctx.Err() == nil {
			slog.Error("timer poller stopped", "error", err)
		}
	}()
	go func() {
		if err := bgWorker.RunTaskRecovery(ctx, 10*time.Second); err != nil && ctx.Err() == nil {
			slog.Error("task recovery stopped", "error", err)
		}
	}()

	// gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	srv := grpc.NewServer(grpcadapter.OTelServerOptions()...)
	apiv1.RegisterDandoriServiceServer(srv, handler)
	grpc_health_v1.RegisterHealthServer(srv, grpcadapter.NewHealthServer(db))
	reflection.Register(srv)

	go func() {
		slog.Info("dandori gRPC server listening", "port", grpcPort)
		if err := srv.Serve(lis); err != nil {
			slog.Error("failed to serve gRPC", "error", err)
			os.Exit(1)
		}
	}()

	// HTTP server (gRPC-Gateway + /healthz + /metrics)
	grpcAddr := fmt.Sprintf("localhost:%s", grpcPort)
	gatewayMux, err := httpadapter.NewGatewayMux(ctx, grpcAddr)
	if err != nil {
		slog.Error("failed to create gateway mux", "error", err)
		os.Exit(1)
	}

	httpHandler := httpadapter.NewHTTPHandler(gatewayMux, map[string]http.Handler{
		"/healthz": httpadapter.NewHealthHandler(db),
		"/metrics": promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
		"/ui/":     httpadapter.NewUIHandler(),
	})
	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%s", httpPort),
		Handler: httpHandler,
	}

	go func() {
		slog.Info("dandori HTTP server listening", "port", httpPort)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to serve HTTP", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("received signal, shutting down", "signal", sig.String())

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	httpSrv.Shutdown(shutdownCtx)

	srv.GracefulStop()
	db.Close()
	slog.Info("shutdown complete")
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
