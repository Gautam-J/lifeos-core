package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/Gautam-J/lifeos-core/internal/config"
	"github.com/Gautam-J/lifeos-core/migrations"
)

func migrate(args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	configPath := flags.String("config", "config/lifeos.yaml", "path to the config file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	pc, err := poolConfig(cfg)
	if err != nil {
		return err
	}
	db := stdlib.OpenDB(*pc.ConnConfig)
	defer db.Close()

	applied, err := migrations.Up(context.Background(), db)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("database is up to date")
		return nil
	}
	for _, name := range applied {
		fmt.Println("applied", name)
	}
	return nil
}
