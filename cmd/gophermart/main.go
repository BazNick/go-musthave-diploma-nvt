package main

import (
	"context"
	"log"
	"net/http"

	"gophermart/internal/app"
	"gophermart/internal/config"
	"golang.org/x/sync/errgroup"
)

func main() {
	cfg := config.Load()

	application, err := app.NewApp(cfg)
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}
	defer application.DB.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	for range 5 {
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