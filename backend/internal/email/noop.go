package email

import (
	"context"

	"github.com/rs/zerolog/log"
)

// NoopProvider loga o email no stdout em vez de enviá-lo.
// Usar em desenvolvimento quando RESEND_API_KEY não está configurada.
type NoopProvider struct{}

func NewNoopProvider() *NoopProvider { return &NoopProvider{} }

func (p *NoopProvider) Send(_ context.Context, msg Message) error {
	log.Info().
		Str("to", msg.To).
		Str("subject", msg.Subject).
		Msg("[email:noop] email não enviado — configure RESEND_API_KEY")
	return nil
}
