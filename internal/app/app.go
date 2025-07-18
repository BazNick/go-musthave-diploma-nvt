package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"gophermart/internal/config"
	"gophermart/internal/handlers"
	md "gophermart/internal/middleware"
	"gophermart/internal/services"
	"gophermart/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type App struct {
	DB      *sql.DB
	Router  *chi.Mux
	Config  config.Config
	Storage storage.Storage
	Accrual *services.AccrualService
}

func NewApp(cfg config.Config, storage storage.Storage, accrual *services.AccrualService) (*App, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURI)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err = db.PingContext(ctx); err != nil {
		return nil, err
	}

	if err := storage.InitDB(); err != nil {
		return nil, err
	}

	app := &App{
		DB:      db,
		Config:  cfg,
		Storage: storage,
		Accrual: accrual,
	}

	app.initRouter()
	return app, nil
}

func (a *App) initRouter() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	authHandler := handlers.NewAuthHandler(a.Storage)
	r.Post("/api/user/register", authHandler.Register)
	r.Post("/api/user/login", authHandler.Login)

	orderHandler := handlers.NewOrderHandler(a.Storage)
	balanceHandler := handlers.NewBalanceHandler(a.Storage)

	r.Group(func(r chi.Router) {
		r.Use(md.Verifier())
		r.Use(md.Authenticator())

		r.Post("/api/user/orders", orderHandler.UploadOrder)
		r.Get("/api/user/orders", orderHandler.GetOrders)
		r.Get("/api/user/balance", balanceHandler.GetBalance)
		r.Post("/api/user/balance/withdraw", balanceHandler.Withdraw)
		r.Get("/api/user/withdrawals", balanceHandler.GetWithdrawals)
	})

	a.Router = r
}

func (a *App) ProcessOrdersWorker(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := a.processOrdersBatch(ctx); err != nil {
				log.Printf("Order processing error: %v", err)
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (a *App) processOrdersBatch(ctx context.Context) error {
	orders, err := a.Storage.GetPendingOrders(ctx, 10)
	if err != nil {
		return fmt.Errorf("get pending orders: %w", err)
	}
	if len(orders) == 0 {
		return nil
	}

	userIDs, err := a.Storage.SetOrderProcessing(ctx, orders)
	if err != nil {
		return fmt.Errorf("set processing status: %w", err)
	}

	for _, number := range orders {
		accrual, err := a.Accrual.GetAccrual(ctx, number)
		if err != nil {
			log.Printf("Failed to get accrual for order %s: %v", number, err)
			continue
		}

		if err := a.Storage.UpdateOrder(ctx, number, accrual.Status, accrual.Accrual); err != nil {
			log.Printf("Failed to update order %s: %v", number, err)
			continue
		}

		if accrual.Status == "PROCESSED" && accrual.Accrual > 0 {
			userID := userIDs[number]
			if _, err := a.DB.ExecContext(ctx,
				`INSERT INTO balances (user_id, current, withdrawn) 
				 VALUES ($1, $2, 0)
				 ON CONFLICT (user_id) DO UPDATE 
				 SET current = balances.current + EXCLUDED.current`,
				userID, accrual.Accrual,
			); err != nil {
				log.Printf("Failed to update balance for user %d: %v", userID, err)
			}
		}
	}
	return nil
}