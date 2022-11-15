package ethereum

import (
	"errors"
	"fmt"
	"hexbridge/db"
	"hexbridge/utils/msg"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	solsha3 "github.com/miguelmota/go-solidity-sha3"

	utils "hexbridge/shared/ethereum"

	monitoring "hexbridge/utils/monitoring"
)

// Number of blocks to wait for an finalization event
const ExecuteBlockWatchLimit = 100

// Time between retrying a failed tx
const TxRetryInterval = time.Second * 10

// Maximum number of tx retries before exiting
const TxRetryLimit = 10

var ErrNonceTooLow = errors.New("nonce too low")
var ErrTxUnderpriced = errors.New("replacement transaction underpriced")
var ErrFatalTx = errors.New("submission of transaction failed")
var ErrFatalQuery = errors.New("query of chain state failed")

// proposalIsComplete returns true if the proposal state is either Passed, Transferred or Cancelled
func (w *writer) proposalIsComplete(srcId msg.ChainId, nonce msg.Nonce, dataHash [32]byte) bool {
	prop, err := w.bridgeContract.GetProposal(w.conn.CallOpts(), uint8(srcId), uint64(nonce), dataHash)
	if err != nil {
		w.log.Error("Failed to check proposal existence 1", "err", err)
		return false
	}
	return prop.Status == PassedStatus || prop.Status == TransferredStatus || prop.Status == CancelledStatus
}

// proposalIsComplete returns true if the proposal state is Transferred or Cancelled
func (w *writer) proposalIsFinalized(srcId msg.ChainId, nonce msg.Nonce, dataHash [32]byte) bool {
	prop, err := w.bridgeContract.GetProposal(w.conn.CallOpts(), uint8(srcId), uint64(nonce), dataHash)
	if err != nil {
		w.log.Error("Failed to check proposal existence 2", "err", err)
		return false
	}
	return prop.Status == TransferredStatus || prop.Status == CancelledStatus // Transferred (3)
}

func (w *writer) proposalIsPassed(srcId msg.ChainId, nonce msg.Nonce, dataHash [32]byte) bool {
	prop, err := w.bridgeContract.GetProposal(w.conn.CallOpts(), uint8(srcId), uint64(nonce), dataHash)
	if err != nil {
		w.log.Error("Failed to check proposal existence 3", "err", err)
		return false
	}
	return prop.Status == PassedStatus
}

// hasVoted checks if this relayer has already voted
func (w *writer) hasVoted(srcId msg.ChainId, nonce msg.Nonce, dataHash [32]byte) bool {
	hasVoted, err := w.bridgeContract.HasVotedOnProposal(w.conn.CallOpts(), utils.IDAndNonce(srcId, nonce), dataHash, w.conn.Opts().From)
	if err != nil {
		w.log.Error("Failed to check proposal existence 4", "err", err)
		return false
	}

	return hasVoted
}

func (w *writer) shouldVote(m msg.Message, dataHash [32]byte) bool {
	// Check if proposal has passed and skip if Passed or Transferred
	if w.proposalIsComplete(m.Source, m.DepositNonce, dataHash) {
		w.log.Info("Proposal complete, not voting", "src", m.Source, "nonce", m.DepositNonce)
		return false
	}

	// Check if relayer has previously voted
	if w.hasVoted(m.Source, m.DepositNonce, dataHash) {
		w.log.Info("Relayer has already voted, not voting", "src", m.Source, "nonce", m.DepositNonce)
		return false
	}

	return true
}

func signDataWithMpc(mpcPrivateKey string, originDomainID uint8, destinationDomainID uint8, depositNonce uint64, resourceID [32]byte) []byte {

	//hexResourceId := convertHexString(m.ResourceId)
	//hexDepositData := convertHexString(depositData)
	messageHash := solsha3.SoliditySHA3(
		// types
		[]string{"uint8", "uint8", "uint64", "bytes32"},
		// values
		[]interface{}{
			originDomainID,
			destinationDomainID,
			depositNonce,
			resourceID,
		},
	)

	//hexMessageHash := hex.EncodeToString(messageHash)
	//w.log.Info("signDataWithMpc", "hexMessageHash", hexMessageHash)
	privateKey, err := crypto.HexToECDSA(mpcPrivateKey)
	if err != nil {
		fmt.Errorf("signDataWithMpc crypto.HexToECDSA : %w", err)
	}
	signatureBytes, err := crypto.Sign(messageHash, privateKey)
	if err != nil {
		fmt.Errorf("signDataWithMpc crypto.Sign : %w", err)
	}

	if signatureBytes[64] == 0 || signatureBytes[64] == 1 {
		signatureBytes[64] += 27
	}
	return signatureBytes

	//signature := hex.EncodeToString(signatureBytes)
	//w.log.Info("signDataWithMpc", "rawSignature", signature)
}

