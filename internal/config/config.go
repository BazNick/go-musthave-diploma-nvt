package config

import (
	"flag"
	"os"
)

type Config struct {
	RunAddress           string `env:"ADDRESS"`
	DatabaseURI          string `env:"DATABASE_URI"`
	AccrualSystemAddress string `env:"ACCRUAL_SYSTEM_ADDRESS"`
	JWTSecret            string `env:"JWT_SECRET"`
	NumWorkers           string `env:"NUM_WORKERS"`
}

func Load() Config {
	runAddr := flag.String("a", ":8081", "Server address")
	dbURI := flag.String("d", "", "Database URI")
	accrualAddr := flag.String("r", "http://localhost:8080", "Accrual system address")
	jwtSecret := flag.String("jwt", "secret", "JWT secret key")
	numWorkers := flag.String("w", "5", "Number of workers")

	flag.Parse()

	cfg := Config{
		RunAddress:           getEnv("RUN_ADDRESS", *runAddr),
		DatabaseURI:          getEnv("DATABASE_URI", *dbURI),
		AccrualSystemAddress: getEnv("ACCRUAL_SYSTEM_ADDRESS", *accrualAddr),
		JWTSecret:            getEnv("JWT_SECRET", *jwtSecret),
		NumWorkers:           getEnv("NUM_WORKERS", *numWorkers),
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
