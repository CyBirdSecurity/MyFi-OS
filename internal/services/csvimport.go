package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// csvColumns holds the detected column indices for a credit-card CSV file.
type csvColumns struct {
	date     int
	desc     int
	amount   int
	category int // -1 if not present
}

// csvBankColumns holds the detected column indices for a bank-statement CSV
// (separate Debit and Credit columns, optional Status column).
type csvBankColumns struct {
	date   int
	desc   int
	debit  int
	credit int
	status int // -1 if absent
}

// ParsedTransaction is a transaction parsed from CSV before saving.
type ParsedTransaction struct {
	TransDate      time.Time
	Merchant       string
	RawDescription string
	Amount         float64
	SourceCategory string
	CategoryID     *uuid.UUID
	CategoryName   string
	NeedsReview    bool
}

// CSVImportService parses credit card CSV exports into classified transactions.
type CSVImportService struct {
	classifier *ClassifierService
	llm        LLMProvider
}

func NewCSVImportService(classifier *ClassifierService, llm LLMProvider) *CSVImportService {
	return &CSVImportService{
		classifier: classifier,
		llm:        llm,
	}
}

// Parse reads CSV bytes, detects columns, filters non-expense rows, and classifies each transaction
// using vendor rules and the user's history. It skips the LLM so a request never waits on it;
// unresolved rows are marked needsReview and the background categorizer suggests categories later.
func (s *CSVImportService) Parse(ctx context.Context, userID uuid.UUID, data []byte, filename string) ([]ParsedTransaction, error) {
	return s.parseInternal(ctx, userID, data, false)
}

// ParseQuick is like Parse but skips the LLM step. Transactions that cannot be resolved
// by vendor rules or history are left uncategorized and marked needsReview=true.
func (s *CSVImportService) ParseQuick(ctx context.Context, userID uuid.UUID, data []byte, filename string) ([]ParsedTransaction, error) {
	return s.parseInternal(ctx, userID, data, false)
}

