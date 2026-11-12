package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// discoverCategoryMap maps Discover's own CSV category labels to our category names.
// Using Discover's pre-classified data is the highest-confidence signal available.
var discoverCategoryMap = map[string]string{
	"restaurants":           "Dining & Restaurants",
	"supermarkets":          "Groceries",
	"gasoline":              "Transportation",
	"automotive":            "Auto",
	"home improvement":      "Shopping",
	"merchandise":           "Shopping",
	"warehouse clubs":       "Shopping",
	"travel/ entertainment": "Entertainment",
	"travel/entertainment":  "Entertainment",
	"education":             "Education",
	"government services":   "Fees & Banking",
}

// skipCategories are CSV category/type values we drop entirely (not real
// expenses or income): Discover's credit categories, and the "Payment" type
// card exports use for paying the card off, which is a transfer.
var skipCategories = map[string]bool{
	"payments and credits":      true,
	"awards and rebate credits": true,
	"payment":                   true,
}

type ClassifierService struct {
	llm          LLMProvider
	rules        []models.VendorRule // sorted by keyword length DESC for greedy matching
	categories   []models.BudgetCategory
	transactions *db.TransactionStore
}

func NewClassifierService(
	llm LLMProvider,
	catStore *db.BudgetCategoryStore,
	txStore *db.TransactionStore,
) (*ClassifierService, error) {
	ctx := context.Background()

	rules, err := catStore.LoadVendorRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("load vendor rules: %w", err)
	}
	// Sort longer keywords first so more-specific rules win.
	sort.Slice(rules, func(i, j int) bool {
		return len(rules[i].Keyword) > len(rules[j].Keyword)
	})

	cats, err := catStore.List(ctx, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}

	return &ClassifierService{
		llm:          llm,
		rules:        rules,
		categories:   cats,
		transactions: txStore,
	}, nil
}

// ShouldSkip returns true for rows that should be filtered out entirely
// (payments, credits, cashback rewards).
func (c *ClassifierService) ShouldSkip(sourceCategory string) bool {
	return skipCategories[strings.ToLower(strings.TrimSpace(sourceCategory))]
}

// Classify returns a category ID and needsReview flag for a given merchant.
// For bulk imports, prefer ClassifyBatch which makes at most one LLM call.
func (c *ClassifierService) Classify(ctx context.Context, userID uuid.UUID, merchant, sourceCategory string) (categoryID *uuid.UUID, needsReview bool) {
	if id, ok := c.classifyFast(ctx, userID, merchant, sourceCategory); ok {
		return id, false
	}
	if id := c.classifyWithLLM(ctx, merchant); id != uuid.Nil {
		return &id, true
	}
	if id := c.categoryIDByName("Other"); id != uuid.Nil {
		return &id, true
	}
	return nil, true
}

// classifyFast applies the source-category hint, vendor rules, and history
// without calling the LLM. Returns (id, true) when resolved.
func (c *ClassifierService) classifyFast(ctx context.Context, userID uuid.UUID, merchant, sourceCategory string) (*uuid.UUID, bool) {
	if sourceCategory != "" {
		if name, ok := discoverCategoryMap[strings.ToLower(strings.TrimSpace(sourceCategory))]; ok {
			if id := c.categoryIDByName(name); id != uuid.Nil {
				return &id, true
			}
		}
	}
	normalized := strings.ToUpper(merchant)
	for _, r := range c.rules {
		if strings.Contains(normalized, strings.ToUpper(r.Keyword)) {
			id := r.CategoryID
			return &id, true
		}
	}
	if userID != uuid.Nil && c.transactions != nil {
		if id := c.transactions.FindCategoryByMerchant(ctx, userID, merchant); id != nil {
			return id, true
		}
	}
	return nil, false
}

