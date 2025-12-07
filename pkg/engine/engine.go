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

func (e *Engine) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)
	// subscribe to candles for each strategy symbol via exchange (mock supports it)
	for _, s := range e.strategies {
		go func(st Strategy) {
			st.OnStart()
			// For demo, assume symbol is retrievable via type assertion if needed.
		}(s)
	}
	// In a loop we would dispatch candles; the mock exchange pushes into channels observed by strategies directly in this skeleton.
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		<-e.ctx.Done()
	}()
	log.Println("Engine started")
	e.status = "Started"
	// block until canceled
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
