package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
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
	Client  *http.Client
	Storage *storage.Storage
	Accrual *services.AccrualService
}

func NewApp(cfg config.Config) (*App, error) {
	db, err := sql.Open("pgx", cfg.DatabaseURI)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err = db.PingContext(ctx); err != nil {
		return nil, err
	}

	storage := storage.NewStorage(db)
	accrualService := services.NewAccrualService(&http.Client{Timeout: 10 * time.Second}, cfg.AccrualSystemAddress)
	
	app := &App{
		DB:      db,
		Config:  cfg,
		Client:  &http.Client{Timeout: 10 * time.Second},
		Storage: storage,
		Accrual: accrualService,
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

	orderHandler := handlers.NewOrderHandler(a.Storage, a.Accrual)
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
			err := a.processPendingOrders()
			if err != nil {
				log.Printf("Error processing orders: %v", err)
			}
			time.Sleep(1 * time.Second)
		}
	}
}

func (a *App) processPendingOrders() error {
	rows, err := a.DB.Query(
		"SELECT number FROM orders WHERE status IN ('NEW', 'PROCESSING') ORDER BY uploaded_at LIMIT 10",
	)
	if err != nil {
		return fmt.Errorf("failed to get pending orders: %w", err)
	}
	defer rows.Close()

	var orders []string
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return fmt.Errorf("failed to scan order number: %w", err)
		}
		orders = append(orders, number)
	}

	if len(orders) == 0 {
		return nil
	}

	for _, orderNumber := range orders {
		_, err := a.DB.Exec(
			"UPDATE orders SET status = 'PROCESSING' WHERE number = $1 AND status = 'NEW'",
			orderNumber,
		)
		if err != nil {
			log.Printf("Failed to update order %s to PROCESSING: %v", orderNumber, err)
			continue
		}

		accrual, err := a.Accrual.GetAccrual(context.Background(), orderNumber)
		if err != nil {
			log.Printf("Failed to query accrual for order %s: %v", orderNumber, err)
			continue
		}

		tx, err := a.DB.Begin()
		if err != nil {
			log.Printf("Failed to begin transaction for order %s: %v", orderNumber, err)
			continue
		}

		var userID int
		err = tx.QueryRow(
			"SELECT user_id FROM orders WHERE number = $1",
			orderNumber,
		).Scan(&userID)
		if err != nil {
			tx.Rollback()
			log.Printf("Failed to get user ID for order %s: %v", orderNumber, err)
			continue
		}

		_, err = tx.Exec(
			"UPDATE orders SET status = $1, accrual = $2 WHERE number = $3",
			accrual.Status, accrual.Accrual, orderNumber,
		)
		if err != nil {
			tx.Rollback()
			log.Printf("Failed to update order %s: %v", orderNumber, err)
			continue
		}

		if accrual.Status == "PROCESSED" && accrual.Accrual > 0 {
			_, err = tx.Exec(
				`INSERT INTO balances (user_id, current, withdrawn) 
				 VALUES ($1, $2, 0)
				 ON CONFLICT (user_id) DO UPDATE 
				 SET current = balances.current + EXCLUDED.current`,
				userID, accrual.Accrual,
			)
			if err != nil {
				tx.Rollback()
				log.Printf("Failed to update balance for user %d: %v", userID, err)
				continue
			}
		}

		if err := tx.Commit(); err != nil {
			log.Printf("Failed to commit transaction for order %s: %v", orderNumber, err)
		}
	}

	return nil
}