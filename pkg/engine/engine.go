package engine

import (
	"context"
	"log"
	"sync"

	"trading-engine/pkg/store"
)

type Engine struct {
	strategies []Strategy
	exchange   ExchangeAdapter
	om         OrderExecutor
	store      *store.SQLiteStore
	status     string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	lock   sync.Mutex
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) RegisterStrategy(s Strategy) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.strategies = append(e.strategies, s)
}

func (e *Engine) SetExchangeAdapter(x ExchangeAdapter) {
	e.exchange = x
}

func (e *Engine) SetOrderManager(o OrderExecutor) {
	e.om = o
}

func (e *Engine) SetStore(s *store.SQLiteStore) {
	e.store = s
}

func (e *Engine) ExchangeAdapter() ExchangeAdapter {
	return e.exchange
}

func (e *Engine) OrderManager() OrderExecutor {
	return e.om
}

func (e *Engine) Store() *store.SQLiteStore {
	return e.store
}

func (e *Engine) Strategies() []Strategy {
	return e.strategies
}

// Start subscribes strategies to candle feeds and runs them
func (e *Engine) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)

	e.lock.Lock()
	defer e.lock.Unlock()
	log.Println("Loading strategies")
	for _, s := range e.strategies {
		// Start each strategy
		s.OnStart()

		// Subscribe to exchange candles for strategy symbol
		symbol := s.Symbol()  // assume Strategy interface has Symbol()
		interval := int64(60) // 1-min candles, adjust as needed
		candleCh, err := e.exchange.SubscribeCandles(e.ctx, symbol, interval)
		if err != nil {
			log.Printf("failed to subscribe candles for %s: %v", symbol, err)
			continue
		}

		// Launch a goroutine to feed candles to the strategy
		e.wg.Add(1)
		cc := 0
		go func(st Strategy, ch <-chan Candle) {
			log.Printf("Candle receiver started, now feeding candle to Strategy : %s", s.Name())
			defer e.wg.Done()
			for {
				select {
				case c, ok := <-ch:
					if !ok {
						log.Printf("Candle sending closed. Sent total %d candles", cc)
						return
					}
					cc++
					st.OnCandle(c)
				case <-e.ctx.Done():
					log.Printf("Candle sending stopped. Sent total %d candles", cc)
					return
				}
			}
		}(s, candleCh)
	}

	log.Println("Engine started")
	e.status = "Started"

	// Wait until canceled
	<-e.ctx.Done()
}

func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	for _, s := range e.strategies {
		s.OnStop()
	}
	e.status = "Stopped"
	log.Println("Engine stopped")
}

func (e *Engine) Status() interface{} {
	return struct {
		Message string `json:"message"`
	}{
		Message: e.status,
	}
}
