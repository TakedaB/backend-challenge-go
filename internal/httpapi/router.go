package httpapi

import "net/http"

// NewRouter builds the HTTP route table.
func NewRouter(walletHandler *WalletHandler, wagerHandler *WagerTransactionHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /wallets", walletHandler.HandleOpenWallet)
	mux.HandleFunc("GET /wallets/{id}", walletHandler.HandleGetWallet)
	mux.HandleFunc("POST /wagering/transactions", wagerHandler.HandleProcessTransaction)
	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
