package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleProfile é o subconjunto do userinfo do Google que usamos.
type GoogleProfile struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

type OAuthService struct {
	cfg     *oauth2.Config
	hmacKey []byte
}

// NewOAuthService cria o serviço. hmacKey pode ser vazio — nesse caso o state
// param usa apenas bytes aleatórios sem HMAC (desenvolvimento sem chave).
func NewOAuthService(clientID, clientSecret, redirectURL, hmacKeyHex string) *OAuthService {
	key, _ := hex.DecodeString(hmacKeyHex)
	return &OAuthService{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
		hmacKey: key,
	}
}

// AuthCodeURL gera a URL de autorização do Google com state assinado via HMAC.
func (s *OAuthService) AuthCodeURL() (url, state string, err error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := make([]byte, 8)
	if _, err = rand.Read(nonce); err != nil {
		return "", "", fmt.Errorf("gerar nonce oauth: %w", err)
	}
	raw := ts + "." + hex.EncodeToString(nonce)

	var sig string
	if len(s.hmacKey) > 0 {
		mac := hmac.New(sha256.New, s.hmacKey)
		mac.Write([]byte(raw))
		sig = hex.EncodeToString(mac.Sum(nil))
	}
	state = base64.RawURLEncoding.EncodeToString([]byte(raw + "." + sig))
	url = s.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
	return url, state, nil
}

// ValidateState verifica a assinatura HMAC e a idade do state param.
func (s *OAuthService) ValidateState(state string) error {
	decoded, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil {
		return fmt.Errorf("state inválido: %w", err)
	}
	parts := strings.SplitN(string(decoded), ".", 3)
	if len(parts) < 2 {
		return fmt.Errorf("state malformado")
	}
	tsStr := parts[0]
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("state timestamp inválido")
	}
	if time.Since(time.Unix(ts, 0)) > 10*time.Minute {
		return fmt.Errorf("state expirado")
	}
	if len(s.hmacKey) == 0 {
		return nil
	}
	// Verificar HMAC: partes[0].partes[1] são o payload; partes[2] é a assinatura.
	if len(parts) < 3 {
		return fmt.Errorf("state sem assinatura")
	}
	payload := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.hmacKey)
	mac.Write([]byte(payload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return fmt.Errorf("state com assinatura inválida")
	}
	return nil
}

// Exchange troca o code pelo perfil Google.
func (s *OAuthService) Exchange(ctx context.Context, code string) (*GoogleProfile, error) {
	tok, err := s.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("trocar code oauth: %w", err)
	}
	client := s.cfg.Client(ctx, tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return nil, fmt.Errorf("buscar userinfo google: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo google status %d: %s", resp.StatusCode, body)
	}
	var profile GoogleProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decodificar perfil google: %w", err)
	}
	return &profile, nil
}

// IsConfigured retorna true se as credenciais Google estão presentes.
func (s *OAuthService) IsConfigured() bool {
	return s.cfg.ClientID != "" && s.cfg.ClientSecret != ""
}
