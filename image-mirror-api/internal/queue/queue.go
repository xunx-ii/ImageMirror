package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/linxunxi/image-mirror/internal/images"
	"github.com/linxunxi/image-mirror/internal/usage"
)

const (
	TypeImageGenerate = "image:generate"
	TypeImageCleanup  = "image:cleanup"
)

type ImageGeneratePayload struct {
	ImageID string `json:"imageId"`
}

type Client struct {
	client          *asynq.Client
	generateTimeout time.Duration
}

func NewClient(redis asynq.RedisClientOpt, generateTimeout time.Duration) *Client {
	if generateTimeout <= 0 {
		generateTimeout = 11 * time.Minute
	}
	return &Client{client: asynq.NewClient(redis), generateTimeout: generateTimeout}
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) EnqueueGenerate(ctx context.Context, imageID string) error {
	payload, err := json.Marshal(ImageGeneratePayload{ImageID: imageID})
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, asynq.NewTask(TypeImageGenerate, payload), asynq.Queue("image-generation"), asynq.Timeout(c.generateTimeout), asynq.MaxRetry(2))
	return err
}

type Processor struct {
	images *images.Service
	usage  *usage.Service
	logger *slog.Logger
}

func NewProcessor(imagesSvc *images.Service, usageSvc *usage.Service, logger *slog.Logger) *Processor {
	return &Processor{images: imagesSvc, usage: usageSvc, logger: logger}
}

func (p *Processor) Mux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeImageGenerate, p.handleGenerate)
	mux.HandleFunc(TypeImageCleanup, p.handleCleanup)
	return mux
}

func (p *Processor) handleGenerate(ctx context.Context, task *asynq.Task) error {
	var payload ImageGeneratePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	p.logger.Info("processing image generation", "image_id", payload.ImageID)
	return p.images.Process(ctx, payload.ImageID)
}

func (p *Processor) handleCleanup(ctx context.Context, _ *asynq.Task) error {
	count, err := p.images.ExpireOld(ctx, 100)
	if err == nil {
		p.logger.Info("cleanup completed", "expired", count)
	}
	recovered, recoverErr := p.images.RecoverStaleProcessing(ctx, 100)
	if recoverErr == nil && recovered > 0 {
		p.logger.Warn("stale image generations recovered", "count", recovered)
	}
	if err == nil {
		err = recoverErr
	}
	if p.usage != nil {
		deleted, usageErr := p.usage.CleanupExpired(ctx, 1000)
		if usageErr == nil && deleted > 0 {
			p.logger.Info("usage log cleanup completed", "deleted", deleted)
		}
		if err == nil {
			err = usageErr
		}
	}
	return err
}

func RunServer(redis asynq.RedisClientOpt, processor *Processor) error {
	server := asynq.NewServer(redis, asynq.Config{
		Concurrency: 4,
		Queues: map[string]int{
			"image-generation": 8,
			"default":          1,
		},
	})
	return server.Run(processor.Mux())
}

func StartScheduler(redis asynq.RedisClientOpt, logger *slog.Logger) (*asynq.Scheduler, error) {
	scheduler := asynq.NewScheduler(redis, &asynq.SchedulerOpts{})
	if _, err := scheduler.Register("@hourly", asynq.NewTask(TypeImageCleanup, nil)); err != nil {
		return nil, err
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			logger.Error("scheduler stopped", "error", err)
		}
	}()
	return scheduler, nil
}