func (s *CSVImportService) parseInternal(ctx context.Context, userID uuid.UUID, data []byte, useLLM bool) ([]ParsedTransaction, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("csv has no data rows")
	}

	headers := records[0]

	// Check for bank-statement format (separate Debit and Credit columns) before
	// falling through to the credit-card single-amount column detection.
	if bankCols, ok := detectBankColumns(headers); ok {
		return s.parseBankRows(ctx, userID, records[1:], bankCols, useLLM)
	}

	cols, err := s.detectColumns(ctx, headers)
	if err != nil {
		return nil, fmt.Errorf("detect columns: %w", err)
	}

	type rawRow struct {
		transDate      time.Time
		merchant       string
		rawDescription string
		amount         float64
		sourceCat      string
	}

	// A single amount column is signed one of two ways: credit-card exports
	// show charges as positive, checking-account exports show spending as
	// negative and deposits as positive. Spending is nearly always most of a
	// statement's rows, so the more common sign is taken as spending. On a tie
	// the credit-card reading is kept.
	var positives, negatives int
	for _, row := range records[1:] {
		if cols.amount >= len(row) {
			continue
		}
		if cols.category >= 0 && cols.category < len(row) && s.classifier.ShouldSkip(row[cols.category]) {
			continue
		}
		if amount, err := parseCSVAmount(row[cols.amount]); err == nil {
			switch {
			case amount > 0:
				positives++
			case amount < 0:
				negatives++
			}
		}
	}
	spendingIsNegative := negatives > positives

	var rows, incomeRows []rawRow
	for i, row := range records[1:] {
		if len(row) == 0 {
			continue
		}
		maxCol := max(cols.date, max(cols.desc, cols.amount))
		for len(row) <= maxCol {
			row = append(row, "")
		}

		sourceCat := ""
		if cols.category >= 0 && cols.category < len(row) {
			sourceCat = strings.TrimSpace(row[cols.category])
		}

		if s.classifier.ShouldSkip(sourceCat) {
			continue
		}

		amount, err := parseCSVAmount(row[cols.amount])
		if err != nil {
			log.Printf("csvimport: row %d: skip bad amount %q", i+2, row[cols.amount])
			continue
		}
		// Spending is stored as a positive amount. In a negative-spending file
		// the positive rows are deposits, imported as income; in a
		// positive-charge file the negative rows are payments and refunds,
		// which are not spending.
		isIncome := false
		if spendingIsNegative {
			amount = -amount
			if amount < 0 {
				isIncome = true
				amount = -amount
			}
		}
		if amount <= 0 {
			continue
		}
		amount = math.Round(amount*100) / 100

		rawDesc := strings.TrimSpace(row[cols.desc])
		if rawDesc == "" {
			continue
		}
		merchant := normalizeMerchant(rawDesc)

		dateStr := strings.TrimSpace(row[cols.date])
		transDate, err := parseDate(dateStr)
		if err != nil {
			log.Printf("csvimport: row %d: skip bad date %q", i+2, dateStr)
			continue
		}

		if isIncome {
			incomeRows = append(incomeRows, rawRow{transDate, merchant, rawDesc, amount, sourceCat})
			continue
		}
		rows = append(rows, rawRow{transDate, merchant, rawDesc, amount, sourceCat})
	}

	merchants := make([]string, len(rows))
	sourceCats := make([]string, len(rows))
	for i, r := range rows {
		merchants[i] = r.merchant
		sourceCats[i] = r.sourceCat
	}

	var catIDs []*uuid.UUID
	var needsReviews []bool
	if useLLM {
		catIDs, needsReviews = s.classifier.ClassifyBatch(ctx, userID, merchants, sourceCats)
	} else {
		catIDs, needsReviews = s.classifier.ClassifyBatchRulesOnly(ctx, userID, merchants, sourceCats)
	}

	out := make([]ParsedTransaction, len(rows), len(rows)+len(incomeRows))
	for i, r := range rows {
		catName := ""
		if catIDs[i] != nil {
			catName = s.classifier.categoryNameByID(*catIDs[i])
		}
		out[i] = ParsedTransaction{
			TransDate:      r.transDate,
			Merchant:       r.merchant,
			RawDescription: r.rawDescription,
			Amount:         r.amount,
			SourceCategory: r.sourceCat,
			CategoryID:     catIDs[i],
			CategoryName:   catName,
			NeedsReview:    needsReviews[i],
		}
	}

	// Deposits go to the Income category, as in bank files with a Credit column.
	incomeID := s.classifier.CategoryIDByName("Income")
	for _, r := range incomeRows {
		p := ParsedTransaction{
			TransDate:      r.transDate,
			Merchant:       r.merchant,
			RawDescription: r.rawDescription,
			Amount:         r.amount,
			SourceCategory: r.sourceCat,
			NeedsReview:    incomeID == uuid.Nil,
		}
		if incomeID != uuid.Nil {
			id := incomeID
			p.CategoryID = &id
			p.CategoryName = s.classifier.categoryNameByID(id)
		}
		out = append(out, p)
	}
	return out, nil
}

// parseCSVAmount parses a CSV amount such as "$1,234.56" or "-84.12".
func parseCSVAmount(raw string) (float64, error) {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	return strconv.ParseFloat(s, 64)
}

// ToTransactions converts parsed transactions to model transactions for a specific user.
// accountID, if set, is stamped on every transaction (the whole imported batch is treated
// as coming from one account, matching a real bank CSV export); accountLabel is that
// account's display name, resolved by the caller, stored denormalized for display.
func (s *CSVImportService) ToTransactions(parsed []ParsedTransaction, userID uuid.UUID, filename string, accountID *uuid.UUID, accountLabel string) []models.Transaction {
	out := make([]models.Transaction, len(parsed))
	fn := filename
	var label *string
	if accountLabel != "" {
		label = &accountLabel
	}
	for i, p := range parsed {
		out[i] = models.Transaction{
			UserID:         userID,
			TransDate:      p.TransDate,
			Merchant:       p.Merchant,
			RawDescription: p.RawDescription,
			Amount:         p.Amount,
			CategoryID:     p.CategoryID,
			CategoryName:   p.CategoryName,
			AccountID:      accountID,
			AccountLabel:   label,
			SourceFile:     &fn,
			NeedsReview:    p.NeedsReview,
		}
	}
	return out
}

