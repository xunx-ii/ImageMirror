package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/linxunxi/image-mirror/internal/admin"
	"github.com/linxunxi/image-mirror/internal/apikeys"
	"github.com/linxunxi/image-mirror/internal/auth"
	"github.com/linxunxi/image-mirror/internal/billing"
	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/content"
	"github.com/linxunxi/image-mirror/internal/database"
	"github.com/linxunxi/image-mirror/internal/httpapi"
	"github.com/linxunxi/image-mirror/internal/images"
	"github.com/linxunxi/image-mirror/internal/openai"
	"github.com/linxunxi/image-mirror/internal/payments"
	"github.com/linxunxi/image-mirror/internal/pricing"
	"github.com/linxunxi/image-mirror/internal/queue"
	"github.com/linxunxi/image-mirror/internal/redemptions"
	"github.com/linxunxi/image-mirror/internal/storage"
	"github.com/linxunxi/image-mirror/internal/systemconfig"
	"github.com/linxunxi/image-mirror/internal/users"
)

type Container struct {
	Config   config.Config
	Logger   *slog.Logger
	RedisOpt asynq.RedisClientOpt
	Redis    *redis.Client
	DB       *pgxpool.Pool
	Services httpapi.Services
}

func Build(ctx context.Context, cfg config.Config) (*Container, error) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := redisClient.Ping(ctx).Err(); err != nil {
		db.Close()
		return nil, err
	}

	redisOpt := asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	}

	usersRepo := users.NewRepository(db)
	billingSvc := billing.NewService(db)
	pricingSvc := pricing.NewService(db, redisClient)
	apiKeySvc := apikeys.NewService(db, redisClient)
	authSvc := auth.NewService(cfg, usersRepo, redisClient)
	storageSvc := storage.NewLocal(cfg.StorageRoot)
	systemConfigSvc := systemconfig.NewService(db)
	openAIClient := openai.NewClient(cfg.OpenAITimeout, func(ctx context.Context) (string, string, error) {
		return systemConfigSvc.GetOpenAI(ctx, cfg.OpenAIAPIKey, cfg.OpenAIBaseURL)
	})
	imagesSvc := images.NewService(cfg, db, pricingSvc, billingSvc, storageSvc, openAIClient)
	paymentSvc := payments.NewService(db, systemConfigSvc, cfg.PublicBaseURL)
	redemptionSvc := redemptions.NewService(db)
	contentSvc := content.NewService(db, storageSvc)
	queueClient := queue.NewClient(redisOpt)
	adminSvc := admin.NewService(db, usersRepo, billingSvc)

	services := httpapi.Services{
		Config:      cfg,
		Auth:        authSvc,
		Users:       usersRepo,
		APIKeys:     apiKeySvc,
		Billing:     billingSvc,
		Pricing:     pricingSvc,
		Images:      imagesSvc,
		Payments:    paymentSvc,
		Redemptions: redemptionSvc,
		Content:     contentSvc,
		Queue:       queueClient,
		Admin:       adminSvc,
		ConfigStore: systemConfigSvc,
		Redis:       redisClient,
	}

	return &Container{
		Config:   cfg,
		Logger:   logger,
		RedisOpt: redisOpt,
		Redis:    redisClient,
		DB:       db,
		Services: services,
	}, nil
}

func (c *Container) Close() {
	if c.Services.Queue != nil {
		_ = c.Services.Queue.Close()
	}
	if c.Redis != nil {
		_ = c.Redis.Close()
	}
	if c.DB != nil {
		c.DB.Close()
	}
}
