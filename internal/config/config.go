package config

import (
	"flag"
	"os"
)

type Config struct {
	RunAddress           string
	DatabaseURI          string
	AccrualSystemAddress string
}

func Load() Config {
	runAddr := flag.String("a", ":8081", "Server address")
	dbURI := flag.String("d", "", "Database URI")
	accrualAddr := flag.String("r", "http://localhost:8080", "Accrual system address")
	flag.Parse()

	cfg := Config{
		RunAddress:           getEnv("RUN_ADDRESS", *runAddr),
		DatabaseURI:          getEnv("DATABASE_URI", *dbURI),
		AccrualSystemAddress: getEnv("ACCRUAL_SYSTEM_ADDRESS", *accrualAddr),
	}
	if cfg.DatabaseURI == "" {
		cfg.DatabaseURI = "postgres://postgres:postgres@localhost:5432/gophermart?sslmode=disable"
	}
	return cfg
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return def
}