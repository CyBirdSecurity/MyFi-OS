package institutions

import "strings"

// Detect returns the InstitutionOverride for a given OFX BrokerID.
// Falls back to the Generic override when no specific institution matches.
func Detect(brokerID string) InstitutionOverride {
	lower := strings.ToLower(brokerID)
	switch {
	case strings.Contains(lower, "vanguard"):
		return Vanguard{}
	default:
		return Generic{}
	}
}

// Generic is a best-effort override used when the broker is unknown.
type Generic struct{}

func (Generic) Name() string { return "Unknown Institution" }

func (Generic) ResolveTicker(uniqueID, idType, _ string, secList map[string]SecurityInfo) string {
	if sec, ok := secList[uniqueID]; ok && sec.Ticker != "" {
		return strings.ToUpper(sec.Ticker)
	}
	if idType == "TICKER" {
		return strings.ToUpper(uniqueID)
	}
	return strings.ToUpper(uniqueID)
}

func (Generic) InferAssetClass(_, _, posType string) string {
	switch posType {
	case "POSDEBT":
		return "bond"
	default:
		return "equity"
	}
}
