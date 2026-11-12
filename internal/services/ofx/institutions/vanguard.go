package institutions

import "strings"

// Vanguard implements InstitutionOverride for vanguard.com OFX exports.
// Vanguard encodes positions using CUSIPs as the UNIQUEID and provides a
// SECLIST block that maps CUSIP → ticker symbol + security name.
type Vanguard struct{}

func (Vanguard) Name() string { return "Vanguard" }

func (Vanguard) ResolveTicker(uniqueID, idType, _ string, secList map[string]SecurityInfo) string {
	// Prefer the SECLIST mapping (CUSIP → ticker) since Vanguard rarely
	// puts a TICKER directly on the INVPOS element.
	if sec, ok := secList[uniqueID]; ok && sec.Ticker != "" {
		return strings.ToUpper(sec.Ticker)
	}
	if idType == "TICKER" {
		return strings.ToUpper(uniqueID)
	}
	return ""
}

func (Vanguard) InferAssetClass(_, _, posType string) string {
	switch posType {
	case "POSDEBT":
		return "bond"
	case "POSOPT":
		return "other"
	default:
		// POSMF (mutual funds), POSSTOCK (ETFs + equities) all map to "equity"
		// per migration 000034 which consolidated etf/mutual_fund into equity.
		return "equity"
	}
}
