package httpapi

import "net/http"

// NewRouter builds the HTTP route table. /health stays open;
// everything else requires the Authorization bearer token via
// AuthMiddleware.
func NewRouter(cfg AuthConfig, walletHandler *WalletHandler, wagerHandler *WagerTransactionHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)

	protected := http.NewServeMux()
	protected.HandleFunc("POST /wallets", walletHandler.HandleOpenWallet)
	protected.HandleFunc("GET /wallets/{id}", walletHandler.HandleGetWallet)
	protected.HandleFunc("POST /wagering/transactions", wagerHandler.HandleProcessTransaction)
	mux.Handle("/", AuthMiddleware(cfg, protected))

	return mux
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}
