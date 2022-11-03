package utils

import (
	"github.com/ethereum/go-ethereum/crypto"
)

func Hash(data []byte) [32]byte {
	return crypto.Keccak256Hash(data)
}
