package email

import (
	"context"
	"fmt"

	"github.com/resend/resend-go/v2"
)

// ResendProvider envia emails via API Resend.
type ResendProvider struct {
	client *resend.Client
	from   string
}

func NewResendProvider(apiKey, from string) *ResendProvider {
	return &ResendProvider{
		client: resend.NewClient(apiKey),
		from:   from,
	}
}

func (p *ResendProvider) Send(ctx context.Context, msg Message) error {
	params := &resend.SendEmailRequest{
		From:    p.from,
		To:      []string{msg.To},
		Subject: msg.Subject,
		Html:    msg.HTML,
		Text:    msg.Text,
	}
	_, err := p.client.Emails.SendWithContext(ctx, params)
	if err != nil {
		return fmt.Errorf("resend send: %w", err)
	}
	return nil
}