// createErc20Proposal creates an Erc20 proposal.
// Returns true if the proposal is successfully created or is complete
func (w *writer) executeErc20Proposal(m msg.Message) bool {
	//w.log.Info("Creating erc20 proposal", "src", m.Source, "nonce", m.DepositNonce)

	//w.log.Info("message", "depositData", m.Payload[0].([]byte))

	data := m.Payload[0].([]byte)
	txid := m.Payload[1].(string)
	//data := ConstructErc20ProposalData(m.Payload[0].([]byte))
	//dataHash := utils.Hash(append(w.cfg.erc20HandlerContract.Bytes(), data...))

	//if !w.shouldVote(m, dataHash) {
	//	if w.proposalIsPassed(m.Source, m.DepositNonce, dataHash) {
	//		// We should not vote for this proposal but it is ready to be executed
	//		w.executeProposal(m, data, dataHash)
	//		return true
	//	} else {
	//		return false
	//	}
	//}

	var mpcPrivateKey = ""

	if w.cfg.id == 11 {
		mpcPrivateKey = ""
	} else {
		mpcPrivateKey = ""
	}

	signatureBytes := signDataWithMpc(mpcPrivateKey, uint8(m.Source), uint8(w.cfg.id), uint64(m.DepositNonce), m.ResourceId)
	//w.log.Info("Creating erc20 proposal", "signature", hex.EncodeToString(signatureBytes))
	w.executeProposal(txid, m, data, signatureBytes)

	// Capture latest block so when know where to watch from
	latestBlock, err := w.conn.LatestBlock()
	w.log.Info("Creating erc20 proposal", "latestBlock", latestBlock)
	if err != nil {
		w.log.Error("Unable to fetch latest block", "err", err)
		return false
	}

	// watch for execution event
	//go w.watchThenExecute(m, data, dataHash, latestBlock)

	//w.voteProposal(m, dataHash)

	return true
}

// createErc721Proposal creates an Erc721 proposal.
// Returns true if the proposal is succesfully created or is complete
func (w *writer) createErc721Proposal(m msg.Message) bool {
	w.log.Info("Creating erc721 proposal", "src", m.Source, "nonce", m.DepositNonce)

	data := ConstructErc721ProposalData(m.Payload[0].([]byte), m.Payload[1].([]byte), m.Payload[2].([]byte))
	dataHash := utils.Hash(append(w.cfg.erc721HandlerContract.Bytes(), data...))

	if !w.shouldVote(m, dataHash) {
		if w.proposalIsPassed(m.Source, m.DepositNonce, dataHash) {
			// We should not vote for this proposal but it is ready to be executed
			w.executeProposal("", m, data, data)
			return true
		} else {
			return false
		}
	}

	// Capture latest block so we know where to watch from
	latestBlock, err := w.conn.LatestBlock()
	w.log.Info("Creating erc721 proposal", "latestBlock", latestBlock)

	if err != nil {
		w.log.Error("Unable to fetch latest block", "err", err)
		return false
	}

	// watch for execution event
	//go w.watchThenExecute(m, data, dataHash, latestBlock)
	//
	//w.voteProposal(m, dataHash)

	return true
}

