package strategy

import (
	"context"
	"log"
	"sync"

	"trading-engine/pkg/engine"
)

const ST_NAME_EMA = "EMA strategy"

type EMACrossover struct {
	shortP     int
	longP      int
	prices     []float64
	exec       engine.OrderExecutor
	risk       engine.RiskManager
	symbol     string
	lock       sync.Mutex
	accountUSD float64
	name       string
}

func NewEMACrossover(symbol string, shortP, longP int, exec engine.OrderExecutor, risk engine.RiskManager) engine.Strategy {
	return &EMACrossover{
		shortP: shortP,
		longP:  longP,
		prices: []float64{},
		exec:   exec,
		risk:   risk,
		symbol: symbol,
		name:   ST_NAME_EMA,
	}
}

func (e *EMACrossover) Name() string            { return e.name }
func (e *EMACrossover) SetAccountUSD(v float64) { e.accountUSD = v }
func (e *EMACrossover) Symbol() string          { return e.symbol }
func (e *EMACrossover) AccountBalUSD() float64  { return e.accountUSD }
func (e *EMACrossover) OnStart()                { log.Println("Started EMAC Crossover Strategy") }
func (e *EMACrossover) OnStop()                 { log.Println("Stopped EMAC Crossover Strategy") }

func ema(series []float64, period int) []float64 {
	out := make([]float64, len(series))
	if period <= 0 {
		return out
	}
	k := 2.0 / float64(period+1)
	var prev float64
	for i := range series {
		if i == 0 {
			prev = series[0]
			out[0] = prev
			continue
		}
		prev = (series[i]-prev)*k + prev
		out[i] = prev
	}
	return out
}

func (e *EMACrossover) OnCandle(c engine.Candle) {
	e.lock.Lock()
	defer e.lock.Unlock()
	price := c.Close
	e.prices = append(e.prices, price)
	if len(e.prices) < e.longP+2 {
		return
	}
	short := ema(e.prices, e.shortP)
	long := ema(e.prices, e.longP)
	n := len(short) - 1
	prev := n - 1
	if short[prev] <= long[prev] && short[n] > long[n] {
		qty := e.risk.Size(e.symbol, price, e.accountUSD)
		if qty <= 0 {
			return
		}
		o := engine.Order{Price: price, Symbol: e.symbol, Side: engine.SideBuy, Type: engine.OrderMarket, Quantity: qty}
		if _, err := e.exec.Submit(context.TODO(), o); err != nil {
			log.Println("EMA buy error:", err)
		} else {
			log.Println("EMA buy executed", qty)
		}
	}
	if short[prev] >= long[prev] && short[n] < long[n] {
		qty := e.risk.Size(e.symbol, price, e.accountUSD)
		if qty <= 0 {
			return
		}
		o := engine.Order{Symbol: e.symbol, Side: engine.SideSell, Type: engine.OrderMarket, Quantity: qty}
		if _, err := e.exec.Submit(context.TODO(), o); err != nil {
			log.Println("EMA sell error:", err)
		} else {
			log.Println("EMA sell executed", qty)
		}
	}
}
