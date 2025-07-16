package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"gophermart/internal/models"
	"gophermart/internal/services"
	"gophermart/internal/storage"
	"gophermart/internal/utils"

	"github.com/go-chi/jwtauth/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type OrderHandler struct {
	storage *storage.Storage
	accrual *services.AccrualService
}

func NewOrderHandler(storage *storage.Storage, accrual *services.AccrualService) *OrderHandler {
	return &OrderHandler{
		storage: storage,
		accrual: accrual,
	}
}

func (h *OrderHandler) UploadOrder(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	orderNumber := strings.TrimSpace(string(body))
	if orderNumber == "" {
		http.Error(w, "Empty order number", http.StatusBadRequest)
		return
	}

	if !utils.IsValidLuhn(orderNumber) {
		http.Error(w, "Invalid order number format", http.StatusUnprocessableEntity)
		return
	}

	token, _, err := jwtauth.FromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID := int(token.PrivateClaims()["user_id"].(float64))

	var existingUserID int
	err = h.storage.DB.QueryRow(
		"SELECT user_id FROM orders WHERE number = $1",
		orderNumber,
	).Scan(&existingUserID)

	if err == nil {
		if existingUserID == userID {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "Order number already uploaded by another user", http.StatusConflict)
		}
		return
	} else if err != sql.ErrNoRows {
		http.Error(w, "Failed to check order", http.StatusInternalServerError)
		return
	}

	_, err = h.storage.DB.Exec(
		"INSERT INTO orders (number, status, uploaded_at, user_id) VALUES ($1, $2, $3, $4)",
		orderNumber, "NEW", time.Now(), userID,
	)

	if err != nil {
		http.Error(w, "Failed to save order", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	token, _, err := jwtauth.FromContext(r.Context())
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	userID := int(token.PrivateClaims()["user_id"].(float64))

	rows, err := h.storage.DB.Query(
		"SELECT number, status, accrual, uploaded_at FROM orders WHERE user_id = $1 ORDER BY uploaded_at DESC",
		userID,
	)
	if err != nil {
		http.Error(w, "Failed to get orders", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var orders []models.Order
	for rows.Next() {
		var order models.Order
		err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt)
		if err != nil {
			http.Error(w, "Failed to read orders", http.StatusInternalServerError)
			return
		}
		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}