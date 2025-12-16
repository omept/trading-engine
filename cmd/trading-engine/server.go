package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/omept/trading-engine/pkg/engine"
	"github.com/omept/trading-engine/pkg/store"
	"github.com/omept/trading-engine/web/dist"
)

func setUpAPIs(eng *engine.Engine, db *store.SQLiteStore) *http.ServeMux {

	mux := http.NewServeMux()

	// Minimal web UI
	mux.Handle("/", http.FileServer(http.FS(dist.WebDist)))

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

	mux.HandleFunc("/api/backtest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("method not allowed, use POST"))
			return
		}

		// Read query parameters or JSON body (here using query params for simplicity)
		which := r.URL.Query().Get("strategy") // ema | mean | all
		if which == "" {
			which = "all"
		}
		symbol := r.URL.Query().Get("symbol")
		if symbol == "" {
			symbol = "BTCUSD"
		}

		// Optional: start/end for future range selection
		start := r.URL.Query().Get("start")
		end := r.URL.Query().Get("end")
		if start == "" || end == "" {
			// default: ignore or use entire DB range
		}

		log.Println("Running backtest via API:", which, symbol)
		statsJSON := runBacktest(which, symbol, eng, db)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(statsJSON)
	})

	return mux
}
