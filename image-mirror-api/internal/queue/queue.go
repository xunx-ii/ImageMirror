package queue

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/linxunxi/image-mirror/internal/images"
	"github.com/linxunxi/image-mirror/internal/systemconfig"
	"github.com/linxunxi/image-mirror/internal/usage"
)

const (
	TypeImageGenerate = "image:generate"
	TypeImageCleanup  = "image:cleanup"

	imageGenerationActiveKey    = "image-mirror:image-generation:active"
	imageGenerationRequeueDelay = 5 * time.Second
	imageGenerationSlotTTL      = 2 * time.Hour
)

var acquireGenerationSlotScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

if current >= limit then
	return 0
end

current = redis.call("INCR", KEYS[1])
redis.call("EXPIRE", KEYS[1], ttl)

if current > limit then
	redis.call("DECR", KEYS[1])
	return 0
end

return 1
`)

var releaseGenerationSlotScript = redis.NewScript(`
local current = tonumber(redis.call("DECR", KEYS[1]))
if current <= 0 then
	redis.call("DEL", KEYS[1])
end
return current
`)

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
	return c.EnqueueGenerateIn(ctx, imageID, 0)
}

func (c *Client) EnqueueGenerateIn(ctx context.Context, imageID string, delay time.Duration) error {
	payload, err := json.Marshal(ImageGeneratePayload{ImageID: imageID})
	if err != nil {
		return err
	}
	options := []asynq.Option{
		asynq.Queue("image-generation"),
		asynq.Timeout(c.generateTimeout),
		asynq.MaxRetry(2),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = c.client.EnqueueContext(ctx, asynq.NewTask(TypeImageGenerate, payload), options...)
	return err
}

type Processor struct {
	images  *images.Service
	usage   *usage.Service
	configs *systemconfig.Service
	redis   *redis.Client
	queue   *Client
	logger  *slog.Logger
}

func NewProcessor(imagesSvc *images.Service, usageSvc *usage.Service, configSvc *systemconfig.Service, redisClient *redis.Client, queueClient *Client, logger *slog.Logger) *Processor {
	return &Processor{
		images:  imagesSvc,
		usage:   usageSvc,
		configs: configSvc,
		redis:   redisClient,
		queue:   queueClient,
		logger:  logger,
	}
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
	acquired, limit, err := p.acquireGenerateSlot(ctx)
	if err != nil {
		return err
	}
	if !acquired {
		p.logger.Info("image generation waiting for concurrency slot", "image_id", payload.ImageID, "limit", limit)
		return p.queue.EnqueueGenerateIn(ctx, payload.ImageID, imageGenerationRequeueDelay)
	}
	defer p.releaseGenerateSlot()

	p.logger.Info("processing image generation", "image_id", payload.ImageID, "limit", limit)
	return p.images.Process(ctx, payload.ImageID)
}

func (p *Processor) acquireGenerateSlot(ctx context.Context) (bool, int, error) {
	limit := systemconfig.DefaultImageGenerationConcurrency
	if p.configs != nil {
		settings, err := p.configs.GenerationSettings(ctx)
		if err != nil {
			return false, limit, err
		}
		limit = settings.ImageGenerationConcurrency
	}
	if p.redis == nil {
		return true, limit, nil
	}
	acquired, err := acquireGenerationSlotScript.Run(ctx, p.redis, []string{imageGenerationActiveKey}, limit, int(imageGenerationSlotTTL.Seconds())).Int()
	if err != nil {
		return false, limit, err
	}
	return acquired == 1, limit, nil
}

func (p *Processor) releaseGenerateSlot() {
	if p.redis == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := releaseGenerationSlotScript.Run(ctx, p.redis, []string{imageGenerationActiveKey}).Err(); err != nil {
		p.logger.Warn("image generation slot release failed", "error", err)
	}
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
		Concurrency: systemconfig.MaxImageGenerationConcurrency + 2,
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
