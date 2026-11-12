-- Baseline schema for MyFi-OS.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE FUNCTION set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TABLE accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    account_type text NOT NULL,
    tax_treatment text DEFAULT 'taxable'::text NOT NULL,
    institution text,
    currency text DEFAULT 'USD'::text NOT NULL,
    is_manual boolean DEFAULT true NOT NULL,
    external_provider text,
    external_account_id text,
    external_item_id text,
    last_synced_at timestamp with time zone,
    deleted_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    return_rate_override numeric(6,4),
    member_id uuid,
    is_joint boolean DEFAULT false NOT NULL,
    CONSTRAINT accounts_account_type_check CHECK ((account_type = ANY (ARRAY['checking'::text, 'savings'::text, 'money_market'::text, 'brokerage'::text, 'traditional_ira'::text, 'roth_ira'::text, 'sep_ira'::text, 'simple_ira'::text, 'traditional_401k'::text, 'roth_401k'::text, 'traditional_403b'::text, 'roth_403b'::text, '457b'::text, 'hsa'::text, '529'::text, 'crypto'::text, 'other_asset'::text, 'credit_card'::text, 'loan'::text, 'other_liability'::text, 'automobile'::text, 'home'::text]))),
    CONSTRAINT accounts_tax_treatment_check CHECK ((tax_treatment = ANY (ARRAY['taxable'::text, 'tax_deferred'::text, 'tax_free'::text])))
);

CREATE TABLE allocation_targets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    equity_pct double precision DEFAULT 0 NOT NULL,
    bond_pct double precision DEFAULT 0 NOT NULL,
    crypto_pct double precision DEFAULT 0 NOT NULL,
    cash_pct double precision DEFAULT 0 NOT NULL,
    other_pct double precision DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE balance_snapshots (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    snapshot_date date NOT NULL,
    balance numeric(18,4) NOT NULL,
    notes text,
    source text DEFAULT 'manual'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    miles integer,
    CONSTRAINT balance_snapshots_source_check CHECK ((source = ANY (ARRAY['manual'::text, 'import'::text, 'plaid'::text, 'computed'::text])))
);

CREATE TABLE budget_categories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid,
    name text NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE budget_limits (
    user_id uuid NOT NULL,
    category_id uuid NOT NULL,
    monthly_limit numeric(14,2) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT budget_limits_monthly_limit_check CHECK ((monthly_limit > (0)::numeric))
);

CREATE TABLE contribution_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    entry_date date NOT NULL,
    amount numeric(15,2) NOT NULL,
    source text NOT NULL,
    description text,
    external_id text,
    source_edited boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT contribution_entries_amount_check CHECK ((amount > (0)::numeric)),
    CONSTRAINT contribution_entries_source_check CHECK ((source = ANY (ARRAY['employee'::text, 'employer'::text])))
);

CREATE TABLE contributions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    label text,
    amount numeric(15,2) NOT NULL,
    frequency text DEFAULT 'monthly'::text NOT NULL,
    annual_increase_pct numeric(6,4) DEFAULT 0 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    next_pay_date date,
    start_date date,
    end_date date,
    amount_type text DEFAULT 'fixed'::text NOT NULL,
    income_pct numeric(6,4) DEFAULT 0 NOT NULL,
    match_tiers jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT contributions_amount_check CHECK ((amount >= (0)::numeric)),
    CONSTRAINT contributions_amount_type_check CHECK ((amount_type = ANY (ARRAY['fixed'::text, 'percent'::text]))),
    CONSTRAINT contributions_frequency_check CHECK ((frequency = ANY (ARRAY['monthly'::text, 'annually'::text, 'biweekly'::text])))
);

