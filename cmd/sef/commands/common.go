package commands

import (
	"fmt"
	"os"

	"github.com/btcsuite/btcd/wire"
)

func parseNetwork(name string) (wire.BitcoinNet, error) {
	switch name {
	case "mainnet":
		return wire.MainNet, nil
	case "testnet", "testnet3":
		return wire.TestNet3, nil
	case "signet":
		return wire.BitcoinNet(0xead63049), nil
	case "regtest":
		return wire.TestNet, nil
	default:
		return 0, fmt.Errorf("unknown network %q: use mainnet, testnet, signet or regtest", name)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
