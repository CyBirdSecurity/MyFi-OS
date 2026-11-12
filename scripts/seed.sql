-- MyFi Demo Seed Data
-- Creates the demo user with data for every feature: accounts of each kind
-- with 15 months of snapshot history, holdings and dividends, goals of every
-- type, contribution plans (dollar and percent of income, tiered and flat
-- matches) with this year's contributions, a year of budget transactions with
-- limits and a recurring bill, card and loan details, a home with mortgage
-- costs, a partner and a joint account, and a Plaid showcase.
--
-- A MyFi instance holds a single account, so this only runs on an instance
-- that hasn't been set up yet.
--
-- Usage:   make seed        (requires the docker compose stack to be running)
-- Login:   demo@myfi.local / demo1234
--
-- To reset and reseed:
--   make reseed-demo

DO $$
DECLARE
  uid       UUID;
  a1        UUID;   -- checking
  a2        UUID;   -- savings
  a3        UUID;   -- 401k
  a4        UUID;   -- roth ira
  a5        UUID;   -- brokerage
  a6        UUID;   -- credit card
  a7        UUID;   -- hsa
  wk        DATE;   -- weekly cursor for Plaid-style backfill snapshots
  demo_self_id    UUID;
  demo_partner_id UUID;
  a8        UUID;   -- partner's Roth IRA (household demo)
  a9        UUID;   -- crypto
  vti_id    UUID;   -- brokerage VTI holding, for dividend history
  dt        DATE;
  i         INT;
