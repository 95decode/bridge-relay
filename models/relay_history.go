package models

import "gorm.io/gorm"

type Deposit struct {
	gorm.Model
	BlockNumber uint64
	Status      uint8
	TXID        string

	RId         string
	DepositData string
	Source      uint8
	Dest        uint8
	Nonce       uint64
	Memo        string
}

type Proposal struct {
	gorm.Model
	DepositRefer int
	Deposit      Deposit `gorm:"foreignKey:DepositRefer"`
	Status       uint8
	TXID         string

	RId         string
	DepositData string
	Signature   string
	Source      uint8
	Dest        uint8
	Nonce       uint64
	Memo        string
}
