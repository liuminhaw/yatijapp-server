package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/liuminhaw/sessions-of-life/internal/data"
	"github.com/yanyiwu/gojieba"
)

const version = "1.0.0"

type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  time.Duration
	}
	limiter struct {
		rps     float64
		burst   int
		enabled bool
	}
}

type application struct {
	config config
	logger *slog.Logger
	models data.Models
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	var cfg config

	flag.IntVar(&cfg.port, "port", 8080, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
	flag.StringVar(
		&cfg.db.dsn,
		"db-dsn",
		os.Getenv("YATIJAPP_DB_DSN"),
		"PostgreSQL DSN",
	)
	flag.IntVar(
		&cfg.db.maxOpenConns,
		"db-max-open-conns",
		25,
		"Maximum number of open connections to the database",
	)
	flag.IntVar(
		&cfg.db.maxIdleConns,
		"db-max-idle-conns",
		25,
		"Maximum number of idle connections to the database",
	)
	flag.DurationVar(
		&cfg.db.maxIdleTime,
		"db-max-idle-time",
		15*time.Minute,
		"Maximum amount of time a connection may be idle",
	)
	flag.Float64Var(&cfg.limiter.rps, "limiter-rps", 2, "Max requests per second limit")
	flag.IntVar(&cfg.limiter.burst, "limiter-burst", 4, "Max burst size for rate limiter")
	flag.BoolVar(&cfg.limiter.enabled, "limiter-enabled", true, "Enable rate limiting")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	db, err := openDB(cfg)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
	defer db.Close()
	logger.Info("database connection pool established")

	// Initialize the Jieba text segmentation library for Chinese text processing
	jieba := gojieba.NewJieba()
	defer jieba.Free()

	app := application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db, jieba),
	}

	err = app.serve()
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

// openDB returns a sql.DB connection pool
func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	// Set the maximum number of open connections (in-user + idle)
	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	// Set the maximum number of idle connections in the pool.
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	db.SetConnMaxIdleTime(cfg.db.maxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
