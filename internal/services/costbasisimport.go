package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// CostBasisImportService parses portfolio CSV exports to extract per-holding cost basis.
// Column identification is done via LLM, making it broker-agnostic.
type CostBasisImportService struct {
	llm      LLMProvider
	holdings *db.HoldingStore
}

func NewCostBasisImportService(llm LLMProvider, holdings *db.HoldingStore) *CostBasisImportService {
	return &CostBasisImportService{llm: llm, holdings: holdings}
}

type CostBasisColumnMap struct {
	Ticker    string `json:"ticker"`
	CostBasis string `json:"costBasis"`
	Shares    string `json:"shares"`  // empty if not detected
	Account   string `json:"account"` // empty if not detected
}

type MatchedHolding struct {
	ID           string   `json:"id"`
	AccountName  string   `json:"accountName"`
	Shares       float64  `json:"shares"`
	CurrentBasis *float64 `json:"currentBasis"`
}

type CostBasisPreviewRow struct {
	Ticker              string           `json:"ticker"`
	CostBasis           *float64         `json:"costBasis"`
	SharesInCsv         *float64         `json:"sharesInCsv"`
	Account             string           `json:"account"`
	AutoSelectedMatchID string           `json:"autoSelectedMatchId"`
	Matches             []MatchedHolding `json:"matches"`
}

type CostBasisPreviewResult struct {
	Columns CostBasisColumnMap    `json:"columns"`
	Rows    []CostBasisPreviewRow `json:"rows"`
}

// Preview parses a broker portfolio CSV and returns detected column mapping plus
// matched holdings for each row, ready for the user to review before applying.
func (s *CostBasisImportService) Preview(ctx context.Context, userID uuid.UUID, csvData []byte) (*CostBasisPreviewResult, error) {
	header, rows, err := parseCSV(csvData)
	if err != nil {
		return nil, err
	}
	if len(header) == 0 {
		return nil, fmt.Errorf("could not find a header row in the CSV")
	}

	cols, err := s.detectColumns(ctx, header, rows)
	if err != nil {
		return nil, fmt.Errorf("column detection failed: %w", err)
	}

	userHoldings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load holdings: %w", err)
	}

	// Build account name map from holdings for display — account IDs are on each holding
	// but names aren't stored here; we pass them in separately from the handler.
	previewRows := buildPreviewRows(cols, header, rows, userHoldings)
	return &CostBasisPreviewResult{Columns: *cols, Rows: previewRows}, nil
}

// parseCSV finds the header row (skipping broker preamble) and returns header + data rows.
func parseCSV(data []byte) (header []string, rows [][]string, err error) {
	scanner := csv.NewReader(bytes.NewReader(data))
	scanner.LazyQuotes = true
	scanner.TrimLeadingSpace = true
	scanner.FieldsPerRecord = -1 // variable columns

	var allRows [][]string
	for {
		row, e := scanner.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			continue // skip malformed lines (common in broker preambles)
		}
		allRows = append(allRows, row)
	}

	// Find the header: first row containing ≥3 non-empty columns where at least one
	// looks like a financial header keyword.
	keywords := []string{"symbol", "ticker", "fund", "security", "cost", "basis", "shares", "account"}
	for i, row := range allRows {
		if len(row) < 3 {
			continue
		}
		rowLower := strings.ToLower(strings.Join(row, " "))
		for _, kw := range keywords {
			if strings.Contains(rowLower, kw) {
				return row, allRows[i+1:], nil
			}
		}
	}
	// Fallback: use the first row with ≥3 columns
	for i, row := range allRows {
		if len(row) >= 3 {
			return row, allRows[i+1:], nil
		}
	}
	return nil, nil, nil
}

