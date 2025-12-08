package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"trading-engine/pkg/backtest"
	"trading-engine/pkg/engine"
	"trading-engine/pkg/exchange"
	"trading-engine/pkg/store"
	"trading-engine/pkg/strategy"

	"github.com/joho/godotenv"
)

// Updated main that selects exchange by ENV (default: mock), wires SQLite store,
// exposes a minimal HTTP UI + REST endpoints to start/stop strategies and show metrics

func main() {
	log.Println("Starting trading engine ...")

	if err := godotenv.Load(); err != nil {
		log.Println(".env not found or could not be loaded - continuing with environment variables")
	}

	// Initialize persistence (SQLite)
	sqlLiteFD := os.Getenv("SQLITE_FILE_DIR")
	if sqlLiteFD == "" {
		sqlLiteFD = "./data/trading.db"
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

		startS := os.Getenv("BACKTEST_START")
		endS := os.Getenv("BACKTEST_END")

		if startS == "" || endS == "" {
			log.Fatal("BACKTEST_START and BACKTEST_END must be set when BACKTEST=1")
		}

		runBacktest(selectedStrategy, sym, eng, db)
		return
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

// minimalUI returns a tiny HTML+JS UI that calls start/stop/status endpoints
func minimalUI() string {
	return `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Trading Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        body {
            font-family: system-ui, Arial;
            margin: 24px
        }

        button {
            padding: 8px 12px;
            margin: 6px
        }

        canvas {
            max-width: 100%;
            height: 300px;
        }
    </style>
</head>

<body>
    <h2>Trading Engine Dashboard</h2>
    <div>
        <button onclick="start()">Start</button>
        <button onclick="stop()">Stop</button>
        <button onclick="refreshMetrics()">Refresh Metrics</button>
    </div>
    <div>
        Orders: <span id="orders">0</span> | Trades: <span id="trades">0</span> | Runs: <span id="runs">0</span>
    </div>
    <canvas id="chart"></canvas>
    <script>
        let chart;
        async function fetchCandles() {
            const res = await fetch('/api/candles?symbol=BTCUSD&limit=100');
            return await res.json();
        }
        async function fetchMetrics() {
            const res = await fetch('/api/metrics');
            return await res.json();
        }
        async function renderChart() {
            const candles = await fetchCandles();
            const labels = candles.map(c => c.time);
            const data = {
                labels: labels,
                datasets: [
                    { label: 'Close', data: candles.map(c => c.close), borderColor: 'blue', backgroundColor: 'rgba(0,0,255,0.2)' },
                    { label: 'Open', data: candles.map(c => c.open), borderColor: 'green', backgroundColor: 'rgba(0,255,0,0.2)' }
                ]
            };
            if (chart) { chart.data = data; chart.update(); }
            else {
                const ctx = document.getElementById('chart').getContext('2d');
                chart = new Chart(ctx, { type: 'line', data: data });
            }
        }
        async function refreshMetrics() {
            const m = await fetchMetrics();
            document.getElementById('orders').innerText = m.orders;
            document.getElementById('trades').innerText = m.trades;
            document.getElementById('runs').innerText = m.runs;
        }
        async function start() { await fetch('/api/start', { method: 'POST' }); }
        async function stop() { await fetch('/api/stop', { method: 'POST' }); }
        setInterval(() => { renderChart(); refreshMetrics(); }, 3000);
        window.onload = () => { renderChart(); refreshMetrics(); };
    </script>
</body>

</html>`
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

func setUpAPIs(eng *engine.Engine, db *store.SQLiteStore) *http.ServeMux {

	mux := http.NewServeMux()

	// REST endpoints
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		ctx := r.Context()
		go eng.Start(ctx)
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte("started"))
	})

	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		eng.Stop()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("stopped"))
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		st := eng.Status()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	})

	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		// simple metrics from store: counts of orders/trades/runs
		metrics := map[string]int64{}
		orders, _ := db.CountOrders()
		trades, _ := db.CountTrades()
		runs, _ := db.CountRuns()
		metrics["orders"] = orders
		metrics["trades"] = trades
		metrics["runs"] = runs
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics)
	})

	mux.HandleFunc("/api/candles", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			symbol = "BTCUSD"
		}
		limit := 100
		candles, err := db.LoadCandles(symbol, limit)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(candles)
	})

	// Minimal web UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(minimalUI()))
	})

	return mux
}

func runBacktest(which, symbol string, eng *engine.Engine, db *store.SQLiteStore) []byte {
	log.Println("Running backtest:", which, symbol)

	// Load candles directly from SQLite
	data, err := db.LoadCandles(symbol, 300)
	if err != nil {
		log.Fatal("LoadCandlesBetween:", err)
	}
	if len(data) == 0 {
		log.Fatal("no candles found for backtest")
	}

	// Select strategy based on env
	var strats []engine.Strategy

	for _, s := range eng.Strategies() {
		name := s.Name()

		if which == "ema" && name == strategy.ST_NAME_EMA {
			strats = append(strats, s)
		}
		if which == "mean" && name == strategy.ST_NAME_MEAN {
			strats = append(strats, s)
		}
		if which == "all" {
			strats = append(strats, s)
		}
	}

	if len(strats) == 0 {
		log.Fatal("no strategy selected for backtest")
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		log.Fatal(err)
	}
	var candles []engine.Candle
	if err := json.Unmarshal(bytes, &candles); err != nil {
		log.Fatal(err)
	}

	bt := backtest.NewBacktester(candles, eng, db)
	stats, _ := bt.Run("BTCUSDT")

	// convert to JSON
	jsonBytes, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		log.Fatal("Failed to marshal stats:", err)
	}

	log.Println("Backtest complete.")
	return jsonBytes
}
