package finnhub

// quoteResponse mirrors GET /api/v1/quote.
//   c  — current price (0 means symbol unknown / delisted)
//   pc — previous close
//   t  — unix timestamp (seconds) of the quote
//   d, dp, h, l, o — change, change%, high, low, open (unused)
type quoteResponse struct {
	C  float64 `json:"c"`
	PC float64 `json:"pc"`
	T  int64   `json:"t"`
}
