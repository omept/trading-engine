package engine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type OrderManager struct {
	exchange ExchangeAdapter
	mt       sync.Mutex
	pending  map[string]string
}

func NewOrderManager(ex ExchangeAdapter) *OrderManager {
	return &OrderManager{exchange: ex, pending: make(map[string]string)}
}

func (om *OrderManager) Submit(ctx context.Context, o Order) (Order, error) {
	key := fmt.Sprintf("%s:%s:%f:%s", o.Symbol, o.Side, o.Quantity, o.Type)
	om.mt.Lock()
	if id, ok := om.pending[key]; ok {
		om.mt.Unlock()
		return Order{ID: id}, nil
	}
	om.mt.Unlock()

	var lastErr error
	wait := 100 * time.Millisecond
	for i := 0; i < 5; i++ {
		r, err := om.exchange.PlaceOrder(ctx, o)
		if err == nil {
			om.mt.Lock()
			om.pending[key] = r.ID
			om.mt.Unlock()
			return r, nil
		}
		lastErr = err
		time.Sleep(wait)
		wait *= 2
	}
	return Order{}, lastErr
}
