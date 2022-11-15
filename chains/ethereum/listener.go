package ethereum

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"hexbridge/bindings/Bridge"
	"hexbridge/bindings/ERC20Handler"
	"hexbridge/bindings/ERC721Handler"
	"hexbridge/bindings/GenericHandler"
	"hexbridge/chains"
	db "hexbridge/db"
	"hexbridge/models"
	utils "hexbridge/shared/ethereum"
	"hexbridge/utils/blockstore"
	metrics "hexbridge/utils/metrics/types"
	monitoring "hexbridge/utils/monitoring"
	"hexbridge/utils/msg"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ChainSafe/log15"
	eth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethcommon "github.com/ethereum/go-ethereum/common"
)

var BlockRetryInterval = time.Second * 10
var BlockRetryLimit = 5
var ErrFatalPolling = errors.New("listener block polling failed")

type listener struct {
	cfg                    Config
	conn                   Connection
	router                 chains.Router
	bridgeContract         *Bridge.Bridge // instance of bound bridge contract
	erc20HandlerContract   *ERC20Handler.ERC20Handler
	erc721HandlerContract  *ERC721Handler.ERC721Handler
	genericHandlerContract *GenericHandler.GenericHandler
	log                    log15.Logger
	blockstore             blockstore.Blockstorer
	stop                   <-chan int
	sysErr                 chan<- error // Reports fatal error to core
	latestBlock            metrics.LatestBlock
	metrics                *metrics.ChainMetrics
	blockConfirmations     *big.Int
}

// NewListener creates and returns a listener
func NewListener(conn Connection, cfg *Config, log log15.Logger, bs blockstore.Blockstorer, stop <-chan int, sysErr chan<- error, m *metrics.ChainMetrics) *listener {
	return &listener{
		cfg:                *cfg,
		conn:               conn,
		log:                log,
		blockstore:         bs,
		stop:               stop,
		sysErr:             sysErr,
		latestBlock:        metrics.LatestBlock{LastUpdated: time.Now()},
		metrics:            m,
		blockConfirmations: cfg.blockConfirmations,
	}
}

// setContracts sets the listener with the appropriate contracts
func (l *listener) setContracts(bridge *Bridge.Bridge, erc20Handler *ERC20Handler.ERC20Handler, erc721Handler *ERC721Handler.ERC721Handler, genericHandler *GenericHandler.GenericHandler) {
	l.bridgeContract = bridge
	l.erc20HandlerContract = erc20Handler
	l.erc721HandlerContract = erc721Handler
	l.genericHandlerContract = genericHandler
}

// sets the router
func (l *listener) setRouter(r chains.Router) {
	l.router = r
}

// start registers all subscriptions provided by the config
func (l *listener) start() error {
	l.log.Debug("Starting listener...")

	go func() {
		err := l.pollBlocks()
		if err != nil {
			l.log.Error("Polling blocks failed", "err", err)
		}
	}()

	return nil
}

// pollBlocks will poll for the latest block and proceed to parse the associated events as it sees new blocks.
// Polling begins at the block defined in `l.cfg.startBlock`. Failed attempts to fetch the latest block or parse
// a block will be retried up to BlockRetryLimit times before continuing to the next block.
func (l *listener) pollBlocks() error {
	l.log.Info("Polling Blocks...")
	var currentBlock = l.cfg.startBlock
	l.log.Info("Polling Blocks...", "", currentBlock)
	var retry = BlockRetryLimit
	for {
		select {
		case <-l.stop:
			return errors.New("polling terminated")
		default:
			// No more retries, goto next block
			if retry == 0 {
				l.log.Error("Polling failed, retries exceeded")
				l.sysErr <- ErrFatalPolling
				return nil
			}

			latestBlock, err := l.conn.LatestBlock()
			if err != nil {
				errParam := []string{"Unable to get latest block", l.cfg.name}
				errMsg := strings.Join(errParam, " - ")
				l.log.Error(errParam[0], "block", currentBlock, "err", err)
				monitoring.Message(errMsg)
				retry--
				time.Sleep(BlockRetryInterval)
				continue
			}

			if l.metrics != nil {
				l.metrics.LatestKnownBlock.Set(float64(latestBlock.Int64()))
			}

			// Sleep if the difference is less than BlockDelay; (latest - current) < BlockDelay
			if big.NewInt(0).Sub(latestBlock, currentBlock).Cmp(l.blockConfirmations) == -1 {
				l.log.Debug("Block not ready, will retry", "target", currentBlock, "latest", latestBlock)
				time.Sleep(BlockRetryInterval)
				continue
			}

			//l.log.Info("Block", "latest", currentBlock, l.cfg.id)
			db.Instance.UpdateBlock(uint8(l.cfg.id), currentBlock.Uint64())

			// Parse out events
			txid, err := l.getDepositEventsForBlock(currentBlock)
			if err != nil {
				errParam := []string{"Failed to get events for block", l.cfg.name}
				errMsg := strings.Join(errParam, " - ")
				l.log.Error(errParam[0], "block", currentBlock, "err", err)
				monitoring.Message(errMsg)

				if txid != "" {
					db.Instance.UpdateErrOfDeposit(txid, err.Error())
				}

				retry--
				continue
			}

			// Write to block store. Not a critical operation, no need to retry
			err = l.blockstore.StoreBlock(currentBlock)
			if err != nil {
				l.log.Error("Failed to write latest block to blockstore", "block", currentBlock, "err", err)
			}

			if l.metrics != nil {
				l.metrics.BlocksProcessed.Inc()
				l.metrics.LatestProcessedBlock.Set(float64(latestBlock.Int64()))
			}

			l.latestBlock.Height = big.NewInt(0).Set(latestBlock)
			l.latestBlock.LastUpdated = time.Now()

			// Goto next block and reset retry counter
			currentBlock.Add(currentBlock, big.NewInt(1))
			retry = BlockRetryLimit
		}
	}
}

