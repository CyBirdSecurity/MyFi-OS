// Package institutions provides per-broker overrides for the shared OFX parser.
// Each institution implements InstitutionOverride to handle quirks in how that
// broker encodes tickers, asset classes, and security identifiers.
//
// The interface uses primitive types to avoid a circular import with the parent ofx package.
package institutions

// SecurityInfo is a minimal security descriptor passed to override methods.
// It mirrors the fields of ofx.Security without importing that package.
type SecurityInfo struct {
	Ticker string
	Name   string
}

// InstitutionOverride allows broker-specific normalization of parsed OFX data.
type InstitutionOverride interface {
	// Name returns a human-readable institution name (e.g. "Vanguard").
	Name() string

	// ResolveTicker returns the canonical ticker symbol for a position.
	// uniqueID is the UNIQUEID value; idType is UNIQUEIDTYPE ("CUSIP", "TICKER", etc.);
	// posType is the OFX position container ("POSMF", "POSSTOCK", etc.).
	// secList maps UNIQUEID → SecurityInfo from the file's SECLIST block.
	ResolveTicker(uniqueID, idType, posType string, secList map[string]SecurityInfo) string

	// InferAssetClass maps OFX position/security info to a MyFi asset_class value:
	// "equity", "etf", "mutual_fund", "bond", "cash_equivalent", "other".
	InferAssetClass(uniqueID, idType, posType string) string
}