CREATE TABLE credit_card_details (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    apr_percentage numeric(6,4),
    credit_limit numeric(18,4),
    last_statement_balance numeric(18,4),
    last_statement_issue_date date,
    minimum_payment_amount numeric(18,4),
    next_payment_due_date date,
    last_payment_amount numeric(18,4),
    last_payment_date date,
    is_overdue boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE dismissed_subscriptions (
    user_id uuid NOT NULL,
    merchant_key text NOT NULL,
    dismissed_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE goals (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    target_amount numeric(18,4) NOT NULL,
    target_date date,
    notes text,
    is_achieved boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    goal_type text DEFAULT 'net_worth'::text NOT NULL,
    account_id uuid,
    account_type_filter text,
    asset_class_filter text,
    CONSTRAINT goals_goal_type_check CHECK ((goal_type = ANY (ARRAY['net_worth'::text, 'account'::text, 'account_type'::text, 'asset_class'::text, 'spend'::text])))
);

CREATE TABLE holding_dividends (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    holding_id uuid NOT NULL,
    user_id uuid NOT NULL,
    ex_date date NOT NULL,
    dividend_per_share numeric(18,6) NOT NULL,
    shares_before numeric(18,8) NOT NULL,
    dividend_amount numeric(18,4) NOT NULL,
    drip_applied boolean NOT NULL,
    shares_added numeric(18,8),
    price_used numeric(18,4),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE holdings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    ticker text NOT NULL,
    name text,
    asset_class text DEFAULT 'equity'::text NOT NULL,
    shares numeric(18,8) NOT NULL,
    cost_basis numeric(18,4),
    price_per_share numeric(18,4),
    price_fetched_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    manual_price boolean DEFAULT false NOT NULL,
    drip_enabled boolean DEFAULT true NOT NULL,
    CONSTRAINT holdings_asset_class_check CHECK ((asset_class = ANY (ARRAY['equity'::text, 'bond'::text, 'crypto'::text, 'cash_equivalent'::text, 'other'::text])))
);

CREATE TABLE home_details (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    address text DEFAULT ''::text NOT NULL,
    last_zestimate numeric(18,4),
    last_zestimate_at timestamp with time zone,
    interest_rate numeric(6,4) DEFAULT 0 NOT NULL,
    loan_term_years integer DEFAULT 30 NOT NULL,
    original_loan_value numeric(18,4) DEFAULT 0 NOT NULL,
    current_loan_balance numeric(18,4) DEFAULT 0 NOT NULL,
    mortgage_start_date date,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    extra_monthly_payment numeric(15,2) DEFAULT 0 NOT NULL,
    track_mortgage_as_liability boolean DEFAULT false NOT NULL,
    include_in_retirement_calcs boolean DEFAULT false NOT NULL,
    loan_balance_as_of date DEFAULT CURRENT_DATE,
    property_tax_annual numeric(18,4) DEFAULT 0 NOT NULL,
    home_insurance_annual numeric(18,4) DEFAULT 0 NOT NULL,
    hoa_monthly numeric(18,4) DEFAULT 0 NOT NULL,
    pmi_monthly numeric(18,4) DEFAULT 0 NOT NULL,
    purchase_price numeric(18,4) DEFAULT 0 NOT NULL
);

CREATE TABLE household_members (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    relationship text NOT NULL,
    name text DEFAULT ''::text NOT NULL,
    birthday date,
    is_employed boolean DEFAULT true NOT NULL,
    monthly_income double precision DEFAULT 0 NOT NULL,
    retirement_age integer DEFAULT 65 NOT NULL,
    use_social_security boolean DEFAULT false NOT NULL,
    social_security_monthly double precision DEFAULT 0 NOT NULL,
    social_security_start_age integer DEFAULT 67 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    salary_growth_pct numeric(6,4) DEFAULT 0 NOT NULL,
    CONSTRAINT household_members_relationship_check CHECK ((relationship = ANY (ARRAY['self'::text, 'partner'::text])))
);

CREATE TABLE import_logs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    import_type text NOT NULL,
    filename text NOT NULL,
    status text NOT NULL,
    row_count integer DEFAULT 0 NOT NULL,
    skipped integer DEFAULT 0 NOT NULL,
    error_count integer DEFAULT 0 NOT NULL,
    imported_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT import_logs_import_type_check CHECK ((import_type = ANY (ARRAY['csv_snapshot'::text, 'ofx_holdings'::text, 'csv_transactions'::text]))),
    CONSTRAINT import_logs_status_check CHECK ((status = ANY (ARRAY['success'::text, 'partial'::text, 'failed'::text])))
);

CREATE TABLE loan_details (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    apr_percentage numeric(6,4),
    monthly_payment numeric(18,4),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE plaid_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    item_id text NOT NULL,
    access_token_encrypted text NOT NULL,
    institution_id text,
    institution_name text,
    status text DEFAULT 'active'::text NOT NULL,
    error_code text,
    last_synced_at timestamp with time zone,
    last_sync_error text,
    backfilled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    backfill_requested boolean DEFAULT false NOT NULL,
    transactions_cursor text,
    investments_refresh_at timestamp with time zone,
    CONSTRAINT plaid_items_status_check CHECK ((status = ANY (ARRAY['active'::text, 'error'::text, 'login_required'::text])))
);

CREATE TABLE recurring_transactions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    merchant text NOT NULL,
    amount numeric(12,2) NOT NULL,
    category_id uuid,
    account_id uuid,
    account_label text,
    notes text,
    frequency text NOT NULL,
    start_date date NOT NULL,
    generated_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT recurring_transactions_amount_check CHECK ((amount > (0)::numeric)),
    CONSTRAINT recurring_transactions_frequency_check CHECK ((frequency = ANY (ARRAY['monthly'::text, 'biweekly'::text])))
);

