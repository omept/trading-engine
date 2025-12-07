# Trading Engine (Go)
Minimal multi-package trading engine skeleton:
- cmd/trading-engine: entrypoint
- pkg/engine: core engine glue
- pkg/exchange: mock exchange + Binance adapter skeleton
- pkg/strategy: EMA crossover + Mean Reversion
- pkg/backtest: simple backtester
- pkg/store: SQLite persistence (basic)

Build:

```bash
cd cmd/trading-engine
go run .
```

The mock exchange will run and simulate candles. Swap in real adapters in pkg/exchange.
