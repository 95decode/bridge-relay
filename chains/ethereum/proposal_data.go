package ethereum

import (
	"github.com/ChainSafe/log15"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
)

func signingKey(hexPrivatekey string) {
	//privateKey, err := crypto.HexToECDSA(hexPrivateKey[2:])
	//if err != nil {
	//	log.Fatal(err)
	//}

	// keccak256 hash of the data
	//dataBytes := []byte(dataToSign)
	//hashData := crypto.Keccak256Hash(dataBytes)
	//
	//signatureBytes, err := crypto.Sign(hashData.Bytes(), privateKey)
	//
	//if err != nil {
	//	log.Fatal(err)
	//}
	//
	//signature = hexutil.Encode(signatureBytes)
}

//func signDataWithMpc(originDomainID uint8, destinationDomainID uint8, depositNonce uint8, depositData []byte, resourceID [32]byte) {
//
//	const signingKey = new Ethers.utils.SigningKey(mpcPrivateKey)
//	//console.log("private key = " + mpcPrivateKey)
//
//	crypto.Keccak256()
//
//	const messageHash = Ethers.utils.solidityKeccak256(
//	['uint8', 'uint8', 'uint64', 'bytes', 'bytes32'],
//	[originDomainID, destinationDomainID, depositNonce, depositData, resourceID]
//	);
//
//	const signature = signingKey.signDigest(messageHash)
//	const rawSignature = Ethers.utils.joinSignature(signature)
//	return rawSignature
//
//}

// constructErc20ProposalData returns the bytes to construct a proposal suitable for Erc20
func ConstructErc20ProposalData(depositData []byte) []byte {
	var data []byte

	log15.Info("ConstructErc20ProposalData", "depositData", depositData)
	//data = append(data, common.LeftPadBytes(amount, 32)...) // amount (uint256)

	//keccak256 hash of the data

	//dataBytes := []byte(dataToSign)
	//hashData := crypto.Keccak256Hash(dataBytes)
	//
	//signatureBytes, err := crypto.Sign(hashData.Bytes(), privateKey)

	//recipientLen := big.NewInt(int64(len(recipient))).Bytes()
	//data = append(data, common.LeftPadBytes(recipientLen, 32)...) // length of recipient (uint256)
	//data = append(data, recipient...)                             // recipient ([]byte)
	return data
}

// constructErc721ProposalData returns the bytes to construct a proposal suitable for Erc721
func ConstructErc721ProposalData(tokenId []byte, recipient []byte, metadata []byte) []byte {
	var data []byte
	data = append(data, common.LeftPadBytes(tokenId, 32)...) // tokenId ([]byte)

	recipientLen := big.NewInt(int64(len(recipient))).Bytes()
	data = append(data, common.LeftPadBytes(recipientLen, 32)...) // length of recipient
	data = append(data, recipient...)                             // recipient ([]byte)

	metadataLen := big.NewInt(int64(len(metadata))).Bytes()
	data = append(data, common.LeftPadBytes(metadataLen, 32)...) // length of metadata (uint256)
	data = append(data, metadata...)                             // metadata ([]byte)
	return data
}

// constructGenericProposalData returns the bytes to construct a generic proposal
func ConstructGenericProposalData(metadata []byte) []byte {
	var data []byte

	metadataLen := big.NewInt(int64(len(metadata)))
	data = append(data, math.PaddedBigBytes(metadataLen, 32)...) // length of metadata (uint256)
	data = append(data, metadata...)                             // metadata ([]byte)
	return data
}