CREATE TABLE transactions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    trans_date date NOT NULL,
    merchant text NOT NULL,
    amount numeric(12,2) NOT NULL,
    category_id uuid,
    account_label text,
    source_file text,
    raw_description text NOT NULL,
    needs_review boolean DEFAULT false NOT NULL,
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    external_transaction_id text,
    external_provider text,
    account_id uuid,
    auto_categorized_at timestamp with time zone
);

CREATE TABLE user_profiles (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    use_inflation_adjustment boolean DEFAULT false NOT NULL,
    inflation_rate numeric(6,4) DEFAULT 0.03 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    monthly_expenses numeric DEFAULT 0 NOT NULL,
    use_budget_for_expenses boolean DEFAULT false NOT NULL,
    withdrawal_rate numeric(5,4),
    filing_status text DEFAULT 'single'::text NOT NULL,
    state_tax_rate numeric(5,4),
    dependents_count integer DEFAULT 0 NOT NULL,
    state_tax_code text,
    current_monthly_expenses double precision DEFAULT 0 NOT NULL
);

CREATE TABLE user_return_rates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    asset_class text NOT NULL,
    return_rate numeric(6,4) NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_return_rates_asset_class_check CHECK ((asset_class = ANY (ARRAY['equity'::text, 'etf'::text, 'mutual_fund'::text, 'bond'::text, 'crypto'::text, 'cash_equivalent'::text, 'other'::text])))
);

CREATE TABLE users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    display_name text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    failed_login_attempts integer DEFAULT 0 NOT NULL,
    locked_until timestamp with time zone,
    onboarded_at timestamp with time zone,
    dismissed_guides text[] DEFAULT '{}'::text[] NOT NULL,
    sessions_valid_after timestamp with time zone
);

