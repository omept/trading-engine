package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore wraps a *sql.DB
type SQLiteStore struct {
	db *sql.DB
}

func (s *SQLiteStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	store := &SQLiteStore{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	return store, nil
}

// migrate runs initial schema creation
func (s *SQLiteStore) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS orders (
	id TEXT PRIMARY KEY,
	symbol TEXT,
	side TEXT,
	type TEXT,
	price REAL,
	quantity REAL,
	filled INTEGER,
	filled_price REAL,
	created_at DATETIME
);

CREATE TABLE IF NOT EXISTS trades (
	id TEXT PRIMARY KEY,
	order_id TEXT,
	symbol TEXT,
	side TEXT,
	price REAL,
	quantity REAL,
	created_at DATETIME
);

CREATE TABLE IF NOT EXISTS runs (
	id TEXT PRIMARY KEY,
	strategy TEXT,
	started_at DATETIME,
	stopped_at DATETIME
);

CREATE TABLE IF NOT EXISTS candles (
	id TEXT PRIMARY KEY,
	symbol TEXT,
	time DATETIME,
	open REAL,
	high REAL,
	low REAL,
	close REAL,
	volume REAL
);
`
	_, err := s.db.Exec(schema)
	return err
}

// CountOrders returns total orders
func (s *SQLiteStore) CountOrders() (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&count)
	return count, err
}

// CountTrades returns total trades
func (s *SQLiteStore) CountTrades() (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM trades`).Scan(&count)
	return count, err
}

// CountRuns returns total runs
func (s *SQLiteStore) CountRuns() (int64, error) {
	var count int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&count)
	return count, err
}

// Persist candle
func (s *SQLiteStore) SaveCandle(symbol string, cTime string, open, high, low, close, volume float64) error {
	stmt, err := s.db.Prepare(`INSERT OR IGNORE INTO candles(id,symbol,time,open,high,low,close,volume) VALUES(?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	id := fmt.Sprintf("%s_%s", symbol, cTime)
	_, err = stmt.Exec(id, symbol, cTime, open, high, low, close, volume)
	return err
}

// Load candles for symbol
func (s *SQLiteStore) LoadCandles(symbol string, limit int) ([]map[string]interface{}, error) {
	rows, err := s.db.Query(`SELECT time,open,high,low,close,volume FROM candles WHERE symbol=? ORDER BY time ASC LIMIT ?`, symbol, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candles []map[string]interface{}
	for rows.Next() {
		var t string
		var o, h, l, c, v float64
		if err := rows.Scan(&t, &o, &h, &l, &c, &v); err != nil {
			return nil, err
		}
		candles = append(candles, map[string]interface{}{
			"time": t, "open": o, "high": h, "low": l, "close": c, "volume": v,
		})
	}
	return candles, nil
}

// Save Order
func (s *SQLiteStore) SaveOrder(id string,
	symbol string,
	side string,
	orderType string,
	price float64,
	filledPrice float64,
	quantity float64,
) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO orders(id,symbol,side,type,price,quantity,filled,filled_price,created_at)
VALUES(?,?,?,?,?,?,?,?,?)`,
		id, symbol, side, orderType, price, quantity, true, filledPrice, time.Now())
	if err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) SaveTrade(
	id, orderID, symbol, side string,
	price, quantity float64,
) error {
	_, err := s.db.Exec(`
        INSERT OR REPLACE INTO trades(id,order_id,symbol,side,price,quantity,created_at)
        VALUES(?,?,?,?,?,?,datetime('now'))
    `, id, orderID, symbol, side, price, quantity)
	return err
}

func (s *SQLiteStore) SaveRunStart(id, strategy string) error {
	_, err := s.db.Exec(`
        INSERT OR REPLACE INTO runs(id,strategy,started_at)
        VALUES(?,?,datetime('now'))
    `, id, strategy)
	return err
}

func (s *SQLiteStore) SaveRunStop(id string) error {
	_, err := s.db.Exec(`
        UPDATE runs SET stopped_at=datetime('now')
        WHERE id=?
    `, id)
	return err
}

func (s *SQLiteStore) PnL(symbol string) (float64, error) {
	rows, err := s.db.Query(`
        SELECT side, price, quantity FROM trades WHERE symbol=?
    `, symbol)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var pnl float64
	var pos float64
	var avg float64

	for rows.Next() {
		var side string
		var price, qty float64
		rows.Scan(&side, &price, &qty)

		if side == "BUY" {
			avg = (avg*pos + price*qty) / (pos + qty)
			pos += qty
		} else {
			// SELL
			pnl += (price - avg) * qty
			pos -= qty
		}
	}

	return pnl, nil
}

func (s *SQLiteStore) SaveRun(id string, start, end time.Time, final float64) error {
	const q = `
        INSERT INTO runs (id, start_time, end_time, final_pnl)
        VALUES (?, ?, ?, ?)
    `

	_, err := s.db.Exec(q, id, start.UTC(), end.UTC(), final)
	if err != nil {
		return fmt.Errorf("sqlite: SaveRun failed: %w", err)
	}
	return nil
}
