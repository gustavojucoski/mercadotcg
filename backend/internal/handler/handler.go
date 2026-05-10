// Package handler concentra os adaptadores HTTP da API.
//
// Cada handler recebe os repositórios/serviços via construtor e expõe
// funções `http.Handler` que ficam montadas pelo router chi em cmd/api.
package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// errorBody é o payload JSON de qualquer resposta de erro da API.
type errorBody struct {
	Error string `json:"error"`
}

// writeJSON serializa `payload` como JSON e devolve com o status fornecido.
// Em caso de falha de encoding, loga e devolve 500.
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Error().Err(err).Msg("encode JSON")
	}
}

// writeErr converte um erro de domínio/repositório no status HTTP correto.
// Mapeia sentinelas conhecidos:
//   - postgres.ErrNotFound          → 404
//   - postgres.ErrAlreadyExists     → 409
//   - postgres.ErrInsufficientStock → 422
//   - postgres.ErrInvalidMovement   → 400
func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, postgres.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: "registro não encontrado"})
	case errors.Is(err, postgres.ErrAlreadyExists):
		writeJSON(w, http.StatusConflict, errorBody{Error: "registro já existe"})
	case errors.Is(err, postgres.ErrInsufficientStock):
		writeJSON(w, http.StatusUnprocessableEntity, errorBody{Error: err.Error()})
	case errors.Is(err, postgres.ErrInvalidMovement):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
	default:
		log.Error().Err(err).Msg("internal error")
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "erro interno"})
	}
}

// writeBadRequest é o atalho para 400 com mensagem custom.
func writeBadRequest(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusBadRequest, errorBody{Error: msg})
}

// decodeJSON lê o corpo do request em `dst`. Rejeita campos desconhecidos
// para erros de schema saírem cedo (e barato).
func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("body inválido: %w", err)
	}
	return nil
}

// parseUUID converte uma string em uuid.UUID e devolve erro de validação
// caso a string seja inválida.
func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("uuid inválido: %w", err)
	}
	return id, nil
}
