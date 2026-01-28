package pricing

import (
	"context"
	"errors"
	"time"
)

// FixedPriceProvider returns a fixed price per kWh.
type FixedPriceProvider struct {
	price float64
}

// NewFixedPriceProvider constructs the provider.
func NewFixedPriceProvider(price float64) (*FixedPriceProvider, error) {
	if price < 0 {
		return nil, errors.New("price provider: negative price")
	}
	return &FixedPriceProvider{price: price}, nil
}

// PriceAt returns the configured fixed price.
func (p *FixedPriceProvider) PriceAt(ctx context.Context, subjectID string, at time.Time) (float64, error) {
	_ = ctx
	_ = subjectID
	_ = at
	// TODO: replace with dynamic tariff / pricing service once available.
	return p.price, nil
}
