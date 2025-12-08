package backtest

import (
	"context"
	"sort"
	"time"

	"trading-engine/pkg/engine"
	"trading-engine/pkg/store"
)

type Backtester struct {
	candles []engine.Candle
	strat   engine.Strategy
	store   *store.SQLiteStore

	// Backtest state
	balance   float64
	position  engine.Position
	runID     string
	startTime time.Time
	endTime   time.Time
}

func NewBacktester(candles []engine.Candle, strat engine.Strategy, store *store.SQLiteStore, initialUSD float64) *Backtester {
	return &Backtester{
		candles: candles,
		strat:   strat,
		store:   store,
		balance: initialUSD,
	}
}

// -------------------------------------------------------------------------
// INTERNAL: simulate fill for backtest
// -------------------------------------------------------------------------
func (b *Backtester) simulateFill(o engine.Order, price float64) engine.Order {
	o.Filled = true
	o.FilledPrice = price

	// update balance & position
	switch o.Side {
	case engine.SideBuy:
		cost := o.Quantity * price
		b.balance -= cost

		// update avg price
		newQty := b.position.Quantity + o.Quantity
		if newQty > 0 {
			b.position.AvgPrice = ((b.position.AvgPrice * b.position.Quantity) + cost) / newQty
		}
		b.position.Quantity = newQty

	case engine.SideSell:
		// realize PnL
		realized := (price - b.position.AvgPrice) * o.Quantity
		b.balance += o.Quantity*price + realized

		b.position.Quantity -= o.Quantity

		// if flat, clear avg price
		if b.position.Quantity <= 0 {
			b.position.AvgPrice = 0
		}
	}

	// persist order + trade
	b.store.SaveOrder(
		o.ID, o.Symbol, string(o.Side), string(o.Type),
		o.Price, o.FilledPrice, o.Quantity,
	)

	b.store.SaveTrade(
		"trade_"+o.ID,
		o.ID,
		o.Symbol,
		string(o.Side),
		o.FilledPrice,
		o.Quantity,
	)

	return o
}

// -------------------------------------------------------------------------
// INTERCEPTOR: takes strategy orders and simulates fills
// -------------------------------------------------------------------------
func (b *Backtester) Submit(ctx context.Context, o engine.Order) (engine.Order, error) {
	// fill immediately at candle close price
	last := b.candles[len(b.candles)-1]
	price := last.Close
	o.FilledPrice = price
	o.ID = "bt_" + time.Now().Format("150405.999")

	o = b.simulateFill(o, price)
	return o, nil
}

// -------------------------------------------------------------------------
// MAIN BACKTEST LOOP
// -------------------------------------------------------------------------
func (b *Backtester) Run() (*BacktestStats, error) {
	sort.Slice(b.candles, func(i, j int) bool {
		return b.candles[i].Time.Before(b.candles[j].Time)
	})

	b.startTime = time.Now()
	b.runID = "run_" + b.startTime.Format("150405.999")

	b.strat.SetAccountUSD(b.balance)
	b.strat.OnStart()

	equityCurve := []float64{}

	for _, c := range b.candles {

		// Strategy logic
		b.strat.OnCandle(c)

		// collect equity
		unrealized := (c.Close - b.position.AvgPrice) * b.position.Quantity
		equityCurve = append(equityCurve, b.balance+unrealized)
	}

	b.strat.OnStop()

	// END
	b.endTime = time.Now()

	stats := &BacktestStats{
		RunID:       b.runID,
		Start:       b.startTime,
		End:         b.endTime,
		FinalEquity: equityCurve[len(equityCurve)-1],
		EquityCurve: equityCurve,
	}

	// save run summary
	b.store.SaveRun(b.runID, b.startTime, b.endTime, stats.FinalEquity)

	return stats, nil
}

// -------------------------------------------------------------------------
// STATS STRUCT
// -------------------------------------------------------------------------
type BacktestStats struct {
	RunID       string
	Start       time.Time
	End         time.Time
	FinalEquity float64
	EquityCurve []float64
}
