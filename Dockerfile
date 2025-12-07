FROM golang:1.20-alpine
WORKDIR /app
COPY . .
RUN go build -o /tradingbin ./cmd/trading-engine
COPY .env /tradingbin
CMD ["/tradingbin"]
