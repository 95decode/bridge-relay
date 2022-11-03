package db

import (
	"encoding/hex"
	"fmt"
	"hexbridge/models"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const ENV_DATABASES_HOST = "DATABASES_HOST"
const ENV_DATABASES_PORT = "DATABASES_PORT"
const ENV_DATABASES_USER = "DATABASES_USER"
const ENV_DATABASES_PASSWORD = "DATABASES_PASSWORD"
const ENV_DATABASES_NAME = "DATABASES_NAME"
const ENV_DATABASES_LOCAL_SEOUL = "DATABASES_LOCAL_SEOUL"

type Manager interface {
	InsertDeposit(blocknumber uint64, txid string, rid string, depositData string, source uint8, dest uint8, nonce uint64) error
	UpdateErrOfDeposit(txid string, msg string) error
	InsertProposal(deposit_txid string, proposal_txid string, rid [32]byte, depositData []byte, signature []byte, source uint8, dest uint8, nonce uint64, memo string) error
	UpdateBlock(domain_id uint8, blocknumber uint64)
}

type manager struct {
	db *gorm.DB
}

var Instance Manager

func init() {

	dbHost := os.Getenv(ENV_DATABASES_HOST)
	dbPort := os.Getenv(ENV_DATABASES_PORT)
	dbUser := os.Getenv(ENV_DATABASES_USER)
	dbPassword := os.Getenv(ENV_DATABASES_PASSWORD)
	dbName := os.Getenv(ENV_DATABASES_NAME)
	dbLocalTime := os.Getenv(ENV_DATABASES_LOCAL_SEOUL)

	var dbEndpoint string = fmt.Sprintf("%s:%s", dbHost, dbPort)

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8",
		dbUser, dbPassword, dbEndpoint, dbName,
	)

	if dbLocalTime == "" {
		dsn += "&parseTime=True&loc=Asia%2FSeoul"
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	sqlDB, err := db.DB()
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(100)

	err = sqlDB.Ping()
	if err != nil {
		panic(err)
	}

	err = db.AutoMigrate(&models.Deposit{})
	err = db.AutoMigrate(&models.Proposal{})
	err = db.AutoMigrate(&models.Block{})

	Instance = &manager{db: db}
}

func (mgr *manager) InsertDeposit(blocknumber uint64, txid string, rid string, depositData string, source uint8, dest uint8, nonce uint64) error {

	deposit := &models.Deposit{}
	deposit.BlockNumber = blocknumber
	deposit.TXID = "0x" + txid
	deposit.RId = rid
	deposit.DepositData = depositData
	deposit.Source = source
	deposit.Dest = dest
	deposit.Nonce = nonce
	deposit.Status = 1
	deposit.Memo = ""

	err := mgr.db.Create(deposit).Error
	return err
}

func (mgr *manager) UpdateErrOfDeposit(txid string, msg string) error {

	err := mgr.db.Model(&models.Deposit{}).Where("tx_id = ?", txid).
		Updates(map[string]interface{}{"status": 2, "memo": msg}).Error

	return err
}

func (mgr *manager) InsertProposal(deposit_txid string, proposal_txid string, rid [32]byte, depositData []byte, signature []byte, source uint8, dest uint8, nonce uint64, memo string) error {

	var deposit models.Deposit
	mgr.db.First(&deposit, "tx_id = ?", "0x"+deposit_txid)

	proposal := &models.Proposal{}
	proposal.Deposit = deposit
	proposal.TXID = "0x" + proposal_txid
	proposal.RId = string(rid[0:5])
	proposal.DepositData = hex.EncodeToString(depositData)
	proposal.Signature = hex.EncodeToString(signature)
	proposal.Source = source
	proposal.Dest = dest
	proposal.Nonce = nonce
	proposal.Status = 1
	proposal.Memo = memo

	err := mgr.db.Create(proposal).Error
	return err
}

func (mgr *manager) UpdateBlock(domain_id uint8, blocknumber uint64) {
	mgr.db.Model(&models.Block{}).Where("domain = ?", domain_id).Update("block_number", blocknumber)
}
