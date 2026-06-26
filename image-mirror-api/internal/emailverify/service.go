package emailverify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/linxunxi/image-mirror/internal/systemconfig"
	"github.com/linxunxi/image-mirror/internal/users"
)

const (
	registrationPurpose = "REGISTER"
	codeTTL             = 10 * time.Minute
	resendCooldown      = time.Minute
	maxCodesPerHour     = 5
)

var sixDigitCodePattern = regexp.MustCompile(`^\d{6}$`)

type Service struct {
	db     *pgxpool.Pool
	config *systemconfig.Service
	users  *users.Repository
}

func NewService(db *pgxpool.Pool, configSvc *systemconfig.Service, usersRepo *users.Repository) *Service {
	return &Service{db: db, config: configSvc, users: usersRepo}
}

func (s *Service) SendRegistrationCode(ctx context.Context, email string) error {
	email, err := normalizeQQEmail(email)
	if err != nil {
		return err
	}
	authSettings, err := s.config.PublicAuth(ctx)
	if err != nil {
		return err
	}
	if !authSettings.EmailVerificationEnabled {
		return errors.New("邮箱验证未启用")
	}
	if _, err := s.users.FindByEmail(ctx, email); err == nil {
		return errors.New("该邮箱已注册")
	} else if !errors.Is(err, users.ErrUserNotFound) {
		return err
	}
	if err := s.ensureSendQuota(ctx, email); err != nil {
		return err
	}
	settings, smtpPassword, err := s.config.GetEmailVerificationSender(ctx)
	if err != nil {
		return err
	}
	if err := validateSender(settings, smtpPassword); err != nil {
		return err
	}
	code, err := randomNumericCode(6)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE email_verification_codes
		SET consumed_at=now()
		WHERE email=$1 AND purpose=$2 AND consumed_at IS NULL
	`, email, registrationPurpose); err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO email_verification_codes(email, purpose, code_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, email, registrationPurpose, codeHash(email, registrationPurpose, code), time.Now().Add(codeTTL))
	if err != nil {
		return err
	}
	if err := sendVerificationMail(settings, smtpPassword, email, code); err != nil {
		_, _ = s.db.Exec(ctx, `
			UPDATE email_verification_codes
			SET consumed_at=now()
			WHERE email=$1 AND purpose=$2 AND code_hash=$3 AND consumed_at IS NULL
		`, email, registrationPurpose, codeHash(email, registrationPurpose, code))
		return fmt.Errorf("验证码邮件发送失败: %w", err)
	}
	return nil
}

func (s *Service) ConsumeRegistrationCode(ctx context.Context, email string, code string) error {
	email, err := normalizeQQEmail(email)
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if !sixDigitCodePattern.MatchString(code) {
		return errors.New("验证码无效或已过期")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE email_verification_codes
		SET consumed_at=now()
		WHERE id=(
			SELECT id
			FROM email_verification_codes
			WHERE email=$1
				AND purpose=$2
				AND code_hash=$3
				AND consumed_at IS NULL
				AND expires_at>now()
			ORDER BY created_at DESC
			LIMIT 1
		)
	`, email, registrationPurpose, codeHash(email, registrationPurpose, code))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("验证码无效或已过期")
	}
	return nil
}

func (s *Service) ValidateRegistrationCode(ctx context.Context, email string, code string) error {
	email, err := normalizeQQEmail(email)
	if err != nil {
		return err
	}
	code = strings.TrimSpace(code)
	if !sixDigitCodePattern.MatchString(code) {
		return errors.New("验证码无效或已过期")
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM email_verification_codes
			WHERE email=$1
				AND purpose=$2
				AND code_hash=$3
				AND consumed_at IS NULL
				AND expires_at>now()
		)
	`, email, registrationPurpose, codeHash(email, registrationPurpose, code)).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("验证码无效或已过期")
	}
	return nil
}

func (s *Service) ensureSendQuota(ctx context.Context, email string) error {
	var lastSentAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT sent_at
		FROM email_verification_codes
		WHERE email=$1 AND purpose=$2
		ORDER BY sent_at DESC
		LIMIT 1
	`, email, registrationPurpose).Scan(&lastSentAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && time.Since(lastSentAt) < resendCooldown {
		return errors.New("验证码发送过于频繁，请稍后再试")
	}
	var count int
	if err := s.db.QueryRow(ctx, `
		SELECT count(*)
		FROM email_verification_codes
		WHERE email=$1 AND purpose=$2 AND sent_at>now()-interval '1 hour'
	`, email, registrationPurpose).Scan(&count); err != nil {
		return err
	}
	if count >= maxCodesPerHour {
		return errors.New("验证码发送次数过多，请稍后再试")
	}
	return nil
}

