package contract

import (
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

const TaskEscrowABIJSON = `[
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"},
			{"internalType": "uint256", "name": "amount", "type": "uint256"}
		],
		"name": "createEscrow",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"},
			{"internalType": "address[]", "name": "workers", "type": "address[]"},
			{"internalType": "uint256[]", "name": "shares", "type": "uint256[]"}
		],
		"name": "releasePayment",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"}
		],
		"name": "refund",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"}
		],
		"name": "dispute",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"},
			{"internalType": "bool", "name": "releaseToWorkers", "type": "bool"},
			{"internalType": "address[]", "name": "workers", "type": "address[]"},
			{"internalType": "uint256[]", "name": "shares", "type": "uint256[]"}
		],
		"name": "resolveDispute",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"}
		],
		"name": "getEscrow",
		"outputs": [
			{
				"components": [
					{"internalType": "string", "name": "taskId", "type": "string"},
					{"internalType": "address", "name": "publisher", "type": "address"},
					{"internalType": "uint256", "name": "amount", "type": "uint256"},
					{"internalType": "uint8", "name": "status", "type": "uint8"},
					{"internalType": "uint256", "name": "createdAt", "type": "uint256"}
				],
				"internalType": "struct TaskEscrow.Escrow",
				"name": "",
				"type": "tuple"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "string", "name": "taskId", "type": "string"}
		],
		"name": "taskHash",
		"outputs": [
			{"internalType": "bytes32", "name": "", "type": "bytes32"}
		],
		"stateMutability": "pure",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "platformFeeBps",
		"outputs": [
			{"internalType": "uint256", "name": "", "type": "uint256"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [],
		"name": "platformOperator",
		"outputs": [
			{"internalType": "address", "name": "", "type": "address"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "string", "name": "taskId", "type": "string"},
			{"indexed": true, "internalType": "address", "name": "publisher", "type": "address"},
			{"indexed": false, "internalType": "uint256", "name": "amount", "type": "uint256"}
		],
		"name": "EscrowCreated",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "string", "name": "taskId", "type": "string"},
			{"indexed": false, "internalType": "address[]", "name": "workers", "type": "address[]"},
			{"indexed": false, "internalType": "uint256[]", "name": "shares", "type": "uint256[]"}
		],
		"name": "PaymentReleased",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "string", "name": "taskId", "type": "string"},
			{"indexed": true, "internalType": "address", "name": "publisher", "type": "address"},
			{"indexed": false, "internalType": "uint256", "name": "amount", "type": "uint256"}
		],
		"name": "EscrowRefunded",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "string", "name": "taskId", "type": "string"},
			{"indexed": true, "internalType": "address", "name": "disputant", "type": "address"}
		],
		"name": "EscrowDisputed",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "string", "name": "taskId", "type": "string"},
			{"indexed": false, "internalType": "bool", "name": "releasedToWorkers", "type": "bool"}
		],
		"name": "DisputeResolved",
		"type": "event"
	}
]`

func ParseTaskEscrowABI() (abi.ABI, error) {
	return abi.JSON(strings.NewReader(TaskEscrowABIJSON))
}
