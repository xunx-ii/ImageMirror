package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/linxunxi/image-mirror/internal/admin"
	"github.com/linxunxi/image-mirror/internal/apikeys"
	"github.com/linxunxi/image-mirror/internal/auth"
	"github.com/linxunxi/image-mirror/internal/billing"
	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/images"
	"github.com/linxunxi/image-mirror/internal/payments"
	"github.com/linxunxi/image-mirror/internal/pricing"
	"github.com/linxunxi/image-mirror/internal/queue"
	"github.com/linxunxi/image-mirror/internal/systemconfig"
	"github.com/linxunxi/image-mirror/internal/users"
)

type Services struct {
	Config      config.Config
	Auth        *auth.Service
	Users       *users.Repository
	APIKeys     *apikeys.Service
	Billing     *billing.Service
	Pricing     *pricing.Service
	Images      *images.Service
	Payments    *payments.Service
	Queue       *queue.Client
	Admin       *admin.Service
	ConfigStore *systemconfig.Service
	Redis       *redis.Client
}

func NewRouter(s Services) *gin.Engine {
	if s.Config.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), CORS(s.Config.CORSOrigins))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.GET("/api/pricing", pricingHandler(s.Pricing))

	api := r.Group("/api")
	api.POST("/auth/register", registerHandler(s.Auth))
	api.POST("/auth/login", loginHandler(s.Auth))
	api.POST("/auth/refresh", refreshHandler(s.Auth))

	protected := api.Group("")
	protected.Use(JWTAuth(s.Auth), RateLimit(s.Redis, s.Config.RateLimitPerMinute))
	protected.GET("/users/me", meHandler(s.Users))
	protected.GET("/billing/balance", balanceHandler(s.Billing))
	protected.GET("/billing/transactions", transactionsHandler(s.Billing))
	protected.POST("/billing/epay/orders", createEPayOrderHandler(s.Payments))
	protected.GET("/api-keys", listAPIKeysHandler(s.APIKeys))
	protected.POST("/api-keys", createAPIKeyHandler(s.APIKeys))
	protected.DELETE("/api-keys/:id", revokeAPIKeyHandler(s.APIKeys))
	protected.POST("/images/generate", webGenerateHandler(s))
	protected.GET("/images", listImagesHandler(s.Images))
	protected.GET("/images/:id", getImageHandler(s.Images))
	protected.GET("/images/:id/status", getImageHandler(s.Images))
	protected.GET("/images/:id/file", imageFileHandler(s.Images))
	protected.DELETE("/images/:id", deleteImageHandler(s.Images))
	protected.POST("/images/bulk-delete", bulkDeleteImagesHandler(s.Images))
	protected.POST("/images/bulk-download", bulkDownloadImagesHandler(s.Images))

	adminGroup := api.Group("/admin")
	adminGroup.Use(JWTAuth(s.Auth), AdminOnly())
	adminGroup.GET("/users", adminListUsersHandler(s.Admin))
	adminGroup.POST("/users/:id/adjust-balance", adminAdjustBalanceHandler(s.Admin))
	adminGroup.GET("/pricing", pricingHandler(s.Pricing))
	adminGroup.POST("/pricing", upsertPricingHandler(s.Pricing))
	adminGroup.PUT("/pricing/:id", updatePricingHandler(s.Pricing))
	adminGroup.DELETE("/pricing/:id", deletePricingHandler(s.Pricing))
	adminGroup.GET("/config/openai", openAIConfigHandler(s))
	adminGroup.PUT("/config/openai", updateOpenAIConfigHandler(s))
	adminGroup.GET("/config/epay", epayConfigHandler(s))
	adminGroup.PUT("/config/epay", updateEPayConfigHandler(s))
	adminGroup.GET("/stats/overview", adminStatsHandler(s.Admin))

	api.POST("/payments/epay/notify", epayNotifyHandler(s.Payments))
	api.GET("/payments/epay/notify", epayNotifyHandler(s.Payments))

	v1 := r.Group("/v1")
	v1.Use(APIKeyAuth(s.APIKeys.Lookup), RateLimit(s.Redis, s.Config.RateLimitPerMinute))
	v1.GET("/models", func(c *gin.Context) {
		OK(c, gin.H{"object": "list", "data": []gin.H{{"id": s.Config.DefaultImageModel, "object": "model"}}})
	})
	v1.GET("/billing/balance", balanceHandler(s.Billing))
	v1.GET("/billing/usage", transactionsHandler(s.Billing))
	v1.POST("/images/generations", developerGenerateHandler(s))
	v1.GET("/images/:id", getImageHandler(s.Images))
	v1.GET("/images/:id/file", imageFileHandler(s.Images))

	return r
}

func registerHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		user, tokens, err := authSvc.Register(c.Request.Context(), req.Email, req.Password)
		if err != nil {
			Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
			return
		}
		Created(c, gin.H{"user": user, "tokens": tokens})
	}
}

func loginHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		user, tokens, err := authSvc.Login(c.Request.Context(), req.Email, req.Password)
		if err != nil {
			Abort(c, NewError(http.StatusUnauthorized, "invalid credentials", err))
			return
		}
		OK(c, gin.H{"user": user, "tokens": tokens})
	}
}

func refreshHandler(authSvc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refreshToken"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		tokens, err := authSvc.Refresh(c.Request.Context(), req.RefreshToken)
		if err != nil {
			Abort(c, ErrUnauthorized)
			return
		}
		OK(c, gin.H{"tokens": tokens})
	}
}

func meHandler(usersRepo *users.Repository) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := usersRepo.FindByID(c.Request.Context(), CurrentUserID(c))
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"user": user})
	}
}

func pricingHandler(pricingSvc *pricing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		rules, err := pricingSvc.List(c.Request.Context())
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"data": rules})
	}
}

func balanceHandler(billingSvc *billing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		balance, err := billingSvc.Balance(c.Request.Context(), CurrentUserID(c))
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"balance": balance})
	}
}

func transactionsHandler(billingSvc *billing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset := pagination(c)
		items, err := billingSvc.ListTransactions(c.Request.Context(), CurrentUserID(c), limit, offset)
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"data": items})
	}
}

func createEPayOrderHandler(paymentSvc *payments.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Amount  int64  `json:"amount"`
			PayType string `json:"payType"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
			Abort(c, NewError(http.StatusBadRequest, "amount must be positive", err))
			return
		}
		result, err := paymentSvc.CreateEPayOrder(c.Request.Context(), CurrentUserID(c), req.Amount*100, req.PayType)
		if err != nil {
			Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
			return
		}
		Created(c, result)
	}
}

func epayNotifyHandler(paymentSvc *payments.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := c.Request.ParseForm(); err != nil {
			c.String(http.StatusBadRequest, "fail")
			return
		}
		if _, err := paymentSvc.HandleEPayNotify(c.Request.Context(), c.Request.Form); err != nil {
			c.String(http.StatusBadRequest, "fail")
			return
		}
		c.String(http.StatusOK, "success")
	}
}

func listAPIKeysHandler(svc *apikeys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		keys, err := svc.List(c.Request.Context(), CurrentUserID(c))
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"data": keys})
	}
}

func createAPIKeyHandler(svc *apikeys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Name string `json:"name"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			Abort(c, NewError(http.StatusBadRequest, "name is required", err))
			return
		}
		key, err := svc.Create(c.Request.Context(), CurrentUserID(c), req.Name)
		if err != nil {
			Abort(c, err)
			return
		}
		Created(c, key)
	}
}

func revokeAPIKeyHandler(svc *apikeys.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Revoke(c.Request.Context(), CurrentUserID(c), c.Param("id")); err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"ok": true})
	}
}

