package main

import (
	"context"
	"log"

	"github.com/linxunxi/image-mirror/internal/app"
	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/database"
	"github.com/linxunxi/image-mirror/internal/queue"
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
	scheduler, err := queue.StartScheduler(container.RedisOpt, container.Logger)
	if err != nil {
		log.Fatal(err)
	}
	defer scheduler.Shutdown()

	processor := queue.NewProcessor(container.Services.Images, container.Logger)
	container.Logger.Info("worker listening")
	if err := queue.RunServer(container.RedisOpt, processor); err != nil {
		log.Fatal(err)
	}
}