// detectColumns asks the LLM to identify which columns hold the ticker, cost basis, and account.
func (s *CostBasisImportService) detectColumns(ctx context.Context, header []string, sampleRows [][]string) (*CostBasisColumnMap, error) {
	if s.llm == nil {
		return s.heuristicColumns(header), nil
	}

	sample := strings.Join(header, ", ")
	for i, row := range sampleRows {
		if i >= 3 {
			break
		}
		sample += "\n" + strings.Join(row, ", ")
	}

	prompt := fmt.Sprintf(`You are analyzing a portfolio CSV export to identify columns for importing cost basis data.

CSV header and sample rows:
%s

Identify which column header contains:
1. The stock ticker or fund symbol (e.g. VTSAX, AAPL, BND)
2. The average cost basis per share (a dollar amount per share)
3. Optionally, the number of shares/units held
4. Optionally, the account name or account number

Reply with JSON only, no explanation:
{"ticker": "<exact column header>", "costBasis": "<exact column header>", "shares": "<exact column header or empty string>", "account": "<exact column header or empty string>"}`, sample)

	content, err := s.llm.Complete(ctx, prompt)
	if err != nil {
		return s.heuristicColumns(header), nil
	}

	// Strip markdown code fences if present.
	content = strings.TrimSpace(content)
	if i := strings.Index(content, "{"); i >= 0 {
		content = content[i:]
	}
	if i := strings.LastIndex(content, "}"); i >= 0 {
		content = content[:i+1]
	}

	var cols CostBasisColumnMap
	if err := json.Unmarshal([]byte(content), &cols); err != nil {
		return s.heuristicColumns(header), nil
	}

	if !containsHeader(header, cols.Ticker) || !containsHeader(header, cols.CostBasis) {
		return s.heuristicColumns(header), nil
	}
	return &cols, nil
}

// heuristicColumns is the LLM-free fallback using keyword matching.
// Covers Vanguard, Fidelity, and Schwab common export formats.
func (s *CostBasisImportService) heuristicColumns(header []string) *CostBasisColumnMap {
	cols := &CostBasisColumnMap{}
	for _, h := range header {
		lower := strings.ToLower(strings.TrimSpace(h))
		switch {
		case lower == "symbol" || lower == "ticker" || lower == "fund symbol" ||
			strings.HasPrefix(lower, "symbol/"):
			cols.Ticker = h
		case strings.Contains(lower, "avg cost") || strings.Contains(lower, "average cost") ||
			strings.Contains(lower, "cost basis") || lower == "cost/share" ||
			strings.Contains(lower, "cost per share"):
			cols.CostBasis = h
		case lower == "shares" || lower == "quantity" || lower == "units" ||
			strings.Contains(lower, "share qty") || strings.Contains(lower, "number of shares"):
			cols.Shares = h
		case strings.Contains(lower, "account name") || lower == "account":
			cols.Account = h
		}
	}
	return cols
}

func containsHeader(header []string, name string) bool {
	for _, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

func colIndex(header []string, name string) int {
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), strings.TrimSpace(name)) {
			return i
		}
	}
	return -1
}

// lotAccum accumulates per-lot data for a ticker so we can compute a weighted average.
type lotAccum struct {
	totalShares       float64
	totalCost         float64 // sum of shares_i * cost_per_share_i
	hasSharesAndBasis bool
	account           string // first non-empty account seen
}

