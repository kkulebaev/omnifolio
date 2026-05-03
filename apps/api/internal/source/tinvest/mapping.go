package tinvest

import "strings"

// MOEX class codes seen in T-Invest. Used to classify shares/etfs as RU vs US.
// List grows when we encounter new ones in production.
var moexClassCodes = map[string]struct{}{
	"TQBR":      {}, // акции МосБиржи (основной режим)
	"TQTF":      {}, // ETFs МосБиржи
	"TQOB":      {}, // ОФЗ
	"TQOE":      {}, // евробонды
	"TQBD":      {}, // бонды дисконтные
	"TQIF":      {}, // паи ИФ
	"FQBR":      {}, // акции иностранные T+
	"MXBD":      {}, // бонды
	"EQRP_INFO": {}, // info-сегмент
	"TQTD":      {},
	"PSAU":      {},
	"PSRP":      {},
}

func isMoexClassCode(code string) bool {
	_, ok := moexClassCodes[strings.ToUpper(code)]
	return ok
}

// AssetClassFor maps T-Invest instrument metadata to our internal asset_class enum.
// Returns empty string for unsupported types (caller should skip with warning).
func AssetClassFor(instrumentType, classCode string) string {
	switch strings.ToLower(instrumentType) {
	case "share":
		if isMoexClassCode(classCode) {
			return "ru_stock"
		}
		return "us_stock"
	case "bond":
		return "ru_bond"
	case "etf":
		if isMoexClassCode(classCode) {
			return "ru_etf"
		}
		return "us_etf"
	case "currency":
		return "cash"
	default:
		return ""
	}
}