// createGenericDepositProposal creates a generic proposal
// returns true if the proposal is complete or is succesfully created
func (w *writer) createGenericDepositProposal(m msg.Message) bool {
	w.log.Info("Creating generic proposal", "src", m.Source, "nonce", m.DepositNonce)

	metadata := m.Payload[0].([]byte)
	data := ConstructGenericProposalData(metadata)
	toHash := append(w.cfg.genericHandlerContract.Bytes(), data...)
	dataHash := utils.Hash(toHash)

	if !w.shouldVote(m, dataHash) {
		if w.proposalIsPassed(m.Source, m.DepositNonce, dataHash) {
			// We should not vote for this proposal but it is ready to be executed
			w.executeProposal("", m, data, data)
			return true
		} else {
			return false
		}
	}

	// Capture latest block so when know where to watch from
	latestBlock, err := w.conn.LatestBlock()
	w.log.Info("Creating generic proposal", "latestBlock", latestBlock)
	if err != nil {
		w.log.Error("Unable to fetch latest block", "err", err)
		return false
	}

	// watch for execution event
	//go w.watchThenExecute(m, data, dataHash, latestBlock)
	//
	//w.voteProposal(m, dataHash)

	return true
}

//// watchThenExecute watches for the latest block and executes once the matching finalized event is found
//func (w *writer) watchThenExecute(m msg.Message, data []byte, dataHash [32]byte, latestBlock *big.Int) {
//	w.log.Info("Watching for finalization event", "src", m.Source, "nonce", m.DepositNonce)
//
//	// watching for the latest block, querying and matching the finalized event will be retried up to ExecuteBlockWatchLimit times
//	for i := 0; i < ExecuteBlockWatchLimit; i++ {
//		select {
//		case <-w.stop:
//			return
//		default:
//			// watch for the lastest block, retry up to BlockRetryLimit times
//			for waitRetrys := 0; waitRetrys < BlockRetryLimit; waitRetrys++ {
//				err := w.conn.WaitForBlock(latestBlock, w.cfg.blockConfirmations)
//				if err != nil {
//					w.log.Error("Waiting for block failed", "err", err)
//					// Exit if retries exceeded
//					if waitRetrys+1 == BlockRetryLimit {
//						w.log.Error("Waiting for block retries exceeded, shutting down")
//						w.sysErr <- ErrFatalQuery
//						return
//					}
//				} else {
//					break
//				}
//			}
//
//			// query for logs
//			query := buildQuery(w.cfg.bridgeContract, utils.ProposalEvent, latestBlock, latestBlock)
//			evts, err := w.conn.Client().FilterLogs(context.Background(), query)
//			if err != nil {
//				w.log.Error("Failed to fetch logs", "err", err)
//				return
//			}
//
//			// execute the proposal once we find the matching finalized event
//			for _, evt := range evts {
//				sourceId := evt.Topics[1].Big().Uint64()
//				depositNonce := evt.Topics[2].Big().Uint64()
//				status := evt.Topics[3].Big().Uint64()
//
//				if m.Source == msg.ChainId(sourceId) &&
//					m.DepositNonce.Big().Uint64() == depositNonce &&
//					utils.IsFinalized(uint8(status)) {
//					w.executeProposal(m, data, dataHash)
//					return
//				} else {
//					w.log.Trace("Ignoring event", "src", sourceId, "nonce", depositNonce)
//				}
//			}
//			w.log.Trace("No finalization event found in current block", "block", latestBlock, "src", m.Source, "nonce", m.DepositNonce)
//			latestBlock = latestBlock.Add(latestBlock, big.NewInt(1))
//		}
//	}
//	log.Warn("Block watch limit exceeded, skipping execution", "source", m.Source, "dest", m.Destination, "nonce", m.DepositNonce)
//}

