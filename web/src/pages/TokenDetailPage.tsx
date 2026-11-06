import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { api, ApiError, newIdempotencyKey, type Token, type TokenTransaction } from '../lib/api';
import { CopyButton, ErrorBanner, Loading } from '../components/common';
import { QRScanner } from '../components/QRScanner';
import { useToast } from '../lib/toast';
import { formatAmount, formatTime, shorten } from '../lib/format';

type Tab = 'send' | 'approve' | 'advanced';

export function TokenDetailPage() {
  const { contract = '' } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const [token, setToken] = useState<Token | null>(null);
  const [tab, setTab] = useState<Tab>('send');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [txid, setTxid] = useState('');
  const [gasNeeded, setGasNeeded] = useState(false);
  const [consolidating, setConsolidating] = useState(false);
  const [consolidateTx, setConsolidateTx] = useState('');
  const [scannerOpen, setScannerOpen] = useState(false);

  const [to, setTo] = useState('');
  const [amount, setAmount] = useState('');
  const [spender, setSpender] = useState('');
  const [from, setFrom] = useState('');
  const [sourceAddress, setSourceAddress] = useState('');
  const [gasLimit, setGasLimit] = useState('');
  const [gasPrice, setGasPrice] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const data = await api.get<Token>(`/api/v1/tokens/${contract}`);
      setToken(data);
      if (data.balances && data.balances.length > 0) {
        setSourceAddress((current) => current || data.balances![0].address);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cargar el token.');
    } finally {
      setLoading(false);
    }
  }, [contract]);

  useEffect(() => { load(); }, [load]);

  const submit = async (kind: 'transfer' | 'approve' | 'transfer-from' | 'burn' | 'burn-from') => {
    setBusy(true);
    setError('');
    setTxid('');
    try {
      let body: Record<string, unknown> = { gaslimit: gasLimit, gasprice: gasPrice };
      if (kind === 'transfer') body = { ...body, from: sourceAddress, to, amount };
      if (kind === 'approve') body = { ...body, from: sourceAddress, spender, amount };
      if (kind === 'transfer-from') body = { ...body, from, to, amount };
      if (kind === 'burn') body = { ...body, from: sourceAddress, amount };
      if (kind === 'burn-from') body = { ...body, from, amount };
      const result = await api.post<{ txid: string }>(`/api/v1/tokens/${contract}/${kind}`, body, newIdempotencyKey());
      setTxid(result.txid);
      setGasNeeded(false);
      toast.push('Operación enviada', 'success');
      await load();
    } catch (err) {
      if (err instanceof ApiError && err.code === 'gas_not_ready') {
        setGasNeeded(true);
      }
      setError(err instanceof Error ? err.message : 'No se pudo completar la operación.');
    } finally {
      setBusy(false);
    }
  };

  const moveAllHere = async () => {
    setConsolidating(true);
    setError('');
    try {
      const result = await api.post<{ txid: string }>('/api/v1/wallet/consolidate', { address: sourceAddress }, newIdempotencyKey());
      setConsolidateTx(result.txid);
      setGasNeeded(false);
      toast.push('KSC movido a la dirección de origen. Esperá 1 confirmación.', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo mover el KSC.');
    } finally {
      setConsolidating(false);
    }
  };

  const prepareGas = async () => {
    setConsolidating(true);
    setError('');
    try {
      const result = await api.post<{ txid: string }>('/api/v1/wallet/prepare-gas', { address: sourceAddress }, newIdempotencyKey());
      setConsolidateTx(result.txid);
      setGasNeeded(false);
      toast.push('KSC consolidado. Esperá 1 confirmación y reintentá.', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo preparar el gas.');
    } finally {
      setConsolidating(false);
    }
  };

  const remove = async () => {
    if (!window.confirm('¿Quitar este token de tu portfolio? No se borra el contrato en la cadena.')) return;
    try {
      await api.del(`/api/v1/tokens/${contract}`);
      toast.push('Token quitado', 'success');
      navigate('/tokens');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo quitar el token.');
    }
  };

  if (loading) return <div className="page"><Loading /></div>;
  if (!token) return <div className="page"><ErrorBanner message={error || 'Token no encontrado'} /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>{token.symbol} <span className="muted" style={{ fontSize: 15 }}>{token.name}</span></h2>
          <p className="mono">{token.contract}</p>
        </div>
        <div className="btn-group">
          <CopyButton text={token.contract} label="Copiar contrato" />
          <button className="btn danger small" onClick={remove}>Quitar</button>
        </div>
      </div>

      <ErrorBanner message={error} />
      {gasNeeded ? (
        <div className="alert warn mb">
          La dirección de origen no tiene KSC para pagar gas. Consolidá tu KSC en la dirección principal.
          <div className="btn-group mt">
            <button className="btn secondary small" disabled={consolidating} onClick={prepareGas}>
              {consolidating ? <span className="spinner" /> : null} Preparar gas (0.2 KSC)
            </button>
            <button className="btn ghost small" disabled={consolidating} onClick={moveAllHere}>
              Mover todo el KSC aquí
            </button>
          </div>
        </div>
      ) : null}
      {consolidateTx ? <div className="alert success mb">Gas preparado. Tx: <span className="mono wrap">{consolidateTx}</span>. Esperá una confirmación antes de reintentar.</div> : null}
      {txid ? <div className="alert success mb"><strong>Tx enviada:</strong> <span className="mono wrap">{txid}</span></div> : null}

      <div className="grid two">
        <div className="card balance-card">
          <div className="balance-label">Tu saldo</div>
          <div className="balance-value">{formatAmount(token.balance, token.decimals)}<span className="balance-symbol">{token.symbol}</span></div>
          <div className="balance-sub">Supply total: {formatAmount(token.total_supply, token.decimals)} {token.symbol}</div>
          <div className="row mt">
            <span className="badge accent">{token.decimals} decimales</span>
            {token.verified ? <span className="badge green">Verificado</span> : <span className="badge yellow">No verificado</span>}
          </div>
          {token.balances && token.balances.length > 0 ? (
            <div className="list mt">
              {token.balances.map((entry) => (
                <div className="list-item" key={entry.address}>
                  <div className="main"><div className="sub mono">{shorten(entry.address, 12, 8)}</div></div>
                  <div className="value">{formatAmount(entry.balance, token.decimals)} {token.symbol}</div>
                </div>
              ))}
            </div>
          ) : null}
        </div>

        <div className="card">
          <div className="auth-tabs">
            <button className={tab === 'send' ? 'active' : ''} onClick={() => setTab('send')}>Enviar</button>
            <button className={tab === 'approve' ? 'active' : ''} onClick={() => setTab('approve')}>Aprobar</button>
            <button className={tab === 'advanced' ? 'active' : ''} onClick={() => setTab('advanced')}>Avanzado</button>
          </div>

          {tab === 'send' ? (
            <div>
              {token.balances && token.balances.length > 1 ? (
                <div className="field">
                  <label>Desde dirección</label>
                  <select className="select" value={sourceAddress} onChange={(e) => setSourceAddress(e.target.value)}>
                    {token.balances.map((entry) => <option key={entry.address} value={entry.address}>{shorten(entry.address, 12, 8)} · {entry.balance} {token.symbol}</option>)}
                  </select>
                </div>
              ) : null}
              <div className="field">
                <label>Destinatario</label>
                <div className="input-row">
                  <input className="input mono" value={to} onChange={(e) => setTo(e.target.value)} placeholder="K..." />
                  <button type="button" className="btn secondary" onClick={() => setScannerOpen(true)} title="Escanear QR">▣</button>
                </div>
              </div>
              <div className="field"><label>Monto ({token.symbol})</label><input className="input" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value.replace(',', '.'))} placeholder="0.0" /></div>
              <GasFields gasLimit={gasLimit} gasPrice={gasPrice} setGasLimit={setGasLimit} setGasPrice={setGasPrice} />
              {sourceAddress ? (
                <div className="mb">
                  <button className="btn ghost small" disabled={consolidating} onClick={moveAllHere}>Mover todo el KSC a esta dirección</button>
                </div>
              ) : null}
              <button className="btn wide" disabled={busy} onClick={() => submit('transfer')}>{busy ? <span className="spinner" /> : null} Enviar token</button>
            </div>
          ) : null}

          {tab === 'approve' ? (
            <div>
              {token.balances && token.balances.length > 1 ? (
                <div className="field">
                  <label>Desde dirección</label>
                  <select className="select" value={sourceAddress} onChange={(e) => setSourceAddress(e.target.value)}>
                    {token.balances.map((entry) => <option key={entry.address} value={entry.address}>{shorten(entry.address, 12, 8)} · {entry.balance} {token.symbol}</option>)}
                  </select>
                </div>
              ) : null}
              <div className="field"><label>Spender</label><input className="input mono" value={spender} onChange={(e) => setSpender(e.target.value)} placeholder="K..." /></div>
              <div className="field"><label>Monto autorizado ({token.symbol})</label><input className="input" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value.replace(',', '.'))} placeholder="0.0" /></div>
              <GasFields gasLimit={gasLimit} gasPrice={gasPrice} setGasLimit={setGasLimit} setGasPrice={setGasPrice} />
              <button className="btn wide" disabled={busy} onClick={() => submit('approve')}>{busy ? <span className="spinner" /> : null} Aprobar allowance</button>
            </div>
          ) : null}

          {tab === 'advanced' ? (
            <div>
              <div className="field"><label>From (owner)</label><input className="input mono" value={from} onChange={(e) => setFrom(e.target.value)} placeholder="K..." /></div>
              <div className="field">
                <label>Destinatario</label>
                <div className="input-row">
                  <input className="input mono" value={to} onChange={(e) => setTo(e.target.value)} placeholder="K..." />
                  <button type="button" className="btn secondary" onClick={() => setScannerOpen(true)} title="Escanear QR">▣</button>
                </div>
              </div>
              <div className="field"><label>Monto ({token.symbol})</label><input className="input" inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value.replace(',', '.'))} placeholder="0.0" /></div>
              <GasFields gasLimit={gasLimit} gasPrice={gasPrice} setGasLimit={setGasLimit} setGasPrice={setGasPrice} />
              <div className="btn-group">
                <button className="btn secondary" disabled={busy} onClick={() => submit('transfer-from')}>TransferFrom</button>
                <button className="btn secondary" disabled={busy} onClick={() => submit('burn')}>Burn</button>
                <button className="btn secondary" disabled={busy} onClick={() => submit('burn-from')}>BurnFrom</button>
              </div>
            </div>
          ) : null}
        </div>
      </div>

      <div className="card mt">
        <div className="card-head">
          <h3>Historial del token</h3>
          <button className="btn ghost small" onClick={load}>Actualizar</button>
        </div>
        {token.transactions && token.transactions.length > 0 ? (
          <div className="list">
            {token.transactions.slice(0, 50).map((tx: TokenTransaction) => (
              <div className="list-item" key={tx.transactionHash + tx.amount + tx.blockNumber}>
                <div className="main">
                  <div className="title">{tx.sender === tx.receiver ? 'Transferencia' : tx.amount.startsWith('-') ? 'Enviado' : 'Recibido'}</div>
                  <div className="sub mono">{shorten(tx.sender)} → {shorten(tx.receiver)}</div>
                </div>
                <div>
                  <div className={`value ${tx.amount.startsWith('-') ? 'negative' : 'positive'}`}>{tx.amount} {token.symbol}</div>
                  <div className="sub" style={{ textAlign: 'right' }}>{tx.confirmations} conf. · {formatTime(tx.blocktime)}</div>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <div className="alert">Sin transferencias recientes.</div>
        )}
      </div>

      {scannerOpen ? (
        <QRScanner
          onClose={() => setScannerOpen(false)}
          onResult={(value) => {
            const parsed = parseTokenTarget(value);
            setTo(parsed.address);
            if (parsed.amount) setAmount(parsed.amount);
          }}
        />
      ) : null}
    </div>
  );
}

function parseTokenTarget(input: string): { address: string; amount?: string } {
  const raw = input.trim();
  if (!raw.toLowerCase().startsWith('kron:')) return { address: raw };
  const rest = raw.slice(5);
  const [address, query] = rest.split('?');
  const params = new URLSearchParams(query ?? '');
  return { address, amount: params.get('amount') ?? undefined };
}

function GasFields({ gasLimit, gasPrice, setGasLimit, setGasPrice }: {
  gasLimit: string;
  gasPrice: string;
  setGasLimit: (value: string) => void;
  setGasPrice: (value: string) => void;
}) {
  return (
    <div className="grid two" style={{ gap: 8 }}>
      <div className="field"><label>Gas limit (opcional)</label><input className="input" inputMode="numeric" value={gasLimit} onChange={(e) => setGasLimit(e.target.value.replace(/\D/g, ''))} placeholder="250000" /></div>
      <div className="field"><label>Gas price KSC (opcional)</label><input className="input" inputMode="decimal" value={gasPrice} onChange={(e) => setGasPrice(e.target.value.replace(',', '.'))} placeholder="0.00000040" /></div>
    </div>
  );
}
