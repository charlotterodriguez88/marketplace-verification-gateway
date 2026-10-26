package marketplace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"sync"
	"time"
)

var (
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrInvalidVerification    = errors.New("verification link is invalid or expired")
	ErrVerificationRequired   = errors.New("seller email verification required")
)

type EmailSender interface {
	Send(ctx context.Context, idempotencyKey, to, subject, htmlBody string) (string, error)
}

type SellerAsset struct {
	SKU   string `json:"sku"`
	Title string `json:"title"`
}

type BuyerUpdate struct {
	OrderID string `json:"order_id"`
	Text    string `json:"text"`
}

type OrderHandoff struct {
	OrderID string `json:"order_id"`
	Carrier string `json:"carrier"`
	Status  string `json:"status"`
}

type Seller struct {
	ID            string         `json:"id"`
	Email         string         `json:"email"`
	EmailVerified bool           `json:"email_verified"`
	Assets        []SellerAsset  `json:"assets"`
	BuyerUpdates  []BuyerUpdate  `json:"buyer_updates"`
	Handoffs      []OrderHandoff `json:"handoffs"`
}

type signupRecord struct {
	seller    Seller
	token     string
	expiresAt time.Time
}

type SignupFlow struct {
	mu        sync.Mutex
	sender    EmailSender
	publicURL string
	now       func() time.Time
	newToken  func() (string, error)
	records   map[string]*signupRecord
}

func NewSignupFlow(sender EmailSender, publicURL string) *SignupFlow {
	return &SignupFlow{
		sender: sender, publicURL: publicURL, now: time.Now,
		newToken: randomToken, records: make(map[string]*signupRecord),
	}
}

func (f *SignupFlow) Signup(ctx context.Context, sellerID, email string, assets []SellerAsset) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.records[sellerID]; exists {
		return "", ErrEmailAlreadyRegistered
	}
	token, err := f.newToken()
	if err != nil {
		return "", fmt.Errorf("create verification token: %w", err)
	}
	link := fmt.Sprintf("%s/verify?token=%s", f.publicURL, token)
	body := fmt.Sprintf("<p>Verify your seller email to publish buyer updates and hand off orders.</p><p><a href=\"%s\">Verify email</a></p>", html.EscapeString(link))
	messageID, err := f.sender.Send(ctx, "seller-signup:"+sellerID, email, "Verify your marketplace email", body)
	if err != nil {
		return "", err
	}
	f.records[sellerID] = &signupRecord{
		seller: Seller{ID: sellerID, Email: email, Assets: assets},
		token:  token, expiresAt: f.now().Add(30 * time.Minute),
	}
	return messageID, nil
}

func (f *SignupFlow) Verify(token string) (Seller, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, record := range f.records {
		if record.token == token && f.now().Before(record.expiresAt) && !record.seller.EmailVerified {
			record.seller.EmailVerified = true
			record.token = ""
			return record.seller, nil
		}
	}
	return Seller{}, ErrInvalidVerification
}

func (f *SignupFlow) PublishBuyerUpdate(sellerID string, update BuyerUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.records[sellerID]
	if !ok || !record.seller.EmailVerified {
		return ErrVerificationRequired
	}
	record.seller.BuyerUpdates = append(record.seller.BuyerUpdates, update)
	return nil
}

func (f *SignupFlow) HandoffOrder(sellerID string, handoff OrderHandoff) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	record, ok := f.records[sellerID]
	if !ok || !record.seller.EmailVerified {
		return ErrVerificationRequired
	}
	record.seller.Handoffs = append(record.seller.Handoffs, handoff)
	return nil
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
