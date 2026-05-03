package tinvest

// Tinkoff REST API request/response shapes. We define only the fields we use.

// --- UsersService.GetAccounts ---

type getAccountsRequest struct {
	Status string `json:"status,omitempty"` // ACCOUNT_STATUS_OPEN | _ALL
}

type getAccountsResponse struct {
	Accounts []apiAccount `json:"accounts"`
}

type apiAccount struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // ACCOUNT_TYPE_TINKOFF | _IIS | _INVEST_BOX | ...
	Name        string `json:"name"`
	Status      string `json:"status"`
	OpenedDate  string `json:"openedDate"`
	ClosedDate  string `json:"closedDate"`
	AccessLevel string `json:"accessLevel"`
}

// --- OperationsService.GetPortfolio ---

type getPortfolioRequest struct {
	AccountID string `json:"accountId"`
	Currency  string `json:"currency,omitempty"` // RUB | USD | EUR (default RUB)
}

type getPortfolioResponse struct {
	AccountID string         `json:"accountId"`
	Positions []apiPosition  `json:"positions"`
}

type apiPosition struct {
	FIGI                  string     `json:"figi"`
	InstrumentType        string     `json:"instrumentType"` // share | bond | etf | currency | ...
	Quantity              Quotation  `json:"quantity"`
	AveragePositionPrice  MoneyValue `json:"averagePositionPrice"`
	CurrentPrice          MoneyValue `json:"currentPrice"`
	InstrumentUID         string     `json:"instrumentUid"`
	PositionUID           string     `json:"positionUid"`
}

// --- MarketDataService.GetLastPrices ---

type getLastPricesRequest struct {
	FIGI []string `json:"figi"`
}

type getLastPricesResponse struct {
	LastPrices []apiLastPrice `json:"lastPrices"`
}

type apiLastPrice struct {
	FIGI          string    `json:"figi"`
	Price         Quotation `json:"price"`
	Time          string    `json:"time"`
	InstrumentUID string    `json:"instrumentUid"`
}

// --- InstrumentsService.GetInstrumentBy ---

type getInstrumentByRequest struct {
	IDType    string `json:"idType"` // INSTRUMENT_ID_TYPE_FIGI | _UID | _TICKER
	ID        string `json:"id"`
	ClassCode string `json:"classCode,omitempty"` // required only for ticker lookup
}

type getInstrumentByResponse struct {
	Instrument apiInstrument `json:"instrument"`
}

type apiInstrument struct {
	FIGI            string `json:"figi"`
	Ticker          string `json:"ticker"`
	ClassCode       string `json:"classCode"`
	ISIN            string `json:"isin"`
	Lot             int    `json:"lot"`
	Currency        string `json:"currency"`
	Name            string `json:"name"`
	Exchange        string `json:"exchange"`
	InstrumentType  string `json:"instrumentType"`
	UID             string `json:"uid"`
	BuyAvailableFlag  bool `json:"buyAvailableFlag"`
	SellAvailableFlag bool `json:"sellAvailableFlag"`
}

// Constants used in requests.
const (
	statusOpen          = "ACCOUNT_STATUS_OPEN"
	idTypeFIGI          = "INSTRUMENT_ID_TYPE_FIGI"
)

// Sub-account type display: maps Tinkoff enum to human label (used by handlers
// when sending preview response to UI; UI may further localize).
func subAccountType(t string) string {
	switch t {
	case "ACCOUNT_TYPE_TINKOFF":
		return "BROKER"
	case "ACCOUNT_TYPE_TINKOFF_IIS":
		return "IIS"
	case "ACCOUNT_TYPE_INVEST_BOX":
		return "PREMIUM"
	default:
		return t
	}
}
