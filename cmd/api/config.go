package main

import (
	"os"
	"path/filepath"
	"time"

	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var defaultConfigFile = "yatijapp.toml"

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
	cleanup struct {
		interval time.Duration
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

func configSetup(conf *viper.Viper, config_file string) (config, error) {
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
	conf.SetDefault("server.cleanup.interval", 1*time.Hour)
	conf.SetDefault("database.maxOpenConns", 25)
	conf.SetDefault("database.maxIdleConns", 25)
	conf.SetDefault("database.maxIdleTimeInMinutes", 15)
	conf.SetDefault("smtp.host", "sandbox.smtp.mailtrap.io")
	conf.SetDefault("smtp.port", 25)
	conf.SetDefault("smtp.sender", "Yatijapp <no-reply>@yatijapp.fakemail.com")
	conf.SetDefault("user.dailyTargetsCreationLimit", 10)
	conf.SetDefault("user.dailyActionsCreationLimit", 20)
	conf.SetDefault("user.dailySessionsCreationLimit", 50)

	if config_file != "" {
		conf.SetConfigFile(config_file)
	} else {
		conf.SetConfigName(defaultConfigFile)
		conf.SetConfigType("toml")
		conf.AddConfigPath("/etc/yatijapp/")
		conf.AddConfigPath("$HOME/.yatijapp/")
		conf.AddConfigPath(".")
	}

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
	conf.BindPFlag("server.cleanup.interval", flag.Lookup("cleanup-interval"))
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
		cleanup: struct {
			interval time.Duration
		}{
			interval: conf.GetDuration("server.cleanup.interval"),
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

func writeConfigFile(content, config_file string) error {
	if config_file == "" {
		config_file = defaultConfigFile
	}

	dir := filepath.Dir(config_file)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	return os.WriteFile(config_file, []byte(content), 0640)
}
