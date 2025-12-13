# Trading Engine (Go)
Multi-package trading engine:
- cmd/trading-engine: entrypoint
- pkg/engine: core engine glue
- pkg/exchange: Mock exchange + Binance adapter + Alpaca Adapter
- pkg/strategy: EMA crossover + Mean Reversion
- pkg/backtest: simple backtester
- pkg/store: SQLite persistence



### 1. Execute Source Code Directly

Run the application without generating a binary file:
```bash
go run cmd/trading-engine/.
```

### 2. Generate a Binary and run the binary

Run the application without generating a binary file:
```bash
copy .env bin/.env # copy the env to the bin directory. Replace `copy` with cp on a Mac/Linux
go build -o bin/TradingEngine cmd/trading-engine/.
cd bin
./TradingEngine
```
*NOTE: If you move the binary to any other directory, the  `.env` file needs to be in the directory where the binary is called from.*

# Extra Info
The mock exchange will run by default and simulate candles. Use .env variable to set `EXCHANGE`. Exchanges other than Mock retrieves candles from the exchange provider

# Warning
Work in progress!!!!! Use at your own detriment. 