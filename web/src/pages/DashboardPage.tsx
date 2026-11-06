import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, newIdempotencyKey, type Token, type Transaction, type WalletOverview } from '../lib/api';
import { AddressChip, ErrorBanner, Loading, Stat, TokenRow, TransactionRow } from '../components/common';
import { Modal } from '../components/Modal';
import { formatAmount, shorten } from '../lib/format';
import { useToast } from '../lib/toast';

export function DashboardPage() {
  const [wallet, setWallet] = useState<WalletOverview | null>(null);
  const [tokens, setTokens] = useState<Token[]>([]);
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [consolidating, setConsolidating] = useState(false);
  const [consolidateTx, setConsolidateTx] = useState('');
  const [consolidateOpen, setConsolidateOpen] = useState(false);
  const [targetAddress, setTargetAddress] = useState('');

  const toast = useToast();

  const openConsolidate = (address?: string) => {
    const fallback = wallet?.primary_address || wallet?.address || '';
    const candidate = address || targetAddress;
    const valid = (wallet?.addresses ?? []).some((entry) => entry.address === candidate);
    setTargetAddress(valid ? candidate : fallback);
    setConsolidateOpen(true);
  };

  const consolidate = async (target?: string) => {
    const destination = target || targetAddress || wallet?.primary_address || wallet?.address || '';
    if (!destination) {
      setError('No hay una dirección de destino disponible.');
      return;
    }
    setConsolidating(true);
    setError('');
    try {
      const result = await api.post<{ txid: string }>('/api/v1/wallet/consolidate', { address: destination }, newIdempotencyKey());
      setConsolidateTx(result.txid);
      setConsolidateOpen(false);
      toast.push('KSC movido a la dirección seleccionada. Esperá 1 confirmación.', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo mover el KSC.');
    } finally {
      setConsolidating(false);
    }
  };

  const load = useCallback(async () => {
    setError('');
    try {
      const [walletData, tokenData, txData] = await Promise.all([
        api.get<WalletOverview>('/api/v1/wallet'),
        api.get<{ tokens: Token[] }>('/api/v1/tokens'),
        api.get<{ transactions: Transaction[] }>('/api/v1/wallet/transactions?count=6'),
      ]);
      setWallet(walletData);
      setTargetAddress((current) => current || walletData.primary_address || walletData.address || '');
      setTokens(tokenData.tokens ?? []);
      setTransactions(txData.transactions ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cargar la wallet.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Mi wallet</h2>
          <p>Saldo, tokens y actividad de Kron Smart Chain</p>
        </div>
        <button className="btn ghost small" onClick={load}>Actualizar</button>
      </div>

      <ErrorBanner message={error} />
      {wallet?.node_error ? <div className="alert warn mb">Nodo: {wallet.node_error}</div> : null}
      {consolidateTx ? <div className="alert success mb">KSC movido. Tx: <span className="mono wrap">{consolidateTx}</span>. Esperá una confirmación antes de operar con tokens.</div> : null}
      {wallet?.primary_utxo_count === 0 && Number(wallet?.balance ?? 0) > 0 && !consolidateTx ? (
        <div className="alert warn mb">
          El KSC está en otras direcciones y la dirección principal no tiene gas. Movelo para poder operar con tokens.
          <div className="mt">
            <button className="btn secondary small" disabled={consolidating} onClick={() => consolidate(wallet?.address)}>
              {consolidating ? <span className="spinner" /> : null} Mover todo el KSC a la principal
            </button>
          </div>
        </div>
      ) : null}

      <div className="card balance-card">
        <div className="balance-label">Saldo disponible</div>
        <div className="balance-value">
          {formatAmount(wallet?.balance ?? '0', 8)}
          <span className="balance-symbol">KSC</span>
        </div>
        <div className="balance-sub">
          {wallet?.chain ? `Bloque ${wallet.chain.blocks.toLocaleString('es')} · ${wallet.chain.chain}` : 'Kron Smart Chain'}
        </div>
        <div className="muted small" style={{ marginTop: 6 }}>
          Dirección principal: {formatAmount(wallet?.primary_balance ?? '0', 8)} KSC
          {wallet?.balance_pending && Number(wallet.balance_pending) > 0 ? ` · Pendiente: ${formatAmount(wallet.balance_pending, 8)} KSC` : ''}
          {wallet?.balance_immature && Number(wallet.balance_immature) > 0 ? ` · Inmaduro: ${formatAmount(wallet.balance_immature, 8)} KSC` : ''}
        </div>
        <div className="balance-actions">
          <Link to="/send" className="btn">Enviar</Link>
          <Link to="/receive" className="btn secondary">Recibir</Link>
          {wallet?.addresses && wallet.addresses.length > 1 ? (
            <button className="btn secondary" onClick={() => openConsolidate()}>Mover KSC</button>
          ) : null}
          {wallet?.address ? <AddressChip address={wallet.address} /> : null}
        </div>
      </div>

      <div className="grid stats mt">
        <Stat label="Tokens" value={wallet?.token_count ?? tokens.length} hint="KRC20 agregados" />
        <Stat label="Bloques" value={wallet?.chain?.blocks?.toLocaleString('es') ?? '—'} hint="Red mainnet" />
        <Stat label="Dificultad" value={wallet?.chain ? compact(wallet.chain.difficulty) : '—'} hint="PoS" />
        <Stat label="Gas mínimo" value={wallet?.gas ? `${wallet.gas.min_gas_price_sat} sat` : '—'} hint="Por unidad" />
      </div>

      <div className="grid two mt">
        <div className="card">
          <div className="card-head">
            <h3>Tokens KRC20</h3>
            <Link to="/tokens" className="small">Ver todos</Link>
          </div>
          {tokens.length === 0 ? (
            <div className="alert">
              No tenés tokens agregados. <Link to="/tokens">Agregá un KRC20 por contrato</Link>.
            </div>
          ) : (
            <div>{tokens.slice(0, 5).map((token) => <TokenRow key={token.contract} token={token} />)}</div>
          )}
        </div>

        <div className="card">
          <div className="card-head">
            <h3>Actividad reciente</h3>
            <Link to="/history" className="small">Historial</Link>
          </div>
          {transactions.length === 0 ? (
            <div className="alert">Todavía no hay transacciones.</div>
          ) : (
            <div>{transactions.slice(0, 5).map((tx) => <TransactionRow key={`${tx.txid}-${tx.category}`} tx={tx} />)}</div>
          )}
        </div>
      </div>

      <div className="grid three mt">
        <Link to="/tokens" className="card" style={{ color: 'inherit', textDecoration: 'none' }}>
          <div style={{ fontSize: 24 }}>◈</div>
          <div style={{ fontWeight: 750, marginTop: 7 }}>Gestionar tokens</div>
          <div className="muted small mt">Agregar, enviar, aprobar y burn KRC20.</div>
        </Link>
        <Link to="/contracts" className="card" style={{ color: 'inherit', textDecoration: 'none' }}>
          <div style={{ fontSize: 24 }}>▤</div>
          <div style={{ fontWeight: 750, marginTop: 7 }}>Contratos</div>
          <div className="muted small mt">Leer, escribir y desplegar contratos EVM.</div>
        </Link>
        <Link to="/settings" className="card" style={{ color: 'inherit', textDecoration: 'none' }}>
          <div style={{ fontSize: 24 }}>⚙</div>
          <div style={{ fontWeight: 750, marginTop: 7 }}>Seguridad</div>
          <div className="muted small mt">2FA, sesiones, contraseña y address book.</div>
        </Link>
      </div>

      {consolidateOpen ? (
        <Modal
          title="Mover todo el KSC"
          subtitle="Se envía todo el saldo disponible a la dirección elegida. Se descuenta la comisión."
          onClose={() => !consolidating && setConsolidateOpen(false)}
          actions={(
            <>
              <button className="btn secondary" disabled={consolidating} onClick={() => setConsolidateOpen(false)}>Cancelar</button>
              <button className="btn" disabled={consolidating} onClick={() => consolidate()}>{consolidating ? <span className="spinner" /> : null} Mover todo</button>
            </>
          )}
        >
          <div className="field">
            <label>Dirección destino</label>
            <select className="select" value={targetAddress} onChange={(event) => setTargetAddress(event.target.value)}>
              {(wallet?.addresses ?? []).map((entry) => (
                <option key={entry.address} value={entry.address}>
                  {shorten(entry.address, 12, 8)} · {formatAmount(entry.amount, 8)} KSC{entry.primary ? ' · principal' : ''}
                </option>
              ))}
            </select>
          </div>
          <div className="alert warn small">Esto mueve KSC, no tokens. Los tokens quedan en la dirección donde están.</div>
        </Modal>
      ) : null}
    </div>
  );
}

function compact(value: number): string {
  if (!Number.isFinite(value)) return '—';
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toFixed(0);
}