func buildPreviewRows(cols *CostBasisColumnMap, header []string, rows [][]string, userHoldings []models.Holding) []CostBasisPreviewRow {
	tickerIdx := colIndex(header, cols.Ticker)
	basisIdx := colIndex(header, cols.CostBasis)
	sharesIdx := colIndex(header, cols.Shares)
	accountIdx := colIndex(header, cols.Account)

	byTicker := map[string][]models.Holding{}
	for _, h := range userHoldings {
		byTicker[strings.ToUpper(h.Ticker)] = append(byTicker[strings.ToUpper(h.Ticker)], h)
	}

	// First pass: aggregate all lots per ticker.
	tickerOrder := make([]string, 0)
	accum := map[string]*lotAccum{}

	for _, row := range rows {
		if tickerIdx < 0 || tickerIdx >= len(row) {
			continue
		}
		ticker := strings.TrimSpace(strings.ToUpper(row[tickerIdx]))
		if ticker == "" {
			continue
		}

		if _, seen := accum[ticker]; !seen {
			tickerOrder = append(tickerOrder, ticker)
			accum[ticker] = &lotAccum{}
		}
		a := accum[ticker]

		var lotShares, lotBasis float64
		hasShares, hasBasis := false, false

		if sharesIdx >= 0 && sharesIdx < len(row) {
			raw := strings.TrimSpace(strings.ReplaceAll(row[sharesIdx], ",", ""))
			if v, err := strconv.ParseFloat(raw, 64); err == nil && !math.IsNaN(v) && v > 0 {
				lotShares = v
				hasShares = true
			}
		}
		if basisIdx >= 0 && basisIdx < len(row) {
			raw := strings.TrimSpace(row[basisIdx])
			raw = strings.ReplaceAll(raw, "$", "")
			raw = strings.ReplaceAll(raw, ",", "")
			if v, err := strconv.ParseFloat(raw, 64); err == nil && !math.IsNaN(v) && v >= 0 {
				lotBasis = v
				hasBasis = true
			}
		}

		if hasShares && hasBasis {
			a.totalShares += lotShares
			a.totalCost += lotShares * lotBasis
			a.hasSharesAndBasis = true
		}

		if a.account == "" && accountIdx >= 0 && accountIdx < len(row) {
			a.account = strings.Trim(strings.TrimSpace(row[accountIdx]), `"`)
		}
	}

	// Second pass: build output rows with weighted average cost basis.
	out := make([]CostBasisPreviewRow, 0, len(tickerOrder))
	for _, ticker := range tickerOrder {
		a := accum[ticker]

		var avgBasis *float64
		if a.hasSharesAndBasis && a.totalShares > 0 {
			v := a.totalCost / a.totalShares
			avgBasis = &v
		}

		var sharesInCsv *float64
		if a.totalShares > 0 {
			v := a.totalShares
			sharesInCsv = &v
		}

		matched := make([]MatchedHolding, 0, len(byTicker[ticker]))
		for _, h := range byTicker[ticker] {
			matched = append(matched, MatchedHolding{
				ID:           h.ID.String(),
				AccountName:  h.AccountID.String(), // caller enriches with account names
				Shares:       h.Shares,
				CurrentBasis: h.CostBasis,
			})
		}

		out = append(out, CostBasisPreviewRow{
			Ticker:      ticker,
			CostBasis:   avgBasis,
			SharesInCsv: sharesInCsv,
			Account:     a.account,
			Matches:     matched,
		})
	}
	return out
}

// accountWords normalizes an account name into a lowercase word set for fuzzy matching.
func accountWords(name string) map[string]bool {
	words := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(name)) {
		// Strip punctuation
		w = strings.Trim(w, `.,;:()[]"'`)
		if len(w) > 1 {
			words[w] = true
		}
	}
	return words
}

// bestAccountMatch returns the ID of the matched holding whose account name has
// the most word overlap with the CSV account string. Returns "" if no useful match.
func bestAccountMatch(csvAccount string, matches []MatchedHolding) string {
	if csvAccount == "" || len(matches) == 0 {
		return ""
	}
	csvWords := accountWords(csvAccount)
	if len(csvWords) == 0 {
		return ""
	}
	bestID := ""
	bestScore := 0
	for _, m := range matches {
		score := 0
		for w := range accountWords(m.AccountName) {
			if csvWords[w] {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestID = m.ID
		}
	}
	return bestID
}

// PreviewWithAccounts is the real entry point called by the handler, which passes account names.
func (s *CostBasisImportService) PreviewFull(
	ctx context.Context,
	userID uuid.UUID,
	csvData []byte,
	accountNames map[uuid.UUID]string,
) (*CostBasisPreviewResult, error) {
	header, rows, err := parseCSV(csvData)
	if err != nil {
		return nil, err
	}
	if len(header) == 0 {
		return nil, fmt.Errorf("could not find a header row in the CSV")
	}

	cols, err := s.detectColumns(ctx, header, rows)
	if err != nil {
		return nil, fmt.Errorf("column detection: %w", err)
	}

	userHoldings, err := s.holdings.ListByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load holdings: %w", err)
	}

	previewRows := buildPreviewRows(cols, header, rows, userHoldings)

	// Enrich match account names, then auto-select best account match.
	for i := range previewRows {
		for j := range previewRows[i].Matches {
			accID, err := uuid.Parse(previewRows[i].Matches[j].AccountName)
			if err == nil {
				if name, ok := accountNames[accID]; ok {
					previewRows[i].Matches[j].AccountName = name
				}
			}
		}
		previewRows[i].AutoSelectedMatchID = bestAccountMatch(previewRows[i].Account, previewRows[i].Matches)
	}

	return &CostBasisPreviewResult{Columns: *cols, Rows: previewRows}, nil
}
