package marketplace

import (
	"context"
	"errors"
	"testing"
)

type recordingSender struct {
	key, to, subject, body string
}

func (s *recordingSender) Send(_ context.Context, key, to, subject, body string) (string, error) {
	s.key, s.to, s.subject, s.body = key, to, subject, body
	return "msg_42", nil
}

func TestVerificationControlsMarketplaceActions(t *testing.T) {
	tests := []struct {
		name   string
		action func(*SignupFlow) error
	}{
		{"buyer update", func(f *SignupFlow) error {
			return f.PublishBuyerUpdate("seller-42", BuyerUpdate{OrderID: "order-7", Text: "Packed"})
		}},
		{"order handoff", func(f *SignupFlow) error {
			return f.HandoffOrder("seller-42", OrderHandoff{OrderID: "order-7", Carrier: "ParcelCo", Status: "ready"})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingSender{}
			flow := NewSignupFlow(sender, "http://localhost:8080")
			flow.newToken = func() (string, error) { return "token-42", nil }
			messageID, err := flow.Signup(context.Background(), "seller-42", "seller@example.com", []SellerAsset{{SKU: "lamp-1", Title: "Desk lamp"}})
			if err != nil || messageID != "msg_42" {
				t.Fatalf("signup = %q, %v", messageID, err)
			}
			if sender.key != "seller-signup:seller-42" {
				t.Fatalf("idempotency key = %q", sender.key)
			}
			if err := tt.action(flow); !errors.Is(err, ErrVerificationRequired) {
				t.Fatalf("before verification = %v", err)
			}
			seller, err := flow.Verify("token-42")
			if err != nil || !seller.EmailVerified {
				t.Fatalf("verify = %#v, %v", seller, err)
			}
			if err := tt.action(flow); err != nil {
				t.Fatalf("after verification = %v", err)
			}
		})
	}
}
