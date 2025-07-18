package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"gophermart/internal/models"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var (
	ErrUserExists      = errors.New("user already exists")
	ErrInvalidData     = errors.New("invalid data")
	ErrOrderExists     = errors.New("order already exists")
	ErrOrderNotFound   = errors.New("order not found")
	ErrInsufficientFunds = errors.New("insufficient funds")
	ErrDuplicateWithdrawal = errors.New("duplicate withdrawal")
	ErrNotFound = errors.New("not found")
)

type Storage interface {
	InitDB() error
	CreateUser(ctx context.Context, user *models.User) error
	GetUserByLogin(ctx context.Context, login string) (*models.User, error)
	CreateOrder(ctx context.Context, order *models.Order) error
	GetOrderByNumber(ctx context.Context, number string) (*models.Order, error)
	GetOrders(ctx context.Context, userID int) ([]models.Order, error)
	UpdateOrder(ctx context.Context, number string, status string, accrual float64) error
	GetBalance(ctx context.Context, userID int) (*models.Balance, error)
	ProcessWithdrawal(ctx context.Context, userID int, order string, sum float64) error
	GetWithdrawals(ctx context.Context, userID int) ([]models.Withdrawal, error)
	GetPendingOrders(ctx context.Context, limit int) ([]string, error)
	SetOrderProcessing(ctx context.Context, orders []string) (map[string]int, error)
}

type DBStorage struct {
	DB *sql.DB
}

func NewStorage(db *sql.DB) *DBStorage {
	return &DBStorage{DB: db}
}

func (s *DBStorage) InitDB() error {
	_, err := s.DB.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			login TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS users_login_idx ON users(login);

		CREATE TABLE IF NOT EXISTS orders (
			number TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			accrual FLOAT DEFAULT 0,
			uploaded_at TIMESTAMP WITH TIME ZONE NOT NULL,
			user_id INTEGER REFERENCES users(id) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS orders_user_id_idx ON orders(user_id);

		CREATE TABLE IF NOT EXISTS withdrawals (
			order_number TEXT PRIMARY KEY,
			sum FLOAT NOT NULL,
			processed_at TIMESTAMP WITH TIME ZONE NOT NULL,
			user_id INTEGER REFERENCES users(id) NOT NULL
		);
		CREATE INDEX IF NOT EXISTS withdrawals_user_id_idx ON withdrawals(user_id);

		CREATE TABLE IF NOT EXISTS balances (
			user_id INTEGER PRIMARY KEY REFERENCES users(id),
			current FLOAT DEFAULT 0,
			withdrawn FLOAT DEFAULT 0
		);
	`)
	return err
}

func (s *DBStorage) CreateUser(ctx context.Context, user *models.User) error {
	if user.Login == "" || user.Password == "" {
		return ErrInvalidData
	}

	err := s.DB.QueryRowContext(ctx,
		"INSERT INTO users (login, password) VALUES ($1, $2) RETURNING id",
		user.Login, user.Password,
	).Scan(&user.ID)

	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return ErrUserExists
	}
	return err
}

func (s *DBStorage) GetUserByLogin(ctx context.Context, login string) (*models.User, error) {
	var user models.User
	err := s.DB.QueryRowContext(ctx,
		"SELECT id, login, password FROM users WHERE login = $1",
		login,
	).Scan(&user.ID, &user.Login, &user.Password)
	
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *DBStorage) ProcessWithdrawal(ctx context.Context, userID int, order string, sum float64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var current float64
	err = tx.QueryRowContext(ctx,
		"SELECT current FROM balances WHERE user_id = $1 FOR UPDATE",
		userID,
	).Scan(&current)

	if err == sql.ErrNoRows {
		if _, err = tx.ExecContext(ctx,
			"INSERT INTO balances (user_id, current, withdrawn) VALUES ($1, 0, 0)",
			userID,
		); err != nil {
			return err
		}
		current = 0
	} else if err != nil {
		return err
	}

	if current < sum {
		return ErrInsufficientFunds
	}

	if _, err = tx.ExecContext(ctx,
		"INSERT INTO withdrawals (order_number, sum, processed_at, user_id) VALUES ($1, $2, $3, $4)",
		order, sum, time.Now(), userID,
	); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrDuplicateWithdrawal
		}
		return err
	}

	if _, err = tx.ExecContext(ctx,
		"UPDATE balances SET current = current - $1, withdrawn = withdrawn + $2 WHERE user_id = $3",
		sum, sum, userID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *DBStorage) CreateOrder(ctx context.Context, order *models.Order) error {
	_, err := s.DB.ExecContext(ctx,
		"INSERT INTO orders (number, status, uploaded_at, user_id) VALUES ($1, $2, $3, $4)",
		order.Number, order.Status, order.UploadedAt, order.UserID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return ErrOrderExists
		}
		return err
	}
	return nil
}

func (s *DBStorage) GetOrderByNumber(ctx context.Context, number string) (*models.Order, error) {
	var order models.Order
	err := s.DB.QueryRowContext(ctx,
		"SELECT number, status, accrual, uploaded_at, user_id FROM orders WHERE number = $1",
		number,
	).Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt, &order.UserID)
	
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (s *DBStorage) GetOrders(ctx context.Context, userID int) ([]models.Order, error) {
	rows, err := s.DB.QueryContext(ctx,
		"SELECT number, status, accrual, uploaded_at FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		if err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}
	return orders, rows.Err()
}

func (s *DBStorage) UpdateOrder(ctx context.Context, number string, status string, accrual float64) error {
	_, err := s.DB.ExecContext(ctx,
		"UPDATE orders SET status = $1, accrual = $2 WHERE number = $3",
		status, accrual, number,
	)
	return err
}

func (s *DBStorage) GetBalance(ctx context.Context, userID int) (*models.Balance, error) {
	var balance models.Balance
	err := s.DB.QueryRowContext(ctx,
		"SELECT current, withdrawn FROM balances WHERE user_id = $1",
		userID,
	).Scan(&balance.Current, &balance.Withdrawn)

	if errors.Is(err, sql.ErrNoRows) {
		return &models.Balance{Current: 0, Withdrawn: 0}, nil
	}
	return &balance, err
}

func (s *DBStorage) GetWithdrawals(ctx context.Context, userID int) ([]models.Withdrawal, error) {
	rows, err := s.DB.QueryContext(ctx,
		"SELECT order_number, sum, processed_at FROM withdrawals WHERE user_id = $1 ORDER BY processed_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []models.Withdrawal
	for rows.Next() {
		var w models.Withdrawal
		if err := rows.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, err
		}
		withdrawals = append(withdrawals, w)
	}
	return withdrawals, rows.Err()
}

func (s *DBStorage) GetPendingOrders(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx,
		"SELECT number FROM orders WHERE status IN ('NEW', 'PROCESSING') ORDER BY uploaded_at LIMIT $1",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []string
	for rows.Next() {
		var number string
		if err := rows.Scan(&number); err != nil {
			return nil, err
		}
		orders = append(orders, number)
	}
	return orders, rows.Err()
}

func (s *DBStorage) SetOrderProcessing(ctx context.Context, orders []string) (map[string]int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		"UPDATE orders SET status = 'PROCESSING' WHERE number = $1 AND status = 'NEW' RETURNING user_id")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	userIDs := make(map[string]int)
	for _, number := range orders {
		var userID int
		if err := stmt.QueryRowContext(ctx, number).Scan(&userID); err == nil {
			userIDs[number] = userID
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return userIDs, nil
}