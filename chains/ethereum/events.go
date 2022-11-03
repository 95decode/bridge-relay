package ethereum

import (
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"hexbridge/utils/msg"
)

func (l *listener) handleErc20DepositedEvent(txid string, destId msg.ChainId, rId msg.ResourceId, nonce msg.Nonce, depositData []byte) (msg.Message, error) {
	l.log.Info("Handling fungible deposit event", "dest", destId, "nonce", nonce)

	//record, err := l.erc20HandlerContract.GetDepositRecord(&bind.CallOpts{From: l.conn.Keypair().CommonAddress()}, uint64(nonce), uint8(destId))
	//if err != nil {
	//	l.log.Error("Error Unpacking ERC20 Deposit Record", "err", err)
	//	return msg.Message{}, err
	//}

	return msg.NewFungibleTransfer(
		txid,
		l.cfg.id,
		destId,
		nonce,
		rId,
		depositData,
	), nil
}

func (l *listener) handleErc721DepositedEvent(destId msg.ChainId, nonce msg.Nonce) (msg.Message, error) {
	l.log.Info("Handling nonfungible deposit event")

	record, err := l.erc721HandlerContract.GetDepositRecord(&bind.CallOpts{From: l.conn.Keypair().CommonAddress()}, uint64(nonce), uint8(destId))
	if err != nil {
		l.log.Error("Error Unpacking ERC721 Deposit Record", "err", err)
		return msg.Message{}, err
	}

	return msg.NewNonFungibleTransfer(
		l.cfg.id,
		destId,
		nonce,
		record.ResourceID,
		record.TokenID,
		record.DestinationRecipientAddress,
		record.MetaData,
	), nil
}

func (l *listener) handleGenericDepositedEvent(destId msg.ChainId, nonce msg.Nonce) (msg.Message, error) {
	l.log.Info("Handling generic deposit event")

	record, err := l.genericHandlerContract.GetDepositRecord(&bind.CallOpts{From: l.conn.Keypair().CommonAddress()}, uint64(nonce), uint8(destId))
	if err != nil {
		l.log.Error("Error Unpacking Generic Deposit Record", "err", err)
		return msg.Message{}, nil
	}

	return msg.NewGenericTransfer(
		l.cfg.id,
		destId,
		nonce,
		record.ResourceID,
		record.MetaData[:],
	), nil
}
