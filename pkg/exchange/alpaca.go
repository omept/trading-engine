package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/omept/trading-engine/pkg/engine"
	"github.com/omept/trading-engine/pkg/store"
)

type AlpacaAdapter struct {
	key     string
	secret  string
	baseURL string
	client  *http.Client
	mt      sync.Mutex
	db      *store.SQLiteStore
}

func NewAlpacaAdapter(key, secret, base string, db *store.SQLiteStore) (engine.ExchangeAdapter, error) {
	return &AlpacaAdapter{
		key:     key,
		secret:  secret,
		baseURL: base,
		client:  &http.Client{Timeout: 15 * time.Second},
		db:      db,
	}, nil
}

func (a *AlpacaAdapter) do(ctx context.Context, method, path string, body io.Reader) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, method, a.baseURL+path, body)
	req.Header.Set("APCA-API-KEY-ID", a.key)
	req.Header.Set("APCA-API-SECRET-KEY", a.secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alpaca error: %s", string(b))
	}

	return b, nil
}

// -----------------------------------------------------------------------------
// Implement ExchangeAdapter interface
// -----------------------------------------------------------------------------

func (a *AlpacaAdapter) PlaceOrder(ctx context.Context, o engine.Order) (engine.Order, error) {
	req := map[string]interface{}{
		"symbol":        o.Symbol,
		"side":          strings.ToLower(string(o.Side)),
		"type":          "market",
		"notional":      o.Quantity,
		"time_in_force": "gtc",
	}

	if o.Type == engine.OrderLimit {
		req["type"] = "limit"
		req["limit_price"] = o.Price
		delete(req, "notional")
		req["qty"] = o.Quantity
	}

	b, _ := json.Marshal(req)
	a.mt.Lock()
	defer a.mt.Unlock()
	resp, err := a.do(ctx, "POST", "/v2/orders", strings.NewReader(string(b)))
	if err != nil {
		return o, err
	}

	var out struct {
		ID        string `json:"id"`
		FilledQty string `json:"filled_qty"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return o, err
	}

	o.ID = out.ID
	o.Created = time.Now().Unix()
	o.Filled = out.Status == "filled"

	return o, nil
}

func (a *AlpacaAdapter) AdapterName() string {
	return "Aplaca"
}

func (a *AlpacaAdapter) CancelOrder(ctx context.Context, orderID string) error {
	a.mt.Lock()
	defer a.mt.Unlock()
	_, err := a.do(ctx, "DELETE", "/v2/orders/"+orderID, nil)
	return err
}

func (a *AlpacaAdapter) GetBalances(ctx context.Context) (map[string]float64, error) {
	a.mt.Lock()
	defer a.mt.Unlock()
	b, err := a.do(ctx, "GET", "/v2/account", nil)
	if err != nil {
		return nil, err
	}

	var acct struct {
		Cash      string `json:"cash"`
		Portfolio string `json:"portfolio_value"`
	}
	if err := json.Unmarshal(b, &acct); err != nil {
		return nil, err
	}

	cash, _ := strconv.ParseFloat(acct.Cash, 64)

	return map[string]float64{
		"USD": cash,
	}, nil
}

func (a *AlpacaAdapter) GetPosition(ctx context.Context, symbol string) (engine.Position, error) {

	a.mt.Lock()
	b, err := a.do(ctx, "GET", "/v2/positions/"+symbol, nil)
	a.mt.Unlock()

	if err != nil {
		return engine.Position{Symbol: symbol}, nil // no position = zero
	}

	var pos struct {
		Qty string `json:"qty"`
	}
	if err := json.Unmarshal(b, &pos); err != nil {
		return engine.Position{Symbol: symbol}, err
	}

	q, _ := strconv.ParseFloat(pos.Qty, 64)

	return engine.Position{
		Symbol:   symbol,
		Quantity: q,
	}, nil
}

func (a *AlpacaAdapter) SubscribeCandles(ctx context.Context, symbol string, interval int64) (<-chan engine.Candle, error) {
	// Alpaca REST bars polling
	ch := make(chan engine.Candle, 1024)
	log.Printf("Subscribing to Candles from %s", a.AdapterName())

	go func() {
		defer close(ch)

		for {
			url := fmt.Sprintf("/v2/stocks/%s/bars?timeframe=1Min&limit=200", symbol)

			resp, err := a.do(ctx, "GET", url, nil)
			if err != nil {
				return
			}

			var bars struct {
				Bars []struct {
					T string  `json:"t"`
					O float64 `json:"o"`
					H float64 `json:"h"`
					L float64 `json:"l"`
					C float64 `json:"c"`
					V float64 `json:"v"`
				} `json:"bars"`
			}

			if err := json.Unmarshal(resp, &bars); err != nil {
				return
			}

			for _, b := range bars.Bars {
				t, _ := time.Parse(time.RFC3339, b.T)
				ch <- engine.Candle{
					Time:   t,
					Open:   b.O,
					High:   b.H,
					Low:    b.L,
					Close:  b.C,
					Volume: b.V,
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()

	return ch, nil
}
