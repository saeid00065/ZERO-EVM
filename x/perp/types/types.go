package types

const StoreKey = "perp"

// Side - جهت پوزیشن
type Side uint8

const (
	Long  Side = 0
	Short Side = 1
)

// Position - یه پوزیشن باز
type Position struct {
	ID               string `json:"id"`
	Trader           string `json:"trader"`
	Market           string `json:"market"`
	Side             Side   `json:"side"`
	Size             uint64 `json:"size"`             // با ۶ دسیمال
	EntryPrice       uint64 `json:"entry_price"`      // با ۶ دسیمال
	Collateral       uint64 `json:"collateral"`       // مارجین (USDC)
	Leverage         uint8  `json:"leverage"`
	// محاسبه‌شده
	LiquidationPrice uint64 `json:"liquidation_price"`
	UnrealizedPnL    int64  `json:"unrealized_pnl"`   // می‌تونه منفی باشه
	// Funding
	FundingIndex     int64  `json:"funding_index"`    // برای محاسبه funding پرداختی
	OpenTimestamp    int64  `json:"open_timestamp"`
}

// UserAccount - اکانت کاربر
type UserAccount struct {
	Address         string `json:"address"`
	Collateral      uint64 `json:"collateral"`      // USDC موجود
	TotalPnL        int64  `json:"total_pnl"`
	TotalVolume     uint64 `json:"total_volume"`
}

// MarketState - وضعیت یه بازار perp
type MarketState struct {
	Symbol           string `json:"symbol"`
	MarkPrice        uint64 `json:"mark_price"`
	IndexPrice       uint64 `json:"index_price"`     // قیمت از oracle
	OpenInterestLong uint64 `json:"oi_long"`
	OpenInterestShort uint64 `json:"oi_short"`
	FundingRate      int64  `json:"funding_rate"`    // per hour, با ۸ دسیمال
	FundingIndex     int64  `json:"funding_index"`   // تجمیع funding
	LastFundingTime  int64  `json:"last_funding_time"`
	TotalVolume24h   uint64 `json:"volume_24h"`
}

// LiquidationRecord - ثبت لیکوییدیشن
type LiquidationRecord struct {
	PositionID  string `json:"position_id"`
	Trader      string `json:"trader"`
	Market      string `json:"market"`
	Size        uint64 `json:"size"`
	Price       uint64 `json:"price"`
	Timestamp   int64  `json:"timestamp"`
}

// Errors
var (
	ErrInsufficientCollateral = fmt.Errorf("insufficient collateral")
	ErrPositionNotFound       = fmt.Errorf("position not found")
	ErrMarketNotFound         = fmt.Errorf("market not found")
	ErrInvalidLeverage        = fmt.Errorf("leverage out of range (1-50)")
	ErrPositionTooSmall       = fmt.Errorf("position size too small")
)
