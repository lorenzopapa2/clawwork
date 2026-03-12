#!/usr/bin/env node

import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import { ethers } from "ethers";
import * as fs from "fs";
import * as path from "path";
import * as os from "os";

// ─── Config persistence ───────────────────────────────────────────────────────

interface AgentHubConfig {
  server_url: string;
  agent_id: string;
  api_key: string;
  wallet_address: string;
  private_key: string;
  role: "worker" | "publisher" | "";
  contract_addr: string;
  usdt_addr: string;
  chain_id: number;
  rpc_url: string;
}

const CONFIG_DIR = path.join(os.homedir(), ".agenthub");
const CONFIG_PATH = path.join(CONFIG_DIR, "config.json");

const DEFAULT_CONFIG: AgentHubConfig = {
  server_url: "https://agenthub.example.com",
  agent_id: "",
  api_key: "",
  wallet_address: "",
  private_key: "",
  role: "",
  contract_addr: "",
  usdt_addr: "0x55d398326f99059fF775485246999027B3197955",
  chain_id: 56,
  rpc_url: "https://bsc-dataseed1.binance.org/",
};

function loadConfig(): AgentHubConfig {
  try {
    if (fs.existsSync(CONFIG_PATH)) {
      const raw = fs.readFileSync(CONFIG_PATH, "utf-8");
      return { ...DEFAULT_CONFIG, ...JSON.parse(raw) };
    }
  } catch {
    // ignore parse errors, return default
  }
  return { ...DEFAULT_CONFIG };
}

function saveConfig(cfg: AgentHubConfig): void {
  if (!fs.existsSync(CONFIG_DIR)) {
    fs.mkdirSync(CONFIG_DIR, { recursive: true });
  }
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(cfg, null, 2), "utf-8");
}

let config = loadConfig();

// ─── API helper ───────────────────────────────────────────────────────────────

async function apiCall(
  method: string,
  path: string,
  body?: unknown
): Promise<unknown> {
  const baseUrl =
    process.env.AGENTHUB_URL || config.server_url || "http://localhost:8080";
  const url = `${baseUrl}/api/v1${path}`;
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  const apiKey = process.env.AGENTHUB_API_KEY || config.api_key;
  if (apiKey) {
    headers["X-API-Key"] = apiKey;
  }

  const res = await fetch(url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  const data = await res.json();
  if (!res.ok) {
    throw new Error(
      `API error ${res.status}: ${JSON.stringify((data as Record<string, unknown>).error || data)}`
    );
  }
  return data;
}

// ─── Blockchain helpers ───────────────────────────────────────────────────────

const ERC20_ABI = [
  "function approve(address spender, uint256 amount) external returns (bool)",
  "function allowance(address owner, address spender) external view returns (uint256)",
  "function balanceOf(address account) external view returns (uint256)",
];

const ESCROW_ABI = [
  "function createEscrow(string calldata taskId, uint256 amount) external",
  "function getEscrow(string calldata taskId) external view returns (tuple(string taskId, address publisher, uint256 amount, uint8 status, uint256 createdAt))",
];

function getProvider(): ethers.JsonRpcProvider {
  const rpcUrl = config.rpc_url || "https://bsc-dataseed1.binance.org/";
  return new ethers.JsonRpcProvider(rpcUrl, config.chain_id || 56);
}

function getSigner(): ethers.Wallet {
  if (!config.private_key) {
    throw new Error(
      "Private key not configured. Run setup_publisher first."
    );
  }
  return new ethers.Wallet(config.private_key, getProvider());
}

function generateTaskId(): string {
  const chars = "abcdefghijklmnopqrstuvwxyz0123456789";
  let id = "task_";
  for (let i = 0; i < 8; i++) {
    id += chars[Math.floor(Math.random() * chars.length)];
  }
  return id;
}

/** Convert USDT amount (float) to BigInt with 18 decimals (BSC USDT) */
function toTokenAmount(amount: number): bigint {
  return ethers.parseUnits(amount.toString(), 18);
}

// ─── Text response helper ─────────────────────────────────────────────────────

function textResult(msg: string) {
  return { content: [{ type: "text" as const, text: msg }] };
}

function jsonResult(data: unknown) {
  return {
    content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }],
  };
}

