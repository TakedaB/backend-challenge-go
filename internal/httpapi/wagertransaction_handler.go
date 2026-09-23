package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/TakedaB/backend-challenge-go/internal/app"
	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wagertransaction"
)

// WagerTransactionHandler exposes the wagering endpoint.
type WagerTransactionHandler struct {
	service *app.WalletService
}

// NewWagerTransactionHandler constructs a WagerTransactionHandler.
func NewWagerTransactionHandler(service *app.WalletService) *WagerTransactionHandler {
	return &WagerTransactionHandler{service: service}
}

// HandleProcessTransaction handles POST /wagering/transactions.
//
// IdempotencyKey is read from the Idempotency-Key header when
// present, falling back to the JSON body's idempotencyKey field —
// this keeps curl/Postman testing simple (body-only) while still
// supporting the header-based convention the README describes.
func (h *WagerTransactionHandler) HandleProcessTransaction(w http.ResponseWriter, r *http.Request) {
	var req processWagerTransactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		idempotencyKey = req.IdempotencyKey
	}

	amount, err := money.NewFromString(req.Amount, req.Currency, false)
	if err != nil {
		writeError(w, err)
		return
	}

	input := app.ProcessWagerTransactionInput{
		ExternalTransactionID:          req.ExternalTransactionID,
		ProviderID:                     req.ProviderID,
		IdempotencyKey:                 idempotencyKey,
		PayloadHash:                    hashPayload(req),
		WalletID:                       req.WalletID,
		PlayerID:                       req.PlayerID,
		RoundID:                        req.RoundID,
		GameID:                         req.GameID,
		Kind:                           wagertransaction.Kind(req.Kind),
		Amount:                         amount,
		ReferenceExternalTransactionID: req.ReferenceExternalTransactionID,
	}

	tx, err := h.service.ProcessWagerTransaction(r.Context(), input)
	if err != nil {
		writeError(w, err)
		return
	}

	status := http.StatusOK
	if tx.State() == wagertransaction.StateRejected {
		status = http.StatusUnprocessableEntity
	}

	writeJSON(w, status, wagerTransactionResponse{
		ID:                    tx.ID(),
		ExternalTransactionID: tx.ExternalTransactionID(),
		WalletID:              tx.WalletID(),
		Kind:                  string(tx.Kind()),
		Amount:                tx.Amount().String(),
		State:                 string(tx.State()),
		FailureCode:           tx.FailureCode(),
	})
}