CREATE TABLE vehicle_details (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_id uuid NOT NULL,
    user_id uuid NOT NULL,
    vehicle_year integer NOT NULL,
    vehicle_make text DEFAULT ''::text NOT NULL,
    vehicle_model text DEFAULT ''::text NOT NULL,
    vehicle_trim text,
    last_value numeric(18,4),
    last_value_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE TABLE vendor_rules (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    keyword text NOT NULL,
    category_id uuid NOT NULL
);

ALTER TABLE ONLY accounts
    ADD CONSTRAINT accounts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY accounts
    ADD CONSTRAINT accounts_user_id_external_provider_external_account_id_key UNIQUE (user_id, external_provider, external_account_id);

ALTER TABLE ONLY allocation_targets
    ADD CONSTRAINT allocation_targets_pkey PRIMARY KEY (id);

ALTER TABLE ONLY allocation_targets
    ADD CONSTRAINT allocation_targets_user_id_key UNIQUE (user_id);

ALTER TABLE ONLY balance_snapshots
    ADD CONSTRAINT balance_snapshots_account_id_snapshot_date_key UNIQUE (account_id, snapshot_date);

ALTER TABLE ONLY balance_snapshots
    ADD CONSTRAINT balance_snapshots_pkey PRIMARY KEY (id);

ALTER TABLE ONLY budget_categories
    ADD CONSTRAINT budget_categories_pkey PRIMARY KEY (id);

ALTER TABLE ONLY budget_limits
    ADD CONSTRAINT budget_limits_pkey PRIMARY KEY (user_id, category_id);

ALTER TABLE ONLY contribution_entries
    ADD CONSTRAINT contribution_entries_pkey PRIMARY KEY (id);

ALTER TABLE ONLY contributions
    ADD CONSTRAINT contributions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY credit_card_details
    ADD CONSTRAINT credit_card_details_account_id_key UNIQUE (account_id);

ALTER TABLE ONLY credit_card_details
    ADD CONSTRAINT credit_card_details_pkey PRIMARY KEY (id);

ALTER TABLE ONLY dismissed_subscriptions
    ADD CONSTRAINT dismissed_subscriptions_pkey PRIMARY KEY (user_id, merchant_key);

ALTER TABLE ONLY goals
    ADD CONSTRAINT goals_pkey PRIMARY KEY (id);

ALTER TABLE ONLY holding_dividends
    ADD CONSTRAINT holding_dividends_holding_id_ex_date_key UNIQUE (holding_id, ex_date);

ALTER TABLE ONLY holding_dividends
    ADD CONSTRAINT holding_dividends_pkey PRIMARY KEY (id);

ALTER TABLE ONLY holdings
    ADD CONSTRAINT holdings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY home_details
    ADD CONSTRAINT home_details_account_id_key UNIQUE (account_id);

ALTER TABLE ONLY home_details
    ADD CONSTRAINT home_details_pkey PRIMARY KEY (id);

ALTER TABLE ONLY household_members
    ADD CONSTRAINT household_members_pkey PRIMARY KEY (id);

ALTER TABLE ONLY household_members
    ADD CONSTRAINT household_members_user_id_relationship_key UNIQUE (user_id, relationship);

ALTER TABLE ONLY import_logs
    ADD CONSTRAINT import_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY loan_details
    ADD CONSTRAINT loan_details_account_id_key UNIQUE (account_id);

ALTER TABLE ONLY loan_details
    ADD CONSTRAINT loan_details_pkey PRIMARY KEY (id);

ALTER TABLE ONLY plaid_items
    ADD CONSTRAINT plaid_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY plaid_items
    ADD CONSTRAINT plaid_items_user_id_item_id_key UNIQUE (user_id, item_id);

ALTER TABLE ONLY recurring_transactions
    ADD CONSTRAINT recurring_transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY transactions
    ADD CONSTRAINT transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY transactions
    ADD CONSTRAINT transactions_user_id_trans_date_raw_description_amount_key UNIQUE (user_id, trans_date, raw_description, amount);

ALTER TABLE ONLY user_profiles
    ADD CONSTRAINT user_profiles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_profiles
    ADD CONSTRAINT user_profiles_user_id_key UNIQUE (user_id);

ALTER TABLE ONLY user_return_rates
    ADD CONSTRAINT user_return_rates_pkey PRIMARY KEY (id);

ALTER TABLE ONLY user_return_rates
    ADD CONSTRAINT user_return_rates_user_id_asset_class_key UNIQUE (user_id, asset_class);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_email_key UNIQUE (email);

ALTER TABLE ONLY users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

ALTER TABLE ONLY vehicle_details
    ADD CONSTRAINT vehicle_details_account_id_key UNIQUE (account_id);

ALTER TABLE ONLY vehicle_details
    ADD CONSTRAINT vehicle_details_pkey PRIMARY KEY (id);

ALTER TABLE ONLY vendor_rules
    ADD CONSTRAINT vendor_rules_pkey PRIMARY KEY (id);

CREATE INDEX idx_accounts_external ON accounts USING btree (external_provider, external_account_id) WHERE (external_provider IS NOT NULL);

CREATE INDEX idx_accounts_user_id ON accounts USING btree (user_id);

CREATE INDEX idx_budget_categories_user_id ON budget_categories USING btree (user_id) WHERE (user_id IS NOT NULL);

CREATE INDEX idx_contribution_entries_account_date ON contribution_entries USING btree (account_id, entry_date);

CREATE UNIQUE INDEX idx_contribution_entries_external ON contribution_entries USING btree (account_id, external_id) WHERE (external_id IS NOT NULL);

CREATE INDEX idx_contributions_account_id ON contributions USING btree (account_id);

CREATE INDEX idx_contributions_user_id ON contributions USING btree (user_id);

CREATE INDEX idx_credit_card_details_account ON credit_card_details USING btree (account_id);

CREATE INDEX idx_goals_user ON goals USING btree (user_id);

CREATE INDEX idx_holding_dividends_holding ON holding_dividends USING btree (holding_id);

CREATE INDEX idx_holding_dividends_user ON holding_dividends USING btree (user_id);

CREATE INDEX idx_holdings_account ON holdings USING btree (account_id);

CREATE INDEX idx_holdings_ticker ON holdings USING btree (ticker);

CREATE INDEX idx_holdings_user ON holdings USING btree (user_id);

CREATE INDEX idx_home_details_account ON home_details USING btree (account_id);

CREATE INDEX idx_import_logs_user_id ON import_logs USING btree (user_id, imported_at DESC);

CREATE INDEX idx_loan_details_account ON loan_details USING btree (account_id);

CREATE INDEX idx_plaid_items_user ON plaid_items USING btree (user_id);

CREATE INDEX idx_recurring_transactions_user ON recurring_transactions USING btree (user_id);

CREATE INDEX idx_snapshots_account_date ON balance_snapshots USING btree (account_id, snapshot_date DESC);

CREATE INDEX idx_snapshots_user_date ON balance_snapshots USING btree (user_id, snapshot_date DESC);

CREATE INDEX idx_transactions_account ON transactions USING btree (account_id) WHERE (account_id IS NOT NULL);

CREATE INDEX idx_transactions_auto_categorize_pending ON transactions USING btree (user_id) WHERE (needs_review AND (category_id IS NULL) AND (auto_categorized_at IS NULL));

CREATE UNIQUE INDEX idx_transactions_external ON transactions USING btree (user_id, external_transaction_id) WHERE (external_transaction_id IS NOT NULL);

CREATE INDEX idx_transactions_user_category ON transactions USING btree (user_id, category_id);

CREATE INDEX idx_transactions_user_date ON transactions USING btree (user_id, trans_date);

CREATE INDEX idx_user_profiles_user_id ON user_profiles USING btree (user_id);

CREATE INDEX idx_user_return_rates_user_id ON user_return_rates USING btree (user_id);

CREATE INDEX idx_vehicle_details_account ON vehicle_details USING btree (account_id);

CREATE UNIQUE INDEX users_single_account ON users USING btree ((true));

CREATE TRIGGER set_updated_at_credit_card_details BEFORE UPDATE ON credit_card_details FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_updated_at_home_details BEFORE UPDATE ON home_details FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER set_updated_at_loan_details BEFORE UPDATE ON loan_details FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_accounts_updated_at BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_balance_snapshots_updated_at BEFORE UPDATE ON balance_snapshots FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_contribution_entries_updated_at BEFORE UPDATE ON contribution_entries FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_contributions_updated_at BEFORE UPDATE ON contributions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_goals_updated_at BEFORE UPDATE ON goals FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_holdings_updated_at BEFORE UPDATE ON holdings FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_user_profiles_updated_at BEFORE UPDATE ON user_profiles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_user_return_rates_updated_at BEFORE UPDATE ON user_return_rates FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER trg_users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE ONLY accounts
    ADD CONSTRAINT accounts_member_id_fkey FOREIGN KEY (member_id) REFERENCES household_members(id) ON DELETE SET NULL;

ALTER TABLE ONLY accounts
    ADD CONSTRAINT accounts_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY allocation_targets
    ADD CONSTRAINT allocation_targets_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY balance_snapshots
    ADD CONSTRAINT balance_snapshots_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY balance_snapshots
    ADD CONSTRAINT balance_snapshots_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY budget_categories
    ADD CONSTRAINT budget_categories_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY budget_limits
    ADD CONSTRAINT budget_limits_category_id_fkey FOREIGN KEY (category_id) REFERENCES budget_categories(id) ON DELETE CASCADE;

ALTER TABLE ONLY budget_limits
    ADD CONSTRAINT budget_limits_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id);

