package main

import (
	"encoding/json"
	"log"

	"github.com/omept/trading-engine/pkg/backtest"
	"github.com/omept/trading-engine/pkg/engine"
	"github.com/omept/trading-engine/pkg/store"
	"github.com/omept/trading-engine/pkg/strategy"
)

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
