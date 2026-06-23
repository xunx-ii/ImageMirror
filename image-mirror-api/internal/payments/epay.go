package payments

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linxunxi/image-mirror/internal/systemconfig"
)

type Service struct {
	db          *pgxpool.Pool
	configStore *systemconfig.Service
	publicBase  string
}

type Order struct {
	ID              string     `json:"id"`
	UserID          string     `json:"userId"`
	Provider        string     `json:"provider"`
	OutTradeNo      string     `json:"outTradeNo"`
	ProviderTradeNo *string    `json:"providerTradeNo,omitempty"`
	Name            string     `json:"name"`
	AmountCents     int64      `json:"amountCents"`
	Credits         int64      `json:"credits"`
	Status          string     `json:"status"`
	PaidAt          *time.Time `json:"paidAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type CreateOrderResult struct {
	Order  Order  `json:"order"`
	PayURL string `json:"payUrl"`
}

type NotifyResult struct {
	OutTradeNo string
	Status     string
}

func NewService(db *pgxpool.Pool, configStore *systemconfig.Service, publicBaseURL string) *Service {
	return &Service{db: db, configStore: configStore, publicBase: strings.TrimRight(publicBaseURL, "/")}
}

func (s *Service) CreateEPayOrder(ctx context.Context, userID string, amountCents int64, payType string) (CreateOrderResult, error) {
	if amountCents <= 0 {
		return CreateOrderResult{}, errors.New("amount must be positive")
	}
	if amountCents > 100_000_000 {
		return CreateOrderResult{}, errors.New("amount is too large")
	}
	settings, key, err := s.configStore.GetEPay(ctx)
	if err != nil {
		return CreateOrderResult{}, err
	}
	if !settings.Enabled {
		return CreateOrderResult{}, errors.New("payment is not enabled")
	}
	if settings.PID == "" || key == "" {
		return CreateOrderResult{}, errors.New("epay merchant config is incomplete")
	}
	payType, err = normalizePayType(payType)
	if err != nil {
		return CreateOrderResult{}, err
	}
	credits := amountCents * settings.CreditsPerYuan / 100
	if credits <= 0 {
		credits = 1
	}
	outTradeNo, err := randomOutTradeNo()
	if err != nil {
		return CreateOrderResult{}, err
	}
	name := settings.Name
	if name == "" {
		name = "ImageMirror credits"
	}
	row := s.db.QueryRow(ctx, `
		INSERT INTO payment_orders(user_id, provider, out_trade_no, name, amount_cents, credits)
		VALUES ($1, 'epay', $2, $3, $4, $5)
		RETURNING id, user_id, provider, out_trade_no, provider_trade_no, name, amount_cents, credits, status, paid_at, created_at, updated_at
	`, userID, outTradeNo, name, amountCents, credits)
	order, err := scanOrder(row)
	if err != nil {
		return CreateOrderResult{}, err
	}
	params := url.Values{}
	params.Set("pid", settings.PID)
	params.Set("type", payType)
	params.Set("out_trade_no", order.OutTradeNo)
	params.Set("notify_url", s.publicBase+"/api/payments/epay/notify")
	params.Set("return_url", s.publicBase+"/billing")
	params.Set("name", order.Name)
	params.Set("money", formatMoney(order.AmountCents))
	params.Set("sitename", "ImageMirror")
	params.Set("sign_type", "MD5")
	params.Set("sign", sign(params, key))
	return CreateOrderResult{Order: order, PayURL: settings.Gateway + "/submit.php?" + params.Encode()}, nil
}

func (s *Service) HandleEPayNotify(ctx context.Context, values url.Values) (NotifyResult, error) {
	settings, key, err := s.configStore.GetEPay(ctx)
	if err != nil {
		return NotifyResult{}, err
	}
	if settings.PID == "" || key == "" {
		return NotifyResult{}, errors.New("epay merchant config is incomplete")
	}
	if !verify(values, key) {
		return NotifyResult{}, errors.New("invalid signature")
	}
	if strings.TrimSpace(values.Get("pid")) != settings.PID {
		return NotifyResult{}, errors.New("invalid merchant id")
	}
	outTradeNo := strings.TrimSpace(values.Get("out_trade_no"))
	if outTradeNo == "" {
		return NotifyResult{}, errors.New("missing out_trade_no")
	}
	tradeStatus := strings.ToUpper(strings.TrimSpace(values.Get("trade_status")))
	if tradeStatus != "TRADE_SUCCESS" {
		return NotifyResult{OutTradeNo: outTradeNo, Status: "ignored"}, nil
	}
	providerTradeNo := strings.TrimSpace(values.Get("trade_no"))
	if providerTradeNo == "" {
		return NotifyResult{}, errors.New("missing trade_no")
	}
	amountCents, err := parseMoneyCents(values.Get("money"))
	if err != nil {
		return NotifyResult{}, err
	}
	order, err := s.markPaid(ctx, outTradeNo, providerTradeNo, amountCents)
	if err != nil {
		return NotifyResult{}, err
	}
	return NotifyResult{OutTradeNo: order.OutTradeNo, Status: order.Status}, nil
}

func (s *Service) markPaid(ctx context.Context, outTradeNo string, providerTradeNo string, expectedAmountCents int64) (Order, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row := tx.QueryRow(ctx, `
		SELECT id, user_id, provider, out_trade_no, provider_trade_no, name, amount_cents, credits, status, paid_at, created_at, updated_at
		FROM payment_orders
		WHERE out_trade_no=$1
		FOR UPDATE
	`, outTradeNo)
	order, err := scanOrder(row)
	if err != nil {
		return Order{}, err
	}
	if order.AmountCents != expectedAmountCents {
		return Order{}, errors.New("payment amount does not match order")
	}
	if order.Status != "PAID" {
		var tradeNo *string
		if providerTradeNo != "" {
			tradeNo = &providerTradeNo
		}
		row = tx.QueryRow(ctx, `
			UPDATE payment_orders
			SET status='PAID', provider_trade_no=$2, paid_at=now(), updated_at=now()
			WHERE id=$1
			RETURNING id, user_id, provider, out_trade_no, provider_trade_no, name, amount_cents, credits, status, paid_at, created_at, updated_at
		`, order.ID, tradeNo)
		order, err = scanOrder(row)
		if err != nil {
			return Order{}, err
		}
		if err := changeWithTx(ctx, tx, order.UserID, order.Credits, "RECHARGE", "epay recharge "+order.OutTradeNo, order.ID); err != nil {
			return Order{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return order, nil
}

func scanOrder(row pgx.Row) (Order, error) {
	var order Order
	var paidAt pgtype.Timestamptz
	if err := row.Scan(&order.ID, &order.UserID, &order.Provider, &order.OutTradeNo, &order.ProviderTradeNo, &order.Name, &order.AmountCents, &order.Credits, &order.Status, &paidAt, &order.CreatedAt, &order.UpdatedAt); err != nil {
		return Order{}, err
	}
	if paidAt.Valid {
		order.PaidAt = &paidAt.Time
	}
	return order, nil
}

func changeWithTx(ctx context.Context, tx pgx.Tx, userID string, delta int64, txType string, description string, relatedID string) error {
	var balance int64
	if err := tx.QueryRow(ctx, `SELECT balance FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&balance); err != nil {
		return err
	}
	next := balance + delta
	related := relatedID
	if _, err := tx.Exec(ctx, `UPDATE users SET balance=$1, updated_at=now() WHERE id=$2`, next, userID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO credit_transactions(user_id, type, amount, balance_after, description, related_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, txType, delta, next, description, &related)
	return err
}