ALTER TABLE ONLY contribution_entries
    ADD CONSTRAINT contribution_entries_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY contribution_entries
    ADD CONSTRAINT contribution_entries_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY contributions
    ADD CONSTRAINT contributions_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY contributions
    ADD CONSTRAINT contributions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY credit_card_details
    ADD CONSTRAINT credit_card_details_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY credit_card_details
    ADD CONSTRAINT credit_card_details_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY dismissed_subscriptions
    ADD CONSTRAINT dismissed_subscriptions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY goals
    ADD CONSTRAINT goals_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE ONLY goals
    ADD CONSTRAINT goals_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY holding_dividends
    ADD CONSTRAINT holding_dividends_holding_id_fkey FOREIGN KEY (holding_id) REFERENCES holdings(id) ON DELETE CASCADE;

ALTER TABLE ONLY holding_dividends
    ADD CONSTRAINT holding_dividends_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY holdings
    ADD CONSTRAINT holdings_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY holdings
    ADD CONSTRAINT holdings_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY home_details
    ADD CONSTRAINT home_details_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY home_details
    ADD CONSTRAINT home_details_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY household_members
    ADD CONSTRAINT household_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY import_logs
    ADD CONSTRAINT import_logs_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY loan_details
    ADD CONSTRAINT loan_details_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY loan_details
    ADD CONSTRAINT loan_details_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY plaid_items
    ADD CONSTRAINT plaid_items_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY recurring_transactions
    ADD CONSTRAINT recurring_transactions_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE ONLY recurring_transactions
    ADD CONSTRAINT recurring_transactions_category_id_fkey FOREIGN KEY (category_id) REFERENCES budget_categories(id) ON DELETE SET NULL;

