package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"github.com/linxunxi/image-mirror/internal/config"
	"github.com/linxunxi/image-mirror/internal/users"
)

type Claims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type Service struct {
	cfg   config.Config
	users *users.Repository
	redis *redis.Client
}

func NewService(cfg config.Config, usersRepo *users.Repository, redisClient *redis.Client) *Service {
	return &Service{cfg: cfg, users: usersRepo, redis: redisClient}
}

func (s *Service) Register(ctx context.Context, email string, password string) (users.User, TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(password) < 8 {
		return users.User{}, TokenPair{}, errors.New("email and password are required")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}
	user, err := s.users.Create(ctx, email, hash, "USER", 0)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}
	tokens, err := s.issueTokens(ctx, user)
	return user, tokens, err
}

func (s *Service) Login(ctx context.Context, email string, password string) (users.User, TokenPair, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.users.FindByEmail(ctx, email)
	if err != nil {
		return users.User{}, TokenPair{}, err
	}
	if user.Status != "ACTIVE" {
		return users.User{}, TokenPair{}, errors.New("account is suspended")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return users.User{}, TokenPair{}, errors.New("invalid credentials")
	}
	if err := s.users.UpdateLoginAt(ctx, user.ID); err != nil {
		return users.User{}, TokenPair{}, err
	}
	tokens, err := s.issueTokens(ctx, user)
	return user, tokens, err
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	userID, err := s.redis.Get(ctx, "refresh:"+refreshToken).Result()
	if err != nil {
		return TokenPair{}, errors.New("invalid refresh token")
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return TokenPair{}, err
	}
	if user.Status != "ACTIVE" {
		return TokenPair{}, errors.New("account is suspended")
	}
	_ = s.redis.Del(ctx, "refresh:"+refreshToken).Err()
	return s.issueTokens(ctx, user)
}

func (s *Service) ParseAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func (s *Service) issueTokens(ctx context.Context, user users.User) (TokenPair, error) {
	expiresAt := time.Now().Add(s.cfg.AccessTokenTTL)
	claims := Claims{
		UserID: user.ID,
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return TokenPair{}, err
	}
	refreshToken, err := randomToken(32)
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.redis.Set(ctx, "refresh:"+refreshToken, user.ID, s.cfg.RefreshTokenTTL).Err(); err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: accessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt}, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func randomToken(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