// detectColumns identifies which CSV columns hold date, description, amount, and optional category.
// It tries static keyword matching first and falls back to the LLM.
func (s *CSVImportService) detectColumns(ctx context.Context, headers []string) (csvColumns, error) {
	cols := csvColumns{date: -1, desc: -1, amount: -1, category: -1}

	for i, h := range headers {
		lower := strings.ToLower(strings.TrimSpace(h))
		switch {
		case strings.Contains(lower, "trans") && strings.Contains(lower, "date"):
			cols.date = i // prefer "trans. date" over generic "date" or "post date"
		case strings.Contains(lower, "date") && cols.date == -1:
			cols.date = i
		case strings.Contains(lower, "desc") || lower == "merchant" || lower == "name" ||
			strings.Contains(lower, "payee") || strings.Contains(lower, "detail"):
			if cols.desc == -1 {
				cols.desc = i
			}
		case strings.Contains(lower, "amount") || lower == "debit" || lower == "charge":
			if cols.amount == -1 {
				cols.amount = i
			}
		case strings.Contains(lower, "categ") || strings.Contains(lower, "type"):
			cols.category = i
		}
	}

	if cols.date >= 0 && cols.desc >= 0 && cols.amount >= 0 {
		return cols, nil
	}

	// Fall back to LLM.
	return s.detectColumnsWithLLM(ctx, headers)
}

func (s *CSVImportService) detectColumnsWithLLM(ctx context.Context, headers []string) (csvColumns, error) {
	cols := csvColumns{date: -1, desc: -1, amount: -1, category: -1}
	if s.llm == nil {
		return cols, fmt.Errorf("could not auto-detect CSV columns and no LLM is configured")
	}

	prompt := fmt.Sprintf(
		`Given these CSV column headers from a credit card statement, identify the 0-based index of:
- date column (transaction date)
- description column (merchant / payee name)
- amount column (charge amount)
- category column (spending category, or -1 if absent)

Headers: %v

Reply with JSON only: {"date":N,"description":N,"amount":N,"category":N}`,
		headers,
	)

	content, err := s.llm.Complete(ctx, prompt)
	if err != nil {
		return cols, fmt.Errorf("llm unavailable: %w", err)
	}

	// Extract JSON from response (LLM may wrap it in prose).
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end < 0 {
		return cols, fmt.Errorf("no JSON in column-detection response")
	}
	var result struct {
		Date        int `json:"date"`
		Description int `json:"description"`
		Amount      int `json:"amount"`
		Category    int `json:"category"`
	}
	if err := json.Unmarshal([]byte(content[start:end+1]), &result); err != nil {
		return cols, fmt.Errorf("parse column-detection JSON: %w", err)
	}

	cols.date = result.Date
	cols.desc = result.Description
	cols.amount = result.Amount
	cols.category = result.Category
	return cols, nil
}

