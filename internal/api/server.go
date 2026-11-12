package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmoiron/sqlx"
	"golang.org/x/time/rate"

	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/services"
	ofxsvc "github.com/cybirdsecurity/myfi-os/internal/services/ofx"
	plaidsvc "github.com/cybirdsecurity/myfi-os/internal/services/plaid"
)

type Config struct {
	JWTSecret     string
	Env           string
	SecureCookies bool
}

type Server struct {
	cfg                 Config
	router              *chi.Mux
	db                  *sqlx.DB
	autoCategorize      interface{ Notify() }
	startedAt           time.Time
	users               *db.UserStore
	accounts            *db.AccountStore
	snapshots           *db.SnapshotStore
	holdings            *db.HoldingStore
	goals               *db.GoalStore
	profiles            *db.ProfileStore
	contributions       *db.ContributionStore
	contributionEntries *db.ContributionEntryStore
	returnRates         *db.ReturnRateStore
	householdMembers    *db.HouseholdMembersStore
	home                *db.HomeStore
	forecastCache       *forecastCache
	vehicle             *db.VehicleStore
	creditCards         *db.CreditCardStore
	loans               *db.LoanStore
	allocationTargets   *db.AllocationTargetStore
	networth            *services.NetWorthService
	concentration       *services.ConcentrationService
	importer            *services.ImporterService
	ofxImporter         *ofxsvc.Service
	forecast            *services.ForecastService
	priceService        *services.PriceService
	autosnap            *services.AutoSnapshotService
	dividends           *db.DividendStore
	autoDividend        *services.AutoDividendService
	budgetCategories    *db.BudgetCategoryStore
	budgetLimits        *db.BudgetLimitStore
	dismissedSubs       *db.DismissedSubscriptionStore
	transactions        *db.TransactionStore
	importLogs          *db.ImportLogStore
	csvImport           *services.CSVImportService
	costBasisImport     *services.CostBasisImportService
	dataPort            *services.DataPortService
	taxService          *services.TaxService
	// Plaid integration — nil when MYFI_ENCRYPTION_KEY is not configured.
	plaid          *plaidsvc.Service
	plaidScheduler *plaidsvc.Scheduler
	plaidItems     *db.PlaidItemStore
}

// SetAutoCategorizer registers the background job that suggests categories
// for uncategorized imports; import handlers nudge it after saving.
func (s *Server) SetAutoCategorizer(a interface{ Notify() }) { s.autoCategorize = a }

// notifyAutoCategorize wakes the background categorizer, if one is set.
func (s *Server) notifyAutoCategorize() {
	if s.autoCategorize != nil {
		s.autoCategorize.Notify()
	}
}