ALTER TABLE ONLY recurring_transactions
    ADD CONSTRAINT recurring_transactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY transactions
    ADD CONSTRAINT transactions_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE SET NULL;

ALTER TABLE ONLY transactions
    ADD CONSTRAINT transactions_category_id_fkey FOREIGN KEY (category_id) REFERENCES budget_categories(id);

ALTER TABLE ONLY transactions
    ADD CONSTRAINT transactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY user_profiles
    ADD CONSTRAINT user_profiles_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY user_return_rates
    ADD CONSTRAINT user_return_rates_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY vehicle_details
    ADD CONSTRAINT vehicle_details_account_id_fkey FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE;

ALTER TABLE ONLY vehicle_details
    ADD CONSTRAINT vehicle_details_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE ONLY vendor_rules
    ADD CONSTRAINT vendor_rules_category_id_fkey FOREIGN KEY (category_id) REFERENCES budget_categories(id);

-- System budget categories.
INSERT INTO budget_categories (name, is_system, sort_order) VALUES
    ('Dining & Restaurants', true, 1),
    ('Groceries', true, 2),
    ('Transportation', true, 3),
    ('Auto', true, 4),
    ('Housing', true, 5),
    ('Utilities', true, 6),
    ('Health & Medical', true, 7),
    ('Fitness & Wellness', true, 8),
    ('Entertainment', true, 9),
    ('Shopping', true, 10),
    ('Travel', true, 11),
    ('Education', true, 12),
    ('Personal Care', true, 13),
    ('Subscriptions', true, 14),
    ('Childcare & Kids', true, 15),
    ('Pets', true, 16),
    ('Gifts & Donations', true, 17),
    ('Fees & Banking', true, 18),
    ('Other', true, 19),
    ('Transfer', true, 20),
    ('Income', true, 21);