// normalizeMerchant strips common payment processor prefixes and trailing location info
// to produce a readable merchant name.
func normalizeMerchant(raw string) string {
	s := strings.TrimSpace(raw)

	prefixes := []string{
		"TST* ", "SQ *", "DD *", "PAYPAL *",
		"AMZN MKTP US*", "AMAZON MKTPL*", "AMAZON MKTPLACE PMTS",
		"AMAZON.COM*", "AMZN MKTP*", "AMZN DIGITAL*", "AMAZON PRIME*",
		"VSI*",
	}
	upper := strings.ToUpper(s)
	for _, p := range prefixes {
		if strings.HasPrefix(upper, p) {
			s = s[len(p):]
			upper = strings.ToUpper(s)
			break
		}
	}

	// Trim trailing location garbage (e.g. " SAINT LOUIS MO", " 800-xxx-xxxx XX").
	// Keep it simple: truncate at the first occurrence of a 2-char state code pattern
	// that follows a space + city name, but only if name is already meaningful.
	parts := strings.Fields(s)
	if len(parts) > 2 {
		// Look for a US state abbreviation near the end.
		for i := len(parts) - 1; i >= 2; i-- {
			p := strings.ToUpper(parts[i])
			if len(p) == 2 && p >= "AA" && p <= "WY" && !strings.ContainsAny(p, "0123456789") {
				s = strings.Join(parts[:i], " ")
				break
			}
		}
	}

	return strings.TrimSpace(s)
}

// detectBankColumns returns (cols, true) when headers contain separate Debit and Credit
// columns, indicating a bank or checking account statement rather than a credit card export.
func detectBankColumns(headers []string) (csvBankColumns, bool) {
	cols := csvBankColumns{date: -1, desc: -1, debit: -1, credit: -1, status: -1}
	for i, h := range headers {
		lower := strings.ToLower(strings.TrimSpace(h))
		switch {
		case lower == "debit":
			cols.debit = i
		case lower == "credit":
			cols.credit = i
		case strings.Contains(lower, "date") && cols.date == -1:
			cols.date = i
		case strings.Contains(lower, "desc") || lower == "description" || lower == "name":
			if cols.desc == -1 {
				cols.desc = i
			}
		case lower == "status":
			cols.status = i
		}
	}
	return cols, cols.debit >= 0 && cols.credit >= 0 && cols.date >= 0 && cols.desc >= 0
}

