package exchange

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"trading-engine/pkg/engine"
	"trading-engine/pkg/store"
)

type MockExchange struct {
	mt        sync.RWMutex
	balances  map[string]float64
	positions map[string]engine.Position
	feeds     map[string]chan engine.Candle
	orders    map[string]engine.Order
	db        *store.SQLiteStore
}

func NewMockExchange(bal float64, db *store.SQLiteStore) engine.ExchangeAdapter {
	me := &MockExchange{
		balances:  map[string]float64{"USD": bal},
		positions: make(map[string]engine.Position),
		feeds:     make(map[string]chan engine.Candle),
		orders:    make(map[string]engine.Order),
		db:        db,
	}
	me.SetDefaultBalances()
	return me
}

func (m *MockExchange) SetDefaultBalances() {
	m.mt.Lock()
	defer m.mt.Unlock()

	// initialize map if nil
	if m.balances == nil {
		m.balances = make(map[string]float64)
	}

	// set common defaults
	m.balances["USDT"] = 10000 // starting quote balance
	m.balances["BTC"] = 1000.0 // starting base balance
}

// PushCandleInBacktest allows the backtester to manually feed candles
func (m *MockExchange) PushCandleInBacktest(symbol string, c engine.Candle) {
	m.mt.RLock()
	ch, ok := m.feeds[symbol]
	m.mt.RUnlock()

	if ok {
		ch <- c
	}
}

func (a *MockExchange) AdapterName() string {
	return "Mock"
}

func (m *MockExchange) PlaceOrder(ctx context.Context, o engine.Order) (engine.Order, error) {
	m.mt.Lock()
	defer m.mt.Unlock()

	o.ID = "mock_" + time.Now().Format("150405.000")
	o.Created = time.Now().Unix()

	// immediate fill for MARKET in this mock
	if o.Type == engine.OrderMarket {
		o.Filled = true

		// ---- BALANCE ADJUSTMENTS ----
		base, quote, err := parseSymbol(o.Symbol)
		if err != nil {
			return o, err
		}

		amount := o.Quantity // base amount
		price := o.Price     // assumed available on order

		cost := amount * price

		switch o.Side {
		case engine.SideBuy:
			// check sufficient balance
			if m.balances[quote] < cost {
				return o, fmt.Errorf("insufficient %s balance: need %.4f", quote, cost)
			}
			// deduct quote
			m.balances[quote] -= cost
			// add base
			m.balances[base] += amount

		case engine.SideSell:
			// check sufficient balance
			if m.balances[base] < amount {
				return o, fmt.Errorf("insufficient %s balance: need %.4f", base, amount)
			}
			// deduct base
			m.balances[base] -= amount
			// add quote
			m.balances[quote] += cost
		}

		// ------------------------------

		m.orders[o.ID] = o
		return o, nil
	}

	m.orders[o.ID] = o
	return o, nil
}

func (m *MockExchange) GetPosition(ctx context.Context, symbol string) (engine.Position, error) {
	m.mt.RLock()
	defer m.mt.RUnlock()
	if p, ok := m.positions[symbol]; ok {
		return p, nil
	}
	return engine.Position{Symbol: symbol}, nil
}

func (m *MockExchange) GetBalances(ctx context.Context) (map[string]float64, error) {
	m.mt.RLock()
	defer m.mt.RUnlock()
	out := make(map[string]float64, len(m.balances))
	for k, v := range m.balances {
		out[k] = v
	}
	return out, nil
}

func (m *MockExchange) SubscribeCandles(ctx context.Context, symbol string, interval int64) (<-chan engine.Candle, error) {
	m.mt.Lock()
	defer m.mt.Unlock()
	ch := make(chan engine.Candle, 1024)
	m.feeds[symbol] = ch
	// start a small generator for demo
	log.Printf("Subscribing to Candles from %s", m.AdapterName())
	go func() {
		now := time.Now().Add(-time.Duration(200) * time.Minute)
		price := 30000.0
		for i := 0; i < 200; i++ {
			price *= 1 + (0.0005 - 0.0002*float64(i%3))
			c := engine.Candle{
				Time:   now.Add(time.Duration(i) * time.Minute),
				Open:   price * 0.999,
				High:   price * 1.001,
				Low:    price * 0.998,
				Close:  price,
				Volume: 10 + float64(i%5),
			}
			select {
			case ch <- c:
			case <-ctx.Done():
				close(ch)
				return
			}
			time.Sleep(2 * time.Second)
		}
		close(ch)
	}()
	return ch, nil
}

func (m *MockExchange) CancelOrder(ctx context.Context, orderID string) error {
	m.mt.Lock()
	defer m.mt.Unlock()
	if _, ok := m.orders[orderID]; ok {
		delete(m.orders, orderID)
		return nil
	}
	return errors.New("order not found")
}

// parseSymbol tries to split a symbol into base and quote.
// Supports forms: "BTCUSDT", "BTC/USDT", "BTC-USDT".
// If symbol is the concatenation form it attempts to match known quote suffixes.
func parseSymbol(sym string) (base, quote string, err error) {
	sym = strings.TrimSpace(sym)
	if sym == "" {
		return "", "", fmt.Errorf("empty symbol")
	}

	// common separators
	if strings.Contains(sym, "/") {
		parts := strings.SplitN(sym, "/", 2)
		return strings.ToUpper(parts[0]), strings.ToUpper(parts[1]), nil
	}
	if strings.Contains(sym, "-") {
		parts := strings.SplitN(sym, "-", 2)
		return strings.ToUpper(parts[0]), strings.ToUpper(parts[1]), nil
	}

	// fallback: try to detect quote from known suffixes
	knownQuotes := []string{"USDT", "USDC", "BTC", "ETH", "USD", "EUR", "BUSD", "SOL"}
	upper := strings.ToUpper(sym)
	for _, q := range knownQuotes {
		if strings.HasSuffix(upper, q) && len(upper) > len(q) {
			base = upper[:len(upper)-len(q)]
			quote = q
			return strings.ToUpper(base), quote, nil
		}
	}

	return "", "", fmt.Errorf("unable to parse symbol: %s", sym)
}
