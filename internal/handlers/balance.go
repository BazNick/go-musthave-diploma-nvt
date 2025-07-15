package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gophermart/internal/models"
	"gophermart/internal/storage"
	"gophermart/internal/middleware"
	"gophermart/internal/utils"
	"github.com/go-chi/jwtauth/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	
)

type BalanceHandler struct {
	storage *storage.Storage
}

func NewBalanceHandler(storage *storage.Storage) *BalanceHandler {
	return &BalanceHandler{storage: storage}
}

func (h *BalanceHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
    userID, err := middleware.GetUserIDFromToken(r)
    if err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    var balance models.Balance
    
    var userExists bool
    err = h.storage.DB.QueryRow(
        "SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", 
        userID,
    ).Scan(&userExists)
    
    if err != nil {
        http.Error(w, "Failed to verify user", http.StatusInternalServerError)
        return
    }
    
    if !userExists {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }

    err = h.storage.DB.QueryRow(
        `INSERT INTO balances (user_id, current, withdrawn) 
         VALUES ($1, 0, 0)
         ON CONFLICT (user_id) 
         DO UPDATE SET user_id = EXCLUDED.user_id
         RETURNING current, withdrawn`,
        userID,
    ).Scan(&balance.Current, &balance.Withdrawn)

    if err != nil {
        http.Error(w, "Failed to get or create balance", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(balance)
}

func (h *BalanceHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	token, _, err := jwtauth.FromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID := int(token.PrivateClaims()["user_id"].(float64))

	var withdrawal struct {
		Order string  `json:"order"`
		Sum   float64 `json:"sum"`
	}
	if err := json.NewDecoder(r.Body).Decode(&withdrawal); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if withdrawal.Sum <= 0 {
		http.Error(w, "Sum must be positive", http.StatusBadRequest)
		return
	}

	if !utils.IsValidLuhn(withdrawal.Order) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	tx, err := h.storage.DB.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var currentBalance float64
	err = tx.QueryRow(
		"SELECT current FROM balances WHERE user_id = $1 FOR UPDATE",
		userID,
	).Scan(&currentBalance)

	if err != nil {
		if err == sql.ErrNoRows {
			_, err = tx.Exec(
				"INSERT INTO balances (user_id, current, withdrawn) VALUES ($1, 0, 0)",
				userID,
			)
			if err != nil {
				http.Error(w, "Failed to initialize balance", http.StatusInternalServerError)
				return
			}
			currentBalance = 0
		} else {
			http.Error(w, "Failed to check balance", http.StatusInternalServerError)
			return
		}
	}

	if currentBalance < withdrawal.Sum {
		http.Error(w, "Insufficient funds", http.StatusPaymentRequired)
		return
	}

	_, err = tx.Exec(
		"INSERT INTO withdrawals (order_number, sum, processed_at, user_id) VALUES ($1, $2, $3, $4)",
		withdrawal.Order, withdrawal.Sum, time.Now(), userID,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			http.Error(w, "Order number already used for withdrawal", http.StatusConflict)
		} else {
			http.Error(w, "Failed to save withdrawal", http.StatusInternalServerError)
		}
		return
	}

	_, err = tx.Exec(
		"UPDATE balances SET current = current - $1, withdrawn = withdrawn + $2 WHERE user_id = $3",
		withdrawal.Sum, withdrawal.Sum, userID,
	)
	if err != nil {
		http.Error(w, "Failed to update balance", http.StatusInternalServerError)
		return
	}

	err = tx.Commit()
	if err != nil {
		http.Error(w, "Failed to commit transaction", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *BalanceHandler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	token, _, err := jwtauth.FromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID := int(token.PrivateClaims()["user_id"].(float64))

	rows, err := h.storage.DB.Query(
		"SELECT order_number, sum, processed_at FROM withdrawals WHERE user_id = $1 ORDER BY processed_at DESC",
		userID,
	)
	if err != nil {
		http.Error(w, "Failed to get withdrawals", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var withdrawals []models.Withdrawal
	for rows.Next() {
		var wd models.Withdrawal
		err := rows.Scan(&wd.Order, &wd.Sum, &wd.ProcessedAt)
		if err != nil {
			http.Error(w, "Failed to read withdrawals", http.StatusInternalServerError)
			return
		}
		withdrawals = append(withdrawals, wd)
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(withdrawals)
}