// parseBankRows processes rows from a bank/checking account CSV.
// Credits are classified as Income; debits run through the normal classifier
// (Transfer vendor rules catch credit-card payments automatically).
// When useLLM is false, the LLM step is skipped and unresolved debits are marked needsReview.
func (s *CSVImportService) parseBankRows(ctx context.Context, userID uuid.UUID, records [][]string, cols csvBankColumns, useLLM bool) ([]ParsedTransaction, error) {
	incomeID := s.classifier.CategoryIDByName("Income")

	type rawRow struct {
		transDate      time.Time
		merchant       string
		rawDescription string
		amount         float64
		isIncome       bool
	}

	var incomeRows, debitRows []rawRow

	for i, row := range records {
		if len(row) == 0 {
			continue
		}
		// Ensure row is wide enough.
		maxCol := max(cols.date, max(cols.desc, max(cols.debit, cols.credit)))
		for len(row) <= maxCol {
			row = append(row, "")
		}

		// Skip pending transactions — they may not post.
		if cols.status >= 0 && cols.status < len(row) {
			if strings.EqualFold(strings.TrimSpace(row[cols.status]), "pending") {
				continue
			}
		}

		rawDesc := strings.TrimSpace(row[cols.desc])
		if rawDesc == "" {
			continue
		}
		dateStr := strings.TrimSpace(row[cols.date])
		transDate, err := parseDate(dateStr)
		if err != nil {
			log.Printf("csvimport bank: row %d: skip bad date %q", i+2, dateStr)
			continue
		}

		debitStr := strings.TrimSpace(row[cols.debit])
		creditStr := strings.TrimSpace(row[cols.credit])
		debitStr = strings.ReplaceAll(debitStr, "$", "")
		debitStr = strings.ReplaceAll(debitStr, ",", "")
		creditStr = strings.ReplaceAll(creditStr, "$", "")
		creditStr = strings.ReplaceAll(creditStr, ",", "")

		merchant := normalizeBankMerchant(rawDesc)

		if creditStr != "" {
			amount, err := strconv.ParseFloat(creditStr, 64)
			if err != nil || amount <= 0 {
				continue
			}
			amount = math.Round(amount*100) / 100
			incomeRows = append(incomeRows, rawRow{transDate, merchant, rawDesc, amount, true})
		} else if debitStr != "" {
			amount, err := strconv.ParseFloat(debitStr, 64)
			if err != nil || amount <= 0 {
				continue
			}
			amount = math.Round(amount*100) / 100
			debitRows = append(debitRows, rawRow{transDate, merchant, rawDesc, amount, false})
		}
	}

	// Classify debit rows through the normal pipeline (vendor rules catch CC payments).
	debitMerchants := make([]string, len(debitRows))
	emptyCats := make([]string, len(debitRows))
	for i, r := range debitRows {
		debitMerchants[i] = r.merchant
	}
	var debitCatIDs []*uuid.UUID
	var debitNeedsReview []bool
	if useLLM {
		debitCatIDs, debitNeedsReview = s.classifier.ClassifyBatch(ctx, userID, debitMerchants, emptyCats)
	} else {
		debitCatIDs, debitNeedsReview = s.classifier.ClassifyBatchRulesOnly(ctx, userID, debitMerchants, emptyCats)
	}

	out := make([]ParsedTransaction, 0, len(incomeRows)+len(debitRows))

	// Income rows default to the Income category; no LLM call needed.
	for _, r := range incomeRows {
		var catID *uuid.UUID
		needsReview := false
		if incomeID != uuid.Nil {
			id := incomeID
			catID = &id
		} else {
			needsReview = true
		}
		catName := ""
		if catID != nil {
			catName = s.classifier.categoryNameByID(*catID)
		}
		out = append(out, ParsedTransaction{
			TransDate:      r.transDate,
			Merchant:       r.merchant,
			RawDescription: r.rawDescription,
			Amount:         r.amount,
			CategoryID:     catID,
			CategoryName:   catName,
			NeedsReview:    needsReview,
		})
	}

	// Debit rows use classifier results.
	for i, r := range debitRows {
		catName := ""
		if debitCatIDs[i] != nil {
			catName = s.classifier.categoryNameByID(*debitCatIDs[i])
		}
		out = append(out, ParsedTransaction{
			TransDate:      r.transDate,
			Merchant:       r.merchant,
			RawDescription: r.rawDescription,
			Amount:         r.amount,
			CategoryID:     debitCatIDs[i],
			CategoryName:   catName,
			NeedsReview:    debitNeedsReview[i],
		})
	}

	return out, nil
}

// normalizeBankMerchant cleans up a bank-statement description into a readable merchant name.
// Bank statements use multiple consecutive spaces as field separators and append ACH/Zelle
// suffixes; this strips those artifacts.
func normalizeBankMerchant(raw string) string {
	s := strings.TrimSpace(raw)
	lower := strings.ToLower(s)

	// Strip trailing ACH and Zelle type suffixes.
	for _, suf := range []string{" ach credit", " ach debit", " zelle credit", " zelle debit"} {
		if strings.HasSuffix(lower, suf) {
			s = s[:len(s)-len(suf)]
			lower = lower[:len(lower)-len(suf)]
			break
		}
	}
	s = strings.TrimSpace(s)

	// Replace runs of 2+ spaces with a single space.
	// Bank descriptions use extra spaces as column separators (e.g. "ATT              PAYMENT").
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}

	return strings.TrimSpace(s)
}

func parseDate(s string) (time.Time, error) {
	formats := []string{
		"01/02/2006",
		"1/2/2006",
		"2006-01-02",
		"01-02-2006",
		"01/02/06",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised date format: %q", s)
}

// categoryNameByID is a helper used when building ParsedTransaction.CategoryName.
func (s *ClassifierService) categoryNameByID(id uuid.UUID) string {
	for _, c := range s.categories {
		if c.ID == id {
			return c.Name
		}
	}
	return ""
}
