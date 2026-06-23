package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/linxunxi/image-mirror/internal/app"
	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/database"
	"github.com/linxunxi/image-mirror/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	container, err := app.Build(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer container.Close()

	if err := database.Migrate(ctx, container.DB, "db/migrations"); err != nil {
		log.Fatal(err)
	}

	router := httpapi.NewRouter(container.Services)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		container.Logger.Info("api listening", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
