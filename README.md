# ZERO-EVM

A HyperLiquid-inspired custom EVM blockchain built with Cosmos SDK + Ethermint.

## Architecture

```
┌─────────────────────────────────────────┐
│         EVM Layer (Ethermint)           │  ← MetaMask, Solidity, web3.js
├─────────────────────────────────────────┤
│         Native Perp Trading             │  ← x/perp module
│         Native Order Book              │  ← x/orderbook module  
│         Oracle Price Feeds             │  ← x/oracle module
├─────────────────────────────────────────┤
│         HyperBFT Consensus             │  ← 200ms blocks
│         (CometBFT based)               │
└─────────────────────────────────────────┘
```

## Quick Start

```bash
# 1. Install Go 1.21+
# 2. Build and init
make init

# 3. Start node
make start
```

## Connect MetaMask

- Network Name: ZERO-EVM
- RPC URL: http://localhost:8545
- Chain ID: 9000
- Currency: ZERO

## Modules

| Module | Description |
|--------|-------------|
| `x/perp` | Perpetual futures: positions, PnL, liquidations, funding rates |
| `x/orderbook` | Native on-chain order book with in-memory matching engine |
| `x/oracle` | Price feeds from external sources (CoinGecko / Pyth) |
| `x/evm` | EVM compatibility via Ethermint |