func webGenerateHandler(s Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			webMultipartGenerate(c, s)
			return
		}
		var req struct {
			Model   string `json:"model"`
			Prompt  string `json:"prompt"`
			Size    string `json:"size"`
			Quality string `json:"quality"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		gen, err := s.Images.CreatePending(c.Request.Context(), images.CreateRequest{
			UserID:  CurrentUserID(c),
			Model:   req.Model,
			Prompt:  req.Prompt,
			Size:    req.Size,
			Quality: req.Quality,
		})
		if err != nil {
			AbortGenerationError(c, err)
			return
		}
		if err := s.Queue.EnqueueGenerate(c.Request.Context(), gen.ID); err != nil {
			_ = s.Images.CancelPending(c.Request.Context(), gen.ID, "image enqueue failed")
			Abort(c, err)
			return
		}
		Created(c, gin.H{"image": gen})
	}
}

func webMultipartGenerate(c *gin.Context, s Services) {
	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		Abort(c, NewError(http.StatusBadRequest, "invalid multipart request", err))
		return
	}
	form := c.Request.MultipartForm
	files := form.File["referenceImages"]
	gen, err := s.Images.CreatePending(c.Request.Context(), images.CreateRequest{
		UserID:  CurrentUserID(c),
		Model:   c.PostForm("model"),
		Prompt:  c.PostForm("prompt"),
		Size:    c.PostForm("size"),
		Quality: c.PostForm("quality"),
	})
	if err != nil {
		AbortGenerationError(c, err)
		return
	}
	if len(files) > 0 {
		keys, err := s.Images.SaveReferenceFiles(c.Request.Context(), CurrentUserID(c), gen.ID, files)
		if err != nil {
			_ = s.Images.CancelPending(c.Request.Context(), gen.ID, err.Error())
			Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
			return
		}
		gen.ReferenceCount = len(keys)
	}
	if err := s.Queue.EnqueueGenerate(c.Request.Context(), gen.ID); err != nil {
		_ = s.Images.CancelPending(c.Request.Context(), gen.ID, "image enqueue failed")
		Abort(c, err)
		return
	}
	Created(c, gin.H{"image": gen})
}

func developerGenerateHandler(s Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Model   string `json:"model"`
			Prompt  string `json:"prompt"`
			Size    string `json:"size"`
			Quality string `json:"quality"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		gen, err := s.Images.CreatePending(c.Request.Context(), images.CreateRequest{
			UserID:   CurrentUserID(c),
			APIKeyID: CurrentAPIKeyID(c),
			Model:    req.Model,
			Prompt:   req.Prompt,
			Size:     req.Size,
			Quality:  req.Quality,
		})
		if err != nil {
			AbortGenerationError(c, err)
			return
		}
		if err := s.Queue.EnqueueGenerate(c.Request.Context(), gen.ID); err != nil {
			_ = s.Images.CancelPending(c.Request.Context(), gen.ID, "image enqueue failed")
			Abort(c, err)
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), s.Config.DeveloperAPITimeout)
		defer cancel()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			current, err := s.Images.FindForUser(ctx, CurrentUserID(c), gen.ID)
			if err != nil {
				Abort(c, err)
				return
			}
			if current.Status == "COMPLETED" {
				OK(c, gin.H{"created": current.CreatedAt.Unix(), "data": []gin.H{{"id": current.ID, "url": current.StorageURL, "expiresAt": current.ExpiresAt}}})
				return
			}
			if current.Status == "FAILED" || current.Status == "EXPIRED" {
				Abort(c, NewError(http.StatusBadGateway, "image generation failed", errors.New("image generation failed")))
				return
			}
			select {
			case <-ctx.Done():
				Abort(c, NewError(http.StatusGatewayTimeout, "image generation timed out", ctx.Err()))
				return
			case <-ticker.C:
			}
		}
	}
}

func listImagesHandler(svc *images.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset := pagination(c)
		items, err := svc.ListForUser(c.Request.Context(), CurrentUserID(c), limit, offset)
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"data": items})
	}
}

func getImageHandler(svc *images.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		gen, err := svc.FindForUser(c.Request.Context(), CurrentUserID(c), c.Param("id"))
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"image": gen})
	}
}

func imageFileHandler(svc *images.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		bytes, _, err := svc.ReadFile(c.Request.Context(), CurrentUserID(c), c.Param("id"))
		if err != nil {
			Abort(c, NewError(http.StatusNotFound, "image file is not available", err))
			return
		}
		c.Data(http.StatusOK, "image/png", bytes)
	}
}

func deleteImageHandler(svc *images.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.DeleteForUser(c.Request.Context(), CurrentUserID(c), c.Param("id")); err != nil {
			Abort(c, NewError(http.StatusNotFound, "image is not available", err))
			return
		}
		OK(c, gin.H{"ok": true})
	}
}

func bulkDeleteImagesHandler(svc *images.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
			Abort(c, NewError(http.StatusBadRequest, "image ids are required", err))
			return
		}
		count, err := svc.DeleteManyForUser(c.Request.Context(), CurrentUserID(c), req.IDs)
		if err != nil {
			Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
			return
		}
		OK(c, gin.H{"deleted": count})
	}
}

func bulkDownloadImagesHandler(svc *images.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
			Abort(c, NewError(http.StatusBadRequest, "image ids are required", err))
			return
		}
		data, err := svc.ZipFilesForUser(c.Request.Context(), CurrentUserID(c), req.IDs)
		if err != nil {
			Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
			return
		}
		c.Header("Content-Disposition", `attachment; filename="image-mirror-selected.zip"`)
		c.Data(http.StatusOK, "application/zip", data)
	}
}