// ClassifyBatch classifies a slice of merchants, making at most one LLM call
// for those that cannot be resolved by rules or history.
func (c *ClassifierService) ClassifyBatch(ctx context.Context, userID uuid.UUID, merchants, sourceCategories []string) ([]*uuid.UUID, []bool) {
	n := len(merchants)
	catIDs := make([]*uuid.UUID, n)
	needsReview := make([]bool, n)

	var llmIdx []int
	for i, merchant := range merchants {
		if id, ok := c.classifyFast(ctx, userID, merchant, sourceCategories[i]); ok {
			catIDs[i] = id
		} else {
			llmIdx = append(llmIdx, i)
			needsReview[i] = true
		}
	}

	if len(llmIdx) == 0 {
		return catIDs, needsReview
	}

	// Deduplicate merchants before the LLM call to keep the prompt compact.
	uniqueMerchants := make([]string, 0, len(llmIdx))
	merchantToUnique := make(map[string]int, len(llmIdx)) // merchant → index in uniqueMerchants
	for _, i := range llmIdx {
		m := merchants[i]
		if _, seen := merchantToUnique[m]; !seen {
			merchantToUnique[m] = len(uniqueMerchants)
			uniqueMerchants = append(uniqueMerchants, m)
		}
	}

	batchResults, _ := c.classifyBatchWithLLM(ctx, uniqueMerchants) // errors are logged; unmatched fall back to "Other"
	for _, i := range llmIdx {
		uid := merchantToUnique[merchants[i]]
		if id, ok := batchResults[uid]; ok {
			catIDs[i] = &id
		} else if id := c.categoryIDByName("Other"); id != uuid.Nil {
			catIDs[i] = &id
		}
	}

	return catIDs, needsReview
}

// ClassifyBatchRulesOnly classifies merchants using only vendor rules and transaction history —
// no LLM call. Items with no rule match are marked needsReview=true with no category.
// Used by the quick-import path so the user gets an instant response.
func (c *ClassifierService) ClassifyBatchRulesOnly(ctx context.Context, userID uuid.UUID, merchants, sourceCategories []string) ([]*uuid.UUID, []bool) {
	n := len(merchants)
	catIDs := make([]*uuid.UUID, n)
	needsReview := make([]bool, n)
	for i, merchant := range merchants {
		if id, ok := c.classifyFast(ctx, userID, merchant, sourceCategories[i]); ok {
			catIDs[i] = id
		} else {
			needsReview[i] = true
		}
	}
	return catIDs, needsReview
}

// HasLLM reports whether an LLM is configured for category suggestions.
func (c *ClassifierService) HasLLM() bool { return c.llm != nil }

// SuggestCategories asks the LLM for a category per merchant, in batches.
// Merchants without a usable answer are omitted from the result. A non-nil
// error means the LLM itself couldn't be reached, so callers should retry
// later rather than treat the merchants as unclassifiable.
func (c *ClassifierService) SuggestCategories(ctx context.Context, merchants []string) (map[string]uuid.UUID, error) {
	byIndex, err := c.classifyBatchWithLLM(ctx, merchants)
	if err != nil {
		return nil, err
	}
	out := make(map[string]uuid.UUID, len(merchants))
	for i, id := range byIndex {
		out[merchants[i]] = id
	}
	return out, nil
}

// llmBatchSize caps merchants per LLM request so the reply fits the response
// token limit (and the HTTP timeout on CPU-only hosts).
const llmBatchSize = 20

// classifyBatchWithLLM returns category IDs by merchant index. It stops and
// returns an error if the LLM can't be reached; results from earlier batches
// are still returned alongside it.
func (c *ClassifierService) classifyBatchWithLLM(ctx context.Context, merchants []string) (map[int]uuid.UUID, error) {
	results := make(map[int]uuid.UUID)
	if c.llm == nil || len(merchants) == 0 {
		return results, nil
	}

	names := make([]string, 0, len(c.categories))
	for _, cat := range c.categories {
		if cat.IsSystem {
			names = append(names, cat.Name)
		}
	}

	for start := 0; start < len(merchants); start += llmBatchSize {
		end := min(start+llmBatchSize, len(merchants))
		chunk, err := c.classifyChunkWithLLM(ctx, names, merchants[start:end])
		if err != nil {
			return results, err
		}
		for i, name := range chunk {
			if id := c.categoryIDByName(strings.TrimSpace(name)); id != uuid.Nil {
				results[start+i] = id
			}
		}
	}
	return results, nil
}

