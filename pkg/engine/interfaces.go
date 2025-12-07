package engine

import (
	"context"
	"time"
)

type Side string
type OrderType string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"

	OrderMarket OrderType = "MARKET"
	OrderLimit  OrderType = "LIMIT"
)

type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type Order struct {
	ID       string
	Symbol   string
	Side     Side
	Type     OrderType
	Price    float64
	Quantity float64
	Created  int64
	Filled   bool
}

type Position struct {
	Symbol   string
	Quantity float64
	AvgPrice float64
}

type Strategy interface {
	OnCandle(c Candle)
	OnStart()
	OnStop()
	Name() string
}

type ExchangeAdapter interface {
	PlaceOrder(ctx context.Context, o Order) (Order, error)
	GetPosition(ctx context.Context, symbol string) (Position, error)
	GetBalances(ctx context.Context) (map[string]float64, error)
	SubscribeCandles(ctx context.Context, symbol string, interval int64) (<-chan Candle, error)
	CancelOrder(ctx context.Context, orderID string) error
}

type RiskManager interface {
	Size(symbol string, price float64, accountBalance float64) float64
}

type OrderExecutor interface {
	Submit(ctx context.Context, o Order) (Order, error)
}
