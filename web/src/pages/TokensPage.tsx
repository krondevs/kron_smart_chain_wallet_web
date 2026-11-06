import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { api, type Token } from '../lib/api';
import { Empty, ErrorBanner, Loading, TokenRow } from '../components/common';
import { Modal } from '../components/Modal';
import { useToast } from '../lib/toast';

export function TokensPage() {
  const toast = useToast();
  const [tokens, setTokens] = useState<Token[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [addOpen, setAddOpen] = useState(false);
  const [contract, setContract] = useState('');
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setError('');
    try {
      const data = await api.get<{ tokens: Token[] }>('/api/v1/tokens');
      setTokens(data.tokens ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudieron cargar los tokens.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const add = async (event: React.FormEvent) => {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.post('/api/v1/tokens', { contract: contract.trim() });
      toast.push('Token agregado', 'success');
      setAddOpen(false);
      setContract('');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo agregar el token.');
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Tokens KRC20</h2>
          <p>Agregá contratos KRC20/QRC20 y gestioná tus saldos</p>
        </div>
        <button className="btn" onClick={() => setAddOpen(true)}>+ Agregar token</button>
      </div>

      <ErrorBanner message={error} />

      {tokens.length === 0 ? (
        <Empty title="No hay tokens agregados">
          <p>Pegá la dirección de un contrato KRC20 para agregarlo a tu portfolio.</p>
          <button className="btn secondary mt" onClick={() => setAddOpen(true)}>Agregar token</button>
        </Empty>
      ) : (
        <div className="card">
          {tokens.map((token) => <TokenRow key={token.contract} token={token} />)}
        </div>
      )}

      <div className="card mt">
        <h3 style={{ marginTop: 0 }}>¿Qué es KRC20?</h3>
        <p className="muted small">
          KRC20 es el nombre de Kron Smart Chain para el estándar QRC20. Los contratos, ABI y eventos son compatibles.
          Agregar un token solo lo muestra en tu portfolio; no crea ni transfiere tokens.
        </p>
        <p className="muted small">KRC721 no está implementado en este cliente.</p>
        <Link to="/contracts" className="small">Explorar contratos →</Link>
      </div>

      {addOpen ? (
        <Modal
          title="Agregar token KRC20"
          subtitle="Ingresá la dirección del contrato en Kron"
          onClose={() => !busy && setAddOpen(false)}
        >
          <form onSubmit={add}>
            <div className="field">
              <label>Dirección del contrato</label>
              <input className="input mono" value={contract} onChange={(event) => setContract(event.target.value)} placeholder="40 caracteres hex" required minLength={40} maxLength={42} />
              <span className="hint">La wallet consultará name, symbol, decimals y totalSupply al nodo.</span>
            </div>
            {error ? <div className="alert error mb">{error}</div> : null}
            <div className="btn-group">
              <button className="btn secondary" type="button" disabled={busy} onClick={() => setAddOpen(false)}>Cancelar</button>
              <button className="btn" type="submit" disabled={busy}>{busy ? <span className="spinner" /> : null} Agregar</button>
            </div>
          </form>
        </Modal>
      ) : null}
    </div>
  );
}
