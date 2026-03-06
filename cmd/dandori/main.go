package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
	databaseURL := envOrDefault("DATABASE_URL", "postgres://dandori:dandori@localhost:5432/dandori?sslmode=disable")
	grpcPort := envOrDefault("GRPC_PORT", "7233")

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	log.Println("connected to database")

	if err := postgres.RunMigrations(context.Background(), db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}
	log.Println("migrations complete")

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
			log.Printf("activity timeout checker stopped: %v", err)
		}
	}()
	go func() {
		if err := bgWorker.RunHeartbeatTimeoutChecker(ctx, 5*time.Second); err != nil && ctx.Err() == nil {
			log.Printf("heartbeat timeout checker stopped: %v", err)
		}
	}()
	go func() {
		if err := bgWorker.RunTimerPoller(ctx, 1*time.Second); err != nil && ctx.Err() == nil {
			log.Printf("timer poller stopped: %v", err)
		}
	}()
	go func() {
		if err := bgWorker.RunTaskRecovery(ctx, 10*time.Second); err != nil && ctx.Err() == nil {
			log.Printf("task recovery stopped: %v", err)
		}
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	apiv1.RegisterDandoriServiceServer(srv, handler)
	reflection.Register(srv)

	go func() {
		log.Printf("dandori server listening on :%s", grpcPort)
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("received signal %v, shutting down...", sig)

	cancel()
	srv.GracefulStop()
	db.Close()
	log.Println("shutdown complete")
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