func NewServer(
	cfg Config,
	sqlxDB *sqlx.DB,
	startedAt time.Time,
	users *db.UserStore,
	accounts *db.AccountStore,
	snapshots *db.SnapshotStore,
	holdings *db.HoldingStore,
	goals *db.GoalStore,
	profiles *db.ProfileStore,
	contributions *db.ContributionStore,
	returnRates *db.ReturnRateStore,
	householdMembers *db.HouseholdMembersStore,
	home *db.HomeStore,
	vehicle *db.VehicleStore,
	creditCards *db.CreditCardStore,
	allocationTargets *db.AllocationTargetStore,
	networth *services.NetWorthService,
	concentration *services.ConcentrationService,
	importer *services.ImporterService,
	ofxImporter *ofxsvc.Service,
	forecast *services.ForecastService,
	priceService *services.PriceService,
	autosnap *services.AutoSnapshotService,
	dividends *db.DividendStore,
	autoDividend *services.AutoDividendService,
	budgetCategories *db.BudgetCategoryStore,
	transactions *db.TransactionStore,
	importLogs *db.ImportLogStore,
	csvImport *services.CSVImportService,
	costBasisImport *services.CostBasisImportService,
	dataPort *services.DataPortService,
	taxService *services.TaxService,
	plaid *plaidsvc.Service,
	plaidScheduler *plaidsvc.Scheduler,
	plaidItems *db.PlaidItemStore,
) *Server {
	s := &Server{
		cfg:                 cfg,
		db:                  sqlxDB,
		startedAt:           startedAt,
		users:               users,
		accounts:            accounts,
		snapshots:           snapshots,
		holdings:            holdings,
		goals:               goals,
		profiles:            profiles,
		contributions:       contributions,
		returnRates:         returnRates,
		householdMembers:    householdMembers,
		home:                home,
		forecastCache:       newForecastCache(),
		vehicle:             vehicle,
		creditCards:         creditCards,
		loans:               db.NewLoanStore(sqlxDB),
		allocationTargets:   allocationTargets,
		networth:            networth,
		concentration:       concentration,
		importer:            importer,
		ofxImporter:         ofxImporter,
		forecast:            forecast,
		priceService:        priceService,
		autosnap:            autosnap,
		dividends:           dividends,
		autoDividend:        autoDividend,
		budgetLimits:        db.NewBudgetLimitStore(sqlxDB),
		contributionEntries: db.NewContributionEntryStore(sqlxDB),
		dismissedSubs:       db.NewDismissedSubscriptionStore(sqlxDB),
		budgetCategories:    budgetCategories,
		transactions:        transactions,
		importLogs:          importLogs,
		csvImport:           csvImport,
		costBasisImport:     costBasisImport,
		dataPort:            dataPort,
		taxService:          taxService,
		plaid:               plaid,
		plaidScheduler:      plaidScheduler,
		plaidItems:          plaidItems,
	}
	s.router = s.buildRouter()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *Server) buildRouter() *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public health-check endpoint (no auth required).
	r.Get("/health", s.handleHealth)

	// Auth endpoints: burst of 10 then 1 req / 2 s per IP.
	authLimiter := rateLimitMiddleware(rate.Every(2), 10)

	r.Route("/api/v1", func(r chi.Router) {
		// Public routes.
		r.Get("/config", s.handleConfig)
		r.Post("/auth/logout", s.handleLogout)

		// Rate-limited auth routes.
		r.Group(func(r chi.Router) {
			r.Use(authLimiter)
			r.Post("/auth/register", s.handleRegister)
			r.Post("/auth/login", s.handleLogin)
		})

		// Protected routes.
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Use(s.invalidateForecastOnWrite)

			r.Get("/auth/me", s.handleMe)
			r.With(rateLimitMiddleware(rate.Every(10*time.Second), 5)).Post("/auth/password/change", s.handlePasswordChange)
			// Each report sends an email: 3 quickly, then one every 30 s per IP.
			r.Post("/auth/onboarded", s.handleMarkOnboarded)
			r.Post("/auth/guides/{guide}/dismiss", s.handleDismissGuide)

			// Accounts.
			r.Get("/accounts", s.handleListAccounts)
			r.Post("/accounts", s.handleCreateAccount)
			r.Get("/accounts/{id}", s.handleGetAccount)
			r.Put("/accounts/{id}", s.handleUpdateAccount)
			r.Delete("/accounts/{id}", s.handleDeleteAccount)

			// Account return rate override + effective rate.
			r.Put("/accounts/{id}/return-rate", s.handleSetAccountReturnRate)
			r.Get("/accounts/{id}/effective-rate", s.handleGetAccountEffectiveRate)

			// Snapshots (nested under account).
			r.Get("/accounts/{id}/snapshots", s.handleListSnapshots)
			r.Post("/accounts/{id}/snapshots", s.handleCreateSnapshot)
			r.Put("/accounts/{id}/snapshots/{snapId}", s.handleUpdateSnapshot)
			r.Delete("/accounts/{id}/snapshots/{snapId}", s.handleDeleteSnapshot)
			r.Post("/snapshots/trigger", s.handleTriggerUserSnapshot)

			// Holdings (nested under account + cross-account).
			r.Get("/accounts/{id}/holdings", s.handleListHoldingsByAccount)
			r.Post("/accounts/{id}/holdings", s.handleCreateHolding)
			r.Put("/accounts/{id}/holdings/{hId}", s.handleUpdateHolding)
			r.Delete("/accounts/{id}/holdings/{hId}", s.handleDeleteHolding)
			r.Get("/holdings", s.handleListAllHoldings)
			r.Get("/holdings/concentration", s.handleGetConcentration)
			r.Post("/holdings/refresh-prices", s.handleRefreshPrices)
			r.Put("/holdings/cost-basis", s.handleBulkUpdateCostBasis)
			// Dividend (DRIP) history + manual trigger.
			r.Get("/accounts/{id}/holdings/{hId}/dividends", s.handleListHoldingDividends)
			r.Post("/dividends/trigger", s.handleTriggerDividends)
			// Ticker detail (registered before generic {hId} routes).
			r.Get("/holdings/ticker/{ticker}", s.handleGetTickerDetail)
			r.Get("/holdings/ticker/{ticker}/history", s.handleGetTickerHistory)

			// All active contributions for the current user (used by forecast page).
			r.Get("/contributions", s.handleListAllContributions)

			// Contributions (nested under account).
			r.Get("/accounts/{id}/contributions", s.handleListContributions)
			r.Post("/accounts/{id}/contributions", s.handleCreateContribution)
			r.Put("/accounts/{id}/contributions/{cId}", s.handleUpdateContribution)
			r.Delete("/accounts/{id}/contributions/{cId}", s.handleDeleteContribution)

			// Actual contributions this year, and how they compare to the plan.
			r.Get("/contribution-progress", s.handleContributionProgress)
			r.Get("/accounts/{id}/contribution-entries", s.handleListContributionEntries)
			r.Post("/accounts/{id}/contribution-entries", s.handleCreateContributionEntry)
			r.Put("/accounts/{id}/contribution-entries/{eId}", s.handleUpdateContributionEntry)
			r.Delete("/accounts/{id}/contribution-entries/{eId}", s.handleDeleteContributionEntry)

			// Home details + home value refresh + amortization schedule.
			r.Get("/accounts/{id}/home-details", s.handleGetHomeDetails)
			r.Put("/accounts/{id}/home-details", s.handleUpsertHomeDetails)
			r.Get("/accounts/{id}/amortization", s.handleGetAmortization)

			// Vehicle details.
			r.Get("/accounts/{id}/vehicle-details", s.handleGetVehicleDetails)
			r.Put("/accounts/{id}/vehicle-details", s.handleUpsertVehicleDetails)

			// Credit card details (Plaid-synced or manual entry).
			r.Get("/accounts/{id}/credit-card-details", s.handleGetCreditCardDetails)
			r.Put("/accounts/{id}/credit-card-details", s.handleUpsertCreditCardDetails)
			r.Get("/accounts/{id}/loan-details", s.handleGetLoanDetails)
			r.Put("/accounts/{id}/loan-details", s.handleUpsertLoanDetails)

			// Plaid sync diagnostics for one account.
			r.Get("/accounts/{id}/plaid-sync-info", s.handlePlaidAccountSyncInfo)

			// Net worth.
			r.Get("/networth/current", s.handleNetWorthCurrent)
			r.Get("/networth/history", s.handleNetWorthHistory)
			r.Get("/networth/history/accounts", s.handleNetWorthAccountHistory)

			// Goals.
			r.Get("/goals", s.handleListGoals)
			r.Post("/goals", s.handleCreateGoal)
			r.Put("/goals/{id}", s.handleUpdateGoal)
			r.Delete("/goals/{id}", s.handleDeleteGoal)

			// User profile & settings.
			r.Get("/profile", s.handleGetProfile)
			r.Put("/profile", s.handleUpsertProfile)

			// Allocation targets.
			r.Get("/allocation-targets", s.handleGetAllocationTargets)
			r.Put("/allocation-targets", s.handleUpsertAllocationTargets)

			// Return rate customization.
			r.Get("/return-rates", s.handleListReturnRates)
			r.Put("/return-rates/{assetClass}", s.handleUpsertReturnRate)
			r.Delete("/return-rates/{assetClass}", s.handleDeleteReturnRate)

			// Household members.
			r.Get("/household/members", s.handleListHouseholdMembers)
			r.Post("/household/members", s.handleUpsertHouseholdMember)
			r.Put("/household/members/{id}", s.handleUpdateHouseholdMember)
			r.Delete("/household/members/{id}", s.handleDeleteHouseholdMember)

			// Portfolio forecast.
			r.Get("/forecast", s.handleGetForecast)
			r.Post("/forecast/scenarios", s.handleRunScenarios)
			r.Post("/forecast/solve", s.handleSolveScenario)
			r.Get("/forecast/backtest", s.handleGetHistoricalBacktest)

			// Retirement planner.
			r.Get("/retirement", s.handleGetRetirementPlan)

			// Import.
			r.Get("/import/history", s.handleImportHistory)
			r.Post("/import/csv", s.handleImportCSV)
			r.Post("/import/ofx/preview", s.handleOFXPreview)
			r.Post("/import/ofx/apply", s.handleOFXApply)
			r.Post("/import/cost-basis/preview", s.handleCostBasisImportPreview)
			r.Post("/import/cost-basis/apply", s.handleCostBasisImportApply)
			r.Post("/import/full", s.handleImportFull)

			// Plaid integration.
			r.Post("/plaid/link-token", s.handlePlaidLinkToken)
			r.Post("/plaid/exchange", s.handlePlaidExchange)
			r.Get("/plaid/items", s.handlePlaidListItems)
			r.Post("/plaid/items/{itemId}/mappings", s.handlePlaidApplyMappings)
			r.Post("/plaid/items/{itemId}/sync", s.handlePlaidSyncItem)
			r.Delete("/plaid/items/{itemId}", s.handlePlaidRemoveItem)
			r.Post("/plaid/sync", s.handlePlaidSyncAll)

			// Export.
			r.Get("/export", s.handleExportJSON)

			// Tax analysis.
			r.Get("/taxes", s.handleTaxAnalysis)

			// Budget.
			r.Get("/budget/categories", s.handleListBudgetCategories)
			r.Post("/budget/categories", s.handleCreateBudgetCategory)
			r.Get("/budget/limits", s.handleListBudgetLimits)
			r.Put("/budget/limits/{categoryId}", s.handleSetBudgetLimit)
			r.Delete("/budget/limits/{categoryId}", s.handleDeleteBudgetLimit)
			r.Post("/budget/import/preview", s.handleBudgetImportPreview)
			r.Post("/budget/import/confirm", s.handleBudgetImportConfirm)
			r.Post("/budget/import/quick", s.handleBudgetImportQuick)
			r.Get("/budget/review-count", s.handleBudgetReviewCount)
			r.Get("/budget/summary", s.handleBudgetSummary)
			r.Get("/budget/monthly-breakdown", s.handleBudgetMonthlyBreakdown)
			r.Get("/budget/income-breakdown", s.handleBudgetIncomeBreakdown)
			r.Get("/budget/transactions", s.handleListTransactions)
			r.Post("/budget/transactions", s.handleCreateTransaction)
			r.Post("/budget/transactions/bulk-delete", s.handleBulkDeleteTransactions)
			r.Put("/budget/transactions/{id}", s.handleUpdateTransaction)
			r.Delete("/budget/transactions/{id}", s.handleDeleteTransaction)
			r.Get("/budget/recurring", s.handleListRecurring)
			r.Post("/budget/recurring", s.handleCreateRecurring)
			r.Delete("/budget/recurring/{id}", s.handleDeleteRecurring)
			r.Get("/budget/duplicates", s.handleListDuplicateTransactions)
			r.Get("/budget/subscriptions/dismissed", s.handleListDismissedSubscriptions)
			r.Post("/budget/subscriptions/dismissed", s.handleDismissSubscription)
			r.Post("/budget/subscriptions/restore", s.handleRestoreSubscription)
		})
	})

	return r
}
