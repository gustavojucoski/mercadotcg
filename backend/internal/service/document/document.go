// Package document provides CNPJ and CPF validation and lookup.
package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid   = errors.New("documento inválido")
var ErrRateLimit = errors.New("rate limit da ReceitaWS atingido")

var nonDigit = regexp.MustCompile(`\D`)

// CNPJInfo holds the result of a ReceitaWS lookup.
type CNPJInfo struct {
	LegalName           string
	TradeName           string
	Situation           string // "ATIVA" | "BAIXADA" | etc.
	Phone               string
	AddressZip          string // 8 digits, no mask
	AddressStreet       string
	AddressNumber       string
	AddressComplement   string
	AddressNeighborhood string
	AddressCity         string
	AddressState        string // UF, 2 chars
}

// ValidateCNPJ strips formatting, checks length, and validates the two check digits.
// Returns the 14-digit string on success.
func ValidateCNPJ(s string) (string, error) {
	d := nonDigit.ReplaceAllString(s, "")
	if len(d) != 14 {
		return "", fmt.Errorf("%w: CNPJ deve ter 14 dígitos", ErrInvalid)
	}
	// all-same digit is invalid
	allSame := true
	for i := 1; i < 14; i++ {
		if d[i] != d[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return "", fmt.Errorf("%w: CNPJ com todos os dígitos iguais", ErrInvalid)
	}
	if !cnpjCheckDigits(d) {
		return "", fmt.Errorf("%w: dígitos verificadores incorretos", ErrInvalid)
	}
	return d, nil
}

func cnpjCheckDigits(d string) bool {
	w1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	w2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	return cnpjDigit(d[:12], w1) == int(d[12]-'0') &&
		cnpjDigit(d[:13], w2) == int(d[13]-'0')
}

func cnpjDigit(s string, weights []int) int {
	sum := 0
	for i, c := range s {
		sum += int(c-'0') * weights[i]
	}
	r := (sum * 10) % 11
	if r >= 10 {
		return 0
	}
	return r
}

// ValidateCPF strips formatting, checks length, and validates the two check digits.
// Returns the 11-digit string on success.
func ValidateCPF(s string) (string, error) {
	d := nonDigit.ReplaceAllString(s, "")
	if len(d) != 11 {
		return "", fmt.Errorf("%w: CPF deve ter 11 dígitos", ErrInvalid)
	}
	allSame := true
	for i := 1; i < 11; i++ {
		if d[i] != d[0] {
			allSame = false
			break
		}
	}
	if allSame {
		return "", fmt.Errorf("%w: CPF com todos os dígitos iguais", ErrInvalid)
	}
	if !cpfCheckDigits(d) {
		return "", fmt.Errorf("%w: dígitos verificadores incorretos", ErrInvalid)
	}
	return d, nil
}

func cpfCheckDigits(d string) bool {
	w1 := []int{10, 9, 8, 7, 6, 5, 4, 3, 2}
	w2 := []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2}
	return cpfDigit(d[:9], w1) == int(d[9]-'0') &&
		cpfDigit(d[:10], w2) == int(d[10]-'0')
}

func cpfDigit(s string, weights []int) int {
	sum := 0
	for i, c := range s {
		sum += int(c-'0') * weights[i]
	}
	r := (sum * 10) % 11
	if r >= 10 {
		return 0
	}
	return r
}

// LookupCNPJ queries ReceitaWS for public CNPJ data.
// digits must be a 14-character string with no formatting.
func LookupCNPJ(ctx context.Context, digits string) (*CNPJInfo, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	url := "https://www.receitaws.com.br/v1/cnpj/" + digits

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("criar requisição ReceitaWS: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("consultar ReceitaWS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, ErrRateLimit
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ReceitaWS retornou status %d", resp.StatusCode)
	}

	var payload struct {
		Nome        string `json:"nome"`
		Fantasia    string `json:"fantasia"`
		Situacao    string `json:"situacao"`
		Status      string `json:"status"` // "ERROR" quando não encontrado
		Message     string `json:"message"`
		Telefone1   string `json:"ddd_telefone_1"`
		CEP         string `json:"cep"`
		Logradouro  string `json:"logradouro"`
		Numero      string `json:"numero"`
		Complemento string `json:"complemento"`
		Bairro      string `json:"bairro"`
		Municipio   string `json:"municipio"`
		UF          string `json:"uf"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decodificar resposta ReceitaWS: %w", err)
	}
	if payload.Status == "ERROR" {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, payload.Message)
	}
	return &CNPJInfo{
		LegalName:           payload.Nome,
		TradeName:           payload.Fantasia,
		Situation:           payload.Situacao,
		Phone:               payload.Telefone1,
		AddressZip:          strings.ReplaceAll(payload.CEP, "-", ""),
		AddressStreet:       payload.Logradouro,
		AddressNumber:       payload.Numero,
		AddressComplement:   payload.Complemento,
		AddressNeighborhood: payload.Bairro,
		AddressCity:         payload.Municipio,
		AddressState:        payload.UF,
	}, nil
}
