package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func HashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := sha256.New()
	hash.Write(salt)
	hash.Write([]byte(password))
	return hex.EncodeToString(salt) + ":" + hex.EncodeToString(hash.Sum(nil)), nil
}

func CheckPasswordHash(password, hash string) bool {
	parts := strings.Split(hash, ":")
	if len(parts) != 2 {
		return false
	}
	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	expectedHash, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	h := sha256.New()
	h.Write(salt)
	h.Write([]byte(password))
	return string(h.Sum(nil)) == string(expectedHash)
}

func IsValidLuhn(number string) bool {
	sum := 0
	alt := false
	for i := len(number) - 1; i >= 0; i-- {
		d := int(number[i] - '0')
		if d < 0 || d > 9 {
			return false
		}
		if alt {
			d *= 2
			if d > 9 {
				d = d%10 + 1
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

func BuildInClause(values []string, startIndex int) (string, []interface{}) {
	placeholders := make([]string, len(values))
	args := make([]interface{}, len(values))
	for i, v := range values {
		placeholders[i] = fmt.Sprintf("$%d", i+startIndex)
		args[i] = v
	}
	return strings.Join(placeholders, ", "), args
}