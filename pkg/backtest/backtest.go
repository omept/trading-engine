package backtest

import (
	"sort"
	"time"

	"trading-engine/pkg/engine"
)

type Backtester struct {
	candles []engine.Candle
	strat   engine.Strategy
	exec    engine.OrderExecutor
	start   time.Time
	end     time.Time
}

func NewBacktester(candles []engine.Candle, strat engine.Strategy, exec engine.OrderExecutor) *Backtester {
	return &Backtester{
		candles: candles,
		strat:   strat,
		exec:    exec,
	}
}

func GenerateSyntheticCandles(n int, start time.Time) []engine.Candle {
	out := make([]engine.Candle, 0, n)
	p := 20000.0
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			p *= 1.0005
		} else {
			p *= 0.9997
		}
		c := engine.Candle{
			Time:   start.Add(time.Duration(i) * time.Minute),
			Open:   p * 0.999,
			High:   p * 1.001,
			Low:    p * 0.998,
			Close:  p,
			Volume: 5 + float64(i%10),
		}
		out = append(out, c)
	}
	return out
}

func (b *Backtester) Run() {
	// sort candles by time
	sort.Slice(b.candles, func(i, j int) bool {
		return b.candles[i].Time.Before(b.candles[j].Time)
	})
	b.strat.OnStart()
	for _, c := range b.candles {
		b.strat.OnCandle(c)
	}
	b.strat.OnStop()
}
