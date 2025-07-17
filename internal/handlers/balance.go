package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"gophermart/internal/middleware"
	"gophermart/internal/storage"
	"gophermart/internal/utils"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type BalanceHandler struct {
	storage storage.Storage
}

func NewBalanceHandler(storage storage.Storage) *BalanceHandler {
	return &BalanceHandler{storage: storage}
}

func (h *BalanceHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	balance, err := h.storage.GetBalance(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to get balance", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(balance)
}

func (h *BalanceHandler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

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

	if err := h.storage.ProcessWithdrawal(r.Context(), userID, withdrawal.Order, withdrawal.Sum); err != nil {
		switch {
		case errors.Is(err, storage.ErrInsufficientFunds):
			http.Error(w, "Insufficient funds", http.StatusPaymentRequired)
		case errors.Is(err, storage.ErrDuplicateWithdrawal):
			http.Error(w, "Order number already used", http.StatusConflict)
		default:
			http.Error(w, "Failed to process withdrawal", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (h *BalanceHandler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	withdrawals, err := h.storage.GetWithdrawals(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to get withdrawals", http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(withdrawals)
}