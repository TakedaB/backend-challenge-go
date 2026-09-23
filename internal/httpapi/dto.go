package httpapi

import "time"

type openWalletRequest struct {
	PlayerID       string `json:"playerId"`
	Currency       string `json:"currency"`
	InitialBalance string `json:"initialBalance"`
}

type walletResponse struct {
	ID        string    `json:"id"`
	PlayerID  string    `json:"playerId"`
	Currency  string    `json:"currency"`
	Balance   string    `json:"balance"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type processWagerTransactionRequest struct {
	ExternalTransactionID          string `json:"externalTransactionId"`
	ProviderID                     string `json:"providerId"`
	IdempotencyKey                 string `json:"idempotencyKey"`
	WalletID                       string `json:"walletId"`
	PlayerID                       string `json:"playerId"`
	RoundID                        string `json:"roundId"`
	GameID                         string `json:"gameId"`
	Kind                           string `json:"kind"`
	Amount                         string `json:"amount"`
	Currency                       string `json:"currency"`
	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId"`
}

type wagerTransactionResponse struct {
	ID                    string `json:"id"`
	ExternalTransactionID string `json:"externalTransactionId,omitempty"`
	WalletID              string `json:"walletId"`
	Kind                  string `json:"kind"`
	Amount                string `json:"amount"`
	State                 string `json:"state"`
	FailureCode           string `json:"failureCode,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}
