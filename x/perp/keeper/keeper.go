package keeper

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	orderbookkeeper "github.com/saeid/zero-evm/x/orderbook/keeper"
	"github.com/saeid/zero-evm/x/perp/types"
)

// Keeper - مدیر perp trading
type Keeper struct {
	cdc             *codec.ProtoCodec
	storeKey        storetypes.StoreKey
	bankKeeper      bankkeeper.Keeper
	orderBookKeeper orderbookkeeper.Keeper
}

func NewKeeper(
	cdc *codec.ProtoCodec,
	storeKey storetypes.StoreKey,
	bankKeeper bankkeeper.Keeper,
	orderBookKeeper orderbookkeeper.Keeper,
) Keeper {
	return Keeper{
		cdc:             cdc,
		storeKey:        storeKey,
		bankKeeper:      bankKeeper,
		orderBookKeeper: orderBookKeeper,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// OPEN POSITION
// ─────────────────────────────────────────────────────────────────────────────

// OpenPosition - باز کردن پوزیشن
func (k Keeper) OpenPosition(
	ctx sdk.Context,
	trader string,
	market string,
	side types.Side,
	size uint64,      // با ۶ دسیمال
	leverage uint8,
	collateral uint64, // USDC که قفل میشه
) (*types.Position, error) {

	// ۱. اعتبارسنجی
	if leverage < 1 || leverage > 50 {
		return nil, types.ErrInvalidLeverage
	}
	if size == 0 {
		return nil, types.ErrPositionTooSmall
	}

	// ۲. گرفتن قیمت فعلی بازار
	mktState, err := k.GetMarketState(ctx, market)
	if err != nil {
		return nil, types.ErrMarketNotFound
	}
	markPrice := mktState.MarkPrice

	// ۳. محاسبه notional و مارجین مورد نیاز
	// notional = size × price
	// margin = notional / leverage
	notional := size * markPrice / 1_000_000
	requiredMargin := notional / uint64(leverage)

	// ۴. چک کن کاربر مارجین کافی داره
	if collateral < requiredMargin {
		return nil, fmt.Errorf("%w: need %d, have %d",
			types.ErrInsufficientCollateral, requiredMargin, collateral)
	}

	// ۵. محاسبه قیمت لیکوییدیشن
	// Long:  liqPrice = entryPrice × (1 - 1/leverage + maintenanceMargin)
	// Short: liqPrice = entryPrice × (1 + 1/leverage - maintenanceMargin)
	maintenanceMarginRate := uint64(5) // ۰.۵٪ = 5/1000
	var liqPrice uint64
	if side == types.Long {
		// اگه ضرر = مارجین شد، لیکویید میشه
		// liqPrice ≈ entryPrice × (1 - margin/notional)
		liqPrice = markPrice * (1000 - uint64(1000/leverage) + maintenanceMarginRate) / 1000
	} else {
		liqPrice = markPrice * (1000 + uint64(1000/leverage) - maintenanceMarginRate) / 1000
	}

	// ۶. ساخت پوزیشن
	pos := &types.Position{
		ID:               k.generatePositionID(trader, market),
		Trader:           trader,
		Market:           market,
		Side:             side,
		Size:             size,
		EntryPrice:       markPrice,
		Collateral:       collateral,
		Leverage:         leverage,
		LiquidationPrice: liqPrice,
		FundingIndex:     mktState.FundingIndex,
		OpenTimestamp:    ctx.BlockTime().UnixNano(),
	}

	// ۷. ذخیره پوزیشن
	k.savePosition(ctx, pos)

	// ۸. آپدیت Open Interest
	if side == types.Long {
		mktState.OpenInterestLong += size
	} else {
		mktState.OpenInterestShort += size
	}
	k.saveMarketState(ctx, mktState)

	return pos, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// CLOSE POSITION
// ─────────────────────────────────────────────────────────────────────────────

// ClosePosition - بستن پوزیشن و محاسبه سود/ضرر
func (k Keeper) ClosePosition(
	ctx sdk.Context,
	trader string,
	positionID string,
) (pnl int64, err error) {

	pos, err := k.GetPosition(ctx, positionID)
	if err != nil {
		return 0, err
	}

	if pos.Trader != trader {
		return 0, fmt.Errorf("unauthorized")
	}

	mktState, _ := k.GetMarketState(ctx, pos.Market)
	markPrice := mktState.MarkPrice

	// محاسبه PnL
	pnl = k.calculatePnL(pos, markPrice)

	// محاسبه funding پرداخت نشده
	fundingPayment := k.calculateFundingPayment(pos, mktState.FundingIndex)
	pnl -= fundingPayment

	// پرداخت به تریدر
	finalAmount := int64(pos.Collateral) + pnl
	if finalAmount > 0 {
		// سود: مارجین + سود رو برگردون
		k.creditTrader(ctx, trader, uint64(finalAmount))
	}
	// ضرر: مارجین از قبل قفل شده بود، همونجا میمونه

	// آپدیت Open Interest
	if pos.Side == types.Long {
		mktState.OpenInterestLong -= pos.Size
	} else {
		mktState.OpenInterestShort -= pos.Size
	}
	k.saveMarketState(ctx, mktState)

	// حذف پوزیشن
	k.deletePosition(ctx, positionID)

	return pnl, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// FUNDING RATE
// ─────────────────────────────────────────────────────────────────────────────

// SettleFundingRates - هر ساعت صدا زده میشه
// funding rate = (markPrice - indexPrice) / indexPrice × 0.01
// اگه مثبت → longها به shortها میدن
// اگه منفی → shortها به longها میدن
func (k Keeper) SettleFundingRates(ctx sdk.Context) {
	markets := k.GetAllMarkets(ctx)

	for _, mkt := range markets {
		// محاسبه funding rate
		var fundingRate int64
		markP := int64(mkt.MarkPrice)
		indexP := int64(mkt.IndexPrice)

		if indexP > 0 {
			// فرمول: (mark - index) / index
			// ضربدر 0.01 برای dampen کردن
			fundingRate = (markP - indexP) * 10000 / indexP / 100
		}

		// آپدیت funding index (تجمیعی)
		mkt.FundingRate = fundingRate
		mkt.FundingIndex += fundingRate
		mkt.LastFundingTime = ctx.BlockTime().Unix()

		k.saveMarketState(ctx, mkt)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// LIQUIDATION
// ─────────────────────────────────────────────────────────────────────────────

// CheckLiquidations - هر بلاک چک می‌کنه آیا پوزیشنی باید لیکویید بشه
func (k Keeper) CheckLiquidations(ctx sdk.Context) {
	positions := k.GetAllPositions(ctx)

	for _, pos := range positions {
		mktState, err := k.GetMarketState(ctx, pos.Market)
		if err != nil {
			continue
		}

		if k.shouldLiquidate(pos, mktState.MarkPrice) {
			k.liquidate(ctx, pos, mktState)
		}
	}
}

// shouldLiquidate - آیا پوزیشن باید لیکویید بشه؟
func (k Keeper) shouldLiquidate(pos *types.Position, markPrice uint64) bool {
	if pos.Side == types.Long {
		return markPrice <= pos.LiquidationPrice
	}
	return markPrice >= pos.LiquidationPrice
}

// liquidate - لیکویید کردن پوزیشن
func (k Keeper) liquidate(ctx sdk.Context, pos *types.Position, mkt *types.MarketState) {
	// ۱۰٪ از مارجین به liquidator میرسه (انگیزه برای liquidate کردن)
	liquidatorReward := pos.Collateral / 10
	// بقیه میره به Insurance Fund
	insuranceFund := pos.Collateral - liquidatorReward

	_ = liquidatorReward // در پیاده‌سازی واقعی به liquidator داده میشه
	_ = insuranceFund    // به insurance fund میره

	// Open Interest آپدیت
	if pos.Side == types.Long {
		mkt.OpenInterestLong -= pos.Size
	} else {
		mkt.OpenInterestShort -= pos.Size
	}
	k.saveMarketState(ctx, mkt)

	// پوزیشن حذف بشه
	k.deletePosition(ctx, pos.ID)

	// رویداد emit کن
	ctx.EventManager().EmitEvent(sdk.NewEvent(
		"liquidation",
		sdk.NewAttribute("position_id", pos.ID),
		sdk.NewAttribute("trader", pos.Trader),
		sdk.NewAttribute("market", pos.Market),
	))
}

// ─────────────────────────────────────────────────────────────────────────────
// HELPERS
// ─────────────────────────────────────────────────────────────────────────────

func (k Keeper) calculatePnL(pos *types.Position, markPrice uint64) int64 {
	entry := int64(pos.EntryPrice)
	mark := int64(markPrice)
	size := int64(pos.Size)

	if pos.Side == types.Long {
		return (mark - entry) * size / 1_000_000
	}
	return (entry - mark) * size / 1_000_000
}

func (k Keeper) calculateFundingPayment(pos *types.Position, currentFundingIndex int64) int64 {
	indexDiff := currentFundingIndex - pos.FundingIndex
	size := int64(pos.Size)

	if pos.Side == types.Long {
		// longها funding پرداخت می‌کنن وقتی مثبته
		return indexDiff * size / 1_000_000
	}
	// shortها وقتی funding مثبته، دریافت می‌کنن
	return -indexDiff * size / 1_000_000
}

// ─────────────────────────────────────────────────────────────────────────────
// STORAGE
// ─────────────────────────────────────────────────────────────────────────────

func (k Keeper) savePosition(ctx sdk.Context, pos *types.Position) {
	store := ctx.KVStore(k.storeKey)
	key := []byte(fmt.Sprintf("pos/%s", pos.ID))
	bz, _ := json.Marshal(pos)
	store.Set(key, bz)
}

func (k Keeper) GetPosition(ctx sdk.Context, id string) (*types.Position, error) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get([]byte(fmt.Sprintf("pos/%s", id)))
	if bz == nil {
		return nil, types.ErrPositionNotFound
	}
	var pos types.Position
	json.Unmarshal(bz, &pos)
	return &pos, nil
}

func (k Keeper) deletePosition(ctx sdk.Context, id string) {
	store := ctx.KVStore(k.storeKey)
	store.Delete([]byte(fmt.Sprintf("pos/%s", id)))
}

func (k Keeper) GetAllPositions(ctx sdk.Context) []*types.Position {
	store := ctx.KVStore(k.storeKey)
	prefix := []byte("pos/")
	iter := store.Iterator(prefix, nil)
	defer iter.Close()

	var positions []*types.Position
	for ; iter.Valid(); iter.Next() {
		var pos types.Position
		if err := json.Unmarshal(iter.Value(), &pos); err == nil {
			positions = append(positions, &pos)
		}
	}
	return positions
}

func (k Keeper) saveMarketState(ctx sdk.Context, mkt *types.MarketState) {
	store := ctx.KVStore(k.storeKey)
	key := []byte(fmt.Sprintf("mkt/%s", mkt.Symbol))
	bz, _ := json.Marshal(mkt)
	store.Set(key, bz)
}

func (k Keeper) GetMarketState(ctx sdk.Context, symbol string) (*types.MarketState, error) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get([]byte(fmt.Sprintf("mkt/%s", symbol)))
	if bz == nil {
		return nil, types.ErrMarketNotFound
	}
	var mkt types.MarketState
	json.Unmarshal(bz, &mkt)
	return &mkt, nil
}

func (k Keeper) GetAllMarkets(ctx sdk.Context) []*types.MarketState {
	store := ctx.KVStore(k.storeKey)
	prefix := []byte("mkt/")
	iter := store.Iterator(prefix, nil)
	defer iter.Close()

	var markets []*types.MarketState
	for ; iter.Valid(); iter.Next() {
		var mkt types.MarketState
		if err := json.Unmarshal(iter.Value(), &mkt); err == nil {
			markets = append(markets, &mkt)
		}
	}
	return markets
}

func (k Keeper) creditTrader(ctx sdk.Context, trader string, amount uint64) {
	// در پیاده‌سازی واقعی: bank.SendCoins به آدرس تریدر
	_ = ctx
	_ = trader
	_ = amount
}

func (k Keeper) generatePositionID(trader, market string) string {
	return fmt.Sprintf("%s-%s-%d", trader[:8], market, time.Now().UnixNano())
}
