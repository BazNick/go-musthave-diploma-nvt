package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"gophermart/internal/middleware"
	"gophermart/internal/models"
	"gophermart/internal/storage"
	"gophermart/internal/utils"
)

type OrderHandler struct {
	storage storage.Storage
}

func NewOrderHandler(storage storage.Storage) *OrderHandler {
	return &OrderHandler{storage: storage}
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

	userID, err := middleware.GetUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	existingOrder, err := h.storage.GetOrderByNumber(r.Context(), orderNumber)
	if err == nil {
		if existingOrder.UserID == userID {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "Order already uploaded by another user", http.StatusConflict)
		}
		return
	} else if !errors.Is(err, storage.ErrOrderNotFound) {
		http.Error(w, "Failed to check order", http.StatusInternalServerError)
		return
	}

	newOrder := models.Order{
		Number:     orderNumber,
		Status:     "NEW",
		UploadedAt: time.Now(),
		UserID:     userID,
	}

	if err := h.storage.CreateOrder(r.Context(), &newOrder); err != nil {
		http.Error(w, "Failed to save order", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *OrderHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, err := middleware.GetUserIDFromToken(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	orders, err := h.storage.GetOrders(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to get orders", http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orders)
}
