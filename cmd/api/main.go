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
	"github.com/liuminhaw/yatijapp/internal/vcs"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/yanyiwu/gojieba"
)

var version = vcs.Version()

type config struct {
	port   int
	env    string
	pepper string
	db     struct {
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
	tokens struct {
		activationTokenTTL    time.Duration
		passwordResetTokenTTL time.Duration
		accessTokenTTL        time.Duration
		refreshTokenTTL       time.Duration
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		sender   string
	}
	cors struct {
		trustedOrigins []string
	}
	user struct {
		dailyTargetsCreationLimit  int
		dailyActionsCreationLimit  int
		dailySessionsCreationLimit int
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
	flag.Duration("ttl-activation-token", 10*time.Minute, "Activation token lifetime")
	flag.Duration("ttl-password-reset-token", 10*time.Minute, "Password reset token lifetime")
	flag.Duration("ttl-access-token", 1*time.Hour, "Access token lifetime")
	flag.Duration("ttl-refresh-token", 24*time.Hour, "Refresh token lifetime")
	flag.Int("daily-targets-creation-limit", 10, "Daily targets creation limit per user")
	flag.Int("daily-actions-creation-limit", 20, "Daily actions creation limit per user")
	flag.Int("daily-sessions-creation-limit", 50, "Daily sessions creation limit per user")
	flag.StringSlice("cors-trusted-origins", []string{}, "Trusted CORS origins (comma separated)")

	displayVersion := flag.Bool("version", false, "Display version and exit")
	flag.Parse()

	if *displayVersion {
		fmt.Printf("Version:\t%s\n", version)
		os.Exit(0)
	}

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

func configSetup(conf *viper.Viper) (config, error) {
	conf.SetDefault("server.port", 8080)
	conf.SetDefault("server.env", "development")
	conf.SetDefault("server.pepper", "")
	conf.SetDefault("server.corsTrustedOrigins", []string{})
	conf.SetDefault("server.limiter.rps", 2.0)
	conf.SetDefault("server.limiter.burst", 4)
	conf.SetDefault("server.limiter.enabled", true)
	conf.SetDefault("server.tokens.activationTokenTTL", 10*time.Minute)
	conf.SetDefault("server.tokens.passwordResetTokenTTL", 10*time.Minute)
	conf.SetDefault("server.tokens.accessTokenTTL", 1*time.Hour)
	conf.SetDefault("server.tokens.refreshTokenTTL", 24*time.Hour)
	conf.SetDefault("database.maxOpenConns", 25)
	conf.SetDefault("database.maxIdleConns", 25)
	conf.SetDefault("database.maxIdleTimeInMinutes", 15)
	conf.SetDefault("smtp.host", "sandbox.smtp.mailtrap.io")
	conf.SetDefault("smtp.port", 25)
	conf.SetDefault("smtp.sender", "Yatijapp <no-reply>@yatijapp.fakemail.com")
	conf.SetDefault("user.dailyTargetsCreationLimit", 10)
	conf.SetDefault("user.dailyActionsCreationLimit", 20)
	conf.SetDefault("user.dailySessionsCreationLimit", 50)

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
	conf.BindPFlag("server.corsTrustedOrigins", flag.Lookup("cors-trusted-origins"))
	conf.BindPFlag("server.limiter.rps", flag.Lookup("limiter-rps"))
	conf.BindPFlag("server.limiter.burst", flag.Lookup("limiter-burst"))
	conf.BindPFlag("server.limiter.enabled", flag.Lookup("limiter-enabled"))
	conf.BindPFlag("server.tokens.activationTokenTTL", flag.Lookup("ttl-activation-token"))
	conf.BindPFlag("server.tokens.passwordResetTokenTTL", flag.Lookup("ttl-password-reset-token"))
	conf.BindPFlag("server.tokens.accessTokenTTL", flag.Lookup("ttl-access-token"))
	conf.BindPFlag("server.tokens.refreshTokenTTL", flag.Lookup("ttl-refresh-token"))
	conf.BindPFlag("database.dsn", flag.Lookup("db-dsn"))
	conf.BindPFlag("database.maxOpenConns", flag.Lookup("db-max-open-conns"))
	conf.BindPFlag("database.maxIdleConns", flag.Lookup("db-max-idle-conns"))
	conf.BindPFlag("database.maxIdleTime", flag.Lookup("db-max-idle-time"))
	conf.BindPFlag("mailer.sender", flag.Lookup("smtp-sender"))
	conf.BindPFlag("mailer.smtp.host", flag.Lookup("smtp-host"))
	conf.BindPFlag("mailer.smtp.port", flag.Lookup("smtp-port"))
	conf.BindPFlag("mailer.smtp.username", flag.Lookup("smtp-username"))
	conf.BindPFlag("mailer.smtp.password", flag.Lookup("smtp-password"))
	conf.BindPFlag("user.dailyTargetsCreationLimit", flag.Lookup("daily-targets-creation-limit"))
	conf.BindPFlag("user.dailyActionsCreationLimit", flag.Lookup("daily-actions-creation-limit"))
	conf.BindPFlag("user.dailySessionsCreationLimit", flag.Lookup("daily-sessions-creation-limit"))

	return config{
		port:   conf.GetInt("server.port"),
		env:    conf.GetString("server.env"),
		pepper: conf.GetString("server.pepper"),
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
		tokens: struct {
			activationTokenTTL    time.Duration
			passwordResetTokenTTL time.Duration
			accessTokenTTL        time.Duration
			refreshTokenTTL       time.Duration
		}{
			activationTokenTTL:    conf.GetDuration("server.tokens.activationTokenTTL"),
			passwordResetTokenTTL: conf.GetDuration("server.tokens.passwordResetTokenTTL"),
			accessTokenTTL:        conf.GetDuration("server.tokens.accessTokenTTL"),
			refreshTokenTTL:       conf.GetDuration("server.tokens.refreshTokenTTL"),
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
		cors: struct {
			trustedOrigins []string
		}{
			trustedOrigins: conf.GetStringSlice("server.corsTrustedOrigins"),
		},
		user: struct {
			dailyTargetsCreationLimit  int
			dailyActionsCreationLimit  int
			dailySessionsCreationLimit int
		}{
			dailyTargetsCreationLimit:  conf.GetInt("user.dailyTargetsCreationLimit"),
			dailyActionsCreationLimit:  conf.GetInt("user.dailyActionsCreationLimit"),
			dailySessionsCreationLimit: conf.GetInt("user.dailySessionsCreationLimit"),
		},
	}, nil
}
