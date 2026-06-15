package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"plankroad-backend/config"
)

var Pool *pgxpool.Pool

func Init(cfg *config.DatabaseConfig) error {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s pool_max_conns=%d",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Name, cfg.SSLMode, cfg.PoolMax,
	)

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return fmt.Errorf("parse db config: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.PoolMax)
	poolCfg.MinConns = 5
	poolCfg.MaxConnLifetime = time.Hour
	poolCfg.HealthCheckPeriod = time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	Pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("create db pool: %w", err)
	}

	if err = Pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	log.Printf("Database connected: %s:%d/%s", cfg.Host, cfg.Port, cfg.Name)
	return nil
}

func Close() {
	if Pool != nil {
		Pool.Close()
		log.Println("Database connection closed")
	}
}
