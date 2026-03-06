package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	apiv1 "github.com/asakaida/dandori/api/v1"
	grpcadapter "github.com/asakaida/dandori/internal/adapter/grpc"
	"github.com/asakaida/dandori/internal/adapter/postgres"
	"github.com/asakaida/dandori/internal/engine"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	databaseURL := envOrDefault("DATABASE_URL", "postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable")
	grpcPort := envOrDefault("GRPC_PORT", "7233")

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
	handler := grpcadapter.NewHandler(eng, eng, eng)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	apiv1.RegisterDandoriServiceServer(srv, handler)
	reflection.Register(srv)

	go func() {
		slog.Info("dandori server listening", "port", grpcPort)
		if err := srv.Serve(lis); err != nil {
			slog.Error("failed to serve", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("received signal, shutting down", "signal", sig.String())

	cancel()
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
