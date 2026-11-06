import { useEffect, useMemo, useState } from 'react';
import { api, newIdempotencyKey } from '../lib/api';
import { useToast } from '../lib/toast';
import { ErrorBanner, Loading } from '../components/common';
import { Modal } from '../components/Modal';
import { QRScanner } from '../components/QRScanner';
import { formatAmount, shorten } from '../lib/format';
import type { WalletOverview } from '../lib/api';

type AddressEntry = { id: number; label: string; address: string };
type FeeEstimate = { blocks: number; feerate_ksc_per_kvb: number; estimated_fee: string; estimated_fee_sat: number; source: string };

function parseTarget(input: string): { address: string; amount?: string } {
  const raw = input.trim();
  if (!raw.toLowerCase().startsWith('kron:')) return { address: raw };
  const rest = raw.slice(5);
  const [address, query] = rest.split('?');
  const params = new URLSearchParams(query ?? '');
  return { address, amount: params.get('amount') ?? undefined };
}

export function SendPage() {
  const toast = useToast();
  const [wallet, setWallet] = useState<WalletOverview | null>(null);
  const [book, setBook] = useState<AddressEntry[]>([]);
  const [fee, setFee] = useState<FeeEstimate | null>(null);
  const [target, setTarget] = useState('');
  const [amount, setAmount] = useState('');
  const [comment, setComment] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [sending, setSending] = useState(false);
  const [successTx, setSuccessTx] = useState('');
  const [scannerOpen, setScannerOpen] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const [walletData, bookData, feeData] = await Promise.all([
          api.get<WalletOverview>('/api/v1/wallet'),
          api.get<{ entries: AddressEntry[] }>('/api/v1/address-book'),
          api.get<FeeEstimate>('/api/v1/wallet/fee-estimate?blocks=6').catch(() => null),
        ]);
        setWallet(walletData);
        setBook(bookData.entries ?? []);
        setFee(feeData);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'No se pudo cargar la wallet.');
      } finally {
        setLoading(false);
      }
    })();
  }, []);

  const parsed = useMemo(() => parseTarget(target), [target]);
  const available = Number(wallet?.balance ?? 0);
  const amountNumber = Number(amount);
  const feeNumber = Number(fee?.estimated_fee ?? 0);
  const total = amountNumber + feeNumber;
  const insufficient = amountNumber > 0 && total > available;

  const openConfirm = (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    if (!parsed.address) {
      setError('Ingresá una dirección de destino.');
      return;
    }
    if (!Number.isFinite(amountNumber) || amountNumber <= 0) {
      setError('Ingresá un monto mayor a cero.');
      return;
    }
    if (insufficient) {
      setError('Saldo insuficiente para cubrir el monto y la comisión estimada.');
      return;
    }
    setConfirmOpen(true);
  };

  const send = async () => {
    setSending(true);
    setError('');
    try {
      const result = await api.post<{ txid: string }>('/api/v1/wallet/send', {
        address: parsed.address,
        amount,
        comment,
      }, newIdempotencyKey());
      setSuccessTx(result.txid);
      setConfirmOpen(false);
      setTarget('');
      setAmount('');
      setComment('');
      toast.push('Transacción enviada', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo enviar la transacción.');
      setConfirmOpen(false);
    } finally {
      setSending(false);
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Enviar KSC</h2>
          <p>Transferí Kron Smart Chain a otra dirección</p>
        </div>
      </div>

      <ErrorBanner message={error} />
      {successTx ? (
        <div className="alert success mb">
          <strong>Transacción enviada.</strong>
          <div className="mono wrap mt">{successTx}</div>
        </div>
      ) : null}

      <div className="grid two">
        <form className="card" onSubmit={openConfirm}>
          <div className="spread mb">
            <span className="muted small">Disponible</span>
            <strong>{formatAmount(wallet?.balance, 8)} KSC</strong>
          </div>

          {book.length > 0 ? (
            <div className="field">
              <label>Desde address book</label>
              <select className="select" value="" onChange={(event) => {
                const entry = book.find((item) => String(item.id) === event.target.value);
                if (entry) {
                  setTarget(entry.address);
                  if (!comment) setComment(entry.label);
                }
              }}>
                <option value="">Seleccionar contacto…</option>
                {book.map((entry) => <option key={entry.id} value={entry.id}>{entry.label} · {shorten(entry.address)}</option>)}
              </select>
            </div>
          ) : null}

          <div className="field">
            <label>Dirección de destino</label>
            <div className="input-row">
              <input className="input mono" value={target} onChange={(event) => setTarget(event.target.value)} placeholder="K... o kron:K..." required />
              <button type="button" className="btn secondary" onClick={() => setScannerOpen(true)} title="Escanear QR">▣</button>
            </div>
            <span className="hint">Acepta dirección base58 o URI kron:</span>
          </div>

          <div className="field">
            <label>Monto (KSC)</label>
            <input className="input" inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value.replace(',', '.'))} placeholder="0.00000000" required />
          </div>

          <div className="field">
            <label>Nota (opcional)</label>
            <input className="input" value={comment} onChange={(event) => setComment(event.target.value)} maxLength={120} placeholder="Pago, ahorro, etc." />
          </div>

          <div className="card" style={{ padding: 12, marginBottom: 14 }}>
            <div className="spread small"><span className="muted">Comisión estimada</span><span>{fee ? `${formatAmount(fee.estimated_fee, 8)} KSC` : '—'}</span></div>
            <div className="spread small mt"><span className="muted">Total estimado</span><span>{Number.isFinite(total) ? `${formatAmount(total, 8)} KSC` : '—'}</span></div>
          </div>

          {insufficient ? <div className="alert error mb">Saldo insuficiente.</div> : null}

          <button className="btn wide" type="submit" disabled={sending}>Continuar</button>
        </form>

        <div className="card">
          <h3 style={{ marginTop: 0 }}>Antes de enviar</h3>
          <ul className="muted small" style={{ paddingLeft: 18, lineHeight: 1.8 }}>
            <li>Verificá la dirección completa. Las transacciones son irreversibles.</li>
            <li>La comisión se estima con el nodo; puede ajustarse al firmar.</li>
            <li>El envío requiere 2FA y respeta tus límites diarios.</li>
            <li>No envíes tokens KRC20 a esta pantalla: usá la sección Tokens.</li>
          </ul>
          <div className="divider" />
          <div className="small muted">Red</div>
          <div className="mono small">Kron Smart Chain · Chain ID 2026</div>
        </div>
      </div>

      {confirmOpen ? (
        <Modal
          title="Confirmar envío"
          subtitle="Revisá los datos antes de firmar"
          onClose={() => !sending && setConfirmOpen(false)}
          actions={(
            <>
              <button className="btn secondary" disabled={sending} onClick={() => setConfirmOpen(false)}>Cancelar</button>
              <button className="btn" disabled={sending} onClick={send}>{sending ? <span className="spinner" /> : null} Enviar</button>
            </>
          )}
        >
          <div className="list">
            <div className="list-item"><span className="muted">Destino</span><span className="mono wrap" style={{ textAlign: 'right' }}>{parsed.address}</span></div>
            <div className="list-item"><span className="muted">Monto</span><strong>{formatAmount(amount, 8)} KSC</strong></div>
            <div className="list-item"><span className="muted">Comisión estimada</span><span>{fee ? `${formatAmount(fee.estimated_fee, 8)} KSC` : '—'}</span></div>
            <div className="list-item"><span className="muted">Total estimado</span><strong>{formatAmount(total, 8)} KSC</strong></div>
          </div>
          <div className="alert warn mt">La comisión final puede variar levemente. El servidor firmará con la wallet de tu cuenta.</div>
        </Modal>
      ) : null}

      {scannerOpen ? (
        <QRScanner
          onClose={() => setScannerOpen(false)}
          onResult={(value) => {
            const parsedValue = parseTarget(value);
            setTarget(parsedValue.address);
            if (parsedValue.amount) setAmount(parsedValue.amount);
          }}
        />
      ) : null}
    </div>
  );
}