// ─── MCP Server ───────────────────────────────────────────────────────────────

const server = new McpServer({
  name: "agenthub",
  version: "1.0.0",
});

// ═══════════════════════════════════════════════════════════════════════════════
// 1. setup_worker — Register as a Worker
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "setup_worker",
  "Register as a worker agent on AgentHub. Provide your wallet address to receive USDT payments.",
  {
    name: z.string().describe("Your agent name"),
    wallet_address: z
      .string()
      .describe("Your BSC wallet address to receive USDT payments"),
    capabilities: z
      .array(z.string())
      .default(["general"])
      .describe('Your skill tags, e.g. ["code", "research", "translation"]'),
    server_url: z
      .string()
      .optional()
      .describe("AgentHub server URL (default: https://agenthub.example.com)"),
  },
  async (params) => {
    if (params.server_url) {
      config.server_url = params.server_url;
    }

    // Register via API
    const result = (await apiCall("POST", "/agents/register", {
      name: params.name,
      owner: params.wallet_address,
      capabilities: params.capabilities,
      wallet_address: params.wallet_address,
    })) as Record<string, unknown>;

    // Persist config
    config.agent_id = result.id as string;
    config.api_key = result.api_key as string;
    config.wallet_address = params.wallet_address;
    config.role = "worker";
    saveConfig(config);

    return textResult(
      `✅ Worker setup complete!\n\n` +
        `Agent ID: ${config.agent_id}\n` +
        `API Key: ${config.api_key}\n` +
        `Wallet: ${config.wallet_address}\n` +
        `Role: worker\n\n` +
        `Config saved to ${CONFIG_PATH}\n` +
        `You can now use browse_tasks, bid_task, submit_result, etc.`
    );
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 2. setup_publisher — Register as a Publisher
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "setup_publisher",
  "Register as a task publisher on AgentHub. Provide your private key for local on-chain signing (never sent to server).",
  {
    name: z.string().describe("Your agent name"),
    private_key: z
      .string()
      .describe("Your BSC private key (stored locally only, used for signing)"),
    capabilities: z
      .array(z.string())
      .default(["publishing"])
      .describe("Your capability tags"),
    server_url: z
      .string()
      .optional()
      .describe("AgentHub server URL (default: https://agenthub.example.com)"),
    contract_addr: z
      .string()
      .optional()
      .describe("TaskEscrow contract address on BSC"),
    rpc_url: z
      .string()
      .optional()
      .describe("BSC RPC URL (default: https://bsc-dataseed1.binance.org/)"),
  },
  async (params) => {
    if (params.server_url) config.server_url = params.server_url;
    if (params.contract_addr) config.contract_addr = params.contract_addr;
    if (params.rpc_url) config.rpc_url = params.rpc_url;

    // Derive wallet address from private key
    const wallet = new ethers.Wallet(params.private_key);
    const walletAddress = wallet.address;

    // Register via API
    const result = (await apiCall("POST", "/agents/register", {
      name: params.name,
      owner: walletAddress,
      capabilities: params.capabilities,
      wallet_address: walletAddress,
    })) as Record<string, unknown>;

    // Persist config — private key stays local
    config.agent_id = result.id as string;
    config.api_key = result.api_key as string;
    config.wallet_address = walletAddress;
    config.private_key = params.private_key;
    config.role = "publisher";
    saveConfig(config);

    return textResult(
      `✅ Publisher setup complete!\n\n` +
        `Agent ID: ${config.agent_id}\n` +
        `API Key: ${config.api_key}\n` +
        `Wallet: ${walletAddress}\n` +
        `Role: publisher\n` +
        `Contract: ${config.contract_addr || "(not set — set before publishing)"}\n\n` +
        `Config saved to ${CONFIG_PATH}\n` +
        `⚠️ Private key is stored locally at ${CONFIG_PATH} — keep it safe!\n` +
        `You can now use publish_task, view_bids, assign_worker, approve_task, etc.`
    );
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 3. browse_tasks — Browse task marketplace
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "browse_tasks",
  "Browse available tasks on the AgentHub marketplace with optional filters",
  {
    status: z
      .string()
      .optional()
      .describe("Filter by status: open, assigned, in_progress, review, completed, disputed"),
    capability: z.string().optional().describe("Filter by required capability"),
    min_bounty: z.number().optional().describe("Minimum bounty filter (USDT)"),
    max_bounty: z.number().optional().describe("Maximum bounty filter (USDT)"),
    sort: z
      .string()
      .optional()
      .describe("Sort: bounty_asc, bounty_desc, deadline_asc, created_desc"),
    page: z.number().int().positive().default(1).describe("Page number"),
    limit: z.number().int().positive().default(20).describe("Items per page"),
  },
  async (params) => {
    const q: string[] = [];
    if (params.status) q.push(`status=${params.status}`);
    if (params.capability) q.push(`capability=${params.capability}`);
    if (params.min_bounty) q.push(`min_bounty=${params.min_bounty}`);
    if (params.max_bounty) q.push(`max_bounty=${params.max_bounty}`);
    if (params.sort) q.push(`sort=${params.sort}`);
    q.push(`page=${params.page}`);
    q.push(`limit=${params.limit}`);

    const qs = q.length > 0 ? `?${q.join("&")}` : "";
    const result = await apiCall("GET", `/tasks${qs}`);
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 4. get_task — View single task details + bids
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "get_task",
  "Get detailed information about a specific task, including its bids",
  {
    task_id: z.string().describe("Task ID to look up"),
  },
  async (params) => {
    const result = await apiCall("GET", `/tasks/${params.task_id}`);
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 5. bid_task — Submit a bid on a task (Worker)
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "bid_task",
  "Submit a bid on an open task in the marketplace (Worker role)",
  {
    task_id: z.string().describe("ID of the task to bid on"),
    proposal: z
      .string()
      .describe("Your proposal explaining how you will complete the task"),
    price: z.number().positive().describe("Your bid price in USDT"),
    estimated_tokens: z
      .number()
      .int()
      .default(0)
      .describe("Estimated token consumption"),
  },
  async (params) => {
    const result = await apiCall("POST", `/tasks/${params.task_id}/bid`, {
      proposal: params.proposal,
      price: params.price,
      estimated_tokens: params.estimated_tokens,
    });
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 6. submit_result — Submit work result (Worker)
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "submit_result",
  "Submit completed work result for an assigned task (Worker role)",
  {
    task_id: z.string().describe("ID of the task"),
    result: z.string().describe("The completed work result"),
    prompt_tokens: z.number().int().default(0).describe("Prompt tokens consumed"),
    completion_tokens: z
      .number()
      .int()
      .default(0)
      .describe("Completion tokens consumed"),
  },
  async (params) => {
    const result = await apiCall("PUT", `/tasks/${params.task_id}/submit`, {
      result: params.result,
      token_usage: {
        prompt_tokens: params.prompt_tokens,
        completion_tokens: params.completion_tokens,
      },
    });
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 7. my_tasks — View my published/accepted tasks
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "my_tasks",
  "View tasks you published or are working on",
  {
    role_filter: z
      .enum(["published", "working", "all"])
      .default("all")
      .describe("Filter: 'published' for tasks you created, 'working' for tasks you bid on, 'all' for both"),
    status: z
      .string()
      .optional()
      .describe("Filter by task status"),
    page: z.number().int().positive().default(1).describe("Page number"),
    limit: z.number().int().positive().default(20).describe("Items per page"),
  },
  async (params) => {
    if (!config.agent_id) {
      return textResult("❌ Not configured. Run setup_worker or setup_publisher first.");
    }

    const q: string[] = [];
    if (params.status) q.push(`status=${params.status}`);
    q.push(`page=${params.page}`);
    q.push(`limit=${params.limit}`);

    if (params.role_filter === "published" || params.role_filter === "all") {
      q.push(`publisher_id=${config.agent_id}`);
    }
    const qs = q.length > 0 ? `?${q.join("&")}` : "";
    const result = await apiCall("GET", `/tasks${qs}`);
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 8. publish_task — Publish task with on-chain escrow (Publisher)
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "publish_task",
  "Publish a new task: locally sign USDT approve + createEscrow on BSC, then register task on AgentHub server. Private key never leaves your machine.",
  {
    title: z.string().describe("Task title"),
    description: z.string().describe("Detailed task description"),
    requirements: z
      .array(z.string())
      .describe('Required capability tags, e.g. ["code", "research"]'),
    bounty: z.number().positive().describe("Bounty amount in USDT"),
    max_workers: z.number().int().positive().default(1).describe("Maximum number of workers"),
    payment_model: z
      .enum(["fixed", "token_based", "weighted"])
      .default("fixed")
      .describe("Payment distribution model"),
    deadline: z.string().describe("Deadline in ISO 8601 format, e.g. 2025-02-01T00:00:00Z"),
  },
  async (params) => {
    if (!config.private_key) {
      return textResult("❌ Private key not configured. Run setup_publisher first.");
    }
    if (!config.contract_addr) {
      return textResult("❌ Contract address not configured. Run setup_publisher with contract_addr.");
    }

    const signer = getSigner();
    const usdtContract = new ethers.Contract(config.usdt_addr, ERC20_ABI, signer);
    const escrowContract = new ethers.Contract(config.contract_addr, ESCROW_ABI, signer);

    const taskId = generateTaskId();
    const amountWei = toTokenAmount(params.bounty);

    // Step 1: Check USDT balance
    const balance = await usdtContract.balanceOf(signer.address);
    if (balance < amountWei) {
      return textResult(
        `❌ Insufficient USDT balance.\n` +
          `Required: ${params.bounty} USDT\n` +
          `Available: ${ethers.formatUnits(balance, 18)} USDT`
      );
    }

    // Step 2: Approve USDT spending
    const currentAllowance = await usdtContract.allowance(
      signer.address,
      config.contract_addr
    );

    let approveTxHash = "";
    if (currentAllowance < amountWei) {
      const approveTx = await usdtContract.approve(
        config.contract_addr,
        amountWei
      );
      const approveReceipt = await approveTx.wait();
      approveTxHash = approveReceipt.hash;
    }

    // Step 3: Create escrow on-chain
    const escrowTx = await escrowContract.createEscrow(taskId, amountWei);
    const escrowReceipt = await escrowTx.wait();
    const escrowTxHash = escrowReceipt.hash;

    // Step 4: Register task on AgentHub server
    const result = await apiCall("POST", "/tasks", {
      id: taskId,
      title: params.title,
      description: params.description,
      requirements: params.requirements,
      bounty: params.bounty,
      escrow_tx: escrowTxHash,
      max_workers: params.max_workers,
      payment_model: params.payment_model,
      deadline: params.deadline,
    });

    return textResult(
      `✅ Task published successfully!\n\n` +
        `Task ID: ${taskId}\n` +
        `Bounty: ${params.bounty} USDT\n` +
        (approveTxHash ? `Approve TX: ${approveTxHash}\n` : "") +
        `Escrow TX: ${escrowTxHash}\n\n` +
        `Task details:\n${JSON.stringify(result, null, 2)}`
    );
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 9. view_bids — View bids on my task (Publisher)
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "view_bids",
  "View all bids received on a specific task you published",
  {
    task_id: z.string().describe("Task ID to view bids for"),
  },
  async (params) => {
    const task = (await apiCall("GET", `/tasks/${params.task_id}`)) as Record<
      string,
      unknown
    >;
    // The task detail endpoint already includes bids
    return jsonResult(task);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 10. assign_worker — Select worker for a task (Publisher)
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "assign_worker",
  "Accept bids and assign workers to your task (Publisher role)",
  {
    task_id: z.string().describe("Task ID"),
    bid_ids: z
      .array(z.string())
      .describe("Array of bid IDs to accept"),
  },
  async (params) => {
    const result = await apiCall("PUT", `/tasks/${params.task_id}/assign`, {
      bid_ids: params.bid_ids,
    });
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 11. approve_task — Approve and release payment (Publisher)
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "approve_task",
  "Approve completed task and trigger payment release to workers (Publisher role). On-chain release is handled by the server.",
  {
    task_id: z.string().describe("Task ID to approve"),
    worker_weights: z
      .record(z.string(), z.number().int())
      .optional()
      .describe("Custom worker weight map for 'weighted' payment model, e.g. {\"agent_abc\": 70, \"agent_def\": 30}"),
  },
  async (params) => {
    const body: Record<string, unknown> = {};
    if (params.worker_weights) {
      body.worker_weights = params.worker_weights;
    }
    const result = await apiCall(
      "PUT",
      `/tasks/${params.task_id}/approve`,
      body
    );
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 12. dispute_task — Dispute a task
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "dispute_task",
  "Raise a dispute on a task (either publisher or worker)",
  {
    task_id: z.string().describe("Task ID to dispute"),
    reason: z.string().describe("Reason for the dispute"),
  },
  async (params) => {
    const result = await apiCall("PUT", `/tasks/${params.task_id}/dispute`, {
      reason: params.reason,
    });
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 13. check_earnings — View earnings / spending
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "check_earnings",
  "Check your agent's earnings, spending, and payment history",
  {
    type: z
      .enum(["earned", "spent", "all"])
      .default("all")
      .describe("Filter payment type"),
    page: z.number().int().positive().default(1).describe("Page number"),
    limit: z.number().int().positive().default(20).describe("Items per page"),
  },
  async (params) => {
    const result = await apiCall(
      "GET",
      `/payments/history?type=${params.type}&page=${params.page}&limit=${params.limit}`
    );
    return jsonResult(result);
  }
);

// ═══════════════════════════════════════════════════════════════════════════════
// 14. my_status — View current agent status
// ═══════════════════════════════════════════════════════════════════════════════
server.tool(
  "my_status",
  "View your current agent identity, balance, reputation, and configuration status",
  {},
  async () => {
    if (!config.agent_id) {
      return textResult(
        `❌ Not configured yet.\n\n` +
          `Run setup_worker (to accept tasks) or setup_publisher (to publish tasks) first.`
      );
    }

    // Fetch agent info from server
    let agentInfo: Record<string, unknown> = {};
    try {
      agentInfo = (await apiCall(
        "GET",
        `/agents/${config.agent_id}`
      )) as Record<string, unknown>;
    } catch {
      // Server may be unreachable — show local config
    }

    // Check on-chain USDT balance if we have a wallet
    let usdtBalance = "N/A";
    if (config.wallet_address) {
      try {
        const provider = getProvider();
        const usdt = new ethers.Contract(config.usdt_addr, ERC20_ABI, provider);
        const bal = await usdt.balanceOf(config.wallet_address);
        usdtBalance = ethers.formatUnits(bal, 18) + " USDT";
      } catch {
        usdtBalance = "(unable to query)";
      }
    }

    return textResult(
      `🔍 Agent Status\n\n` +
        `Agent ID: ${config.agent_id}\n` +
        `Name: ${agentInfo.name || "N/A"}\n` +
        `Role: ${config.role}\n` +
        `Wallet: ${config.wallet_address}\n` +
        `USDT Balance: ${usdtBalance}\n` +
        `Reputation: ${agentInfo.reputation ?? "N/A"}\n` +
        `Total Earned: ${agentInfo.total_earned ?? "N/A"}\n` +
        `Total Spent: ${agentInfo.total_spent ?? "N/A"}\n` +
        `Status: ${agentInfo.status || "N/A"}\n` +
        `Server: ${config.server_url}\n` +
        `Contract: ${config.contract_addr || "(not set)"}\n` +
        `Config: ${CONFIG_PATH}`
    );
  }
);

// ─── Main ─────────────────────────────────────────────────────────────────────

async function main() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("AgentHub MCP Server running on stdio");
}

main().catch((err) => {
  console.error("Fatal error:", err);
  process.exit(1);
});
