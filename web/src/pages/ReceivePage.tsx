import { useEffect, useMemo, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { api } from '../lib/api';
import { AddressChip, CopyButton, ErrorBanner, Loading } from '../components/common';
import { useToast } from '../lib/toast';
import { formatAmount } from '../lib/format';

type AddressResponse = { address: string; uri: string };

export function ReceivePage() {
  const toast = useToast();
  const [data, setData] = useState<AddressResponse | null>(null);
  const [amount, setAmount] = useState('');
  const [label, setLabel] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    api.get<AddressResponse>('/api/v1/wallet/address')
      .then(setData)
      .catch((err: Error) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const uri = useMemo(() => {
    if (!data) return '';
    const params = new URLSearchParams();
    if (amount && Number(amount) > 0) params.set('amount', amount);
    if (label.trim()) params.set('label', label.trim());
    const query = params.toString();
    return `kron:${data.address}${query ? `?${query}` : ''}`;
  }, [data, amount, label]);

  const share = async () => {
    if (!data) return;
    if (navigator.share) {
      try {
        await navigator.share({ title: 'Mi dirección Kron', text: uri });
      } catch {
        // User cancelled or share is unavailable.
      }
    } else {
      await navigator.clipboard.writeText(uri);
      toast.push('URI copiado', 'success');
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Recibir KSC</h2>
          <p>Compartí tu dirección o un pedido de pago</p>
        </div>
      </div>

      <ErrorBanner message={error} />

      <div className="grid two">
        <div className="card center">
          {data ? (
            <>
              <div className="qr-box">
                <QRCodeSVG value={uri} size={216} bgColor="#ffffff" fgColor="#0b0d15" level="M" />
              </div>
              <div className="mt"><AddressChip address={data.address} /></div>
              <div className="btn-group mt" style={{ justifyContent: 'center' }}>
                <CopyButton text={data.address} label="Copiar dirección" />
                <button className="btn secondary small" onClick={share}>Compartir</button>
              </div>
            </>
          ) : null}
        </div>

        <div className="card">
          <h3 style={{ marginTop: 0 }}>Pedido de pago</h3>
          <div className="field">
            <label>Monto (opcional)</label>
            <input className="input" inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value.replace(',', '.'))} placeholder="0.00" />
          </div>
          <div className="field">
            <label>Etiqueta (opcional)</label>
            <input className="input" value={label} onChange={(event) => setLabel(event.target.value)} maxLength={80} placeholder="Ej: almuerzo" />
          </div>
          <div className="field">
            <label>URI generado</label>
            <div className="address-chip"><span className="mono wrap">{uri}</span></div>
          </div>
          <CopyButton text={uri} label="Copiar URI" />
          <div className="alert mt small">
            Cualquier persona con esta dirección puede enviarte KSC. No compartas tus códigos de recuperación ni tu contraseña.
          </div>
          {amount ? <div className="muted small mt">Pedido por {formatAmount(amount, 8)} KSC</div> : null}
        </div>
      </div>
    </div>
  );
}
