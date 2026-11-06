import { useCallback, useEffect, useMemo, useState } from 'react';
import { api, type APIKeyInfo } from '../lib/api';
import { CopyButton, ErrorBanner, Loading } from '../components/common';
import { Modal } from '../components/Modal';
import { useToast } from '../lib/toast';
import { downloadText, formatDate } from '../lib/format';

const PRESETS = [
  { label: '1 día', days: 1 },
  { label: '7 días', days: 7 },
  { label: '30 días', days: 30 },
  { label: '90 días', days: 90 },
  { label: '1 año', days: 365 },
];

function statusTone(status: APIKeyInfo['status']) {
  if (status === 'active') return 'green';
  if (status === 'expired') return 'yellow';
  return 'red';
}

function localDateTime(days: number) {
  const date = new Date(Date.now() + days * 24 * 60 * 60 * 1000);
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function ApiKeysPage() {
  const toast = useToast();
  const [keys, setKeys] = useState<APIKeyInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [createOpen, setCreateOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  const [expiration, setExpiration] = useState('30d');
  const [customDate, setCustomDate] = useState(localDateTime(30));
  const [newKey, setNewKey] = useState('');
  const [newKeyName, setNewKeyName] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const data = await api.get<{ api_keys: APIKeyInfo[] }>('/api/v1/api-keys');
      setKeys(data.api_keys ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudieron cargar las API keys.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const activeCount = useMemo(() => keys.filter((key) => key.status === 'active').length, [keys]);

  const openCreate = () => {
    setName('');
    setExpiration('30d');
    setCustomDate(localDateTime(30));
    setCreateOpen(true);
  };

  const create = async (event: React.FormEvent) => {
    event.preventDefault();
    setCreating(true);
    setError('');
    try {
      let expiresAt: string;
      if (expiration === 'custom') {
        const parsed = new Date(customDate);
        if (Number.isNaN(parsed.getTime())) throw new Error('Fecha de expiración inválida.');
        expiresAt = parsed.toISOString();
      } else {
        const days = Number(expiration.replace('d', ''));
        expiresAt = new Date(Date.now() + days * 24 * 60 * 60 * 1000).toISOString();
      }
      const data = await api.post<{ key: string; api_key: APIKeyInfo }>('/api/v1/api-keys', {
        name: name.trim(),
        expires_at: expiresAt,
      });
      setCreateOpen(false);
      setNewKey(data.key);
      setNewKeyName(data.api_key.name);
      await load();
      toast.push('API key creada', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo crear la API key.');
    } finally {
      setCreating(false);
    }
  };

  const revoke = async (key: APIKeyInfo) => {
    if (!window.confirm(`¿Revocar la API key "${key.name}"? Las aplicaciones que la usen dejarán de funcionar.`)) return;
    setError('');
    try {
      await api.del(`/api/v1/api-keys/${key.id}`);
      toast.push('API key revocada', 'success');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo revocar la API key.');
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>API Keys</h2>
          <p>Conectá aplicaciones externas a tu wallet</p>
        </div>
        <button className="btn" onClick={openCreate} disabled={activeCount >= 3}>+ Crear API key</button>
      </div>

      <ErrorBanner message={error} />
      <div className="alert warn mb">
        <strong>Acceso total.</strong> Una API key puede consultar saldos, enviar KSC, operar tokens y contratos.
        Guardala como una contraseña. Tiene un límite de <strong>30 llamadas por minuto</strong>.
      </div>

      <div className="card">
        <div className="card-head">
          <h3>Keys activas</h3>
          <span className="badge accent">{activeCount} / 3</span>
        </div>
        {keys.length === 0 ? (
          <div className="alert">Todavía no creaste ninguna API key.</div>
        ) : (
          <div className="list">
            {keys.map((key) => (
              <div className="list-item" key={key.id}>
                <div className="main">
                  <div className="title">{key.name} <span className={`badge ${statusTone(key.status)}`}>{key.status}</span></div>
                  <div className="sub mono">{key.prefix}… · límite {key.rate_limit}/min</div>
                  <div className="sub">Creada: {formatDate(key.created_at)} · Expira: {formatDate(key.expires_at)}</div>
                  <div className="sub">Último uso: {key.last_used_at ? formatDate(key.last_used_at) : 'nunca'}</div>
                </div>
                <button className="btn danger small" onClick={() => revoke(key)} disabled={key.status === 'revoked'}>Revocar</button>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className="card mt">
        <h3 style={{ marginTop: 0 }}>Conexión</h3>
        <p className="muted small">Usá la key en el header:</p>
        <pre className="mono small" style={{ whiteSpace: 'pre-wrap' }}>{`Authorization: Bearer ksc_...`}</pre>
        <p className="muted small">También podés usar:</p>
        <pre className="mono small" style={{ whiteSpace: 'pre-wrap' }}>{`X-API-Key: ksc_...`}</pre>
        <p className="muted small">Documentación para LLMs: <a href="/llms.txt" target="_blank" rel="noreferrer">/llms.txt</a> · OpenAPI: <a href="/api/v1/openapi.json" target="_blank" rel="noreferrer">/api/v1/openapi.json</a></p>
      </div>

      {createOpen ? (
        <Modal
          title="Crear API key"
          subtitle="Elegí un nombre y una expiración"
          onClose={() => !creating && setCreateOpen(false)}
        >
          <form onSubmit={create}>
            <div className="field">
              <label>Nombre de la aplicación</label>
              <input className="input" value={name} onChange={(event) => setName(event.target.value)} placeholder="Mi tienda / bot / integración" required maxLength={80} />
            </div>
            <div className="field">
              <label>Expiración</label>
              <select className="select" value={expiration} onChange={(event) => setExpiration(event.target.value)}>
                {PRESETS.map((preset) => <option key={preset.days} value={`${preset.days}d`}>{preset.label}</option>)}
                <option value="custom">Fecha personalizada</option>
              </select>
            </div>
            {expiration === 'custom' ? (
              <div className="field">
                <label>Fecha y hora</label>
                <input className="input" type="datetime-local" value={customDate} onChange={(event) => setCustomDate(event.target.value)} required />
              </div>
            ) : null}
            {activeCount >= 3 ? <div className="alert error mb">Ya tenés 3 API keys activas. Revocá una para crear otra.</div> : null}
            {error ? <div className="alert error mb">{error}</div> : null}
            <div className="btn-group">
              <button className="btn secondary" type="button" disabled={creating} onClick={() => setCreateOpen(false)}>Cancelar</button>
              <button className="btn" type="submit" disabled={creating || activeCount >= 3}>{creating ? <span className="spinner" /> : null} Crear key</button>
            </div>
          </form>
        </Modal>
      ) : null}

      {newKey ? (
        <Modal
          title="API key creada"
          subtitle="Copiala ahora. No se vuelve a mostrar."
          onClose={() => setNewKey('')}
        >
          <div className="alert warn mb">Esta key da acceso total a la wallet. Guardala en un gestor de secretos.</div>
          <div className="field">
            <label>{newKeyName || 'API key'}</label>
            <div className="address-chip"><span className="mono wrap">{newKey}</span></div>
          </div>
          <div className="btn-group">
            <CopyButton text={newKey} label="Copiar key" />
            <button className="btn secondary" onClick={() => downloadText('kron-api-key.txt', newKey)}>Descargar</button>
            <button className="btn" onClick={() => setNewKey('')}>Listo</button>
          </div>
        </Modal>
      ) : null}
    </div>
  );
}
