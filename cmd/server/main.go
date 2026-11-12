package main

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/joho/godotenv"

	myfi "github.com/cybirdsecurity/myfi-os"
	"github.com/cybirdsecurity/myfi-os/internal/api"
	"github.com/cybirdsecurity/myfi-os/internal/crypto"
	"github.com/cybirdsecurity/myfi-os/internal/db"
	"github.com/cybirdsecurity/myfi-os/internal/prices"
	"github.com/cybirdsecurity/myfi-os/internal/services"
	ofxsvc "github.com/cybirdsecurity/myfi-os/internal/services/ofx"
	plaidsvc "github.com/cybirdsecurity/myfi-os/internal/services/plaid"
)

func main() {
	// Load .env in development; no-op if file absent.
	_ = godotenv.Load()

	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		os.Exit(runResetPassword(os.Args[2:]))
	}

	databaseURL := requireEnv("DATABASE_URL")
	jwtSecret := requireEnv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters (generate one with: openssl rand -hex 32)")
	}
	port := getEnv("PORT", "8080")
	env := getEnv("ENV", "development")
	ollamaURL := getEnv("OLLAMA_URL", "http://localhost:11434")
	ollamaModel := getEnv("OLLAMA_MODEL", "qwen3:0.6b")
	ollamaNumThread, _ := strconv.Atoi(getEnv("OLLAMA_NUM_THREAD", "0"))
	llmProvider := getEnv("LLM_PROVIDER", "ollama")
	anthropicKey := getEnv("ANTHROPIC_API_KEY", "")
	anthropicModel := getEnv("ANTHROPIC_MODEL", "")
	secureCookies := getEnv("SECURE_COOKIES", "false") == "true"
	encryptionKey := getEnv("MYFI_ENCRYPTION_KEY", "")
	plaidClientID := getEnv("PLAID_CLIENT_ID", "")
	plaidSecret := getEnv("PLAID_SECRET", "")

	// Connect to database.
	sqlxDB, err := db.New(databaseURL)
	if err != nil {
		log.Fatalf("connect to db: %v", err)
	}
	defer sqlxDB.Close()

	// Run migrations.
	if err := runMigrations(sqlxDB.DB, databaseURL); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	log.Println("migrations applied successfully")

	// Wire dependencies.
	userStore := db.NewUserStore(sqlxDB)
	accountStore := db.NewAccountStore(sqlxDB)
	snapshotStore := db.NewSnapshotStore(sqlxDB)
	holdingStore := db.NewHoldingStore(sqlxDB)
	goalStore := db.NewGoalStore(sqlxDB)
	profileStore := db.NewProfileStore(sqlxDB)
	contributionStore := db.NewContributionStore(sqlxDB)
	returnRateStore := db.NewReturnRateStore(sqlxDB)
	householdMembersStore := db.NewHouseholdMembersStore(sqlxDB)
	homeStore := db.NewHomeStore(sqlxDB)
	vehicleStore := db.NewVehicleStore(sqlxDB)
	creditCardStore := db.NewCreditCardStore(sqlxDB)
	allocationTargetStore := db.NewAllocationTargetStore(sqlxDB)

	dividendStore := db.NewDividendStore(sqlxDB)

	budgetCategoryStore := db.NewBudgetCategoryStore(sqlxDB)
	transactionStore := db.NewTransactionStore(sqlxDB)
	importLogStore := db.NewImportLogStore(sqlxDB)

	yahooProvider := prices.NewYahooProvider()
	priceService := services.NewPriceService(yahooProvider)

	llm, err := services.NewLLMProvider(llmProvider, ollamaURL, ollamaModel, ollamaNumThread, anthropicKey, anthropicModel)
	if err != nil {
		log.Fatalf("init LLM provider: %v", err)
	}
	log.Printf("LLM provider: %s", llm.ProviderName())

	networthService := services.NewNetWorthService(accountStore, snapshotStore, holdingStore, priceService, homeStore)
	concentrationService := services.NewConcentrationService(networthService, allocationTargetStore)
	importerService := services.NewImporterService(accountStore, snapshotStore)
	ofxImporter := ofxsvc.NewService(accountStore, holdingStore)
	forecastService := services.NewForecastService(
		accountStore, snapshotStore, holdingStore,
		profileStore, contributionStore, returnRateStore, householdMembersStore, homeStore, goalStore,
	)
	forecastService.SetTransactionStore(transactionStore)
	forecastService.SetLiabilityStores(creditCardStore, db.NewLoanStore(sqlxDB))
	autoSnapshotService := services.NewAutoSnapshotService(holdingStore, snapshotStore, priceService)
	autoDividendService := services.NewAutoDividendService(holdingStore, dividendStore, priceService)

	classifierService, err := services.NewClassifierService(llm, budgetCategoryStore, transactionStore)
	if err != nil {
		log.Fatalf("init classifier: %v", err)
	}
	csvImportService := services.NewCSVImportService(classifierService, llm)
	costBasisImportService := services.NewCostBasisImportService(llm, holdingStore)
	dataPortService := services.NewDataPortService(
		accountStore, snapshotStore, holdingStore, contributionStore,
		goalStore, profileStore, allocationTargetStore, returnRateStore,
		homeStore, vehicleStore, householdMembersStore,
	)
	taxService := services.NewTaxService(accountStore, snapshotStore, profileStore, householdMembersStore)

	// Plaid is optional: it is on only when PLAID_CLIENT_ID, PLAID_SECRET and
	// MYFI_ENCRYPTION_KEY are all set, and the feature is hidden otherwise. The
	// encryption key protects every Plaid access token at rest.
	var plaidService *plaidsvc.Service
	var plaidScheduler *plaidsvc.Scheduler
	plaidItemStore := db.NewPlaidItemStore(sqlxDB)
	plaidVarsSet := 0
	for _, v := range []string{plaidClientID, plaidSecret, encryptionKey} {
		if v != "" {
			plaidVarsSet++
		}
	}
	if plaidVarsSet != 0 && plaidVarsSet != 3 {
		log.Fatal("Plaid needs PLAID_CLIENT_ID, PLAID_SECRET and MYFI_ENCRYPTION_KEY all set (or none of them to turn Plaid off)")
	}
	if plaidVarsSet == 3 {
		cipher, err := crypto.NewCipher(encryptionKey)
		if err != nil {
			log.Fatalf("invalid MYFI_ENCRYPTION_KEY: %v", err)
		}
		plaidAPI, err := plaidsvc.NewClient(plaidClientID, plaidSecret, "production")
		if err != nil {
			log.Fatalf("init Plaid client: %v", err)
		}
		plaidService = plaidsvc.NewService(
			plaidAPI, cipher, plaidItemStore,
			accountStore, holdingStore, snapshotStore,
			homeStore, creditCardStore,
			yahooProvider,
		)
		// Enable banking transaction sync into the budget: reuses the same
		// classifier as CSV import (rules + merchant history, no per-sync LLM).
		plaidService.SetTransactionSync(transactionStore, classifierService)
		// Record retirement contributions from investment transactions, to
		// compare with the contribution plan.
		plaidService.SetContributionSync(db.NewContributionEntryStore(sqlxDB))
		plaidScheduler = plaidsvc.NewScheduler(plaidService)
		log.Println("Plaid integration enabled")
	}

	// Start the weekly auto-snapshot scheduler in the background.
	// It shuts down cleanly when the process receives SIGINT or SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if plaidScheduler == nil {
		go autoSnapshotService.Run(ctx)
	} else {
		// With Plaid on, the weekly snapshot runs right after the daily Plaid
		// sync so it uses fresh holdings instead of racing it.
		plaidScheduler.SetAfterSync(autoSnapshotService.RunIfDue)
	}
	go autoDividendService.Run(ctx)
	autoCategorizeService := services.NewAutoCategorizeService(transactionStore, classifierService)
	go autoCategorizeService.Run(ctx)
	go services.NewRecurringTransactionService(transactionStore).Run(ctx)
	if plaidScheduler != nil {
		go plaidScheduler.Run(ctx)
	}

	startedAt := time.Now()

	apiServer := api.NewServer(
		api.Config{JWTSecret: jwtSecret, Env: env, SecureCookies: secureCookies},
		sqlxDB,
		startedAt,
		userStore,
		accountStore,
		snapshotStore,
		holdingStore,
		goalStore,
		profileStore,
		contributionStore,
		returnRateStore,
		householdMembersStore,
		homeStore,
		vehicleStore,
		creditCardStore,
		allocationTargetStore,
		networthService,
		concentrationService,
		importerService,
		ofxImporter,
		forecastService,
		priceService,
		autoSnapshotService,
		dividendStore,
		autoDividendService,
		budgetCategoryStore,
		transactionStore,
		importLogStore,
		csvImportService,
		costBasisImportService,
		dataPortService,
		taxService,
		plaidService,
		plaidScheduler,
		plaidItemStore,
	)

	apiServer.SetAutoCategorizer(autoCategorizeService)

	mux := http.NewServeMux()
	mux.Handle("/api/", apiServer)
	mux.Handle("/health", apiServer) // expose health at root path too

	// Serve the embedded React SPA in non-development environments.
	if env != "development" {
		distFS, err := fs.Sub(myfi.WebFS, "web/dist")
		if err != nil {
			log.Fatalf("embed web/dist: %v", err)
		}
		fileServer := http.FileServer(http.FS(distFS))
		mux.HandleFunc("/", spaHandler(distFS, fileServer))
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("MyFi listening on %s (env=%s)", addr, env)
	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		log.Println("shutting down server")
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}

// spaHandler serves static files and falls back to index.html for unmatched paths.
//
// Files under /assets/ have a content hash in their name, so browsers may keep
// them for a year. index.html must be revalidated so a deploy is picked up.
func spaHandler(distFS fs.FS, fileServer http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" || path == "" {
			w.Header().Set("Cache-Control", "no-cache")
			fileServer.ServeHTTP(w, r)
			return
		}
		_, err := fs.Stat(distFS, path[1:])
		switch {
		case strings.HasPrefix(path, "/assets/") && err == nil:
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasPrefix(path, "/assets/"):
			// A chunk from an older build: a 404 (not index.html) lets the
			// page notice and reload instead of running HTML as a script.
			http.NotFound(w, r)
			return
		case os.IsNotExist(err):
			// Unknown path — let the React router handle it.
			r.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	}
}

func runMigrations(sqlDB *sql.DB, _ string) error {
	src, err := iofs.New(myfi.MigrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
