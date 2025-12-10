# Trading Engine (Go)
Multi-package trading engine:
- cmd/trading-engine: entrypoint
- pkg/engine: core engine glue
- pkg/exchange: Mock exchange + Binance adapter + Alpaca Adapter
- pkg/strategy: EMA crossover + Mean Reversion
- pkg/backtest: simple backtester
- pkg/store: SQLite persistence

Run:

```bash
go run cmd/trading-engine

```

The mock exchange will run by default and simulate candles. Use .env variable to set `EXCHANGE`. Exchanges other than Mock retrieves candles from the exchange provider

# Warning
Work in progress!!!!! Use at your own detriment. 