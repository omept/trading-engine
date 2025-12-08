package strategy

import (
	"context"
	"log"
	"math"
	"sync"

	"trading-engine/pkg/engine"
)

type MeanReversion struct {
	window     int
	k          float64
	prices     []float64
	exec       engine.OrderExecutor
	risk       engine.RiskManager
	accountUSD float64
	symbol     string
	lock       sync.Mutex
	name       string
}

func NewMeanReversion(symbol string, window int, k float64, exec engine.OrderExecutor, risk engine.RiskManager) engine.Strategy {
	return &MeanReversion{
		window: window,
		k:      k,
		prices: []float64{},
		exec:   exec,
		risk:   risk,
		symbol: symbol,
		name:   "MeanReversion",
	}
}

func (m *MeanReversion) Name() string            { return m.name }
func (m *MeanReversion) SetAccountUSD(v float64) { m.accountUSD = v }
func (m *MeanReversion) OnStart()                { log.Println("Started Mean Reversion Strategy") }
func (m *MeanReversion) OnStop()                 { log.Println("Stopped Mean Reversion Strategy") }

func meanStd(xs []float64) (float64, float64) {
	n := float64(len(xs))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range xs {
		sum += v
	}
	mean := sum / n
	var sq float64
	for _, v := range xs {
		diff := v - mean
		sq += diff * diff
	}
	variance := sq / n
	return mean, math.Sqrt(variance)
}

func (m *MeanReversion) OnCandle(c engine.Candle) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.prices = append(m.prices, c.Close)
	if len(m.prices) < m.window {
		return
	}
	window := m.prices[len(m.prices)-m.window:]
	mean, sd := meanStd(window)
	last := c.Close
	if last < mean-m.k*sd {
		qty := m.risk.Size(m.symbol, last, m.accountUSD)
		if qty <= 0 {
			return
		}
		o := engine.Order{Symbol: m.symbol, Side: engine.SideBuy, Type: engine.OrderMarket, Quantity: qty}
		if _, err := m.exec.Submit(context.TODO(), o); err != nil {
			log.Println("MeanRev buy err:", err)
		} else {
			log.Println("MeanRev buy executed", qty)
		}
	} else if last > mean+m.k*sd {
		qty := m.risk.Size(m.symbol, last, m.accountUSD)
		if qty <= 0 {
			return
		}
		o := engine.Order{Symbol: m.symbol, Side: engine.SideSell, Type: engine.OrderMarket, Quantity: qty}
		if _, err := m.exec.Submit(context.TODO(), o); err != nil {
			log.Println("MeanRev sell err:", err)
		} else {
			log.Println("MeanRev sell executed", qty)
		}
	}
}
