package plaid

import "testing"

func TestPFCCategoryName(t *testing.T) {
	cases := []struct {
		primary, detailed, want string
	}{
		{"FOOD_AND_DRINK", "FOOD_AND_DRINK_RESTAURANT", "Dining & Restaurants"},
		{"FOOD_AND_DRINK", "FOOD_AND_DRINK_GROCERIES", "Groceries"}, // detailed override
		{"PERSONAL_CARE", "PERSONAL_CARE_GYMS_AND_FITNESS_CENTERS", "Fitness & Wellness"},
		{"RENT_AND_UTILITIES", "RENT_AND_UTILITIES_RENT", "Housing"},
		{"RENT_AND_UTILITIES", "RENT_AND_UTILITIES_GAS_AND_ELECTRICITY", "Utilities"},
		{"TRANSPORTATION", "TRANSPORTATION_GAS", "Transportation"},
		{"GENERAL_SERVICES", "GENERAL_SERVICES_EDUCATION", "Education"}, // primary unmapped, detailed wins
		{"GENERAL_SERVICES", "GENERAL_SERVICES_CONSULTING", ""},         // both unmapped → no hint
		{"LOAN_PAYMENTS", "LOAN_PAYMENTS_PERSONAL_LOAN_PAYMENT", ""},    // ambiguous primary, unmapped
		{"", "", ""},
	}
	for _, c := range cases {
		if got := PFCCategoryName(c.primary, c.detailed); got != c.want {
			t.Errorf("PFCCategoryName(%q, %q) = %q, want %q", c.primary, c.detailed, got, c.want)
		}
	}
}

func TestClassifyPFC(t *testing.T) {
	cases := []struct {
		name              string
		primary, detailed string
		amount            float64
		wantClass         TxnClass
		wantHint          string
	}{
		{"restaurant spend", "FOOD_AND_DRINK", "FOOD_AND_DRINK_RESTAURANT", 42.50, TxnSpend, "Dining & Restaurants"},
		{"grocery spend", "FOOD_AND_DRINK", "FOOD_AND_DRINK_GROCERIES", 88.10, TxnSpend, "Groceries"},
		{"unmapped spend", "GENERAL_SERVICES", "GENERAL_SERVICES_CONSULTING", 100, TxnSpend, ""},
		{"paycheck", "INCOME", "INCOME_WAGES", -2500, TxnIncome, ""},
		{"transfer in", "TRANSFER_IN", "TRANSFER_IN_DEPOSIT", -500, TxnTransfer, ""},
		{"transfer out", "TRANSFER_OUT", "TRANSFER_OUT_WITHDRAWAL", 500, TxnTransfer, ""},
		{"credit card payment is a transfer", "LOAN_PAYMENTS", "LOAN_PAYMENTS_CREDIT_CARD_PAYMENT", 300, TxnTransfer, ""},
		{"refund inflow becomes income", "GENERAL_MERCHANDISE", "GENERAL_MERCHANDISE_ONLINE_MARKETPLACES", -30, TxnIncome, ""},
		{"car loan payment is spend/Auto", "LOAN_PAYMENTS", "LOAN_PAYMENTS_CAR_PAYMENT", 450, TxnSpend, "Auto"},
	}
	for _, c := range cases {
		gotClass, gotHint := ClassifyPFC(c.primary, c.detailed, c.amount)
		if gotClass != c.wantClass || gotHint != c.wantHint {
			t.Errorf("%s: ClassifyPFC(%q,%q,%.2f) = (%d,%q), want (%d,%q)",
				c.name, c.primary, c.detailed, c.amount, gotClass, gotHint, c.wantClass, c.wantHint)
		}
	}
}
