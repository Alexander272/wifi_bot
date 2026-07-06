package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/subosito/gotenv"

	"wifi_bot/internal/config"
	"wifi_bot/internal/migrate"
	"wifi_bot/internal/repo"
	memRepo "wifi_bot/internal/repo/memory"
	"wifi_bot/internal/services"
	"wifi_bot/internal/transport"
	"wifi_bot/pkg/database/postgres"
	"wifi_bot/pkg/database/redis"
	"wifi_bot/pkg/error_bot"
	"wifi_bot/pkg/logger"
	mm "wifi_bot/pkg/mattermost"
	"wifi_bot/pkg/mikrotik"
)

func main() {
	if os.Getenv("APP_ENV") == "" {
		if err := gotenv.Load(".env"); err != nil {
			log.Fatalf("failed to load .env: %s", err)
		}
	}

	conf, err := config.Init("configs/config.yaml")
	if err != nil {
		log.Fatalf("failed to init config: %s", err)
	}

	logger.NewLogger(logger.WithLevel(conf.LogLevel), logger.WithAddSource(conf.LogSource))

	mikrotikClient, err := mikrotik.NewClient(&mikrotik.Config{
		APIVersion: conf.Mikrotik.APIVersion, Host: conf.Mikrotik.Host,
		Port: conf.Mikrotik.APIPort, Username: conf.Mikrotik.APIUsername,
		Password: conf.Mikrotik.APIPassword, UseSSL: conf.Mikrotik.UseSSL,
	})
	if err != nil {
		log.Fatalf("failed to init mikrotik client: %s", err)
	}

	var repos *repo.Repository
	if conf.TestMode {
		log.Println("⚠️ TEST MODE: using in-memory repositories instead of PostgreSQL/Redis")
		repos = memRepo.NewRepository(conf.Mattermost.CodeTTL)
	} else {
		db, err := postgres.NewPostgresDB(&postgres.Config{
			Host: conf.Postgres.Host, Port: conf.Postgres.Port,
			Username: conf.Postgres.Username, Password: conf.Postgres.Password,
			DBName: conf.Postgres.DbName, SSLMode: conf.Postgres.SSLMode,
		})
		if err != nil {
			log.Fatalf("failed to init postgres: %s", err)
		}

		if err := migrate.Migrate(db); err != nil {
			log.Fatalf("failed to run migrations: %s", err)
		}

		rdb, err := redis.NewRedisClient(&redis.Config{
			Host: conf.Redis.Host, Port: conf.Redis.Port,
			DB: conf.Redis.DB, Password: conf.Redis.Password,
		})
		if err != nil {
			log.Fatalf("failed to init redis: %s", err)
		}

		repos = repo.NewRepository(db, rdb, conf.Mattermost.CodeTTL)
	}

	mmClient := mm.NewClient(conf.Mattermost.Server, conf.Mattermost.Token)
	mmWS := mm.NewWSClient(conf.Mattermost.Server, conf.Mattermost.Token)

	svc := services.NewServices(&services.Deps{
		Repo:            repos,
		MikrotikClient:  mikrotikClient,
		CollectInterval: conf.Mikrotik.CollectInterval,
		CodeTTL:         conf.Mattermost.CodeTTL,
		AuthTimeout:     conf.Mikrotik.AuthTimeout,
		MikrotikHost:    conf.Mikrotik.Host,
		AuthMethod:      conf.Mikrotik.AuthMethod,
		AllowReuse:      conf.Mikrotik.AllowReuse,
		AddressList:     conf.Mikrotik.AddressList,
	})

	mmBot := services.NewMattermostBot(
		mmClient, mmWS, svc.Session, svc.Stats,
		conf.Mattermost.AdminIDs, svc.Collector,
		conf.Mattermost.TeamName,
	)
	svc.MattermostBot = mmBot

	handler := transport.NewHandler(svc, conf.Mattermost.Token,
		conf.Mattermost.Server, conf.Mattermost.TeamName, conf.Mattermost.BotUsername)

	srv := &http.Server{
		Addr:           ":" + conf.Http.Port,
		Handler:        handler.Init(),
		ReadTimeout:    conf.Http.ReadTimeout,
		WriteTimeout:   conf.Http.WriteTimeout,
		MaxHeaderBytes: conf.Http.MaxHeaderBytes << 20,
	}

	botCtx, botCancel := context.WithCancel(context.Background())
	defer botCancel()

	svc.Collector.Start(botCtx)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Sprintf("http server: panic: %v", r)
				logger.Error("http server: panic", logger.StringAttr("panic", err))
				error_bot.Send(nil, err, nil)
			}
		}()
		logger.Info("server started", logger.StringAttr("port", conf.Http.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", logger.ErrAttr(err))
			error_bot.Send(nil, fmt.Sprintf("http server error: %v", err), nil)
		}
	}()

	if conf.Mattermost.Token != "" {
		go mmBot.Start(botCtx)
	} else {
		logger.Warn("mattermost bot token not set, ws listener disabled")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", logger.ErrAttr(err))
	}
}
