package service

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/agenthub/server/config"
	"github.com/agenthub/server/contract"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// OnChainEscrow represents escrow data read from the blockchain.
type OnChainEscrow struct {
	TaskID    string
	Publisher common.Address
	Amount    *big.Int
	Status    uint8 // 0=None, 1=Locked, 2=Released, 3=Refunded, 4=Disputed
	CreatedAt *big.Int
}

// EscrowCreatedEvent represents a parsed EscrowCreated log event.
type EscrowCreatedEvent struct {
	TaskID    string
	Publisher common.Address
	Amount    *big.Int
}

type BlockchainService struct {
	client       *ethclient.Client
	contractAddr common.Address
	contractABI  abi.ABI
	privateKey   *ecdsa.PrivateKey
	chainID      *big.Int
	enabled      bool
}

// NewBlockchainService creates a new BlockchainService. If configuration is
// incomplete (no contract address or private key), it returns a disabled
// instance that gracefully skips all on-chain operations.
func NewBlockchainService(cfg *config.Config) *BlockchainService {
	svc := &BlockchainService{}

	if cfg.ContractAddr == "" || cfg.PlatformPrivateKey == "" {
		log.Println("[blockchain] WARNING: contract address or private key not configured, on-chain operations disabled")
		return svc
	}

	client, err := ethclient.Dial(cfg.BSCRpcURL)
	if err != nil {
		log.Printf("[blockchain] WARNING: failed to connect to BSC RPC (%s): %v", cfg.BSCRpcURL, err)
		return svc
	}

	parsedABI, err := contract.ParseTaskEscrowABI()
	if err != nil {
		log.Printf("[blockchain] WARNING: failed to parse contract ABI: %v", err)
		return svc
	}

	privateKey, err := crypto.HexToECDSA(cfg.PlatformPrivateKey)
	if err != nil {
		log.Printf("[blockchain] WARNING: invalid platform private key: %v", err)
		return svc
	}

	svc.client = client
	svc.contractAddr = common.HexToAddress(cfg.ContractAddr)
	svc.contractABI = parsedABI
	svc.privateKey = privateKey
	svc.chainID = big.NewInt(cfg.ChainID)
	svc.enabled = true

	pubKey := crypto.PubkeyToAddress(privateKey.PublicKey)
	log.Printf("[blockchain] initialized — contract=%s operator=%s chainID=%d", cfg.ContractAddr, pubKey.Hex(), cfg.ChainID)

	return svc
}

// IsEnabled returns whether on-chain operations are available.
func (bs *BlockchainService) IsEnabled() bool {
	return bs.enabled
}