func validateSender(settings systemconfig.EmailVerificationSettings, password string) error {
	if !settings.Enabled {
		return errors.New("邮箱验证未启用")
	}
	if _, err := mail.ParseAddress(settings.SenderEmail); err != nil {
		return errors.New("发信邮箱格式无效")
	}
	if strings.TrimSpace(settings.SMTPUsername) == "" {
		return errors.New("请先配置 SMTP 用户名")
	}
	if strings.TrimSpace(password) == "" {
		return errors.New("请先配置 SMTP 密码")
	}
	if strings.TrimSpace(settings.SMTPHost) == "" || settings.SMTPPort <= 0 {
		return errors.New("邮箱服务器配置无效")
	}
	return nil
}

func normalizeQQEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !strings.HasSuffix(email, "@qq.com") || len(email) <= len("@qq.com") {
		return "", errors.New("仅支持 QQ 邮箱")
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return "", errors.New("邮箱格式无效")
	}
	return email, nil
}

func randomNumericCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid code length")
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, value := range buf {
		out[i] = byte('0' + value%10)
	}
	return string(out), nil
}

func codeHash(email string, purpose string, code string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email)) + ":" + purpose + ":" + strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func sendVerificationMail(settings systemconfig.EmailVerificationSettings, password string, to string, code string) error {
	subject := "ImageMirror 注册验证码"
	body := fmt.Sprintf("您的注册验证码是：%s\n\n验证码 10 分钟内有效。如非本人操作，请忽略本邮件。", code)
	message, err := buildTextMessage(settings, to, subject, body)
	if err != nil {
		return err
	}
	return sendSMTP(settings, password, to, message)
}

func buildTextMessage(settings systemconfig.EmailVerificationSettings, to string, subject string, body string) ([]byte, error) {
	from := mail.Address{Name: settings.SenderName, Address: settings.SenderEmail}
	toAddress := mail.Address{Address: to}
	var buffer bytes.Buffer
	headers := map[string]string{
		"From":                      from.String(),
		"To":                        toAddress.String(),
		"Subject":                   mime.QEncoding.Encode("UTF-8", subject),
		"MIME-Version":              "1.0",
		"Content-Type":              `text/plain; charset="UTF-8"`,
		"Content-Transfer-Encoding": "quoted-printable",
		"Date":                      time.Now().Format(time.RFC1123Z),
	}
	for key, value := range headers {
		buffer.WriteString(key)
		buffer.WriteString(": ")
		buffer.WriteString(value)
		buffer.WriteString("\r\n")
	}
	buffer.WriteString("\r\n")
	writer := quotedprintable.NewWriter(&buffer)
	if _, err := writer.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func sendSMTP(settings systemconfig.EmailVerificationSettings, password string, to string, message []byte) error {
	addr := net.JoinHostPort(settings.SMTPHost, strconv.Itoa(settings.SMTPPort))
	auth := smtp.PlainAuth("", settings.SMTPUsername, password, settings.SMTPHost)
	var client *smtp.Client
	var err error
	if settings.SMTPPort == 465 {
		dialer := &net.Dialer{Timeout: 15 * time.Second}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: settings.SMTPHost, MinVersion: tls.VersionTLS12})
		if err != nil {
			return err
		}
		client, err = smtp.NewClient(conn, settings.SMTPHost)
		if err != nil {
			_ = conn.Close()
			return err
		}
	} else {
		conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			return err
		}
		client, err = smtp.NewClient(conn, settings.SMTPHost)
		if err != nil {
			_ = conn.Close()
			return err
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: settings.SMTPHost, MinVersion: tls.VersionTLS12}); err != nil {
				_ = client.Close()
				return err
			}
		}
	}
	defer client.Close()
	if err := client.Auth(auth); err != nil {
		return err
	}
	if err := client.Mail(settings.SenderEmail); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
