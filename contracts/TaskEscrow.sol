// SPDX-License-Identifier: MIT
pragma solidity ^0.8.19;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

/**
 * @title TaskEscrow
 * @notice Escrow contract for ClawWork task payments on BSC using USDT (BEP-20)
 */
contract TaskEscrow is Ownable, ReentrancyGuard {
    using SafeERC20 for IERC20;

    IERC20 public immutable usdt;

    uint256 public platformFeeBps = 200; // 2% = 200 basis points
    uint256 public constant MAX_FEE_BPS = 1000; // max 10%
    address public feeRecipient;
    address public platformOperator;

    enum EscrowStatus {
        None,
        Locked,
        Released,
        Refunded,
        Disputed
    }

    struct Escrow {
        string taskId;
        address publisher;
        uint256 amount;
        EscrowStatus status;
        uint256 createdAt;
    }

    // taskId hash => Escrow
    mapping(bytes32 => Escrow) public escrows;

    event EscrowCreated(string indexed taskId, address indexed publisher, uint256 amount);
    event PaymentReleased(string indexed taskId, address[] workers, uint256[] shares);
    event EscrowRefunded(string indexed taskId, address indexed publisher, uint256 amount);
    event EscrowDisputed(string indexed taskId, address indexed disputant);
    event DisputeResolved(string indexed taskId, bool releasedToWorkers);
    event PlatformFeeUpdated(uint256 oldFee, uint256 newFee);

    modifier onlyPublisher(bytes32 taskHash) {
        require(escrows[taskHash].publisher == msg.sender, "Not the publisher");
        _;
    }

    modifier onlyPublisherOrOperator(bytes32 taskHash) {
        require(
            escrows[taskHash].publisher == msg.sender || msg.sender == platformOperator,
            "Not publisher or operator"
        );
        _;
    }

    constructor(address _usdt, address _feeRecipient, address _operator) Ownable(msg.sender) {
        require(_usdt != address(0), "Invalid USDT address");
        require(_feeRecipient != address(0), "Invalid fee recipient");
        require(_operator != address(0), "Invalid operator address");
        usdt = IERC20(_usdt);
        feeRecipient = _feeRecipient;
        platformOperator = _operator;
    }

    function setPlatformOperator(address _operator) external onlyOwner {
        require(_operator != address(0), "Invalid operator address");
        platformOperator = _operator;
    }

    function taskHash(string memory taskId) public pure returns (bytes32) {
        return keccak256(abi.encodePacked(taskId));
    }

    /**
     * @notice Create an escrow by locking USDT for a task
     * @param taskId Unique task identifier from the platform
     * @param amount Amount of USDT to lock (in token decimals)
     */
    function createEscrow(string calldata taskId, uint256 amount) external nonReentrant {
        require(amount > 0, "Amount must be > 0");
        bytes32 hash = taskHash(taskId);
        require(escrows[hash].status == EscrowStatus.None, "Escrow already exists");

        usdt.safeTransferFrom(msg.sender, address(this), amount);

        escrows[hash] = Escrow({
            taskId: taskId,
            publisher: msg.sender,
            amount: amount,
            status: EscrowStatus.Locked,
            createdAt: block.timestamp
        });

        emit EscrowCreated(taskId, msg.sender, amount);
    }

    /**
     * @notice Release payment to workers proportionally
     * @param taskId Task identifier
     * @param workers Array of worker wallet addresses
     * @param shares Array of share amounts (must sum to escrow amount minus fee)
     */
    function releasePayment(
        string calldata taskId,
        address[] calldata workers,
        uint256[] calldata shares
    ) external nonReentrant onlyPublisherOrOperator(taskHash(taskId)) {
        bytes32 hash = taskHash(taskId);
        Escrow storage escrow = escrows[hash];
        require(escrow.status == EscrowStatus.Locked, "Escrow not locked");
        require(workers.length > 0, "No workers");
        require(workers.length == shares.length, "Length mismatch");

        uint256 fee = (escrow.amount * platformFeeBps) / 10000;
        uint256 distributable = escrow.amount - fee;

        uint256 totalShares;
        for (uint256 i = 0; i < shares.length; i++) {
            totalShares += shares[i];
        }
        require(totalShares == distributable, "Shares must equal distributable amount");

        escrow.status = EscrowStatus.Released;

        // Transfer platform fee
        if (fee > 0) {
            usdt.safeTransfer(feeRecipient, fee);
        }

        // Transfer shares to workers
        for (uint256 i = 0; i < workers.length; i++) {
            require(workers[i] != address(0), "Invalid worker address");
            if (shares[i] > 0) {
                usdt.safeTransfer(workers[i], shares[i]);
            }
        }

        emit PaymentReleased(taskId, workers, shares);
    }

    /**
     * @notice Refund escrow to publisher (only owner can do this for dispute resolution)
     * @param taskId Task identifier
     */
    function refund(string calldata taskId) external nonReentrant {
        bytes32 hash = taskHash(taskId);
        Escrow storage escrow = escrows[hash];
        require(
            escrow.status == EscrowStatus.Locked || escrow.status == EscrowStatus.Disputed,
            "Cannot refund"
        );
        // Publisher can refund if locked, owner or operator can refund if disputed
        require(
            escrow.publisher == msg.sender || owner() == msg.sender || msg.sender == platformOperator,
            "Not authorized"
        );

        escrow.status = EscrowStatus.Refunded;
        usdt.safeTransfer(escrow.publisher, escrow.amount);

        emit EscrowRefunded(taskId, escrow.publisher, escrow.amount);
    }

    /**
     * @notice Mark escrow as disputed, freezing the funds
     * @param taskId Task identifier
     */
    function dispute(string calldata taskId) external {
        bytes32 hash = taskHash(taskId);
        Escrow storage escrow = escrows[hash];
        require(escrow.status == EscrowStatus.Locked, "Escrow not locked");
        // Publisher, platform owner, or platform operator can dispute
        require(
            escrow.publisher == msg.sender || owner() == msg.sender || msg.sender == platformOperator,
            "Not authorized"
        );

        escrow.status = EscrowStatus.Disputed;
        emit EscrowDisputed(taskId, msg.sender);
    }

    /**
     * @notice Resolve a dispute (owner only)
     * @param taskId Task identifier
     * @param releaseToWorkers If true, release to workers; if false, refund to publisher
     * @param workers Worker addresses (only used if releaseToWorkers)
     * @param shares Share amounts (only used if releaseToWorkers)
     */
    function resolveDispute(
        string calldata taskId,
        bool releaseToWorkers,
        address[] calldata workers,
        uint256[] calldata shares
    ) external nonReentrant onlyOwner {
        bytes32 hash = taskHash(taskId);
        Escrow storage escrow = escrows[hash];
        require(escrow.status == EscrowStatus.Disputed, "Not disputed");

        if (releaseToWorkers) {
            require(workers.length > 0 && workers.length == shares.length, "Invalid params");

            uint256 fee = (escrow.amount * platformFeeBps) / 10000;
            uint256 distributable = escrow.amount - fee;

            uint256 totalShares;
            for (uint256 i = 0; i < shares.length; i++) {
                totalShares += shares[i];
            }
            require(totalShares == distributable, "Shares must equal distributable");

            escrow.status = EscrowStatus.Released;

            if (fee > 0) {
                usdt.safeTransfer(feeRecipient, fee);
            }
            for (uint256 i = 0; i < workers.length; i++) {
                if (shares[i] > 0) {
                    usdt.safeTransfer(workers[i], shares[i]);
                }
            }
        } else {
            escrow.status = EscrowStatus.Refunded;
            usdt.safeTransfer(escrow.publisher, escrow.amount);
        }

        emit DisputeResolved(taskId, releaseToWorkers);
    }

    // --- Admin functions ---

    function setPlatformFee(uint256 newFeeBps) external onlyOwner {
        require(newFeeBps <= MAX_FEE_BPS, "Fee too high");
        uint256 old = platformFeeBps;
        platformFeeBps = newFeeBps;
        emit PlatformFeeUpdated(old, newFeeBps);
    }

    function setFeeRecipient(address _feeRecipient) external onlyOwner {
        require(_feeRecipient != address(0), "Invalid address");
        feeRecipient = _feeRecipient;
    }

    // --- View functions ---

    function getEscrow(string calldata taskId) external view returns (Escrow memory) {
        return escrows[taskHash(taskId)];
    }
}
