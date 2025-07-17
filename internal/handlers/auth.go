package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	md "gophermart/internal/middleware"
	"gophermart/internal/models"
	"gophermart/internal/storage"
	"gophermart/internal/utils"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type AuthHandler struct {
	storage storage.Storage
}

func NewAuthHandler(storage storage.Storage) *AuthHandler {
	return &AuthHandler{storage: storage}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var user models.User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	hashedPassword, err := utils.HashPassword(user.Password)
	if err != nil {
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	user.Password = hashedPassword
	if err := h.storage.CreateUser(r.Context(), &user); err != nil {
		if errors.Is(err, storage.ErrUserExists) {
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
		Name:     "auth_token",
		Value:    tokenString,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var reqUser models.User
	if err := json.NewDecoder(r.Body).Decode(&reqUser); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	if reqUser.Login == "" || reqUser.Password == "" {
		http.Error(w, "Login and password are required", http.StatusBadRequest)
		return
	}

	dbUser, err := h.storage.GetUserByLogin(r.Context(), reqUser.Login)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		} else {
			http.Error(w, "Failed to authenticate", http.StatusInternalServerError)
		}
		return
	}

	if !utils.CheckPasswordHash(reqUser.Password, dbUser.Password) {
		http.Error(w, "Invalid login or password", http.StatusUnauthorized)
		return
	}

	tokenString, err := md.GenerateToken(dbUser.ID)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	w.WriteHeader(http.StatusOK)
}