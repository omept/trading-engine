package engine

import "math"

type FixedPercentRisk struct {
    Percent float64
}

func NewFixedPercentRisk(p float64) *FixedPercentRisk {
    return &FixedPercentRisk{Percent: p}
}

func (r *FixedPercentRisk) Size(symbol string, price float64, accountBalance float64) float64 {
    if price <= 0 {
        return 0
    }
    usd := accountBalance * r.Percent
    qty := usd / price
    // floor to 8 decimal places
    return math.Floor(qty*1e8) / 1e8
}
