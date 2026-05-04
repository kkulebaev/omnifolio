package binance

// Binance Spot API response shapes (only fields we use).

// --- /api/v3/account ---

type accountResponse struct {
	Balances []balanceItem `json:"balances"`
}

type balanceItem struct {
	Asset  string `json:"asset"`
	Free   string `json:"free"`
	Locked string `json:"locked"`
}

// --- error envelope (returned alongside non-2xx and sometimes 200) ---

type errorEnvelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}
