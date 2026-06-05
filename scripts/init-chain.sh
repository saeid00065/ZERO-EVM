#!/bin/bash
# init-chain.sh - راه‌اندازی اولیه chain
# بعد از build کردن این رو اجرا کن

CHAIN_ID="zero_9000-1"
MONIKER="zero-node-1"
BINARY="./build/zeroevm"
HOME_DIR="$HOME/.zeroevm"

echo "=== ZERO-EVM Chain Initialization ==="

# ۱. پاک کردن داده‌های قبلی
rm -rf $HOME_DIR
echo "✓ Cleaned old data"

# ۲. init کردن node
$BINARY init $MONIKER --chain-id $CHAIN_ID
echo "✓ Node initialized"

# ۳. ساخت validator key
$BINARY keys add validator --keyring-backend test
echo "✓ Validator key created"

# ۴. ساخت genesis account
VALIDATOR_ADDR=$($BINARY keys show validator -a --keyring-backend test)
$BINARY genesis add-genesis-account $VALIDATOR_ADDR 1000000000000uzero
echo "✓ Genesis account: $VALIDATOR_ADDR"

# ۵. genesis transaction
$BINARY genesis gentx validator 100000000uzero \
  --chain-id $CHAIN_ID \
  --keyring-backend test
echo "✓ Genesis transaction created"

# ۶. collect gentxs
$BINARY genesis collect-gentxs
echo "✓ Collected genesis transactions"

# ۷. تنظیم EVM chain ID (باید با CHAIN_ID match باشه)
# HyperLiquid از chain ID عددی ۹۰۰۰ استفاده می‌کنه
# ما از ۹۰۰۰ هم استفاده می‌کنیم
sed -i 's/"evm_chain_id": "9001"/"evm_chain_id": "9000"/' $HOME_DIR/config/genesis.json

# ۸. تنظیم سرعت consensus (مثل HyperLiquid)
CONFIG="$HOME_DIR/config/config.toml"
sed -i 's/timeout_commit = "5s"/timeout_commit = "200ms"/' $CONFIG
sed -i 's/timeout_propose = "3s"/timeout_propose = "200ms"/' $CONFIG
sed -i 's/timeout_prevote = "1s"/timeout_prevote = "100ms"/' $CONFIG
sed -i 's/timeout_precommit = "1s"/timeout_precommit = "100ms"/' $CONFIG
echo "✓ Consensus timing configured (200ms blocks)"

# ۹. فعال کردن RPC و API
APP_CONFIG="$HOME_DIR/config/app.toml"
sed -i 's/enable = false/enable = true/' $APP_CONFIG
sed -i 's/swagger = false/swagger = true/' $APP_CONFIG
echo "✓ API/RPC enabled"

echo ""
echo "=== Chain Ready! ==="
echo "Start with: $BINARY start"
echo "RPC:        http://localhost:26657"
echo "API:        http://localhost:1317"
echo "EVM RPC:    http://localhost:8545  ← MetaMask این رو استفاده می‌کنه"
echo "EVM WS:     ws://localhost:8546"