//// voteProposal submits a vote proposal
//// a vote proposal will try to be submitted up to the TxRetryLimit times
//func (w *writer) voteProposal(m msg.Message, dataHash [32]byte) {
//	for i := 0; i < TxRetryLimit; i++ {
//		select {
//		case <-w.stop:
//			return
//		default:
//			err := w.conn.LockAndUpdateOpts()
//			if err != nil {
//				w.log.Error("Failed to update tx opts", "err", err)
//				continue
//			}
//
//			tx, err := w.bridgeContract.VoteProposal(
//				w.conn.Opts(),
//				uint8(m.Source),
//				uint64(m.DepositNonce),
//				m.ResourceId,
//				dataHash,
//			)
//			w.conn.UnlockOpts()
//
//			if err == nil {
//				w.log.Info("Submitted proposal vote", "tx", tx.Hash(), "src", m.Source, "depositNonce", m.DepositNonce)
//				if w.metrics != nil {
//					w.metrics.VotesSubmitted.Inc()
//				}
//				return
//			} else if err.Error() == ErrNonceTooLow.Error() || err.Error() == ErrTxUnderpriced.Error() {
//				w.log.Debug("Nonce too low, will retry")
//				time.Sleep(TxRetryInterval)
//			} else {
//				w.log.Warn("Voting failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce, "err", err)
//				time.Sleep(TxRetryInterval)
//			}
//
//			// Verify proposal is still open for voting, otherwise no need to retry
//			if w.proposalIsComplete(m.Source, m.DepositNonce, dataHash) {
//				w.log.Info("Proposal voting complete on chain", "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce)
//				return
//			}
//		}
//	}
//	w.log.Error("Submission of Vote transaction failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce)
//	w.sysErr <- ErrFatalTx
//}

// executeProposal executes the proposal
func (w *writer) executeProposal(txid string, m msg.Message, depositData []byte, signatureBytes []byte) {

	for i := 0; i < TxRetryLimit; i++ {
		select {
		case <-w.stop:
			return
		default:
			err := w.conn.LockAndUpdateOpts()
			if err != nil {
				w.log.Error("Failed to update nonce", "err", err)
				return
			}

			w.log.Info("executeProposal info", "originDomainID", uint8(m.Source), "destinationDomainID", uint8(w.cfg.id),
				"depositNonce", uint64(m.DepositNonce))

			tx, err := w.bridgeContract.ExecuteProposal(
				w.conn.Opts(),
				uint8(m.Source),
				uint64(m.DepositNonce),
				depositData,
				m.ResourceId,
				signatureBytes,
			)
			w.conn.UnlockOpts()

			if err == nil {
				//w.log.Info("insert proposal txid of deposit", "tx", txid)
				err := db.Instance.InsertProposal(txid, convertHexString(tx.Hash()), m.ResourceId, depositData, signatureBytes, uint8(m.Source), uint8(w.cfg.id), uint64(m.DepositNonce), "")
				if err != nil {
					w.log.Error("failed insert deposit of db", "err", err)
				}
				w.log.Info("Submitted proposal execution", "tx", tx.Hash(), "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce)
				return
			} else if err.Error() == ErrNonceTooLow.Error() || err.Error() == ErrTxUnderpriced.Error() {
				errParam := []string{"Nonce too low, will retry", w.cfg.name}
				if TxRetryLimit == 8 {
					errMsg := strings.Join(errParam, " - ")
					monitoring.Message(errMsg)
				}

				w.log.Error(errParam[0])
				time.Sleep(TxRetryInterval)
			} else {
				errParam := []string{"Execution failed, proposal may already be complete", w.cfg.name}
				if TxRetryLimit == 8 {
					errMsg := strings.Join(errParam, " - ")
					monitoring.Message(errMsg)
				}

				w.log.Warn(errParam[0], "err", err)
				time.Sleep(TxRetryInterval)
			}

			err = db.Instance.InsertProposal(txid, "", m.ResourceId, depositData, signatureBytes, uint8(m.Source), uint8(w.cfg.id), uint64(m.DepositNonce), err.Error())
			if err != nil {
				w.log.Error("failed insert deposit of db", "err", err)
			}

			//// Verify proposal is still open for execution, tx will fail if we aren't the first to execute,
			//// but there is no need to retry
			//if w.proposalIsFinalized(m.Source, m.DepositNonce, dataHash) {
			//	w.log.Info("Proposal finalized on chain", "src", m.Source, "dst", m.Destination, "nonce", m.DepositNonce)
			//	return
			//}
		}
	}
	w.log.Error("Submission of Execute transaction failed", "source", m.Source, "dest", m.Destination, "depositNonce", m.DepositNonce)

	//err := db.InsertProposal(txid, txid, m.ResourceId, depositData, signatureBytes, uint8(m.Source), uint8(w.cfg.id), uint64(m.DepositNonce))
	//if err != nil {
	//	w.log.Error("failed insert deposit of db", "err", err)
	//}

	w.sysErr <- ErrFatalTx
}
