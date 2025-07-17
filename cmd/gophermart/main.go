package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"gophermart/internal/app"
	"gophermart/internal/config"
	"gophermart/internal/middleware"
	"gophermart/internal/services"
	"gophermart/internal/storage"

	"golang.org/x/sync/errgroup"
)

func main() {
	cfg := config.Load()

	middleware.InitJWT(cfg.JWTSecret)

	db, err := sql.Open("pgx", cfg.DatabaseURI)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	storage := storage.NewStorage(db)
	if err := storage.InitDB(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	accrualService := services.NewAccrualService(
		&http.Client{Timeout: 10 * time.Second},
		cfg.AccrualSystemAddress,
	)

	application, err := app.NewApp(cfg, storage, accrualService)
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	for range cfg.NumWorkers {
		g.Go(func() error {
			return application.ProcessOrdersWorker(ctx)
		})
	}

	g.Go(func() error {
		log.Printf("Starting server on %s\n", cfg.RunAddress)
		return http.ListenAndServe(cfg.RunAddress, application.Router)
	})

	if err := g.Wait(); err != nil {
		log.Printf("Server stopped: %v\n", err)
	}
}