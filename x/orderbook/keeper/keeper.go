package keeper

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/saeid/zero-evm/x/orderbook/types"
)

// Keeper - مدیر order book
// این struct توی RAM نگه داشته میشه برای سرعت
// و همزمان توی blockchain state هم ذخیره میشه
type Keeper struct {
	cdc      *codec.ProtoCodec
	storeKey storetypes.StoreKey

	// In-memory order books برای سرعت
	// مثل HyperLiquid که off-chain matching داره
	mu         sync.RWMutex
	orderBooks map[string]*InMemoryOrderBook // key: market symbol
}

// InMemoryOrderBook - order book توی RAM
type InMemoryOrderBook struct {
	mu   sync.RWMutex
	Bids []*types.Order // sorted: high → low
	Asks []*types.Order // sorted: low → high
}

func NewKeeper(cdc *codec.ProtoCodec, storeKey storetypes.StoreKey) Keeper {
	return Keeper{
		cdc:        cdc,
		storeKey:   storeKey,
		orderBooks: make(map[string]*InMemoryOrderBook),
	}
}

// GetOrCreateBook - گرفتن یا ساختن order book برای یه market
func (k *Keeper) GetOrCreateBook(market string) *InMemoryOrderBook {
	k.mu.Lock()
	defer k.mu.Unlock()

	if book, ok := k.orderBooks[market]; ok {
		return book
	}

	book := &InMemoryOrderBook{}
	k.orderBooks[market] = book
	return book
}

// PlaceOrder - ثبت اردر و matching
// این تابع قلب DEX ماست
func (k *Keeper) PlaceOrder(ctx sdk.Context, order *types.Order) ([]types.Trade, error) {
	book := k.GetOrCreateBook(order.Market)

	book.mu.Lock()
	defer book.mu.Unlock()

	var trades []types.Trade

	if order.Type == types.Market {
		// Market order: فوری execute میشه
		trades = k.matchMarketOrder(book, order)
	} else {
		// Limit order: اول match کن، بقیه رو توی book بذار
		trades = k.matchLimitOrder(book, order)
	}

	// اگه اردر کاملاً fill نشد و limit بود، توی book بذار
	if order.Size > order.Filled && order.Type == types.Limit {
		k.insertOrder(book, order)
	}

	// ذخیره trades توی blockchain state
	for _, trade := range trades {
		k.saveTrade(ctx, trade)
	}

	return trades, nil
}

// matchLimitOrder - matching برای limit orders
func (k *Keeper) matchLimitOrder(book *InMemoryOrderBook, order *types.Order) []types.Trade {
	var trades []types.Trade

	if order.Side == types.Buy {
		// خریدار: با asks match کن (ارزون‌ترین ask اول)
		for len(book.Asks) > 0 && order.Size > order.Filled {
			bestAsk := book.Asks[0]

			// اگه قیمت خریدار >= بهترین ask → match!
			if order.Price >= bestAsk.Price {
				tradeSize := min64(order.Size-order.Filled, bestAsk.Size-bestAsk.Filled)
				trade := types.Trade{
					ID:        generateID(order.Market, len(trades)),
					Market:    order.Market,
					Price:     bestAsk.Price, // قیمت maker (ask)
					Size:      tradeSize,
					Buyer:     order.Trader,
					Seller:    bestAsk.Trader,
					Timestamp: time.Now().UnixNano(),
				}
				trades = append(trades, trade)

				order.Filled += tradeSize
				bestAsk.Filled += tradeSize

				// اگه ask کاملاً fill شد، حذفش کن
				if bestAsk.Filled >= bestAsk.Size {
					book.Asks = book.Asks[1:]
				}
			} else {
				break // قیمت‌ها match نمیشن
			}
		}
	} else {
		// فروشنده: با bids match کن (گران‌ترین bid اول)
		for len(book.Bids) > 0 && order.Size > order.Filled {
			bestBid := book.Bids[0]

			if order.Price <= bestBid.Price {
				tradeSize := min64(order.Size-order.Filled, bestBid.Size-bestBid.Filled)
				trade := types.Trade{
					ID:        generateID(order.Market, len(trades)),
					Market:    order.Market,
					Price:     bestBid.Price,
					Size:      tradeSize,
					Buyer:     bestBid.Trader,
					Seller:    order.Trader,
					Timestamp: time.Now().UnixNano(),
				}
				trades = append(trades, trade)

				order.Filled += tradeSize
				bestBid.Filled += tradeSize

				if bestBid.Filled >= bestBid.Size {
					book.Bids = book.Bids[1:]
				}
			} else {
				break
			}
		}
	}

	return trades
}

