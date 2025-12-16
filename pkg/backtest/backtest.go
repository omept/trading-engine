package backtest

import (
	"context"
	"sort"
	"time"

	"github.com/omept/trading-engine/pkg/engine"
	"github.com/omept/trading-engine/pkg/exchange"
	"github.com/omept/trading-engine/pkg/store"
)

type Backtester struct {
	candles  []engine.Candle
	strats   []engine.Strategy
	store    *store.SQLiteStore
	exchange engine.ExchangeAdapter
}

type BacktestStats struct {
	RunID       string
	Start       time.Time
	End         time.Time
	FinalEquity float64
	EquityCurve []float64
}

func NewBacktester(candles []engine.Candle, eng *engine.Engine, store *store.SQLiteStore) *Backtester {
	return &Backtester{
		candles:  candles,
		strats:   eng.Strategies(),
		exchange: eng.ExchangeAdapter(),
		store:    store,
	}
}

func (b *Backtester) Run(symbol string) (*BacktestStats, error) {
	sort.Slice(b.candles, func(i, j int) bool {
		return b.candles[i].Time.Before(b.candles[j].Time)
	})

	ch, _ := b.exchange.SubscribeCandles(context.Background(), symbol, -1)

	stats := &BacktestStats{
		RunID: "backtest_" + time.Now().Format("150405"),
		Start: time.Now(),
	}

	equityCurve := make([]float64, 0, len(b.candles))

	for _, strat := range b.strats {
		strat.OnStart()
	}

	for _, c := range b.candles {
		// 1. push candle manually
		b.exchange.(*exchange.MockExchange).PushCandleInBacktest(symbol, c)

		// 2. read from exchange feed (strategies react inside OnCandle)
		chCandle := <-ch
		for _, strat := range b.strats {
			strat.OnCandle(chCandle)
		}

		// 3. compute equity from exchange balances + positions
		bal, _ := b.exchange.GetBalances(context.Background())
		pos, _ := b.exchange.GetPosition(context.Background(), symbol)

		equity := bal["USD"] + pos.Quantity*chCandle.Close
		equityCurve = append(equityCurve, equity)
	}

	for _, strat := range b.strats {
		strat.OnStop()
	}

	stats.EquityCurve = equityCurve
	stats.FinalEquity = equityCurve[len(equityCurve)-1]
	stats.End = time.Now()

	return stats, nil
}
