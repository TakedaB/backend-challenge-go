package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/TakedaB/backend-challenge-go/internal/domain/money"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wagertransaction"
	"github.com/TakedaB/backend-challenge-go/internal/domain/wallet"
	"github.com/TakedaB/backend-challenge-go/internal/infra/postgres"
)

// writeJSON writes v as the JSON body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError maps a domain/repository error to the appropriate HTTP
// status code. Validation errors from money/wallet/wagertransaction
// (malformed input) map to 400; not-found lookups map to 404;
// anything unrecognized maps to 500 so we never leak internal details
// in the response body.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, postgres.ErrWalletNotFound), errors.Is(err, postgres.ErrWagerTransactionNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
	case isValidationError(err):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal server error"})
	}
}

// isValidationError reports whether err is one of the sentinel
// validation errors from the domain packages — i.e. the request
// itself was malformed, as opposed to a processing/infrastructure
// failure.
func isValidationError(err error) bool {
	validationErrors := []error{
		money.ErrEmptyAmount, money.ErrInvalidFormat, money.ErrExcessiveScale,
		money.ErrNegativeNotAllowed, money.ErrInvalidCurrency, money.ErrCurrencyMismatch, money.ErrOverflow,
		wallet.ErrCurrencyMismatch, wallet.ErrNonPositiveAmount, wallet.ErrEmptyPlayerID, wallet.ErrEmptyWalletID,
		wagertransaction.ErrOpeningNotAllowedExternally, wagertransaction.ErrInvalidKind,
		wagertransaction.ErrMissingExternalMetadata, wagertransaction.ErrLossAmountMustBeZero,
		wagertransaction.ErrAmountMustBePositive, wagertransaction.ErrReferenceRequired,
		wagertransaction.ErrReferenceNotApplicable,
	}
	for _, ve := range validationErrors {
		if errors.Is(err, ve) {
			return true
		}
	}
	return false
}
