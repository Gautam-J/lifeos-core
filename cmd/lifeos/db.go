package main

import (
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Gautam-J/lifeos-core/internal/config"
)

// poolConfig turns database_url into pool settings and adds the password
// from the POSTGRES_PASSWORD environment variable.
func poolConfig(cfg config.Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("database_url: %w", err)
	}
	if password := os.Getenv("POSTGRES_PASSWORD"); password != "" {
		pc.ConnConfig.Password = password
	}
	return pc, nil
}
