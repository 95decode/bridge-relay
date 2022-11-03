/*
The ethereum package contains the logic for interacting with ethereum chains.

There are 3 major components: the connection, the listener, and the writer.
The currently supported transfer types are Fungible (ERC20), Non-Fungible (ERC721), and generic.

Connection

The connection contains the ethereum RPC client and can be accessed by both the writer and listener.

Listener

The listener polls for each new block and looks for deposit events in the bridge contract. If a deposit occurs, the listener will fetch additional information from the handler before constructing a message and forwarding it to the router.

Writer

The writer recieves the message and creates a proposals on-chain. Once a proposal is made, the writer then watches for a finalization event and will attempt to execute the proposal if a matching event occurs. The writer skips over any proposals it has already seen.
*/
package ethereum

import (
	"fmt"
	"hexbridge/utils/blockstore"
	"hexbridge/utils/core"
	"hexbridge/utils/crypto/secp256k1"
	"hexbridge/utils/keystore"
	"hexbridge/utils/msg"
	"math/big"

	"github.com/ChainSafe/log15"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	bridge "hexbridge/bindings/Bridge"
	erc20Handler "hexbridge/bindings/ERC20Handler"
	erc721Handler "hexbridge/bindings/ERC721Handler"
	"hexbridge/bindings/GenericHandler"
	connection "hexbridge/connections/ethereum"
	metrics "hexbridge/utils/metrics/types"
)

var _ core.Chain = &Chain{}

var _ Connection = &connection.Connection{}

type Connection interface {
	Connect() error
	Keypair() *secp256k1.Keypair
	Opts() *bind.TransactOpts
	CallOpts() *bind.CallOpts
	LockAndUpdateOpts() error
	UnlockOpts()
	Client() *ethclient.Client
	EnsureHasBytecode(address common.Address) error
	LatestBlock() (*big.Int, error)
	WaitForBlock(block *big.Int, delay *big.Int) error
	Close()
}

type Chain struct {
	cfg      *core.ChainConfig // The config of the chain
	conn     Connection        // THe chains connection
	listener *listener         // The listener of this chain
	writer   *writer           // The writer of the chain
	stop     chan<- int
}

// checkBlockstore queries the blockstore for the latest known block. If the latest block is
// greater than cfg.startBlock, then cfg.startBlock is replaced with the latest known block.
func setupBlockstore(cfg *Config, kp *secp256k1.Keypair) (*blockstore.Blockstore, error) {
	bs, err := blockstore.NewBlockstore(cfg.blockstorePath, cfg.id, kp.Address())
	if err != nil {
		return nil, err
	}

	if !cfg.freshStart {
		latestBlock, err := bs.TryLoadLatestBlock()
		if err != nil {
			return nil, err
		}

		if latestBlock.Cmp(cfg.startBlock) == 1 {
			cfg.startBlock = latestBlock
		}
	}

	return bs, nil
}

func InitializeChain(chainCfg *core.ChainConfig, logger log15.Logger, sysErr chan<- error, m *metrics.ChainMetrics) (*Chain, error) {
	cfg, err := parseChainConfig(chainCfg)
	if err != nil {
		return nil, err
	}

	logger.Info("InitializeChain Enter")

	kpI, err := keystore.KeypairFromAddress(cfg.from, keystore.EthChain, cfg.keystorePath, chainCfg.Insecure)
	if err != nil {
		return nil, err
	}
	kp, _ := kpI.(*secp256k1.Keypair)

	bs, err := setupBlockstore(cfg, kp)
	if err != nil {
		logger.Error("setupBlockstore Error", err)
		return nil, err
	}

	stop := make(chan int)
	conn := connection.NewConnection(cfg.endpoint, cfg.http, kp, logger, cfg.gasLimit, cfg.maxGasPrice, cfg.gasMultiplier)
	err = conn.Connect()
	if err != nil {
		logger.Error("000 Error", err)
		return nil, err
	}
	err = conn.EnsureHasBytecode(cfg.bridgeContract)
	if err != nil {
		logger.Error("EnsureHasBytecode 1", "Error", err)
		return nil, err
	}
	err = conn.EnsureHasBytecode(cfg.erc20HandlerContract)
	if err != nil {
		logger.Error("EnsureHasBytecode 2", "Error", err)
		return nil, err
	}
	err = conn.EnsureHasBytecode(cfg.genericHandlerContract)
	if err != nil {
		logger.Error("EnsureHasBytecode 3", "Error", err)
		return nil, err
	}

	bridgeContract, err := bridge.NewBridge(cfg.bridgeContract, conn.Client())
	if err != nil {
		logger.Error("NewBridge 4", "Error", err)
		return nil, err
	}

	chainId, err := bridgeContract.DomainID(conn.CallOpts())
	if err != nil {
		logger.Error("DomainID 5", "Error ", err)
		return nil, err
	}

	if chainId != uint8(chainCfg.Id) {
		return nil, fmt.Errorf("chainId (%d) and configuration chainId (%d) do not match", chainId, chainCfg.Id)
	}

	erc20HandlerContract, err := erc20Handler.NewERC20Handler(cfg.erc20HandlerContract, conn.Client())
	if err != nil {
		logger.Error("erc20HandlerContract Error", err)
		return nil, err
	}

	erc721HandlerContract, err := erc721Handler.NewERC721Handler(cfg.erc721HandlerContract, conn.Client())
	if err != nil {
		logger.Error("erc721HandlerContract Error", err)
		return nil, err
	}

	genericHandlerContract, err := GenericHandler.NewGenericHandler(cfg.genericHandlerContract, conn.Client())
	if err != nil {
		logger.Error("genericHandlerContract Error", err)
		return nil, err
	}

	if chainCfg.LatestBlock {
		curr, err := conn.LatestBlock()
		if err != nil {
			logger.Error("999 Error", err)
			return nil, err
		}
		cfg.startBlock = curr
	}

	listener := NewListener(conn, cfg, logger, bs, stop, sysErr, m)
	listener.setContracts(bridgeContract, erc20HandlerContract, erc721HandlerContract, genericHandlerContract)

	writer := NewWriter(conn, cfg, logger, stop, sysErr, m)
	writer.setContract(bridgeContract)

	return &Chain{
		cfg:      chainCfg,
		conn:     conn,
		writer:   writer,
		listener: listener,
		stop:     stop,
	}, nil
}

func (c *Chain) SetRouter(r *core.Router) {
	r.Listen(c.cfg.Id, c.writer)
	c.listener.setRouter(r)
}

func (c *Chain) Start() error {
	err := c.listener.start()
	if err != nil {
		return err
	}

	err = c.writer.start()
	if err != nil {
		return err
	}

	c.writer.log.Debug("Successfully started chain")
	return nil
}

func (c *Chain) Id() msg.ChainId {
	return c.cfg.Id
}

func (c *Chain) Name() string {
	return c.cfg.Name
}

func (c *Chain) LatestBlock() metrics.LatestBlock {
	return c.listener.latestBlock
}

// Stop signals to any running routines to exit
func (c *Chain) Stop() {
	close(c.stop)
	if c.conn != nil {
		c.conn.Close()
	}
}