BEGIN

  -- ── Demo user ─────────────────────────────────────────────────────────────
  IF EXISTS (SELECT 1 FROM users) THEN
    RAISE NOTICE 'This instance already has an account — skipping the demo seed.';
  ELSE
    -- Password: demo1234  (bcrypt cost 10)
    uid := gen_random_uuid();
    INSERT INTO users (id, email, password_hash, display_name, onboarded_at)
    VALUES (
      uid,
      'demo@myfi.local',
      '$2a$10$iDt9xY1GFBg34Z8YHyQpS.GTlnXCmgDvVkloGxf2PEZhZnehQoUZe',
      'Demo User',
      NOW()
    );

    -- ── Accounts ───────────────────────────────────────────────────────────
    a1 := gen_random_uuid();
    a2 := gen_random_uuid();
    a3 := gen_random_uuid();
    a4 := gen_random_uuid();
    a5 := gen_random_uuid();
    a6 := gen_random_uuid();
    a7 := gen_random_uuid();

    INSERT INTO accounts (id, user_id, name, account_type, tax_treatment, institution, is_manual)
    VALUES
      (a1, uid, 'Chase Checking',            'checking',         'taxable',      'Chase',         TRUE),
      (a2, uid, 'Marcus High-Yield Savings',  'savings',          'taxable',      'Goldman Sachs', TRUE),
      (a3, uid, 'Fidelity 401(k)',            'traditional_401k', 'tax_deferred', 'Fidelity',      TRUE),
      (a4, uid, 'Fidelity Roth IRA',          'roth_ira',         'tax_free',     'Fidelity',      TRUE),
      (a5, uid, 'Vanguard Brokerage',         'brokerage',        'taxable',      'Vanguard',      TRUE),
      (a6, uid, 'Chase Visa',                 'credit_card',      'taxable',      'Chase',         TRUE),
      (a7, uid, 'Fidelity HSA',               'hsa',              'tax_free',     'Fidelity',      TRUE);

    -- ── Balance Snapshots: 15 months of history ────────────────────────────
    --   i=0  → 14 months ago (oldest)
    --   i=14 → current month (most recent)
    --
    --   Checking:     $8,200  + $300/mo  → $12,400
    --   Savings:     $28,000  + $1,000/mo → $42,000
    --   401(k):     $135,000  + $2,500/mo → $170,000
    --   Roth IRA:    $52,000  + $1,100/mo → $67,400
    --   Brokerage:   $42,000  + $900/mo  → $54,600
    --   Credit Card:  -$3,100 − $20/mo   → -$3,380
    --   HSA:          $7,500  + $450/mo  → $13,800

    FOR i IN 0..14 LOOP
      dt := (DATE_TRUNC('month', CURRENT_DATE) - ((14 - i) * INTERVAL '1 month'))::DATE;

      INSERT INTO balance_snapshots (id, account_id, user_id, snapshot_date, balance, source)
      VALUES
        (gen_random_uuid(), a1, uid, dt,   8200 + 300  * i,  'manual'),
        (gen_random_uuid(), a2, uid, dt,  28000 + 1000 * i,  'manual'),
        (gen_random_uuid(), a3, uid, dt, 135000 + 2500 * i,  'manual'),
        (gen_random_uuid(), a4, uid, dt,  52000 + 1100 * i,  'manual'),
        (gen_random_uuid(), a5, uid, dt,  42000 + 900  * i,  'manual'),
        (gen_random_uuid(), a6, uid, dt,  -3100 - 20   * i,  'manual'),
        (gen_random_uuid(), a7, uid, dt,   7500 + 450  * i,  'manual');
    END LOOP;

    -- ── Holdings ───────────────────────────────────────────────────────────

    -- Fidelity Roth IRA  (~$67,480 current value)
    INSERT INTO holdings (id, account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share, price_fetched_at)
    VALUES
      (gen_random_uuid(), a4, uid, 'VTSAX', 'Vanguard Total Stock Mkt Idx Adm',     'equity', 200.000,  95.42, TRUE, 145.00, NOW()),
      (gen_random_uuid(), a4, uid, 'VXUS',  'Vanguard Total Intl Stock Index Adm',  'equity', 300.000,  55.10, TRUE,  65.00, NOW()),
      (gen_random_uuid(), a4, uid, 'BND',   'Vanguard Total Bond Market ETF',        'bond',   260.000,  75.80, TRUE,  73.00, NOW());

    -- Fidelity 401(k)  (~$168,250 current value)
    INSERT INTO holdings (id, account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share, price_fetched_at)
    VALUES
      (gen_random_uuid(), a3, uid, 'FXAIX', 'Fidelity 500 Index Fund',              'equity', 550.000, 160.00, TRUE, 215.00, NOW()),
      (gen_random_uuid(), a3, uid, 'FZROX', 'Fidelity ZERO Total Market Index Fund','equity',2500.000,  14.00, TRUE,  20.00, NOW());

    -- Vanguard Brokerage  (~$54,700 current value)
    INSERT INTO holdings (id, account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share, price_fetched_at)
    VALUES
      (gen_random_uuid(), a5, uid, 'VTI',  'Vanguard Total Stock Market ETF',       'equity', 150.000, 200.00, TRUE, 265.00, NOW()),
      (gen_random_uuid(), a5, uid, 'VXUS', 'Vanguard Total Intl Stock ETF',         'equity', 230.000,  58.00, TRUE,  65.00, NOW());

    -- Crypto account: a small BTC position, manually priced so it never
    -- depends on a live quote. Gives the crypto allocation target something.
    a9 := gen_random_uuid();
    INSERT INTO accounts (id, user_id, name, account_type, tax_treatment, institution, is_manual)
    VALUES (a9, uid, 'Coinbase', 'crypto', 'taxable', 'Coinbase', TRUE);
    FOR i IN 0..14 LOOP
      dt := (DATE_TRUNC('month', CURRENT_DATE) - ((14 - i) * INTERVAL '1 month'))::DATE;
      INSERT INTO balance_snapshots (id, account_id, user_id, snapshot_date, balance, source)
      VALUES (gen_random_uuid(), a9, uid, dt, ROUND((2600 + 60 * i + 300 * SIN(i / 2.0))::NUMERIC, 2), 'manual');
    END LOOP;
    INSERT INTO holdings (id, account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share, price_fetched_at)
    VALUES (gen_random_uuid(), a9, uid, 'BTC', 'Bitcoin', 'crypto', 0.0500, 52000.00, TRUE, 68000.00, NOW());

    -- Dividend history for the brokerage's VTI: the last four quarterly
    -- payouts, reinvested (DRIP).
    SELECT id INTO vti_id FROM holdings WHERE account_id = a5 AND ticker = 'VTI';
    INSERT INTO holding_dividends (holding_id, user_id, ex_date, dividend_per_share, shares_before, dividend_amount, drip_applied, shares_added, price_used)
    SELECT vti_id, uid,
           (DATE_TRUNC('quarter', CURRENT_DATE) - (q * INTERVAL '3 months') + INTERVAL '24 days')::DATE,
           0.95, 147.7 + 0.57 * (4 - q), ROUND((0.95 * (147.7 + 0.57 * (4 - q)))::NUMERIC, 2), TRUE, 0.57, 250.00
    FROM generate_series(1, 4) AS q;

    -- Chase Visa card details: paid in full each month, so the forecast pays
    -- it off and the Card Details tab is filled in.
    INSERT INTO credit_card_details (account_id, user_id, apr_percentage, credit_limit, last_statement_balance, last_statement_issue_date,
                                     minimum_payment_amount, next_payment_due_date, last_payment_amount, last_payment_date)
    VALUES (a6, uid, 22.99, 15000, 3350, (DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '10 days')::DATE,
            35, (DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '14 days')::DATE, 3350, (DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '20 days')::DATE);

    -- ── Goals (net worth milestones) ───────────────────────────────────────
    INSERT INTO goals (id, user_id, name, target_amount, target_date, notes)
    VALUES
      (gen_random_uuid(), uid, '$500K Net Worth',   500000,   '2028-01-01', 'On track — keep investing consistently'),
      (gen_random_uuid(), uid, '$1M Net Worth',    1000000,   '2035-06-01', 'First millionaire milestone'),
      (gen_random_uuid(), uid, '$2M Retirement',  2000000,   '2045-01-01', 'Long-term retirement target');

    -- ── Transactions (12 months, demo user) ────────────────────────────────
    INSERT INTO transactions (id, user_id, trans_date, merchant, amount, category_id, account_label, raw_description)
    SELECT gen_random_uuid(), uid, t.trans_date, t.merchant, t.amount,
           (SELECT id FROM budget_categories WHERE name = t.cat AND (is_system OR user_id = uid) LIMIT 1),
           t.acct, t.merchant
    FROM (VALUES
      -- Month -2
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 1, 'Whole Foods Market',       87.43,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 2, 'Chipotle',                 13.75,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 3, 'Comcast',                 109.99,  'Utilities',           'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 4, 'Shell',                    58.20,  'Transportation',      'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 5, 'Amazon',                   34.99,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 6, 'Trader Joe''s',             62.18,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 7, 'Netflix',                  17.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 8, 'Starbucks',                 6.75,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 9, 'Verizon',                  85.00,  'Utilities',           'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 10, 'Whole Foods Market',        91.22,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 11, 'DoorDash',                 28.50,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 12, 'Spotify',                  11.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 13, 'Target',                   73.41,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 14, 'AMC Theatre',              32.00,  'Entertainment',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 15, 'Panera Bread',             14.89,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 16, 'Great Clips',              22.00,  'Personal Care',       'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 17, 'Exxon',                    61.40,  'Transportation',      'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 18, 'Whole Foods Market',       55.67,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 19, 'Hulu',                     18.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 20, 'Chipotle',                 12.50,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 21, 'Home Depot',               48.92,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 22, 'Planet Fitness',           25.00,  'Fitness & Wellness',  'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 23, 'Starbucks',                 7.25,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 24, 'Amazon',                  112.37,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 25, 'Trader Joe''s',             58.44,  'Groceries',          'Chase Checking'),
      -- Month -1
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 1, 'Whole Foods Market',       94.10,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 2, 'DoorDash',                 31.75,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 3, 'Comcast',                 109.99,  'Utilities',           'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 4, 'Shell',                    54.80,  'Transportation',      'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 5, 'Costco',                  187.63,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 6, 'Netflix',                  17.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 7, 'Starbucks',                 6.50,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 8, 'Verizon',                  85.00,  'Utilities',           'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 9, 'Trader Joe''s',             70.92,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 10, 'Southwest Airlines',       218.00,  'Travel',              'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 11, 'Airbnb',                  245.00,  'Travel',              'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 12, 'Spotify',                  11.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 13, 'Whole Foods Market',        79.33,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 14, 'Panera Bread',             16.20,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 15, 'Target',                   44.19,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 16, 'Exxon',                    59.60,  'Transportation',      'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 17, 'Chipotle',                 13.25,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 18, 'Great Clips',              22.00,  'Personal Care',       'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 19, 'Hulu',                     18.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 20, 'Amazon',                   67.82,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 21, 'Planet Fitness',           25.00,  'Fitness & Wellness',  'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 22, 'Home Depot',               93.47,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 23, 'Starbucks',                 5.95,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 24, 'Trader Joe''s',             61.08,  'Groceries',          'Chase Checking'),
      -- Current month
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 1, 'Whole Foods Market',       88.75,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 2, 'Comcast',                 109.99,  'Utilities',           'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 3, 'Chipotle',                 14.00,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 4, 'Shell',                    57.10,  'Transportation',      'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 5, 'Netflix',                  17.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 6, 'Verizon',                  85.00,  'Utilities',           'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 7, 'Trader Joe''s',             66.34,  'Groceries',          'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 8, 'Starbucks',                 6.75,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 9, 'Spotify',                  11.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 10, 'Amazon',                   29.99,  'Shopping',            'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 11, 'DoorDash',                 24.80,  'Dining & Restaurants','Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 12, 'Planet Fitness',           25.00,  'Fitness & Wellness',  'Chase Checking'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 13, 'Hulu',                     18.99,  'Subscriptions',       'Chase Visa'),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 14, 'Target',                   51.23,  'Shopping',            'Chase Visa')
    ) AS t(trans_date, merchant, amount, cat, acct)
    WHERE t.trans_date <= CURRENT_DATE
    ON CONFLICT (user_id, trans_date, raw_description, amount) DO NOTHING;

    -- Payroll income (one deposit per month, current year) — powers the Budget
    -- page's Income tab and monthly income breakdown.
    INSERT INTO transactions (id, user_id, trans_date, merchant, amount, category_id, account_label, raw_description)
    SELECT gen_random_uuid(), uid, t.trans_date, t.merchant, t.amount,
           (SELECT id FROM budget_categories WHERE name = 'Income' AND is_system LIMIT 1),
           'Chase Checking', t.merchant
    FROM (VALUES
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE + 1, 'Acme Corp Payroll', 6200.00),
      ((DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE + 1,  'Acme Corp Payroll', 6200.00),
      ((DATE_TRUNC('month', CURRENT_DATE))::DATE + 1,                      'Acme Corp Payroll', 6200.00)
    ) AS t(trans_date, merchant, amount)
    WHERE t.trans_date <= CURRENT_DATE
    ON CONFLICT (user_id, trans_date, raw_description, amount) DO NOTHING;

    -- Fill in the nine months before that from the oldest month above, so the
    -- 12-month spending chart, cash flow card, and subscription detection
    -- have a year of history. Bills, subscriptions, and pay repeat as-is.
    -- Everyday spending moves up to 12 days and 40% in amount each month, so
    -- it doesn't look like a subscription.
    INSERT INTO transactions (id, user_id, trans_date, merchant, amount, category_id, account_label, raw_description)
    SELECT gen_random_uuid(), t.user_id,
           (t.trans_date - (k * INTERVAL '1 month')
              + CASE WHEN e.everyday THEN ((k * 7 + LENGTH(t.merchant) * 3) % 25 - 12) * INTERVAL '1 day' ELSE INTERVAL '0' END)::DATE,
           t.merchant,
           CASE WHEN e.everyday THEN ROUND((t.amount * (1 + 0.4 * SIN(k * 2.3 + LENGTH(t.merchant))))::NUMERIC, 2) ELSE t.amount END,
           t.category_id, t.account_label, t.raw_description
    FROM transactions t
    LEFT JOIN budget_categories c ON c.id = t.category_id
    CROSS JOIN LATERAL (SELECT c.name IN ('Groceries', 'Dining & Restaurants', 'Shopping', 'Transportation', 'Entertainment', 'Travel', 'Personal Care') AS everyday) e
    CROSS JOIN generate_series(1, 9) AS k
    WHERE t.user_id = uid
      AND t.trans_date >= (DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::DATE
      AND t.trans_date <  (DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '1 month')::DATE
    ON CONFLICT (user_id, trans_date, raw_description, amount) DO NOTHING;

    -- The 3 most recent transactions left unclassified, as a real bank import
    -- would leave them — populates the review queue / dashboard nudge.
    UPDATE transactions
    SET needs_review = TRUE, category_id = NULL
    WHERE id IN (
      SELECT id FROM transactions WHERE user_id = uid
      ORDER BY trans_date DESC, created_at DESC LIMIT 3
    );

    -- Rent as a recurring transaction, with the twelve occurrences so far
    -- already generated (generated_count tells the daily job they're done).
    INSERT INTO recurring_transactions (user_id, merchant, amount, category_id, account_id, account_label, frequency, start_date, generated_count)
    VALUES (uid, 'Oakwood Apartments Rent', 2400,
            (SELECT id FROM budget_categories WHERE name = 'Housing' AND is_system LIMIT 1),
            a1, 'Chase Checking', 'monthly', (DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '11 months')::DATE, 12);
    INSERT INTO transactions (user_id, trans_date, merchant, amount, category_id, account_id, account_label, raw_description)
    SELECT uid, (DATE_TRUNC('month', CURRENT_DATE) - (m * INTERVAL '1 month'))::DATE, 'Oakwood Apartments Rent', 2400,
           (SELECT id FROM budget_categories WHERE name = 'Housing' AND is_system LIMIT 1),
           a1, 'Chase Checking', 'Oakwood Apartments Rent'
    FROM generate_series(0, 11) AS m
    ON CONFLICT (user_id, trans_date, raw_description, amount) DO NOTHING;

    -- Monthly limits: groceries comfortably under, subscriptions close to
    -- the limit, and shopping over it last month.
    INSERT INTO budget_limits (user_id, category_id, monthly_limit)
    SELECT uid, c.id, l.amt
    FROM (VALUES ('Groceries', 350), ('Dining & Restaurants', 90), ('Shopping', 250), ('Subscriptions', 55)) AS l(cat, amt)
    JOIN budget_categories c ON c.name = l.cat AND c.is_system;

    -- ── Profile ────────────────────────────────────────────────────────────
    INSERT INTO user_profiles (
      user_id, use_inflation_adjustment, inflation_rate, monthly_expenses, current_monthly_expenses,
      use_budget_for_expenses, withdrawal_rate, filing_status, state_tax_code, dependents_count
    ) VALUES (uid, TRUE, 0.03, 6000, 5400, TRUE, 0.04, 'married_filing_jointly', 'CA', 1);

    -- ── Household (self + partner) — demos joint net worth & filtering ──────
    demo_self_id := gen_random_uuid();
    demo_partner_id := gen_random_uuid();
    INSERT INTO household_members (
      id, user_id, relationship, name, birthday, is_employed, monthly_income,
      retirement_age, use_social_security,
      social_security_monthly, social_security_start_age
    ) VALUES
      (demo_self_id,    uid, 'self',    'Demo User', '1990-04-12', TRUE, 9500, 60, TRUE, 2400, 67),
      (demo_partner_id, uid, 'partner', 'Jordan',     '1991-08-03', TRUE, 7200, 62, TRUE, 2100, 67);

    -- The savings account is joint, so Household Breakdown shows all three
    -- owners (self, partner, joint).
    UPDATE accounts SET is_joint = TRUE WHERE id = a2;

    -- Partner-owned account, so household filtering has something to show
    a8 := gen_random_uuid();
    INSERT INTO accounts (id, user_id, name, account_type, tax_treatment, institution, is_manual, member_id)
    VALUES (a8, uid, 'Jordan''s Vanguard Roth IRA', 'roth_ira', 'tax_free', 'Vanguard', TRUE, demo_partner_id);

    FOR i IN 0..14 LOOP
      dt := (DATE_TRUNC('month', CURRENT_DATE) - ((14 - i) * INTERVAL '1 month'))::DATE;
      INSERT INTO balance_snapshots (id, account_id, user_id, snapshot_date, balance, source)
      VALUES (gen_random_uuid(), a8, uid, dt, 31000 + 650 * i, 'manual');
    END LOOP;

    INSERT INTO holdings (id, account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share, price_fetched_at)
    VALUES (gen_random_uuid(), a8, uid, 'VOO', 'Vanguard S&P 500 ETF', 'equity', 90.000, 380.00, TRUE, 445.00, NOW());

    -- ── Allocation targets ───────────────────────────────────────────────────
    INSERT INTO allocation_targets (user_id, equity_pct, bond_pct, crypto_pct, cash_pct, other_pct)
    VALUES (uid, 75, 15, 5, 5, 0);

    -- ── Return rate overrides — demos the customization in Settings ─────────
    INSERT INTO user_return_rates (user_id, asset_class, return_rate)
    VALUES
      (uid, 'equity', 0.08),
      (uid, 'bond', 0.035),
      (uid, 'cash_equivalent', 0.02);

    -- ── Recurring contributions (401k w/ tiered employer match, Roth IRA, HSA) ──
    -- The match is 100% of the first 3% of salary, then 50% of the next 2%.
    INSERT INTO contributions (id, account_id, user_id, label, amount, frequency, annual_increase_pct, match_tiers, is_active)
    VALUES
      (gen_random_uuid(), a3, uid, '401(k) payroll deferral', 1250, 'monthly', 0.02,
         '[{"matchPct": 1, "upToPct": 0.03}, {"matchPct": 0.5, "upToPct": 0.02}]', TRUE),
      (gen_random_uuid(), a4, uid, 'Roth IRA contribution',    583, 'monthly', 0, '[]', TRUE),
      (gen_random_uuid(), a7, uid, 'HSA contribution',         300, 'monthly', 0, '[]', TRUE);
    -- Jordan saves a percent of income instead of a fixed amount.
    INSERT INTO contributions (id, account_id, user_id, label, amount, frequency, annual_increase_pct, amount_type, income_pct, match_tiers, is_active)
    VALUES (gen_random_uuid(), a8, uid, 'Roth IRA, 5% of pay', 0, 'monthly', 0, 'percent', 0.05, '[]', TRUE);
    UPDATE household_members SET salary_growth_pct = 0.03 WHERE user_id = uid;

    -- The 401(k) is linked through a Fidelity Plaid item, so this year's
    -- contributions appear as synced from Plaid (external_id set, described as
    -- the institution would), on the 15th of each month. They match the plan,
    -- so the account reads as on plan. The match is 4% of the $114K salary:
    -- $380 a month.
    INSERT INTO contribution_entries (account_id, user_id, entry_date, amount, source, description, external_id)
    SELECT a3, uid, d::date, amt, src, descr, 'demo-' || src || '-' || d::date
    FROM generate_series(date_trunc('year', NOW()) + INTERVAL '14 days', NOW(), INTERVAL '1 month') AS d,
         (VALUES (1250, 'employee', 'EMPLOYEE DEFERRAL'), (380, 'employer', 'EMPLOYER MATCH')) AS e(amt, src, descr);

    -- ── Additional goal types — demos account / account_type / asset_class goals ──
    INSERT INTO goals (id, user_id, name, target_amount, target_date, notes, goal_type, account_id, account_type_filter, asset_class_filter)
    VALUES
      (gen_random_uuid(), uid, 'Grow Brokerage to $75K', 75000,  '2027-01-01', 'Boost taxable investing',        'account',      a5,   NULL,      NULL),
      (gen_random_uuid(), uid, 'Cash Cushion',           50000,  '2026-12-01', 'Keep 6 months expenses liquid',  'account_type', NULL, 'savings', NULL),
      (gen_random_uuid(), uid, 'Equity Exposure',        250000, '2030-01-01', 'Grow public equity exposure',    'asset_class',  NULL, NULL,      'equity'),
      (gen_random_uuid(), uid, 'Kitchen Remodel',        25000,  '2028-06-01', 'Paid from savings; the forecast deducts it that year', 'spend', a2, NULL, NULL);

    -- ── Plaid integration showcase (no real Plaid connection) ───────────────
    -- Demonstrates what a linked account looks like without ever calling Plaid:
    --   • Vanguard item ('active'): the brokerage (a5) becomes a synced account —
    --     "Plaid · synced" badge on the detail page, is_manual=FALSE, and ~24
    --     months of weekly source='plaid' snapshots as the backfill would write.
    --   • Fidelity item ('active'): the 401(k) (a3), whose contributions this
    --     year appear as synced from Plaid (seeded above).
    --   • Chase item ('login_required'): checking (a1) + credit card (a6) linked
    --     balance-only; shows the "reconnect needed" badge and Reconnect button.
    --
    -- The Connect Institution / Sync Now buttons render only when
    -- PLAID_CLIENT_ID is set. Any placeholder value (plus MYFI_ENCRYPTION_KEY)
    -- works for a UI walkthrough — but note the daily scheduler will then try to
    -- sync the fake 'active' item and mark it 'error' (the token below is not a
    -- real encrypted token), which itself demos error surfacing. The account
    -- detail badge and plaid snapshot history display with no Plaid config at all.
    INSERT INTO plaid_items (user_id, item_id, access_token_encrypted, institution_id, institution_name, status, error_code, last_synced_at, backfill_requested, backfilled_at)
    VALUES
      (uid, 'demo-item-vanguard', 'REVOKED-DEMO-TOKEN-NOT-DECRYPTABLE', 'ins_demo_vg', 'Vanguard', 'active',         NULL,                  NOW(),                    TRUE,  NOW()),
      (uid, 'demo-item-fidelity', 'REVOKED-DEMO-TOKEN-NOT-DECRYPTABLE', 'ins_demo_fi', 'Fidelity', 'active',         NULL,                  NOW(),                    FALSE, NULL),
      (uid, 'demo-item-chase',    'REVOKED-DEMO-TOKEN-NOT-DECRYPTABLE', 'ins_demo_ch', 'Chase',    'login_required', 'ITEM_LOGIN_REQUIRED', NOW() - INTERVAL '3 days', FALSE, NULL);

    UPDATE accounts SET external_provider = 'plaid', external_account_id = 'demo-plaid-brokerage',
                        external_item_id = 'demo-item-vanguard', is_manual = FALSE, last_synced_at = NOW()
    WHERE id = a5;
    UPDATE accounts SET external_provider = 'plaid', external_account_id = 'demo-plaid-401k',
                        external_item_id = 'demo-item-fidelity', is_manual = FALSE, last_synced_at = NOW()
    WHERE id = a3;
    UPDATE accounts SET external_provider = 'plaid', external_account_id = 'demo-plaid-checking',
                        external_item_id = 'demo-item-chase', is_manual = FALSE, last_synced_at = NOW() - INTERVAL '3 days'
    WHERE id = a1;
    UPDATE accounts SET external_provider = 'plaid', external_account_id = 'demo-plaid-visa',
                        external_item_id = 'demo-item-chase', is_manual = FALSE, last_synced_at = NOW() - INTERVAL '3 days'
    WHERE id = a6;

    -- Backfilled weekly history for the brokerage: one snapshot per Monday for
    -- ~24 months, trending ~$33.9K → ~$54.6K with mild noise. Mirrors the real
    -- backfill: source='plaid', noted, and never overwriting manual rows
    -- (ON CONFLICT DO NOTHING keeps the monthly manual snapshots seeded above).
    wk := (DATE_TRUNC('week', CURRENT_DATE) - INTERVAL '104 weeks')::DATE;
    i := 0;
    WHILE wk <= CURRENT_DATE LOOP
      INSERT INTO balance_snapshots (id, account_id, user_id, snapshot_date, balance, notes, source)
      VALUES (gen_random_uuid(), a5, uid, wk,
              ROUND((33900 + 199 * i + 450 * SIN(i / 3.5))::NUMERIC, 2),
              'backfilled from Plaid', 'plaid')
      ON CONFLICT (account_id, snapshot_date) DO NOTHING;
      wk := wk + INTERVAL '7 days';
      i  := i + 1;
    END LOOP;

    -- Recent daily syncs (last 10 days) across all three linked accounts —
    -- what the daily scheduler writes. Credit card balance stored negative.
    FOR i IN 0..9 LOOP
      dt := (CURRENT_DATE - (9 - i))::DATE;
      INSERT INTO balance_snapshots (id, account_id, user_id, snapshot_date, balance, source)
      VALUES
        (gen_random_uuid(), a5, uid, dt, ROUND((54400 + 40 * i + 120 * SIN(i / 1.5))::NUMERIC, 2), 'plaid'),
        (gen_random_uuid(), a1, uid, dt, ROUND((12300 + 15 * i)::NUMERIC, 2),                      'plaid'),
        (gen_random_uuid(), a6, uid, dt, ROUND((-3360 - 4 * i)::NUMERIC, 2),                       'plaid')
      ON CONFLICT (account_id, snapshot_date) DO NOTHING;
    END LOOP;

    -- A security without a public ticker, as Plaid reports for money-market /
    -- private funds: identified by CUSIP, manually priced at the institution
    -- price, with a dollar amount in shares (cash_equivalent convention).
    INSERT INTO holdings (id, account_id, user_id, ticker, name, asset_class, shares, cost_basis, manual_price, price_per_share, price_fetched_at)
    VALUES (gen_random_uuid(), a5, uid, '92202E102', 'Vanguard Cash Reserves Federal Money Market', 'cash_equivalent', 2400.00, NULL, TRUE, 1.00, NOW());

    RAISE NOTICE '✓ Demo user seeded. Login: demo@myfi.local / demo1234';
  END IF;
END;
$$;
