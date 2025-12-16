package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/omept/trading-engine/pkg/store"
)

type OrderManager struct {
	exchange ExchangeAdapter
	mt       sync.Mutex
	pending  map[string]string
	db       *store.SQLiteStore
}

func NewOrderManager(ex ExchangeAdapter, db *store.SQLiteStore) OrderExecutor {
	return &OrderManager{exchange: ex, pending: make(map[string]string), db: db}
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
			if om.db != nil {
				//persist order
				err = om.db.SaveOrder(
					r.ID,
					r.Symbol,
					string(r.Side),
					string(r.Type),
					r.Price,
					r.FilledPrice,
					r.Quantity,
				)
				if err != nil {
					return r, err
				}
				// persist trade
				tradeID := r.ID + "_trade"
				err = om.db.SaveTrade(
					tradeID,
					r.ID,
					r.Symbol,
					string(r.Side),
					r.FilledPrice,
					r.Quantity,
				)
				if err != nil {
					return r, err
				}
			}
			return r, nil
		}
		lastErr = err
		time.Sleep(wait)
		wait *= 2
	}
	return Order{}, lastErr
}
