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
	ErrUserExists    = errors.New("user already exists")
	ErrOrderExists   = errors.New("order already exists")
	ErrOrderNotFound = errors.New("order not found")
)

type Storage struct {
	DB *sql.DB
}

func NewStorage(db *sql.DB) *Storage {
	return &Storage{DB: db}
}

func (s *Storage) InitDB() error {
	stmts := []string{
        `CREATE TABLE IF NOT EXISTS users (
            id SERIAL PRIMARY KEY,
            login TEXT UNIQUE NOT NULL,
            password TEXT NOT NULL
        )`,

        `CREATE TABLE IF NOT EXISTS orders (
            number TEXT PRIMARY KEY,
            status TEXT NOT NULL,
            accrual FLOAT DEFAULT 0,
            uploaded_at TIMESTAMP WITH TIME ZONE NOT NULL,
            user_id INTEGER REFERENCES users(id) NOT NULL
        )`,

        `CREATE TABLE IF NOT EXISTS withdrawals (
            order_number TEXT PRIMARY KEY,
            sum FLOAT NOT NULL,
            processed_at TIMESTAMP WITH TIME ZONE NOT NULL,
            user_id INTEGER REFERENCES users(id) NOT NULL
        )`,

        `CREATE TABLE IF NOT EXISTS balances (
            user_id INTEGER PRIMARY KEY REFERENCES users(id),
            current FLOAT DEFAULT 0,
            withdrawn FLOAT DEFAULT 0
        )`,
    }

    for _, stmt := range stmts {
        if _, err := s.DB.Exec(stmt); err != nil {
            return err
        }
    }
    return nil
}

func (s *Storage) CreateUser(user *models.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := s.DB.QueryRowContext(ctx,
		"INSERT INTO users (login, password) VALUES ($1, $2) RETURNING id",
		user.Login, user.Password,
	).Scan(&user.ID)

	if err != nil && strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
		return ErrUserExists
	}
	return err
}