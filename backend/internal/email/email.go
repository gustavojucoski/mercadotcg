package email

import "context"

// Message representa um email a ser enviado.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Provider é a interface de envio de email. Permite trocar Resend por outro
// provedor sem alterar o AuthService.
type Provider interface {
	Send(ctx context.Context, msg Message) error
}
