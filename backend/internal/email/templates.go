package email

import "fmt"

func VerificationEmail(to, displayName, verifyURL string) Message {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<body style="font-family:sans-serif;max-width:600px;margin:0 auto;padding:24px">
  <h2 style="color:#1a1a2e">Bem-vindo ao MercadoTCG, %s!</h2>
  <p>Para ativar sua conta, clique no botão abaixo:</p>
  <a href="%s" style="display:inline-block;background:#6c47ff;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:bold">
    Verificar email
  </a>
  <p style="color:#666;font-size:13px;margin-top:24px">
    O link expira em 24 horas. Se você não criou uma conta, ignore este email.
  </p>
</body>
</html>`, displayName, verifyURL)

	text := fmt.Sprintf(
		"Bem-vindo ao MercadoTCG, %s!\n\nVerifique seu email acessando:\n%s\n\nO link expira em 24 horas.",
		displayName, verifyURL,
	)
	return Message{
		To:      to,
		Subject: "Verifique seu email — MercadoTCG",
		HTML:    html,
		Text:    text,
	}
}

func PasswordResetEmail(to, displayName, resetURL string) Message {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="pt-BR">
<body style="font-family:sans-serif;max-width:600px;margin:0 auto;padding:24px">
  <h2 style="color:#1a1a2e">Redefinição de senha — MercadoTCG</h2>
  <p>Olá, %s. Recebemos uma solicitação para redefinir a senha da sua conta.</p>
  <a href="%s" style="display:inline-block;background:#6c47ff;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:bold">
    Redefinir senha
  </a>
  <p style="color:#666;font-size:13px;margin-top:24px">
    O link expira em 1 hora. Se você não solicitou a redefinição, ignore este email.
  </p>
</body>
</html>`, displayName, resetURL)

	text := fmt.Sprintf(
		"Olá, %s.\n\nRedefina sua senha acessando:\n%s\n\nO link expira em 1 hora.",
		displayName, resetURL,
	)
	return Message{
		To:      to,
		Subject: "Redefinição de senha — MercadoTCG",
		HTML:    html,
		Text:    text,
	}
}