// classifyChunkWithLLM asks for categories keyed by merchant number so a
// skipped or extra answer can't shift every later answer onto the wrong
// merchant. Returns category names by index within chunk; merchants missing
// from the result are left for the caller's fallback. The error is set only
// when the LLM request itself fails.
func (c *ClassifierService) classifyChunkWithLLM(ctx context.Context, names, chunk []string) (map[int]string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Classify each merchant into exactly one of these budget categories: %s\n\n", strings.Join(names, ", "))
	sb.WriteString("Reply with a JSON object mapping each merchant's number to its category name, e.g. {\"0\": \"Groceries\", \"1\": \"Utilities\"}. Use only the exact category names above. If unsure, use \"Other\".\n\nMerchants:\n")
	for i, m := range chunk {
		fmt.Fprintf(&sb, "%d: %q\n", i, m)
	}

	answer, err := c.llm.Complete(ctx, sb.String())
	if err != nil {
		log.Printf("classifier: batch llm error: %v", err)
		return nil, err
	}

	out := make(map[int]string, len(chunk))
	if start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}"); start >= 0 && end > start {
		var keyed map[string]string
		if err := json.Unmarshal([]byte(answer[start:end+1]), &keyed); err == nil {
			for k, v := range keyed {
				if i, err := strconv.Atoi(strings.TrimSpace(k)); err == nil && i >= 0 && i < len(chunk) {
					out[i] = v
				}
			}
			return out, nil
		}
	}

	// Some models answer with a plain array anyway; only trust it when it has
	// exactly one entry per merchant, since a short list can't be aligned.
	if start, end := strings.Index(answer, "["), strings.LastIndex(answer, "]"); start >= 0 && end > start {
		var list []string
		if err := json.Unmarshal([]byte(answer[start:end+1]), &list); err == nil && len(list) == len(chunk) {
			for i, v := range list {
				out[i] = v
			}
			return out, nil
		}
	}

	log.Printf("classifier: unusable batch response for %d merchants: %q", len(chunk), answer)
	return nil, nil
}

func (c *ClassifierService) classifyWithLLM(ctx context.Context, merchant string) uuid.UUID {
	if c.llm == nil {
		return uuid.Nil
	}

	names := make([]string, 0, len(c.categories))
	for _, cat := range c.categories {
		if cat.IsSystem {
			names = append(names, cat.Name)
		}
	}

	prompt := fmt.Sprintf(
		"Classify this merchant into exactly one of these categories: %s\n\nMerchant: %q\n\nReply with only the category name, nothing else.",
		strings.Join(names, ", "),
		merchant,
	)

	answer, err := c.llm.Complete(ctx, prompt)
	if err != nil {
		log.Printf("classifier: llm unavailable: %v", err)
		return uuid.Nil
	}

	return c.categoryIDByName(strings.TrimSpace(answer))
}

func (c *ClassifierService) categoryIDByName(name string) uuid.UUID {
	lower := strings.ToLower(name)
	for _, cat := range c.categories {
		if strings.ToLower(cat.Name) == lower {
			return cat.ID
		}
	}
	return uuid.Nil
}

// CategoryIDByName is an exported wrapper around categoryIDByName, used by callers
// that need to look up special system categories like "Income" and "Transfer".
func (c *ClassifierService) CategoryIDByName(name string) uuid.UUID {
	return c.categoryIDByName(name)
}
