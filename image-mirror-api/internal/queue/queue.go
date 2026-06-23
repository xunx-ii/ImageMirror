package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"

	"github.com/linxunxi/image-mirror/internal/images"
)

const (
	TypeImageGenerate = "image:generate"
	TypeImageCleanup  = "image:cleanup"
)

type ImageGeneratePayload struct {
	ImageID string `json:"imageId"`
}

type Client struct {
	client *asynq.Client
}

func NewClient(redis asynq.RedisClientOpt) *Client {
	return &Client{client: asynq.NewClient(redis)}
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) EnqueueGenerate(ctx context.Context, imageID string) error {
	payload, err := json.Marshal(ImageGeneratePayload{ImageID: imageID})
	if err != nil {
		return err
	}
	_, err = c.client.EnqueueContext(ctx, asynq.NewTask(TypeImageGenerate, payload), asynq.Queue("image-generation"), asynq.Timeout(6*time.Minute), asynq.MaxRetry(2))
	return err
}

type Processor struct {
	images *images.Service
	logger *slog.Logger
}

func NewProcessor(imagesSvc *images.Service, logger *slog.Logger) *Processor {
	return &Processor{images: imagesSvc, logger: logger}
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