func upsertPricingHandler(svc *pricing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Model    string `json:"model"`
			Size     string `json:"size"`
			Quality  string `json:"quality"`
			Credits  int64  `json:"credits"`
			IsActive bool   `json:"isActive"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" || req.Size == "" || req.Quality == "" || req.Credits <= 0 {
			Abort(c, NewError(http.StatusBadRequest, "invalid pricing rule", err))
			return
		}
		rule, err := svc.Upsert(c.Request.Context(), req.Model, req.Size, req.Quality, req.Credits, req.IsActive)
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"rule": rule})
	}
}

func updatePricingHandler(svc *pricing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Model    string `json:"model"`
			Size     string `json:"size"`
			Quality  string `json:"quality"`
			Credits  int64  `json:"credits"`
			IsActive bool   `json:"isActive"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Model == "" || req.Size == "" || req.Quality == "" || req.Credits <= 0 {
			Abort(c, NewError(http.StatusBadRequest, "invalid pricing rule", err))
			return
		}
		rule, err := svc.Update(c.Request.Context(), c.Param("id"), req.Model, req.Size, req.Quality, req.Credits, req.IsActive)
		if err != nil {
			Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
			return
		}
		OK(c, gin.H{"rule": rule})
	}
}

func deletePricingHandler(svc *pricing.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
			Abort(c, NewError(http.StatusNotFound, err.Error(), err))
			return
		}
		OK(c, gin.H{"ok": true})
	}
}

func openAIConfigHandler(s Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := s.ConfigStore.PublicOpenAI(c.Request.Context(), s.Config.OpenAIAPIKey, s.Config.OpenAIBaseURL)
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, settings)
	}
}

func updateOpenAIConfigHandler(s Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			OpenAIBaseURL string  `json:"openaiBaseUrl"`
			OpenAIAPIKey  *string `json:"openaiApiKey"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		settings, err := s.ConfigStore.UpdateOpenAI(c.Request.Context(), req.OpenAIBaseURL, req.OpenAIAPIKey, CurrentUserID(c))
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, settings)
	}
}

func epayConfigHandler(s Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := s.ConfigStore.PublicEPay(c.Request.Context())
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, settings)
	}
}

func updateEPayConfigHandler(s Services) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Gateway        string  `json:"gateway"`
			PID            string  `json:"pid"`
			Key            *string `json:"key"`
			Name           string  `json:"name"`
			CreditsPerYuan int64   `json:"creditsPerYuan"`
			Enabled        bool    `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			Abort(c, NewError(http.StatusBadRequest, "invalid request body", err))
			return
		}
		settings, err := s.ConfigStore.UpdateEPay(c.Request.Context(), systemconfig.EPaySettings{
			Gateway:        req.Gateway,
			PID:            req.PID,
			Name:           req.Name,
			CreditsPerYuan: req.CreditsPerYuan,
			Enabled:        req.Enabled,
		}, req.Key, CurrentUserID(c))
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, settings)
	}
}

func adminListUsersHandler(adminSvc *admin.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset := pagination(c)
		items, err := adminSvc.ListUsers(c.Request.Context(), limit, offset)
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"data": items})
	}
}

func adminAdjustBalanceHandler(adminSvc *admin.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Amount      int64  `json:"amount"`
			Description string `json:"description"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Amount == 0 {
			Abort(c, NewError(http.StatusBadRequest, "amount is required", err))
			return
		}
		if err := adminSvc.AdjustBalance(c.Request.Context(), c.Param("id"), req.Amount, req.Description); err != nil {
			Abort(c, err)
			return
		}
		OK(c, gin.H{"ok": true})
	}
}

func adminStatsHandler(adminSvc *admin.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		stats, err := adminSvc.Overview(c.Request.Context())
		if err != nil {
			Abort(c, err)
			return
		}
		OK(c, stats)
	}
}

func pagination(c *gin.Context) (int32, int32) {
	limit64, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 32)
	offset64, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 32)
	if limit64 <= 0 || limit64 > 100 {
		limit64 = 50
	}
	if offset64 < 0 {
		offset64 = 0
	}
	return int32(limit64), int32(offset64)
}

func AbortGenerationError(c *gin.Context, err error) {
	if errors.Is(err, billing.ErrInsufficientCredits) {
		Abort(c, ErrInsufficientFund)
		return
	}
	Abort(c, NewError(http.StatusBadRequest, err.Error(), err))
}