// VerifyEscrowTx checks that a transaction hash corresponds to a successful
// on-chain transaction and parses the EscrowCreated event from it.
func (bs *BlockchainService) VerifyEscrowTx(txHash string) (*EscrowCreatedEvent, error) {
	if !bs.enabled {
		return nil, fmt.Errorf("blockchain service not enabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hash := common.HexToHash(txHash)

	// Get transaction receipt
	receipt, err := bs.client.TransactionReceipt(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get tx receipt: %w", err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return nil, fmt.Errorf("transaction failed (status=%d)", receipt.Status)
	}

	// Parse EscrowCreated event from logs
	escrowCreatedEvent := bs.contractABI.Events["EscrowCreated"]
	for _, vLog := range receipt.Logs {
		if vLog.Address != bs.contractAddr {
			continue
		}
		if len(vLog.Topics) == 0 || vLog.Topics[0] != escrowCreatedEvent.ID {
			continue
		}

		// Decode non-indexed fields (amount)
		values, err := escrowCreatedEvent.Inputs.NonIndexed().Unpack(vLog.Data)
		if err != nil {
			continue
		}

		event := &EscrowCreatedEvent{}
		if len(values) > 0 {
			if amount, ok := values[0].(*big.Int); ok {
				event.Amount = amount
			}
		}

		// Topic[1] is indexed taskId hash (keccak256), Topic[2] is indexed publisher address
		if len(vLog.Topics) >= 3 {
			event.Publisher = common.BytesToAddress(vLog.Topics[2].Bytes())
		}

		return event, nil
	}

	return nil, fmt.Errorf("EscrowCreated event not found in transaction logs")
}

// GetEscrowOnChain reads the escrow state for a taskId directly from the contract.
func (bs *BlockchainService) GetEscrowOnChain(taskID string) (*OnChainEscrow, error) {
	if !bs.enabled {
		return nil, fmt.Errorf("blockchain service not enabled")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	callData, err := bs.contractABI.Pack("getEscrow", taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to pack getEscrow call: %w", err)
	}

	result, err := bs.client.CallContract(ctx, ethereum.CallMsg{
		To:   &bs.contractAddr,
		Data: callData,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call getEscrow: %w", err)
	}

	outputs, err := bs.contractABI.Methods["getEscrow"].Outputs.Unpack(result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack getEscrow result: %w", err)
	}

	if len(outputs) == 0 {
		return nil, fmt.Errorf("empty result from getEscrow")
	}

	// The result is a struct tuple
	type escrowTuple struct {
		TaskId    string         `abi:"taskId"`
		Publisher common.Address `abi:"publisher"`
		Amount    *big.Int       `abi:"amount"`
		Status    uint8          `abi:"status"`
		CreatedAt *big.Int       `abi:"createdAt"`
	}

	// Re-unpack using the struct approach
	var escrow escrowTuple
	err = bs.contractABI.Methods["getEscrow"].Outputs.Copy(&escrow, outputs)
	if err != nil {
		// Fallback: try manual extraction from the anonymous struct
		if s, ok := outputs[0].(struct {
			TaskId    string         `json:"taskId"`
			Publisher common.Address `json:"publisher"`
			Amount    *big.Int       `json:"amount"`
			Status    uint8          `json:"status"`
			CreatedAt *big.Int       `json:"createdAt"`
		}); ok {
			return &OnChainEscrow{
				TaskID:    s.TaskId,
				Publisher: s.Publisher,
				Amount:    s.Amount,
				Status:    s.Status,
				CreatedAt: s.CreatedAt,
			}, nil
		}
		return nil, fmt.Errorf("failed to decode escrow struct: %w", err)
	}

	return &OnChainEscrow{
		TaskID:    escrow.TaskId,
		Publisher: escrow.Publisher,
		Amount:    escrow.Amount,
		Status:    escrow.Status,
		CreatedAt: escrow.CreatedAt,
	}, nil
}

// ReleasePayment calls the contract's releasePayment function using the
// platform operator wallet.
func (bs *BlockchainService) ReleasePayment(taskID string, workers []common.Address, shares []*big.Int) (string, error) {
	if !bs.enabled {
		return "", fmt.Errorf("blockchain service not enabled")
	}

	txData, err := bs.contractABI.Pack("releasePayment", taskID, workers, shares)
	if err != nil {
		return "", fmt.Errorf("failed to pack releasePayment: %w", err)
	}

	txHash, err := bs.sendTx(txData)
	if err != nil {
		return "", fmt.Errorf("releasePayment tx failed: %w", err)
	}

	return txHash, nil
}

// DisputeOnChain calls the contract's dispute function.
func (bs *BlockchainService) DisputeOnChain(taskID string) (string, error) {
	if !bs.enabled {
		return "", fmt.Errorf("blockchain service not enabled")
	}

	txData, err := bs.contractABI.Pack("dispute", taskID)
	if err != nil {
		return "", fmt.Errorf("failed to pack dispute: %w", err)
	}

	txHash, err := bs.sendTx(txData)
	if err != nil {
		return "", fmt.Errorf("dispute tx failed: %w", err)
	}

	return txHash, nil
}

// RefundOnChain calls the contract's refund function.
func (bs *BlockchainService) RefundOnChain(taskID string) (string, error) {
	if !bs.enabled {
		return "", fmt.Errorf("blockchain service not enabled")
	}

	txData, err := bs.contractABI.Pack("refund", taskID)
	if err != nil {
		return "", fmt.Errorf("failed to pack refund: %w", err)
	}

	txHash, err := bs.sendTx(txData)
	if err != nil {
		return "", fmt.Errorf("refund tx failed: %w", err)
	}

	return txHash, nil
}

// sendTx is a helper that builds, signs, and sends a transaction to the contract.
func (bs *BlockchainService) sendTx(data []byte) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fromAddress := crypto.PubkeyToAddress(bs.privateKey.PublicKey)

	nonce, err := bs.client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return "", fmt.Errorf("failed to get nonce: %w", err)
	}

	gasPrice, err := bs.client.SuggestGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get gas price: %w", err)
	}

	gasLimit, err := bs.client.EstimateGas(ctx, ethereum.CallMsg{
		From: fromAddress,
		To:   &bs.contractAddr,
		Data: data,
	})
	if err != nil {
		return "", fmt.Errorf("failed to estimate gas: %w", err)
	}

	tx := types.NewTransaction(nonce, bs.contractAddr, big.NewInt(0), gasLimit, gasPrice, data)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(bs.chainID), bs.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign tx: %w", err)
	}

	err = bs.client.SendTransaction(ctx, signedTx)
	if err != nil {
		return "", fmt.Errorf("failed to send tx: %w", err)
	}

	txHash := signedTx.Hash().Hex()
	log.Printf("[blockchain] tx sent: %s", txHash)

	// Wait for confirmation
	receipt, err := bind.WaitMined(ctx, bs.client, signedTx)
	if err != nil {
		return txHash, fmt.Errorf("tx sent (%s) but confirmation failed: %w", txHash, err)
	}

	if receipt.Status != types.ReceiptStatusSuccessful {
		return txHash, fmt.Errorf("tx reverted: %s", txHash)
	}

	log.Printf("[blockchain] tx confirmed: %s (gas used: %d)", txHash, receipt.GasUsed)
	return txHash, nil
}