-- Vendor keyword rules used to auto-categorize imported transactions. Order
-- matters: more specific (longer) keywords come before shorter ones.
INSERT INTO vendor_rules (keyword, category_id)
SELECT v.keyword, c.id
FROM (VALUES
    (1, 'TACO BELL', 'Dining & Restaurants'),
    (2, 'SUSHI', 'Dining & Restaurants'),
    (3, 'BURGER', 'Dining & Restaurants'),
    (4, 'PIZZA', 'Dining & Restaurants'),
    (5, 'RESTAURANT', 'Dining & Restaurants'),
    (6, 'DAIRY QUEEN', 'Dining & Restaurants'),
    (7, '5GUYS', 'Dining & Restaurants'),
    (8, 'FIVE GUYS', 'Dining & Restaurants'),
    (9, 'JERSEY MIKE', 'Dining & Restaurants'),
    (10, 'RAISING CANE', 'Dining & Restaurants'),
    (11, 'DUNKIN', 'Dining & Restaurants'),
    (12, 'SUBWAY', 'Dining & Restaurants'),
    (13, 'PANERA', 'Dining & Restaurants'),
    (14, 'STARBUCKS', 'Dining & Restaurants'),
    (15, 'MCDONALD', 'Dining & Restaurants'),
    (16, 'CHIPOTLE', 'Dining & Restaurants'),
    (17, 'GRUBHUB', 'Dining & Restaurants'),
    (18, 'UBEREATS', 'Dining & Restaurants'),
    (19, 'DOOR DASH', 'Dining & Restaurants'),
    (20, 'DOORDASH', 'Dining & Restaurants'),
    (21, 'HY-VEE', 'Groceries'),
    (22, 'COUNTY MARKET', 'Groceries'),
    (23, 'HELLO FRESH', 'Groceries'),
    (24, 'HELLOFRESH', 'Groceries'),
    (25, 'SPROUTS', 'Groceries'),
    (26, 'WEGMANS', 'Groceries'),
    (27, 'PUBLIX', 'Groceries'),
    (28, 'SAFEWAY', 'Groceries'),
    (29, 'KROGER', 'Groceries'),
    (30, 'TRADER JOE', 'Groceries'),
    (31, 'WHOLE FOODS', 'Groceries'),
    (32, 'WU METRO', 'Transportation'),
    (33, 'STL METRO', 'Transportation'),
    (34, 'TRANSIT', 'Transportation'),
    (35, 'PARKING', 'Transportation'),
    (36, 'THECARPARK', 'Transportation'),
    (37, 'PARKMOBILE', 'Transportation'),
    (38, 'LYFT', 'Transportation'),
    (39, 'LOVE''S', 'Transportation'),
    (40, 'SUNOCO', 'Transportation'),
    (41, 'SPEEDWAY', 'Transportation'),
    (42, 'CIRCLE K', 'Transportation'),
    (43, 'CITGO', 'Transportation'),
    (44, 'MARATHON', 'Transportation'),
    (45, 'CHEVRON', 'Transportation'),
    (46, 'SHELL', 'Transportation'),
    (47, 'EXXON', 'Transportation'),
    (48, 'EXXONMOBIL', 'Transportation'),
    (49, 'SAMS FUEL', 'Transportation'),
    (50, 'STATE FARM', 'Auto'),
    (51, 'FIRESTONE', 'Auto'),
    (52, 'JIFFY LUBE', 'Auto'),
    (53, 'ADVANCE AUTO', 'Auto'),
    (54, 'O REILLY AUTO', 'Auto'),
    (55, 'AUTOGLASS', 'Auto'),
    (56, 'AUTOZONE', 'Auto'),
    (57, 'T-MOBILE', 'Utilities'),
    (58, 'VERIZON', 'Utilities'),
    (59, 'AT&T', 'Utilities'),
    (60, 'XFINITY', 'Utilities'),
    (61, 'SPECTRUM', 'Utilities'),
    (62, 'COMCAST', 'Utilities'),
    (63, 'AMERIGAS', 'Utilities'),
    (64, 'LACLEDE', 'Utilities'),
    (65, 'AMEREN', 'Utilities'),
    (66, 'SPIRE', 'Utilities'),
    (67, 'OPTOMETRIST', 'Health & Medical'),
    (68, 'CLINIC', 'Health & Medical'),
    (69, 'DENTAL', 'Health & Medical'),
    (70, 'URGENT CARE', 'Health & Medical'),
    (71, 'HOSPITAL', 'Health & Medical'),
    (72, 'PHARMACY', 'Health & Medical'),
    (73, 'PELOTON', 'Fitness & Wellness'),
    (74, 'YMCA', 'Fitness & Wellness'),
    (75, 'GOLD''S GYM', 'Fitness & Wellness'),
    (76, 'ANYTIME FITNESS', 'Fitness & Wellness'),
    (77, 'PLANET FITNESS', 'Fitness & Wellness'),
    (78, 'RED TAIL RUN', 'Entertainment'),
    (79, 'TOWER TEE', 'Entertainment'),
    (80, 'FOREST PARK GOLF', 'Entertainment'),
    (81, 'CINEMARK', 'Entertainment'),
    (82, 'REGAL', 'Entertainment'),
    (83, 'AMC THEATRE', 'Entertainment'),
    (84, 'ALAMO DRF', 'Entertainment'),
    (85, 'NETFLIX', 'Entertainment'),
    (86, 'SAM''S CLUB', 'Shopping'),
    (87, 'SAMS SCAN', 'Shopping'),
    (88, 'CARPETS PLUS', 'Shopping'),
    (89, 'BEST BUY', 'Shopping'),
    (90, 'OLD NAVY', 'Shopping'),
    (91, 'TJ MAXX', 'Shopping'),
    (92, 'DOLLAR GENERAL', 'Shopping'),
    (93, 'DOLLAR TREE', 'Shopping'),
    (94, 'IKEA', 'Shopping'),
    (95, 'MENARDS', 'Shopping'),
    (96, 'LOWE''S', 'Shopping'),
    (97, 'HOME DEPOT', 'Shopping'),
    (98, 'COSTCO', 'Shopping'),
    (99, 'TARGET', 'Shopping'),
    (100, 'WALMART', 'Shopping'),
    (101, 'AMZN', 'Shopping'),
    (102, 'AMAZON', 'Shopping'),
    (103, 'HERTZ', 'Travel'),
    (104, 'AMERICAN AIR', 'Travel'),
    (105, 'SOUTHWEST', 'Travel'),
    (106, 'UNITED AIR', 'Travel'),
    (107, 'DELTA AIR', 'Travel'),
    (108, 'HILTON', 'Travel'),
    (109, 'MARRIOTT', 'Travel'),
    (110, 'AIRBNB', 'Travel'),
    (111, 'UDEMY', 'Education'),
    (112, 'COURSERA', 'Education'),
    (113, 'VUE*TESTING', 'Education'),
    (114, 'SUPERCUTS', 'Personal Care'),
    (115, 'GREAT CLIPS', 'Personal Care'),
    (116, 'PDGA', 'Subscriptions'),
    (117, 'DISNEY+', 'Subscriptions'),
    (118, 'HULU', 'Subscriptions'),
    (119, 'STARZ', 'Subscriptions'),
    (120, 'RING STANDARD', 'Subscriptions'),
    (121, 'RING PROTECT', 'Subscriptions'),
    (122, 'SPOTIFY', 'Subscriptions'),
    (123, 'AMZN DIGITAL', 'Subscriptions'),
    (124, 'AMAZON PRIME', 'Subscriptions'),
    (125, 'USPS PO', 'Fees & Banking'),
    (126, 'DMV', 'Fees & Banking'),
    (127, 'MUNICIPAL SERVICE', 'Fees & Banking'),
    (128, 'LATE FEE', 'Fees & Banking'),
    (129, 'ANNUAL FEE', 'Fees & Banking'),
    (130, 'ATT PAYMENT', 'Utilities'),
    (131, 'TARGET CARD SRVC', 'Transfer'),
    (132, 'CHASE CREDIT CRD', 'Transfer'),
    (133, 'DISCOVER E-PAYMENT', 'Transfer'),
    (134, 'APPLECARD GSBANK', 'Transfer'),
    (135, 'AMEX EPAYMENT', 'Transfer'),
    (136, 'DIRECT DEP', 'Income'),
    (137, 'PAYROLL', 'Income'),
    (138, 'EZCATER INC', 'Income')
) AS v(ord, keyword, cat_name)
JOIN budget_categories c ON c.name = v.cat_name AND c.is_system AND c.user_id IS NULL
ORDER BY v.ord;