// matchMarketOrder - matching برای market orders (هر قیمتی)
func (k *Keeper) matchMarketOrder(book *InMemoryOrderBook, order *types.Order) []types.Trade {
	// برای market order قیمت مهم نیست - هر قیمتی قبوله
	if order.Side == types.Buy {
		order.Price = ^uint64(0) // max uint64
	} else {
		order.Price = 0
	}
	return k.matchLimitOrder(book, order)
}

// insertOrder - اردر رو توی book بذار (مرتب‌شده)
func (k *Keeper) insertOrder(book *InMemoryOrderBook, order *types.Order) {
	if order.Side == types.Buy {
		book.Bids = append(book.Bids, order)
		// مرتب کن: گران‌ترین اول
		sort.Slice(book.Bids, func(i, j int) bool {
			return book.Bids[i].Price > book.Bids[j].Price
		})
	} else {
		book.Asks = append(book.Asks, order)
		// مرتب کن: ارزون‌ترین اول
		sort.Slice(book.Asks, func(i, j int) bool {
			return book.Asks[i].Price < book.Asks[j].Price
		})
	}
}

// CancelOrder - لغو اردر
func (k *Keeper) CancelOrder(ctx sdk.Context, market, orderID, trader string) error {
	book := k.GetOrCreateBook(market)
	book.mu.Lock()
	defer book.mu.Unlock()

	// پیدا کن توی bids
	for i, o := range book.Bids {
		if o.ID == orderID && o.Trader == trader {
			book.Bids = append(book.Bids[:i], book.Bids[i+1:]...)
			return nil
		}
	}

	// پیدا کن توی asks
	for i, o := range book.Asks {
		if o.ID == orderID && o.Trader == trader {
			book.Asks = append(book.Asks[:i], book.Asks[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("order %s not found", orderID)
}

// GetOrderBook - گرفتن snapshot از order book
func (k *Keeper) GetOrderBook(market string, depth int) (bids, asks []*types.Order) {
	book := k.GetOrCreateBook(market)
	book.mu.RLock()
	defer book.mu.RUnlock()

	if depth > len(book.Bids) {
		depth = len(book.Bids)
	}
	bids = book.Bids[:depth]

	askDepth := depth
	if askDepth > len(book.Asks) {
		askDepth = len(book.Asks)
	}
	asks = book.Asks[:askDepth]

	return
}

// saveTrade - ذخیره trade توی blockchain state
func (k *Keeper) saveTrade(ctx sdk.Context, trade types.Trade) {
	store := ctx.KVStore(k.storeKey)
	key := []byte(fmt.Sprintf("trade/%s/%s", trade.Market, trade.ID))
	bz, _ := json.Marshal(trade)
	store.Set(key, bz)
}

// GetRecentTrades - گرفتن آخرین trades
func (k *Keeper) GetRecentTrades(ctx sdk.Context, market string, limit int) []types.Trade {
	store := ctx.KVStore(k.storeKey)
	prefix := []byte(fmt.Sprintf("trade/%s/", market))

	var trades []types.Trade
	iter := store.ReverseIterator(prefix, nil)
	defer iter.Close()

	count := 0
	for ; iter.Valid() && count < limit; iter.Next() {
		var trade types.Trade
		if err := json.Unmarshal(iter.Value(), &trade); err == nil {
			trades = append(trades, trade)
			count++
		}
	}
	return trades
}

// helpers
func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func generateID(market string, seq int) string {
	return fmt.Sprintf("%s-%d-%d", market, time.Now().UnixNano(), seq)
}
