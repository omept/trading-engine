package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/omept/trading-engine/pkg/engine"
	"github.com/omept/trading-engine/pkg/exchange"
	"github.com/omept/trading-engine/pkg/store"
	"github.com/omept/trading-engine/pkg/strategy"

	"github.com/joho/godotenv"
)

func main() {
	log.Println("Starting trading engine ...")

	if err := godotenv.Load(); err != nil {
		log.Println(".env not found or could not be loaded - continuing with environment variables")
	}

	// Initialize persistence (SQLite)
	sqlLiteFD := os.Getenv("SQLITE_FILE_DIR")
	if sqlLiteFD == "" {
		sqlLiteFD = "engine.db"
	}
	db, err := store.NewSQLiteStore(sqlLiteFD)
	if err != nil {
		log.Fatal("sqlite init:", err)
	}
	defer db.Close()

	// Read config
	exchangeName := os.Getenv("EXCHANGE") // MOCK | BINANCE | ALPACA
	if exchangeName == "" {
		exchangeName = "MOCK"
	}

	// Risk manager
	fdprS := os.Getenv("FIXED_DECIMAL_PERCENT_RISK")
	fdpr, err := strconv.ParseFloat(fdprS, 64)
	if err != nil || fdpr <= 0 {
		fdpr = 0.005 // defaults to  0.5%
	}

	// Strategy names and account balance
	emacSymbol := os.Getenv("EMAC_CROSSOVER_STRATEGY")
	if emacSymbol == "" {
		emacSymbol = "BTCUSD"
	}
	mrSymbol := os.Getenv("MEAN_REVERSION_CROSSOVER_STRATEGY")
	if mrSymbol == "" {
		mrSymbol = "BTCUSD"
	}

	usdBalS := os.Getenv("ACCOUNT_USD_BAL")
	usdBal, err := strconv.ParseFloat(usdBalS, 64)
	if err != nil || usdBal <= 0 {
		usdBal = 300 // defaults to 300
	}

	// Create exchange adapter based on env
	exch := initExhangeAdapter(exchangeName, db)

	// Order manager
	om := engine.NewOrderManager(exch, db)

	// Risk manager
	risk := engine.NewFixedPercentRisk(fdpr)

	// Strategies
	ema := strategy.NewEMACrossover(emacSymbol, 9, 21, om, risk)
	ema.SetAccountUSD(usdBal)
	mr := strategy.NewMeanReversion(mrSymbol, 20, 2.0, om, risk)
	mr.SetAccountUSD(usdBal)

	// Engine
	eng := engine.NewEngine()
	eng.RegisterStrategy(ema)
	eng.RegisterStrategy(mr)
	eng.SetExchangeAdapter(exch)
	eng.SetOrderManager(om)
	eng.SetStore(db)

	// HTTP control server and minimal UI
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8080"
	}

	mux := setUpAPIs(eng, db)
	srv := &http.Server{Addr: httpAddr, Handler: mux}

	// Start HTTP server
	go func() {
		log.Println("HTTP control server listening on", httpAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// ------------------------------------------------------
	// BACKTEST MODE
	// ------------------------------------------------------
	backtestMode := os.Getenv("BACKTEST") == "1"
	selectedStrategy := os.Getenv("STRATEGY") // ema | mean | all
	if selectedStrategy == "" {
		selectedStrategy = "all"
	}

	if backtestMode {
		log.Println("BACKTEST MODE ENABLED")

		sym := os.Getenv("BACKTEST_SYMBOL")
		if sym == "" {
			sym = "BTCUSD"
		}

	}

	// Start engine automatically
	ctx, cancel := context.WithCancel(context.Background())
	go eng.Start(ctx)

	// Wait for interrupt
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")

	// Shutdown HTTP server
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Println("HTTP server Shutdown:", err)
	}

	// Stop engine
	cancel()
	eng.Stop()

	log.Println("done")
}

func initExhangeAdapter(exchangeName string, db *store.SQLiteStore) engine.ExchangeAdapter {
	var exch engine.ExchangeAdapter
	var err error
	switch exchangeName {
	case "BINANCE":
		log.Println("Using Binance adapter (REST)")
		binKey := os.Getenv("BINANCE_API_KEY")
		binSecret := os.Getenv("BINANCE_API_SECRET")
		if binKey == "" || binSecret == "" {
			log.Fatal("BINANCE_API_KEY and BINANCE_API_SECRET must be set for BINANCE exchange")
		}
		// exchange.NewBinanceAdapter should be implemented in your exchange package.
		exch, err = exchange.NewBinanceAdapter(binKey, binSecret, db)
		if err != nil {
			log.Fatal("failed to init binance adapter:", err)
		}

	case "ALPACA":
		log.Println("Using Alpaca adapter (REST)")
		alpKey := os.Getenv("ALPACA_API_KEY")
		alpSecret := os.Getenv("ALPACA_API_SECRET")
		alpBase := os.Getenv("ALPACA_BASE_URL")
		if alpBase == "" {
			alpBase = "https://paper-api.alpaca.markets"
		}
		if alpKey == "" || alpSecret == "" {
			log.Fatal("ALPACA_API_KEY and ALPACA_API_SECRET must be set for ALPACA exchange")
		}
		exch, err = exchange.NewAlpacaAdapter(alpKey, alpSecret, alpBase, db)
		if err != nil {
			log.Fatal("failed to init alpaca adapter:", err)
		}

	default:
		log.Println("Using Mock exchange (default)")
		// Mock exchange config (defaults)
		mockExchangeUSDBalS := os.Getenv("MOCK_EXCHANGE_USD_BAL")
		mockExchangeUSDBal, err := strconv.ParseFloat(mockExchangeUSDBalS, 64)
		if err != nil || mockExchangeUSDBal <= 0 {
			mockExchangeUSDBal = 100000 // defaults to 100000
		}
		exch = exchange.NewMockExchange(mockExchangeUSDBal, db)
	}

	return exch
}
