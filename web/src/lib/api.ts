export type User = {
  id: number;
  username: string;
  role: 'user' | 'admin' | string;
  status: string;
  tier: number;
  totp_enabled: boolean;
  mfa_verified: boolean;
  per_tx_limit: number;
  daily_limit: number;
  wallet_address?: string;
  created_at?: string;
  last_login_at?: string | null;
};

export type WalletOverview = {
  address: string;
  name?: string;
  balance: string;
  balance_sat?: number;
  balance_pending?: string;
  balance_immature?: string;
  balance_stake?: string;
  primary_address?: string;
  primary_balance?: string;
  primary_balance_sat?: number;
  primary_utxo_count?: number;
  addresses?: Array<{ address: string; amount: string; amount_sat: number; label?: string; primary?: boolean }>;
  uri: string;
  token_count?: number;
  node_error?: string;
  chain?: {
    chain: string;
    blocks: number;
    bestblockhash: string;
    difficulty: number;
    initialblockdownload: boolean;
    verificationprogress: number;
    moneysupply: string;
  };
  gas?: {
    min_gas_price_sat: number;
    min_gas_price: string;
    block_gas_limit: number;
  };
  wallet_info?: Record<string, unknown>;
};

export type Transaction = {
  txid: string;
  category?: string;
  amount?: string;
  amount_sat?: number;
  fee?: string;
  fee_sat?: number;
  confirmations?: number;
  address?: string;
  time?: number;
  time_iso?: string;
  comment?: string;
  label?: string;
  [key: string]: unknown;
};

export type Token = {
  contract: string;
  name: string;
  symbol: string;
  decimals: number;
  total_supply: string;
  verified: boolean;
  balance: string;
  balance_error?: string;
  balances?: Array<{ address: string; balance: string }>;
  transactions?: TokenTransaction[];
};

export type TokenTransaction = {
  receiver: string;
  sender: string;
  amount: string;
  confirmations: number;
  blockHash: string;
  blockNumber: number;
  blocktime: number;
  transactionHash: string;
};

export type ArgInfo = {
  name: string;
  type: string;
  indexed?: boolean;
};

export type MethodInfo = {
  name: string;
  signature: string;
  state_mutability: string;
  inputs: ArgInfo[];
  outputs: ArgInfo[];
};

export type EventInfo = {
  name: string;
  signature: string;
  inputs: ArgInfo[];
};

export type Contract = {
  id?: number;
  address: string;
  name: string;
  abi?: string;
  deployed?: boolean;
  imported?: boolean;
  has_abi?: boolean;
  methods?: MethodInfo[];
  events?: EventInfo[];
  created_at?: string;
};

export type APIKeyInfo = {
  id: number;
  name: string;
  prefix: string;
  expires_at: string;
  last_used_at?: string | null;
  revoked_at?: string | null;
  created_at: string;
  status: 'active' | 'expired' | 'revoked';
  rate_limit: number;
};

export type SessionInfo = {
  id: number;
  ip: string;
  user_agent: string;
  created_at: string;
  expires_at: string;
  mfa_verified: boolean;
  current: boolean;
};

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

type MFAHandler = (code: string) => Promise<void>;
let mfaHandler: MFAHandler | null = null;
let unauthorizedHandler: (() => void) | null = null;
let csrfToken: string | null = null;

export function setMFAHandler(handler: MFAHandler | null) {
  mfaHandler = handler;
}

export function setUnauthorizedHandler(handler: (() => void) | null) {
  unauthorizedHandler = handler;
}

export async function ensureCSRF(): Promise<string> {
  if (csrfToken) return csrfToken;
  const response = await fetch('/api/v1/auth/csrf', { credentials: 'same-origin' });
  if (!response.ok) throw new ApiError(response.status, 'csrf_failed', 'No se pudo inicializar la sesión.');
  const body = await response.json();
  const token = (body?.data?.csrf_token ?? '') as string;
  csrfToken = token;
  return token;
}

export function clearCSRF() {
  csrfToken = null;
}

async function request<T>(path: string, options: RequestInit = {}, retry = true): Promise<T> {
  const method = (options.method ?? 'GET').toUpperCase();
  const headers = new Headers(options.headers);
  if (options.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  if (method !== 'GET' && method !== 'HEAD') {
    const token = await ensureCSRF();
    headers.set('X-CSRF-Token', token);
  }
  const response = await fetch(path, {
    ...options,
    method,
    headers,
    credentials: 'same-origin',
  });

  let body: any = null;
  try {
    body = await response.json();
  } catch {
    body = null;
  }

  if (!response.ok) {
    const code = body?.error?.code ?? 'http_error';
    const message = body?.error?.message ?? `Error HTTP ${response.status}`;
    if (response.status === 401 && unauthorizedHandler) unauthorizedHandler();
    if (retry && response.status === 403 && code === 'csrf_failed') {
      clearCSRF();
      await ensureCSRF();
      return request<T>(path, options, false);
    }
    if (retry && response.status === 403 && mfaHandler && ['mfa_required', 'reauth_required', 'totp_setup_required'].includes(code)) {
      await mfaHandler(code);
      return request<T>(path, options, false);
    }
    throw new ApiError(response.status, code, message);
  }
  return (body?.data ?? body) as T;
}

export const api = {
  get<T>(path: string): Promise<T> {
    return request<T>(path);
  },
  post<T>(path: string, data?: unknown, idempotencyKey?: string): Promise<T> {
    const headers = new Headers();
    if (idempotencyKey) headers.set('Idempotency-Key', idempotencyKey);
    return request<T>(path, {
      method: 'POST',
      headers,
      body: data === undefined ? undefined : JSON.stringify(data),
    });
  },
  del<T>(path: string): Promise<T> {
    return request<T>(path, { method: 'DELETE' });
  },
};

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}
