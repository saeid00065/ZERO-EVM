package types

const StoreKey = "orderbook"

// Side - جهت معامله
type Side uint8

const (
	Buy  Side = 0
	Sell Side = 1
)

// OrderType - نوع اردر
type OrderType uint8

const (
	Limit  OrderType = 0
	Market OrderType = 1
	Stop   OrderType = 2
)

// Order - یه اردر در order book
type Order struct {
	ID        string `json:"id"`
	Trader    string `json:"trader"`    // آدرس تریدر
	Market    string `json:"market"`    // مثلاً "BTC-PERP"
	Side      Side   `json:"side"`
	Type      OrderType `json:"type"`
	Price     uint64 `json:"price"`    // با ۶ دسیمال (1000000 = $1)
	Size      uint64 `json:"size"`     // با ۶ دسیمال
	Filled    uint64 `json:"filled"`   // چقدر fill شده
	Timestamp int64  `json:"timestamp"`
}

// Trade - یه معامله انجام شده
type Trade struct {
	ID        string `json:"id"`
	Market    string `json:"market"`
	Price     uint64 `json:"price"`
	Size      uint64 `json:"size"`
	Buyer     string `json:"buyer"`
	Seller    string `json:"seller"`
	Timestamp int64  `json:"timestamp"`
}

// Market - اطلاعات یه بازار
type Market struct {
	Symbol        string `json:"symbol"`         // "BTC-PERP"
	BaseAsset     string `json:"base_asset"`     // "BTC"
	QuoteAsset    string `json:"quote_asset"`    // "USDC"
	MarkPrice     uint64 `json:"mark_price"`
	IndexPrice    uint64 `json:"index_price"`    // از Oracle میاد
	OpenInterest  uint64 `json:"open_interest"`
	FundingRate   int64  `json:"funding_rate"`   // می‌تونه منفی باشه
	MaxLeverage   uint8  `json:"max_leverage"`   // مثلاً 50
	Active        bool   `json:"active"`
}

// OrderBook - یه بازار کامل
type OrderBook struct {
	Market string
	Bids   []*Order // مرتب‌شده از بالاترین قیمت
	Asks   []*Order // مرتب‌شده از پایین‌ترین قیمت
}

// EventOrderPlaced - رویداد ثبت اردر
type EventOrderPlaced struct {
	OrderID string
	Trader  string
	Market  string
	Side    Side
	Price   uint64
	Size    uint64
}

// EventTrade - رویداد انجام معامله
type EventTrade struct {
	TradeID string
	Market  string
	Price   uint64
	Size    uint64
	Buyer   string
	Seller  string
}
