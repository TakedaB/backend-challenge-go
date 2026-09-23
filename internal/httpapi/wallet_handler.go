package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/TakedaB/backend-challenge-go/internal/app"
	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
)

// WalletHandler exposes the wallet-related HTTP endpoints. It talks
// only to the app.WalletService — it never touches postgres or the
// domain packages' constructors directly, keeping HTTP concerns
// (status codes, JSON shapes) fully separate from business logic.
type WalletHandler struct {
	service *app.WalletService
}

// NewWalletHandler constructs a WalletHandler. Fx injects the
// *app.WalletService it already knows how to build.
func NewWalletHandler(service *app.WalletService) *WalletHandler {
	return &WalletHandler{service: service}
}

func toWalletResponse(w interface {
	ID() string
	PlayerID() string
	Currency() string
	Balance() money.Money
	Version() int64
	UpdatedAt() time.Time
}) walletResponse {
	return walletResponse{
		ID:        w.ID(),
		PlayerID:  w.PlayerID(),
		Currency:  w.Currency(),
		Balance:   w.Balance().String(),
		Version:   w.Version(),
		UpdatedAt: w.UpdatedAt(),
	}
}

// HandleOpenWallet handles POST /wallets.
func (h *WalletHandler) HandleOpenWallet(w http.ResponseWriter, r *http.Request) {
	var req openWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}

	initialBalance, err := money.NewFromString(req.InitialBalance, req.Currency, false)
	if err != nil {
		writeError(w, err)
		return
	}

	wal, err := h.service.OpenWallet(r.Context(), req.PlayerID, initialBalance)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, toWalletResponse(wal))
}

// HandleGetWallet handles GET /wallets/{id}.
func (h *WalletHandler) HandleGetWallet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	wal, err := h.service.GetWallet(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, toWalletResponse(wal))
}