func sign(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for k := range values {
		if k == "sign" || k == "sign_type" || values.Get(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+values.Get(k))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + key))
	return hex.EncodeToString(sum[:])
}

func verify(values url.Values, key string) bool {
	expected := strings.ToLower(values.Get("sign"))
	return expected != "" && expected == sign(values, key)
}

func formatMoney(cents int64) string {
	return fmt.Sprintf("%d.%02d", cents/100, cents%100)
}

func randomOutTradeNo() (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("IM%s%s", time.Now().UTC().Format("20060102150405"), strings.ToUpper(hex.EncodeToString(suffix[:]))), nil
}

func normalizePayType(payType string) (string, error) {
	payType = strings.ToLower(strings.TrimSpace(payType))
	if payType == "" {
		return "alipay", nil
	}
	switch payType {
	case "alipay", "wxpay", "qqpay":
		return payType, nil
	default:
		return "", errors.New("unsupported payment type")
	}
}

func parseMoneyCents(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("missing money")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid money")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole < 0 {
		return 0, errors.New("invalid money")
	}
	if whole > 1_000_000_000_000 {
		return 0, errors.New("invalid money")
	}
	cents := whole * 100
	if len(parts) == 1 {
		return cents, nil
	}
	fraction := parts[1]
	if len(fraction) > 2 {
		for _, ch := range fraction[2:] {
			if ch != '0' {
				return 0, errors.New("invalid money")
			}
		}
		fraction = fraction[:2]
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	fractionCents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, errors.New("invalid money")
	}
	return cents + fractionCents, nil
}
