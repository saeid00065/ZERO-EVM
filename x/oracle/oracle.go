package oracle

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Oracle - قیمت‌ها رو از منابع خارجی می‌گیره
// در production از Pyth Network یا Chainlink استفاده میشه
// اینجا یه نسخه ساده با CoinGecko API

type PriceData struct {
	Symbol    string  `json:"symbol"`
	Price     float64 `json:"price"`
	UpdatedAt int64   `json:"updated_at"`
}

type Oracle struct {
	mu     sync.RWMutex
	prices map[string]*PriceData
	done   chan struct{}
}

func NewOracle() *Oracle {
	o := &Oracle{
		prices: make(map[string]*PriceData),
		done:   make(chan struct{}),
	}
	return o
}

// Start - شروع به fetch کردن قیمت‌ها
func (o *Oracle) Start() {
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		// اول یه بار فوری
		o.fetchPrices()

		for {
			select {
			case <-ticker.C:
				o.fetchPrices()
			case <-o.done:
				return
			}
		}
	}()
}

func (o *Oracle) Stop() {
	close(o.done)
}

// GetPrice - گرفتن قیمت یه asset (با ۶ دسیمال)
// مثلاً BTC = $67000 → 67000_000000
func (o *Oracle) GetPrice(symbol string) (uint64, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	data, ok := o.prices[symbol]
	if !ok {
		return 0, fmt.Errorf("price not found for %s", symbol)
	}

	// چک کن قیمت قدیمی نباشه (بیشتر از ۳۰ ثانیه)
	if time.Now().Unix()-data.UpdatedAt > 30 {
		return 0, fmt.Errorf("stale price for %s", symbol)
	}

	return uint64(data.Price * 1_000_000), nil
}

// fetchPrices - گرفتن قیمت از CoinGecko
func (o *Oracle) fetchPrices() {
	url := "https://api.coingecko.com/api/v3/simple/price?ids=bitcoin,ethereum,solana,arbitrum&vs_currencies=usd"

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}

	mapping := map[string]string{
		"bitcoin":  "BTC",
		"ethereum": "ETH",
		"solana":   "SOL",
		"arbitrum": "ARB",
	}

	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().Unix()
	for geckoID, symbol := range mapping {
		if priceMap, ok := result[geckoID]; ok {
			if price, ok := priceMap["usd"]; ok {
				o.prices[symbol] = &PriceData{
					Symbol:    symbol,
					Price:     price,
					UpdatedAt: now,
				}
			}
		}
	}
}

// MockOracle - برای تست، قیمت‌های ثابت
type MockOracle struct {
	prices map[string]uint64
}

func NewMockOracle() *MockOracle {
	return &MockOracle{
		prices: map[string]uint64{
			"BTC": 67_000_000_000, // $67,000 با ۶ دسیمال
			"ETH": 3_500_000_000,  // $3,500
			"SOL": 172_000_000,    // $172
			"ARB": 1_100_000,      // $1.10
		},
	}
}

func (m *MockOracle) GetPrice(symbol string) (uint64, error) {
	if price, ok := m.prices[symbol]; ok {
		return price, nil
	}
	return 0, fmt.Errorf("unknown symbol: %s", symbol)
}
