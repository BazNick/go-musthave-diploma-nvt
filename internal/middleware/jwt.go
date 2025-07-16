package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/jwtauth/v5"
)

var TokenAuth *jwtauth.JWTAuth

func init() {
	TokenAuth = jwtauth.New("HS256", []byte("secret"), nil)
}

func Verifier() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") == "" {
				if cookie, err := r.Cookie("auth_token"); err == nil {
					r.Header.Set("Authorization", "Bearer "+cookie.Value)
				}
			}
			jwtauth.Verifier(TokenAuth)(next).ServeHTTP(w, r)
		})
	}
}

func Authenticator() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil || token == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GenerateToken(userID int) (string, error) {
	claims := map[string]interface{}{
		"user_id": userID,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	}
	_, tokenString, err := TokenAuth.Encode(claims)
	return tokenString, err
}

func GetUserIDFromToken(r *http.Request) (int, error) {
	_, claims, err := jwtauth.FromContext(r.Context())
	if err != nil {
		return 0, err
	}

	userID, ok := claims["user_id"].(float64)
	if !ok {
		return 0, errors.New("invalid user_id in token")
	}

	return int(userID), nil
}