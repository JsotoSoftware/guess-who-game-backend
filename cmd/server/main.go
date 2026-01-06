package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/auth"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/config"
	httphandler "github.com/JsotoSoftware/guess-who-game-backend/internal/http"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	ctx := context.Background()

	st, err := storage.New(ctx, cfg.PostgresDSN, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init storage")
	}
	defer st.Close()

	tokens, err := auth.NewTokenMaker(cfg.JWTSecret)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init token maker")
	}

	r := httphandler.NewRouter(st, tokens, cfg.CookieSecure, cfg.CookieDomain)

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Str("env", cfg.AppEnv).Msg("server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server crashed")
		}
	}()

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-ctx.Done()
	stop()

	log.Info().Msg("server shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info().Msg("bye")
}
