package main

import (
	"context"
	"database/sql"
	"expvar"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/liuminhaw/yatijapp/internal/data"
	"github.com/liuminhaw/yatijapp/internal/mailer"
	"github.com/liuminhaw/yatijapp/internal/platform"
	"github.com/liuminhaw/yatijapp/internal/vcs"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/yanyiwu/gojieba"
)

var version = vcs.Version()

type application struct {
	config config
	logger *slog.Logger
	models data.Models
	mailer *mailer.Mailer
	wg     sync.WaitGroup
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	vConf := viper.New()

	flag.Int("port", 8080, "API server port")
	flag.String("env", "development", "Environment (development|staging|production)")
	flag.String(
		"db-dsn",
		"",
		"PostgreSQL DSN",
	)
	flag.Int(
		"db-max-open-conns",
		25,
		"Maximum number of open connections to the database",
	)
	flag.Int(
		"db-max-idle-conns",
		25,
		"Maximum number of idle connections to the database",
	)
	flag.Duration(
		"db-max-idle-time",
		15*time.Minute,
		"Maximum amount of time a connection may be idle",
	)
	flag.Float64("limiter-rps", 2, "Max requests per second limit")
	flag.Int("limiter-burst", 4, "Max burst size for rate limiter")
	flag.Bool("limiter-enabled", true, "Enable rate limiting")

	flag.String("smtp-host", "sandbox.smtp.mailtrap.io", "SMTP server host")
	flag.Int("smtp-port", 25, "SMTP server port")
	flag.String("smtp-username", "", "SMTP server username")
	flag.String("smtp-password", "", "SMTP server password")
	flag.String(
		"smtp-sender",
		"Yatijapp <no-reply@yatijapp.fakemail.com>",
		"Sender email address",
	)
	flag.Duration("ttl-activation-token", 10*time.Minute, "Activation token lifetime")
	flag.Duration("ttl-password-reset-token", 10*time.Minute, "Password reset token lifetime")
	flag.Duration("ttl-access-token", 1*time.Hour, "Access token lifetime")
	flag.Duration("ttl-refresh-token", 24*time.Hour, "Refresh token lifetime")
	flag.Int("daily-targets-creation-limit", 10, "Daily targets creation limit per user")
	flag.Int("daily-actions-creation-limit", 20, "Daily actions creation limit per user")
	flag.Int("daily-sessions-creation-limit", 50, "Daily sessions creation limit per user")
	flag.StringSlice("cors-trusted-origins", []string{}, "Trusted CORS origins (comma separated)")

	external_config_src := flag.String(
		"external-config-source",
		"",
		"Load configuration from external source (currently only awsParameterStore is supported)",
	)
	external_config_key := flag.String(
		"external-config-key",
		"",
		"External configuration key or path",
	)

	config_file := flag.String("config-file", "", "Path to configuration file")

	displayVersion := flag.Bool("version", false, "Display version and exit")
	flag.Parse()

	if *displayVersion {
		fmt.Printf("Version:\t%s\n", version)
		os.Exit(0)
	}

	switch *external_config_src {
	case "awsParameterStore":
		out, err := platform.LoadAWSParameterStoreConfig(*external_config_key)
		if err != nil {
			logger.Error(
				"Error loading configuration from AWS Parameter Store",
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
		logger.Info("Loaded configuration from AWS Parameter Store", slog.String("result", out))
		// Write content to config file
		if err := writeConfigFile(out, *config_file); err != nil {
			logger.Error(
				"Error writing configuration to file",
				slog.String("error", err.Error()),
			)
			os.Exit(1)
		}
	case "":
		// Do nothing, load from file or environment variables
	default:
		logger.Error(
			"Unsupported external configuration source",
			slog.String("source", *external_config_src),
		)
		os.Exit(1)
	}

	cfg, err := configSetup(vConf, *config_file)
	if err != nil {
		logger.Error("Error loading configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

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

	// Initialize a new Mailer instance for sending emails
	mailer, err := mailer.New(
		cfg.smtp.host,
		cfg.smtp.port,
		cfg.smtp.username,
		cfg.smtp.password,
		cfg.smtp.sender,
	)
	if err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}

	expvar.NewString("version").Set(version)
	// Publish the number of active goroutines
	expvar.Publish("goroutines", expvar.Func(func() any {
		return runtime.NumGoroutine()
	}))
	// Publish the database connection pool statistics
	expvar.Publish("database", expvar.Func(func() any {
		return db.Stats()
	}))
	expvar.Publish("timestamp", expvar.Func(func() any {
		return time.Now().Unix()
	}))

	app := application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db, jieba, logger),
		mailer: mailer,
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
