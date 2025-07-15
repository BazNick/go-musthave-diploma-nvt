package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	md "gophermart/internal/middleware"
	"gophermart/internal/models"
	"gophermart/internal/storage"
	"gophermart/internal/utils"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type AuthHandler struct {
	storage *storage.Storage
}

func NewAuthHandler(storage *storage.Storage) *AuthHandler {
	return &AuthHandler{storage: storage}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if user.Login == "" || user.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	user.Password = hashedPassword
	if err := h.storage.CreateUser(&user); err != nil {
		if err == storage.ErrUserExists {
			http.Error(w, "Login already exists", http.StatusConflict)
		} else {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
		}
		return
	}

	tokenString, err := md.GenerateToken(user.ID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	w.WriteHeader(http.StatusOK)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if user.Login == "" || user.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	var dbUser struct {
		ID       int
		Password string
	}
	err := h.storage.DB.QueryRow(
		"SELECT id, password FROM users WHERE login = $1",
		user.Login,
	).Scan(&dbUser.ID, &dbUser.Password)

	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		} else {
			http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		}
		return
	}

	if !utils.CheckPasswordHash(user.Password, dbUser.Password) {
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	tokenString, err := md.GenerateToken(user.ID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,        
		Secure:   true,        
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusOK)
}