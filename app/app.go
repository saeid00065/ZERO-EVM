package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/std"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	// EVM module (ethermint)
	evmkeeper "github.com/evmos/ethermint/x/evm/keeper"
	evmtypes "github.com/evmos/ethermint/x/evm/types"

	// Our custom modules
	perpkeeper "github.com/saeid/zero-evm/x/perp/keeper"
	perptypes "github.com/saeid/zero-evm/x/perp/types"
	orderbookkeeper "github.com/saeid/zero-evm/x/orderbook/keeper"
	orderbooktypes "github.com/saeid/zero-evm/x/orderbook/types"

	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/libs/log"
)

const (
	AppName         = "ZeroEVM"
	DefaultNodeHome = ".zeroevm"
	// Chain ID - EVM chains need a numeric chain ID
	// متامسک و والت‌ها از این برای تشخیص چین استفاده می‌کنن
	ChainID = "zero_9000-1"
)

// ZeroEVMApp - بلاکچین اصلی ما
// ترکیب Cosmos SDK + EVM + Native Perp Trading
type ZeroEVMApp struct {
	*baseapp.BaseApp

	cdc               *codec.ProtoCodec
	interfaceRegistry codectypes.InterfaceRegistry

	// ─── Cosmos Standard Modules ───────────────────────────────────────────
	AccountKeeper  authkeeper.AccountKeeper
	BankKeeper     bankkeeper.Keeper
	StakingKeeper  stakingkeeper.Keeper

	// ─── EVM Layer (Ethermint) ─────────────────────────────────────────────
	// این لایه باعث میشه MetaMask، Solidity، web3.js همه کار کنن
	EvmKeeper *evmkeeper.Keeper

	// ─── Native Perp Trading (مثل HyperLiquid) ────────────────────────────
	// این ماژول‌ها مستقیم توی چین هستن - نه Smart Contract
	// به همین دلیل سریع‌تر از هر DEX معمولیه
	PerpKeeper      perpkeeper.Keeper
	OrderBookKeeper orderbookkeeper.Keeper

	// store keys
	keys map[string]*storetypes.KVStoreKey
}

// NewZeroEVMApp - سازنده اپ
func NewZeroEVMApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	homePath string,
	baseAppOptions ...func(*baseapp.BaseApp),
) *ZeroEVMApp {

	// ۱. Codec و Interface Registry
	interfaceRegistry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(interfaceRegistry)
	std.RegisterInterfaces(interfaceRegistry)

	// ۲. BaseApp - پایه همه چیز
	bApp := baseapp.NewBaseApp(AppName, logger, db, nil, baseAppOptions...)
	bApp.SetCommitMultiStoreTracer(traceStore)

	// ۳. Store keys - هر ماژول جای خودش رو توی storage داره
	keys := sdk.NewKVStoreKeys(
		authtypes.StoreKey,
		banktypes.StoreKey,
		stakingtypes.StoreKey,
		evmtypes.StoreKey,
		perptypes.StoreKey,
		orderbooktypes.StoreKey,
	)

	app := &ZeroEVMApp{
		BaseApp:           bApp,
		cdc:               cdc,
		interfaceRegistry: interfaceRegistry,
		keys:              keys,
	}

	// ۴. Init ماژول‌ها
	app.AccountKeeper = authkeeper.NewAccountKeeper(
		cdc, keys[authtypes.StoreKey],
		authtypes.ProtoBaseAccount,
		nil, // مدیریت اکانت‌ها
		sdk.Bech32MainPrefix,
		authtypes.NewModuleAddress("gov").String(),
	)

	app.BankKeeper = bankkeeper.NewKeeper(
		cdc, keys[banktypes.StoreKey],
		app.AccountKeeper,
		nil, // blocked addrs
		authtypes.NewModuleAddress("gov").String(),
	)

	// ۵. Native Order Book Keeper
	// این قلب ماجراست - مستقیم توی consensus لایه
	app.OrderBookKeeper = orderbookkeeper.NewKeeper(
		cdc,
		keys[orderbooktypes.StoreKey],
	)

	// ۶. Perp Trading Keeper
	// مدیریت پوزیشن‌ها، لیکوییدیشن، فاندینگ ریت
	app.PerpKeeper = perpkeeper.NewKeeper(
		cdc,
		keys[perptypes.StoreKey],
		app.BankKeeper,
		app.OrderBookKeeper,
	)

	// ۷. Mount stores
	app.MountKVStores(keys)

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			panic(err)
		}
	}

	return app
}

// DefaultGenesis - حالت اولیه چین
func (app *ZeroEVMApp) DefaultGenesis() map[string]json.RawMessage {
	return module.NewBasicManager().DefaultGenesis(app.cdc)
}

// BeginBlock - اول هر بلاک
// اینجا funding rate محاسبه میشه (مثل HyperLiquid)
func (app *ZeroEVMApp) BeginBlock(req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	ctx := app.NewContext(false, req.Header)

	// هر ۳۶۰۰ بلاک (≈۱ ساعت) funding rate پرداخت میشه
	if req.Header.Height%3600 == 0 {
		app.PerpKeeper.SettleFundingRates(ctx)
	}

	// چک لیکوییدیشن برای همه پوزیشن‌ها
	app.PerpKeeper.CheckLiquidations(ctx)

	return app.BaseApp.BeginBlock(req)
}

// NewRootCmd - CLI command
func NewRootCmd() interface{} {
	return nil // در فایل بعدی پیاده‌سازی میشه
}

// AppCodec returns the app codec
func (app *ZeroEVMApp) AppCodec() *codec.ProtoCodec {
	return app.cdc
}

// homePath helper
func getHomePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, DefaultNodeHome)
}
