package bybit

// Bybit V5 API response shapes (only fields we use).

// --- /v5/account/wallet-balance ---

type walletBalanceResponse struct {
	envelope
	Result struct {
		List []walletAccount `json:"list"`
	} `json:"result"`
}

type walletAccount struct {
	AccountType string       `json:"accountType"`
	Coin        []walletCoin `json:"coin"`
}

type walletCoin struct {
	Coin          string `json:"coin"`
	Equity        string `json:"equity"`        // total amount of coin in the wallet
	WalletBalance string `json:"walletBalance"` // realized + unrealized
	UsdValue      string `json:"usdValue"`
}

