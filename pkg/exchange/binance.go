package exchange

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"trading-engine/pkg/engine"
	"trading-engine/pkg/store"
)

type BinanceAdapter struct {
	key     string
	secret  string
	client  *http.Client
	baseURL string
	mt      sync.Mutex
	db      *store.SQLiteStore
}

func NewBinanceAdapter(apiKey, apiSecret string, db *store.SQLiteStore) (engine.ExchangeAdapter, error) {
	return &BinanceAdapter{
		key:     apiKey,
		secret:  apiSecret,
		client:  &http.Client{Timeout: 15 * time.Second},
		baseURL: "https://api.binance.com",
	}, nil
}

// --- internal helpers --------------------------------------------------------

func (b *BinanceAdapter) sign(params string) string {
	h := hmac.New(sha256.New, []byte(b.secret))
	h.Write([]byte(params))
	return hex.EncodeToString(h.Sum(nil))
}

func (b *BinanceAdapter) privatePOST(ctx context.Context, path string, data url.Values) ([]byte, error) {
	data.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))
	query := data.Encode()
	signature := b.sign(query)
	query += "&signature=" + signature

	req, _ := http.NewRequestWithContext(ctx, "POST", b.baseURL+path+"?"+query, nil)
	req.Header.Set("X-MBX-APIKEY", b.key)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance error: %s", string(body))
	}

	return body, nil
}

func (b *BinanceAdapter) privateGET(ctx context.Context, path string, data url.Values) ([]byte, error) {
	data.Set("timestamp", fmt.Sprintf("%d", time.Now().UnixMilli()))
	query := data.Encode()
	signature := b.sign(query)
	query += "&signature=" + signature

	req, _ := http.NewRequestWithContext(ctx, "GET", b.baseURL+path+"?"+query, nil)
	req.Header.Set("X-MBX-APIKEY", b.key)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance error: %s", string(body))
	}

	return body, nil
}

// --- interface implementations -----------------------------------------------

func (b *BinanceAdapter) PlaceOrder(ctx context.Context, o engine.Order) (engine.Order, error) {

	val := url.Values{}
	val.Set("symbol", strings.ToUpper(o.Symbol))
	val.Set("side", string(o.Side))
	val.Set("type", string(o.Type))

	if o.Type == engine.OrderMarket {
		// Binance requires quantity, not notional
		val.Set("quoteOrderQty", fmt.Sprintf("%f", o.Quantity))
	} else {
		val.Set("quantity", fmt.Sprintf("%f", o.Quantity))
		val.Set("price", fmt.Sprintf("%f", o.Price))
	}
	b.mt.Lock()

	body, err := b.privatePOST(ctx, "/api/v3/order", val)
	if err != nil {
		return o, err
	}
	b.mt.Unlock()

	var resp struct {
		OrderID int64  `json:"orderId"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return o, err
	}

	o.ID = fmt.Sprintf("%d", resp.OrderID)
	o.Created = time.Now().Unix()
	o.Filled = resp.Status == "FILLED"

	return o, nil
}

func (b *BinanceAdapter) CancelOrder(ctx context.Context, orderID string) error {
	val := url.Values{}
	val.Set("orderId", orderID)
	b.mt.Lock()
	defer b.mt.Unlock()
	_, err := b.privatePOST(ctx, "/api/v3/order", val)
	return err
}

func (b *BinanceAdapter) GetBalances(ctx context.Context) (map[string]float64, error) {
	b.mt.Lock()
	body, err := b.privateGET(ctx, "/api/v3/account", url.Values{})
	b.mt.Unlock()
	if err != nil {
		return nil, err
	}

	var acct struct {
		Balances []struct {
			Asset  string `json:"asset"`
			Free   string `json:"free"`
			Locked string `json:"locked"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(body, &acct); err != nil {
		return nil, err
	}

	out := make(map[string]float64)
	for _, b := range acct.Balances {
		f, _ := strconv.ParseFloat(b.Free, 64)
		out[b.Asset] = f
	}

	return out, nil
}

func (b *BinanceAdapter) GetPosition(ctx context.Context, symbol string) (engine.Position, error) {
	// Binance Spot does not have real positions; treat spot as amount of asset.
	b.mt.Lock()
	defer b.mt.Unlock()
	balances, err := b.GetBalances(ctx)
	if err != nil {
		return engine.Position{Symbol: symbol}, err
	}

	asset := symbol[:3] // crude split "BTCUSDT" -> "BTC"
	qty := balances[asset]

	return engine.Position{
		Symbol:   symbol,
		Quantity: qty,
	}, nil
}

func (b *BinanceAdapter) SubscribeCandles(ctx context.Context, symbol string, interval int64) (<-chan engine.Candle, error) {
	// For simplicity: fetch recent candles via REST in a goroutine.
	b.mt.Lock()
	defer b.mt.Unlock()

	ch := make(chan engine.Candle, 1024)

	go func() {
		defer close(ch)
		url := fmt.Sprintf("%s/api/v3/klines?symbol=%s&interval=1m&limit=500",
			b.baseURL,
			strings.ToUpper(symbol),
		)

		for {
			// poll every ~3 seconds
			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			resp, err := b.client.Do(req)
			if err != nil {
				return
			}
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var arr [][]interface{}
			if err := json.Unmarshal(body, &arr); err != nil {
				return
			}

			for _, c := range arr {
				ch <- engine.Candle{
					Time:   time.UnixMilli(int64(c[0].(float64))),
					Open:   mustF(c[1]),
					High:   mustF(c[2]),
					Low:    mustF(c[3]),
					Close:  mustF(c[4]),
					Volume: mustF(c[5]),
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

func mustF(v interface{}) float64 {
	s := fmt.Sprintf("%v", v)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
