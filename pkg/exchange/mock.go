package exchange

import (
	"context"
	"errors"
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
	return &MockExchange{
		balances:  map[string]float64{"USD": bal},
		positions: make(map[string]engine.Position),
		feeds:     make(map[string]chan engine.Candle),
		orders:    make(map[string]engine.Order),
		db:        db,
	}
}

func (m *MockExchange) PlaceOrder(ctx context.Context, o engine.Order) (engine.Order, error) {
	m.mt.Lock()
	defer m.mt.Unlock()
	o.ID = "mock_" + time.Now().Format("150405.000")
	o.Created = time.Now().Unix()
	// immediate fill for MARKET in this mock
	if o.Type == engine.OrderMarket {
		o.Filled = true
		// simplistic balance adjustments omitted for brevity
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
			time.Sleep(5 * time.Millisecond)
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
