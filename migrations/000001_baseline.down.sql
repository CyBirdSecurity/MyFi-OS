DROP TABLE IF EXISTS
    accounts,
    allocation_targets,
    balance_snapshots,
    budget_categories,
    budget_limits,
    contribution_entries,
    contributions,
    credit_card_details,
    dismissed_subscriptions,
    goals,
    holding_dividends,
    holdings,
    home_details,
    household_members,
    import_logs,
    loan_details,
    plaid_items,
    recurring_transactions,
    transactions,
    user_profiles,
    user_return_rates,
    users,
    vehicle_details,
    vendor_rules
CASCADE;

DROP FUNCTION IF EXISTS set_updated_at();
