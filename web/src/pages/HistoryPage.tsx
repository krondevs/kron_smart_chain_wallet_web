import { useCallback, useEffect, useState } from 'react';
import { api, type Transaction } from '../lib/api';
import { Empty, ErrorBanner, Loading, TransactionRow } from '../components/common';
import { Modal } from '../components/Modal';
import { copyText } from '../lib/format';

export function HistoryPage() {
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [selected, setSelected] = useState<Transaction | null>(null);
  const [detail, setDetail] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const data = await api.get<{ transactions: Transaction[] }>('/api/v1/wallet/transactions?count=100');
      setTransactions(data.transactions ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cargar el historial.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const open = async (tx: Transaction) => {
    setSelected(tx);
    setDetail('');
    try {
      const data = await api.get<Transaction>(`/api/v1/wallet/transactions/${tx.txid}`);
      setDetail(JSON.stringify(data, null, 2));
    } catch {
      setDetail(JSON.stringify(tx, null, 2));
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Historial</h2>
          <p>Movimientos de tu wallet KSC</p>
        </div>
        <button className="btn ghost small" onClick={load}>Actualizar</button>
      </div>

      <ErrorBanner message={error} />

      {transactions.length === 0 ? (
        <Empty title="Sin movimientos">Cuando recibas o envíes KSC, aparecerán acá.</Empty>
      ) : (
        <div className="card">
          {transactions.map((tx) => (
            <div
              key={`${tx.txid}-${tx.category}-${tx.address}`}
              onClick={() => open(tx)}
              onKeyDown={(event) => { if (event.key === 'Enter') open(tx); }}
              role="button"
              tabIndex={0}
              style={{ cursor: 'pointer' }}
            >
              <TransactionRow tx={tx} />
            </div>
          ))}
        </div>
      )}

      {selected ? (
        <Modal title="Detalle de transacción" subtitle={`${selected.category ?? 'tx'} · ${selected.confirmations ?? 0} confirmaciones`} onClose={() => setSelected(null)}>
          <div className="field">
            <label>TXID</label>
            <div className="address-chip"><span className="mono wrap">{selected.txid}</span></div>
            <button className="btn ghost small" onClick={() => copyText(selected.txid)}>Copiar</button>
          </div>
          <pre className="mono small wrap" style={{ whiteSpace: 'pre-wrap', maxHeight: 320, overflow: 'auto' }}>{detail || 'Cargando…'}</pre>
        </Modal>
      ) : null}
    </div>
  );
}