func passInterface(v interface{}) {
	b, ok := v.(*[]byte)
	fmt.Println(ok)
	fmt.Println(b)
}

func convertHexString(v interface{}) string {
	hexString := ""
	mySlice := []string{}

	switch reflect.TypeOf(v).Kind() {
	case reflect.Array, reflect.Slice:
		s := reflect.ValueOf(v)

		for i := 0; i < s.Len(); i++ {
			mySlice = append(mySlice, fmt.Sprintf("%02x", s.Index(i)))
		}

		hexString = strings.Join(mySlice, "")
	}
	return hexString
}

func hex2bigInt(hexStr string) *big.Int {
	i := new(big.Int)
	i.SetString(hexStr, 16)
	return i
}
func hex2int(hexStr string) uint64 {
	// remove 0x suffix if found in the input string
	//cleaned := strings.Replace(hexStr, "0x", "", -1)
	// base 16 for hexadecimal
	i, _ := strconv.ParseUint(hexStr, 16, 64)
	return i
}

// getDepositEventsForBlock looks for the deposit event in the latest block
func (l *listener) getDepositEventsForBlock(latestBlock *big.Int) (string, error) {
	l.log.Debug("Querying block for deposit events", "block", latestBlock)
	query := buildQuery(l.cfg.bridgeContract, utils.Deposit, latestBlock, latestBlock)

	// querying for logs
	logs, err := l.conn.Client().FilterLogs(context.Background(), query)
	if err != nil {
		return "", fmt.Errorf("unable to Filter Logs: %w", err)
	}

	//l.log.Info("logs", "", logs, "query", query)

	//path, _ := filepath.Abs("../abi/bridge.abi")
	//file, err := ioutil.ReadFile(path)
	//if err != nil {
	//	l.log.Error("Failed to read file:", "", err)
	//}
	bridgeAbi, err := abi.JSON(strings.NewReader(models.BridgeAbi))
	if err != nil {
		l.log.Error("Invalid abi:", "", err)
	}

	// read through the log events and handle their deposit event if handler is recognized
	for _, log := range logs {
		var m msg.Message
		l.log.Info("logs", "", logs, "log", log.Topics)

		logData, err := bridgeAbi.Unpack("Deposit", log.Data)
		if err != nil {
			l.log.Error("Failed to Unpack:", "", err)
		}

		//logData2, err := bridgeAbi.Events["Deposit"].Inputs.Unpack(log.Data)
		//l.log.Info("logs", "bridgeAbi", bridgeAbi)

		//l.log.Info("logs", "", logData)
		//l.log.Info("logs", "logData 0", logData[0], "type", reflect.TypeOf(logData[0]))
		//l.log.Info("logs", "logData 1", logData[1], "type", reflect.TypeOf(logData[1]))
		//l.log.Info("logs", "logData 2", logData[2], "type", reflect.TypeOf(logData[2]))
		//l.log.Info("logs", "logData 3", logData[3], "type", reflect.TypeOf(logData[3]))
		//l.log.Info("logs", "logData 4", logData[4])

		//unpackHex := hex.EncodeToString(log.Data)

		hexRId := convertHexString(logData[1])
		//hexRId := "00000000000000000000001193fbb9d0ca10b0f91ffa3a08a293d89f8d14e50a"
		byteRId, err := hex.DecodeString(hexRId)

		hexDepositData := convertHexString(logData[3])
		byteDepositData, err := hex.DecodeString(hexDepositData)

		if err != nil {
			fmt.Println("Unable to convert hex to byte. ", err)
		}

		//l.log.Info("logs", "hex string hexRId", hexRId)
		//l.log.Info("logs", "hex string hexDepositData", hexDepositData)
		//l.log.Info("logs", "hex string hexDepositData 1", hexDepositData[0:64], "to deciaml", hex2bigInt(hexDepositData[0:64]))
		//l.log.Info("logs", "hex string hexDepositData 2", hexDepositData[64:128], "to deciaml", hex2int(hexDepositData[64:128]))
		//l.log.Info("logs", "hex string hexDepositData 3", hexDepositData[128:])
		//l.log.Info("logs", "logData 1", ev.destinationDomainID)
		//l.log.Info("logs", "logData 2", ev.resourceID)
		//l.log.Info("logs", "logData 3", ev.depositNonce)
		//var buf bytes.Buffer
		//enc := gob.NewEncoder(&buf)
		//err = enc.Encode(logData[1])
		//if err != nil {
		//	l.log.Error("Failed Encode", "err", err)
		//}

		//var result []byte
		//err = json.Unmarshal([]byte(logData[1]), result)
		//if err != nil {
		//	l.log.Error("Failed Unmarshal", "err", err)
		//}
		//l.log.Info("logs", "result", result)

		destId := msg.ChainId(logData[0].(uint8))
		//rId := logData[1]
		//rId := msg.ResourceIdFromSlice(buf.Bytes())
		rId := msg.ResourceIdFromSlice(byteRId)
		nonce := msg.Nonce(logData[2].(uint64))

		//hexRecipientAddress := hexDepositData[128:]
		//amount := hex2bigInt(hexDepositData[0:64])
		//recipientAddress, err := hex.DecodeString(hexRecipientAddress)

		//encodedString := hex.EncodeToString(buf.Bytes())
		//destId := msg.ChainId(log.Topics[1].Big().Uint64())
		//rId := msg.ResourceIdFromSlice(log.Topics[2].Bytes())
		//nonce := msg.Nonce(log.Topics[3].Big().Uint64())
		//l.log.Info("logs", "rId", rId)

		//deposit := &models.Deposit{}
		//deposit.BlockNumber = log.BlockNumber
		//deposit.TXID = convertHexString(log.TxHash)
		//deposit.RId = hexRId
		//deposit.DepositData = hexDepositData
		//deposit.Source = uint8(l.cfg.id)
		//deposit.Dest = uint8(destId)
		//deposit.Nonce = uint64(nonce)

		txid := convertHexString(log.TxHash)

		err = db.Instance.InsertDeposit(log.BlockNumber, txid, hexRId, hexDepositData, uint8(l.cfg.id), uint8(destId), uint64(nonce))
		if err != nil {
			l.log.Error("failed insert deposit of db", "err", err)
		}

		addr, err := l.bridgeContract.ResourceIDToHandlerAddress(&bind.CallOpts{From: l.conn.Keypair().CommonAddress()}, rId)
		if err != nil {
			return txid, fmt.Errorf("failed to get handler from resource ID %x", rId)
		}

		//l.log.Info("logs", "erc20HandlerContract", addr)

		if addr == l.cfg.erc20HandlerContract {
			m, err = l.handleErc20DepositedEvent(txid, destId, rId, nonce, byteDepositData)
		} else if addr == l.cfg.erc721HandlerContract {
			m, err = l.handleErc721DepositedEvent(destId, nonce)
		} else if addr == l.cfg.genericHandlerContract {
			m, err = l.handleGenericDepositedEvent(destId, nonce)
		} else {
			l.log.Error("event has unrecognized handler", "handler", addr.Hex())
			return txid, fmt.Errorf("event has unrecognized handler %x", addr.Hex())
		}

		if err != nil {
			return txid, err
		}

		err = l.router.Send(m)
		if err != nil {
			l.log.Error("subscription error: failed to route message", "err", err)
			return txid, fmt.Errorf("event has unrecognized handler %x", addr.Hex())
		}
	}

	return "", nil
}

// buildQuery constructs a query for the bridgeContract by hashing sig to get the event topic
func buildQuery(contract ethcommon.Address, sig utils.EventSig, startBlock *big.Int, endBlock *big.Int) eth.FilterQuery {
	query := eth.FilterQuery{
		FromBlock: startBlock,
		ToBlock:   endBlock,
		Addresses: []ethcommon.Address{contract},
		Topics: [][]ethcommon.Hash{
			{sig.GetTopic()},
		},
	}
	return query
}
