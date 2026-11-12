package services

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/cybirdsecurity/myfi-os/internal/models"
)

// scriptedLLM answers each Complete call with the next reply in order and
// records the prompts it was sent.
type scriptedLLM struct {
	replies []string
	prompts []string
}

func (s *scriptedLLM) Complete(_ context.Context, prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	if len(s.replies) == 0 {
		return "", fmt.Errorf("no scripted reply")
	}
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r, nil
}
func (s *scriptedLLM) ProviderName() string { return "test" }

func testClassifier(llm LLMProvider) (*ClassifierService, map[string]uuid.UUID) {
	ids := map[string]uuid.UUID{}
	var cats []models.BudgetCategory
	for _, n := range []string{"Groceries", "Utilities", "Dining & Restaurants", "Other"} {
		id := uuid.New()
		ids[n] = id
		cats = append(cats, models.BudgetCategory{ID: id, Name: n, IsSystem: true})
	}
	return &ClassifierService{llm: llm, categories: cats}, ids
}

func TestClassifyBatchWithLLM_KeyedAnswers(t *testing.T) {
	llm := &scriptedLLM{replies: []string{`Sure: {"1": "Utilities", "0": "Groceries", "2": "Dining & Restaurants"}`}}
	c, ids := testClassifier(llm)

	got, _ := c.classifyBatchWithLLM(context.Background(), []string{"KROGER", "SPIRE BILL PAY", "CHIPOTLE"})
	want := map[int]uuid.UUID{0: ids["Groceries"], 1: ids["Utilities"], 2: ids["Dining & Restaurants"]}
	for i, id := range want {
		if got[i] != id {
			t.Errorf("merchant %d: got %v, want %v", i, got[i], id)
		}
	}
}

func TestClassifyBatchWithLLM_ShortArrayIsRejected(t *testing.T) {
	// Three merchants but only two answers: can't tell which one was skipped,
	// so nothing should be assigned (they stay flagged for review).
	llm := &scriptedLLM{replies: []string{`["Groceries", "Utilities"]`}}
	c, _ := testClassifier(llm)

	if got, _ := c.classifyBatchWithLLM(context.Background(), []string{"A", "B", "C"}); len(got) != 0 {
		t.Errorf("expected no assignments from a misaligned list, got %v", got)
	}
}

func TestClassifyBatchWithLLM_ExactArrayIsAccepted(t *testing.T) {
	llm := &scriptedLLM{replies: []string{`["Groceries", "Other"]`}}
	c, ids := testClassifier(llm)

	got, _ := c.classifyBatchWithLLM(context.Background(), []string{"A", "B"})
	if got[0] != ids["Groceries"] || got[1] != ids["Other"] {
		t.Errorf("got %v", got)
	}
}

func TestClassifyBatchWithLLM_Chunks(t *testing.T) {
	merchants := make([]string, 45)
	for i := range merchants {
		merchants[i] = fmt.Sprintf("M%d", i)
	}
	// One reply per chunk of llmBatchSize; each labels its first merchant.
	llm := &scriptedLLM{replies: []string{`{"0": "Groceries"}`, `{"0": "Utilities"}`, `{"0": "Other"}`}}
	c, ids := testClassifier(llm)

	got, _ := c.classifyBatchWithLLM(context.Background(), merchants)
	if len(llm.prompts) != 3 {
		t.Fatalf("expected 3 requests for 45 merchants, got %d", len(llm.prompts))
	}
	if strings.Contains(llm.prompts[0], fmt.Sprintf("%q", merchants[llmBatchSize])) {
		t.Error("first request should not include merchants from the second chunk")
	}
	// Indexes are mapped back to the full list, not the chunk.
	if got[0] != ids["Groceries"] || got[llmBatchSize] != ids["Utilities"] || got[2*llmBatchSize] != ids["Other"] {
		t.Errorf("chunk results mapped to wrong merchants: %v", got)
	}
}

func TestSuggestCategories_LLMErrorIsReported(t *testing.T) {
	// No scripted replies: Complete fails, as if Ollama were down.
	c, _ := testClassifier(&scriptedLLM{})
	if _, err := c.SuggestCategories(context.Background(), []string{"A"}); err == nil {
		t.Error("expected an error when the LLM is unreachable, so the caller retries later")
	}
}

func TestSuggestCategories_UnusableAnswerIsNotAnError(t *testing.T) {
	c, _ := testClassifier(&scriptedLLM{replies: []string{"I'm not sure."}})
	got, err := c.SuggestCategories(context.Background(), []string{"A"})
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want no suggestions and no error", got, err)
	}
}
