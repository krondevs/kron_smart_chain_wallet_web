import { useState, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { copyText, formatAmount, formatTime, shorten, transactionSign, transactionTitle } from '../lib/format';
import { useToast } from '../lib/toast';
import type { Token, Transaction } from '../lib/api';

export function AddressChip({ address }: { address: string }) {
  const toast = useToast();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    const ok = await copyText(address);
    setCopied(ok);
    toast.push(ok ? 'Dirección copiada' : 'No se pudo copiar', ok ? 'success' : 'error');
    window.setTimeout(() => setCopied(false), 1600);
  };
  return (
    <button className="address-chip" onClick={copy} title="Copiar dirección" style={{ border: '1px solid var(--border)' }}>
      <span className="mono">{shorten(address, 12, 8)}</span>
      <span className="faint">{copied ? '✓' : '⧉'}</span>
    </button>
  );
}

export function CopyButton({ text, label = 'Copiar' }: { text: string; label?: string }) {
  const toast = useToast();
  return (
    <button className="btn ghost small" onClick={async () => {
      const ok = await copyText(text);
      toast.push(ok ? 'Copiado al portapapeles' : 'No se pudo copiar', ok ? 'success' : 'error');
    }}>
      {label}
    </button>
  );
}

export function Badge({ children, tone = 'default' }: { children: ReactNode; tone?: 'default' | 'green' | 'yellow' | 'red' | 'accent' }) {
  return <span className={`badge ${tone === 'default' ? '' : tone}`}>{children}</span>;
}

export function Loading({ label = 'Cargando…' }: { label?: string }) {
  return (
    <div className="card center muted" style={{ padding: 36 }}>
      <div className="spinner" style={{ margin: '0 auto 10px' }} />
      {label}
    </div>
  );
}

export function Empty({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="card center" style={{ padding: 34 }}>
      <div style={{ fontSize: 30, marginBottom: 8 }}>◇</div>
      <div style={{ fontWeight: 700 }}>{title}</div>
      {children ? <div className="muted small mt">{children}</div> : null}
    </div>
  );
}

export function ErrorBanner({ message }: { message?: string }) {
  if (!message) return null;
  return <div className="alert error mb">{message}</div>;
}

export function TransactionRow({ tx }: { tx: Transaction }) {
  const sign = transactionSign(tx);
  const amount = tx.amount ?? '0';
  const display = formatAmount(Math.abs(Number(amount)), 8);
  return (
    <div className="list-item">
      <div className="main">
        <div className="title">{transactionTitle(tx)}</div>
        <div className="sub mono">{tx.address ? shorten(tx.address) : shorten(tx.txid)}</div>
      </div>
      <div>
        <div className={`value ${sign > 0 ? 'positive' : 'negative'}`}>
          {sign > 0 ? '+' : '−'}{display} KSC
        </div>
        <div className="sub center" style={{ textAlign: 'right' }}>
          {typeof tx.confirmations === 'number' ? `${tx.confirmations} conf.` : ''}
          {tx.time_iso ? ` · ${formatTime(tx.time_iso)}` : tx.time ? ` · ${formatTime(tx.time)}` : ''}
        </div>
      </div>
    </div>
  );
}

export function TokenRow({ token }: { token: Token }) {
  return (
    <Link to={`/tokens/${token.contract}`} className="token-row" style={{ textDecoration: 'none', color: 'inherit' }}>
      <div className="token-logo">{token.symbol.slice(0, 3).toUpperCase()}</div>
      <div className="meta">
        <div className="symbol">{token.symbol}{token.verified ? ' ✓' : ''}</div>
        <div className="name">{token.name}</div>
      </div>
      <div className="balance">{formatAmount(token.balance, token.decimals)}</div>
    </Link>
  );
}

export function Stat({ label, value, hint }: { label: string; value: ReactNode; hint?: string }) {
  return (
    <div className="card" style={{ padding: 15 }}>
      <div className="faint small">{label}</div>
      <div style={{ fontSize: 20, fontWeight: 750, marginTop: 4 }}>{value}</div>
      {hint ? <div className="faint small" style={{ marginTop: 2 }}>{hint}</div> : null}
    </div>
  );
}
