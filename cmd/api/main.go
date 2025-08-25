package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"sync"
	"time"

	_ "github.com/lib/pq"
	"github.com/liuminhaw/sessions-of-life/internal/data"
	"github.com/liuminhaw/sessions-of-life/internal/mailer"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
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
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
}

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
	flag.Parse()

	cfg, err := configSetup(vConf)
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

	app := application{
		config: cfg,
		logger: logger,
		models: data.NewModels(db, jieba),
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

func configSetup(conf *viper.Viper) (config, error) {
	conf.SetDefault("server.port", 8080)
	conf.SetDefault("server.env", "development")
	conf.SetDefault("server.limiter.rps", 2.0)
	conf.SetDefault("server.limiter.burst", 4)
	conf.SetDefault("server.limiter.enabled", true)
	conf.SetDefault("database.maxOpenConns", 25)
	conf.SetDefault("database.maxIdleConns", 25)
	conf.SetDefault("database.maxIdleTimeInMinutes", 15)
	conf.SetDefault("smtp.host", "sandbox.smtp.mailtrap.io")
	conf.SetDefault("smtp.port", 25)
	conf.SetDefault("smtp.sender", "Yatijapp <no-reply>@yatijapp.fakemail.com")

	conf.SetConfigName("yatijapp.toml")
	conf.SetConfigType("toml")
	conf.AddConfigPath("/etc/yatijapp/")
	conf.AddConfigPath("$HOME/.yatijapp/")
	conf.AddConfigPath(".")
	if err := conf.ReadInConfig(); err != nil {
		return config{}, err
	}

	conf.BindPFlag("server.port", flag.Lookup("port"))
	conf.BindPFlag("server.env", flag.Lookup("env"))
	conf.BindPFlag("server.limiter.rps", flag.Lookup("limiter-rps"))
	conf.BindPFlag("server.limiter.burst", flag.Lookup("limiter-burst"))
	conf.BindPFlag("server.limiter.enabled", flag.Lookup("limiter-enabled"))
	conf.BindPFlag("database.dsn", flag.Lookup("db-dsn"))
	conf.BindPFlag("database.maxOpenConns", flag.Lookup("db-max-open-conns"))
	conf.BindPFlag("database.maxIdleConns", flag.Lookup("db-max-idle-conns"))
	conf.BindPFlag("database.maxIdleTime", flag.Lookup("db-max-idle-time"))
	conf.BindPFlag("mailer.sender", flag.Lookup("smtp-sender"))
	conf.BindPFlag("mailer.smtp.host", flag.Lookup("smtp-host"))
	conf.BindPFlag("mailer.smtp.port", flag.Lookup("smtp-port"))
	conf.BindPFlag("mailer.smtp.username", flag.Lookup("smtp-username"))
	conf.BindPFlag("mailer.smtp.password", flag.Lookup("smtp-password"))

	return config{
		port: conf.GetInt("server.port"),
		env:  conf.GetString("server.env"),
		db: struct {
			dsn          string
			maxOpenConns int
			maxIdleConns int
			maxIdleTime  time.Duration
		}{
			dsn:          conf.GetString("database.dsn"),
			maxOpenConns: conf.GetInt("database.maxOpenConns"),
			maxIdleConns: conf.GetInt("database.maxIdleConns"),
			maxIdleTime:  conf.GetDuration("database.maxIdleTime"),
		},
		limiter: struct {
			rps     float64
			burst   int
			enabled bool
		}{
			rps:     conf.GetFloat64("server.limiter.rps"),
			burst:   conf.GetInt("server.limiter.burst"),
			enabled: conf.GetBool("server.limiter.enabled"),
		},
		smtp: struct {
			host     string
			port     int
			username string
			password string
			sender   string
		}{
			host:     conf.GetString("mailer.smtp.host"),
			port:     conf.GetInt("mailer.smtp.port"),
			username: conf.GetString("mailer.smtp.username"),
			password: conf.GetString("mailer.smtp.password"),
			sender:   conf.GetString("mailer.sender"),
		},
	}, nil
}
