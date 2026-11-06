import { useCallback, useEffect, useState } from 'react';
import { api } from '../lib/api';
import { useAuth } from '../lib/auth';
import { ErrorBanner, Loading, Stat } from '../components/common';
import { Modal } from '../components/Modal';
import { useToast } from '../lib/toast';
import { formatDate, shorten } from '../lib/format';

type AdminUser = {
  id: number;
  username: string;
  role: string;
  status: string;
  tier: number;
  totp_enabled: boolean;
  per_tx_limit: number;
  daily_limit: number;
  created_at: string;
  last_login_at?: string | null;
  wallet_address?: string;
};

type AdminToken = {
  id: number;
  contract: string;
  name: string;
  symbol: string;
  decimals: number;
  verified: boolean;
  users: number;
};

type AuditEvent = {
  id: number;
  user_id?: number;
  action: string;
  detail: string;
  ip: string;
  request_id: string;
  created_at: string;
};

type Stats = {
  users: number;
  wallets: number;
  sessions: number;
  tokens: number;
  contracts: number;
  tx_intents: number;
  chain?: { chain: string; blocks: number; bestblockhash: string; initialblockdownload: boolean };
  gas?: { min_gas_price_sat: number; block_gas_limit: number };
  chain_error?: string;
};

type Tab = 'overview' | 'users' | 'tokens' | 'audit';

export function AdminPage() {
  const { user } = useAuth();
  const toast = useToast();
  const [tab, setTab] = useState<Tab>('overview');
  const [stats, setStats] = useState<Stats | null>(null);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [tokens, setTokens] = useState<AdminToken[]>([]);
  const [audit, setAudit] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [totpResult, setTotpResult] = useState<{ username: string; secret: string; url: string; recovery: string[] } | null>(null);

  const load = useCallback(async () => {
    setError('');
    try {
      const [statsData, usersData, tokensData, auditData] = await Promise.all([
        api.get<Stats>('/api/v1/admin/stats'),
        api.get<{ users: AdminUser[] }>('/api/v1/admin/users?page=1'),
        api.get<{ tokens: AdminToken[] }>('/api/v1/admin/tokens'),
        api.get<{ events: AuditEvent[] }>('/api/v1/admin/audit?page=1'),
      ]);
      setStats(statsData);
      setUsers(usersData.users ?? []);
      setTokens(tokensData.tokens ?? []);
      setAudit(auditData.events ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cargar el panel.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (user?.role === 'admin') load();
  }, [load, user?.role]);

  const setStatus = async (user: AdminUser) => {
    const status = user.status === 'active' ? 'suspended' : 'active';
    if (!window.confirm(`¿Cambiar estado de ${user.username} a ${status}?`)) return;
    try {
      await api.post(`/api/v1/admin/users/${user.id}/status`, { status });
      toast.push('Estado actualizado', 'success');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo actualizar.');
    }
  };

  const setLimits = async (user: AdminUser) => {
    const perTx = window.prompt('Límite por transacción (KSC). 0 = sin límite', String(user.per_tx_limit === 0 ? 0 : user.per_tx_limit / 1e8));
    if (perTx === null) return;
    const daily = window.prompt('Límite diario (KSC). 0 = sin límite', String(user.daily_limit === 0 ? 0 : user.daily_limit / 1e8));
    if (daily === null) return;
    try {
      await api.post(`/api/v1/admin/users/${user.id}/limits`, { per_tx_limit: perTx, daily_limit: daily });
      toast.push('Límites actualizados', 'success');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudieron actualizar los límites.');
    }
  };

  const resetPassword = async (user: AdminUser) => {
    const password = window.prompt(`Nueva contraseña para ${user.username} (mínimo 12, con letra y número)`);
    if (!password) return;
    try {
      await api.post(`/api/v1/admin/users/${user.id}/reset-password`, { new_password: password });
      toast.push('Contraseña reseteada', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo resetear la contraseña.');
    }
  };

  const resetTOTP = async (user: AdminUser) => {
    if (!window.confirm(`¿Resetear 2FA de ${user.username}? Deberá configurarlo de nuevo.`)) return;
    try {
      const data = await api.post<{ secret: string; otpauth_url: string; recovery_codes: string[] }>(`/api/v1/admin/users/${user.id}/reset-totp`);
      setTotpResult({ username: user.username, secret: data.secret, url: data.otpauth_url, recovery: data.recovery_codes });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo resetear 2FA.');
    }
  };

  const provision = async (user: AdminUser) => {
    try {
      const data = await api.post<{ created: boolean; address: string }>(`/api/v1/admin/users/${user.id}/provision`);
      toast.push(data.created ? 'Wallet creada' : 'La wallet ya existía', 'success');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo aprovisionar la wallet.');
    }
  };

  const verifyToken = async (token: AdminToken) => {
    try {
      await api.post(`/api/v1/admin/tokens/${token.id}/verify`, { verified: !token.verified });
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo actualizar el token.');
    }
  };

  if (user?.role !== 'admin') {
    return <div className="page"><ErrorBanner message="Necesitás rol de administrador para ver esta sección." /></div>;
  }

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Panel de administración</h2>
          <p>Usuarios, límites, tokens y auditoría</p>
        </div>
        <button className="btn ghost small" onClick={load}>Actualizar</button>
      </div>

      <ErrorBanner message={error} />

      <div className="auth-tabs mb">
        <button className={tab === 'overview' ? 'active' : ''} onClick={() => setTab('overview')}>Resumen</button>
        <button className={tab === 'users' ? 'active' : ''} onClick={() => setTab('users')}>Usuarios</button>
        <button className={tab === 'tokens' ? 'active' : ''} onClick={() => setTab('tokens')}>Tokens</button>
        <button className={tab === 'audit' ? 'active' : ''} onClick={() => setTab('audit')}>Auditoría</button>
      </div>

      {tab === 'overview' ? (
        <>
          <div className="grid stats">
            <Stat label="Usuarios" value={stats?.users ?? 0} />
            <Stat label="Wallets" value={stats?.wallets ?? 0} />
            <Stat label="Sesiones" value={stats?.sessions ?? 0} />
            <Stat label="Tokens" value={stats?.tokens ?? 0} />
            <Stat label="Contratos" value={stats?.contracts ?? 0} />
            <Stat label="Tx intentos" value={stats?.tx_intents ?? 0} />
            <Stat label="Bloque" value={stats?.chain?.blocks?.toLocaleString('es') ?? '—'} />
            <Stat label="Gas mínimo" value={stats?.gas ? `${stats.gas.min_gas_price_sat} sat` : '—'} />
          </div>
          {stats?.chain_error ? <div className="alert error mt">Nodo: {stats.chain_error}</div> : null}
          <div className="card mt">
            <h3 style={{ marginTop: 0 }}>Nodo</h3>
            <div className="list">
              <div className="list-item"><span className="muted">Red</span><span>{stats?.chain?.chain ?? '—'}</span></div>
              <div className="list-item"><span className="muted">Best block</span><span className="mono">{shorten(stats?.chain?.bestblockhash, 20, 12)}</span></div>
              <div className="list-item"><span className="muted">IBD</span><span>{stats?.chain?.initialblockdownload ? 'Sincronizando' : 'Sincronizado'}</span></div>
              <div className="list-item"><span className="muted">Block gas limit</span><span>{stats?.gas?.block_gas_limit?.toLocaleString('es') ?? '—'}</span></div>
            </div>
          </div>
        </>
      ) : null}

      {tab === 'users' ? (
        <div className="table-wrap">
          <table>
            <thead><tr><th>Usuario</th><th>Estado</th><th>2FA</th><th>Wallet</th><th>Límites (sat)</th><th>Acciones</th></tr></thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id}>
                  <td><strong>{user.username}</strong><div className="faint">{user.role} · {formatDate(user.created_at)}</div></td>
                  <td><span className={`badge ${user.status === 'active' ? 'green' : 'red'}`}>{user.status}</span></td>
                  <td>{user.totp_enabled ? 'Sí' : 'No'}</td>
                  <td className="mono">{user.wallet_address ? shorten(user.wallet_address) : '—'}</td>
                  <td>{user.per_tx_limit === 0 ? '∞' : user.per_tx_limit} / {user.daily_limit === 0 ? '∞' : user.daily_limit}</td>
                  <td>
                    <div className="row">
                      <button className="btn ghost small" onClick={() => setStatus(user)}>{user.status === 'active' ? 'Suspender' : 'Activar'}</button>
                      <button className="btn ghost small" onClick={() => setLimits(user)}>Límites</button>
                      <button className="btn ghost small" onClick={() => resetPassword(user)}>Password</button>
                      <button className="btn ghost small" onClick={() => resetTOTP(user)}>2FA</button>
                      {!user.wallet_address ? <button className="btn ghost small" onClick={() => provision(user)}>Wallet</button> : null}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'tokens' ? (
        <div className="table-wrap">
          <table>
            <thead><tr><th>Token</th><th>Contrato</th><th>Decimales</th><th>Usuarios</th><th>Verificado</th><th></th></tr></thead>
            <tbody>
              {tokens.map((token) => (
                <tr key={token.id}>
                  <td><strong>{token.symbol}</strong><div className="faint">{token.name}</div></td>
                  <td className="mono">{shorten(token.contract, 12, 8)}</td>
                  <td>{token.decimals}</td>
                  <td>{token.users}</td>
                  <td><span className={`badge ${token.verified ? 'green' : 'yellow'}`}>{token.verified ? 'Sí' : 'No'}</span></td>
                  <td><button className="btn ghost small" onClick={() => verifyToken(token)}>{token.verified ? 'Quitar' : 'Verificar'}</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {tab === 'audit' ? (
        <div className="table-wrap">
          <table>
            <thead><tr><th>ID</th><th>Acción</th><th>Usuario</th><th>Detalle</th><th>IP</th><th>Fecha</th></tr></thead>
            <tbody>
              {audit.map((event) => (
                <tr key={event.id}>
                  <td>{event.id}</td>
                  <td className="mono">{event.action}</td>
                  <td>{event.user_id ?? '—'}</td>
                  <td className="wrap" style={{ maxWidth: 320 }}>{event.detail}</td>
                  <td>{event.ip}</td>
                  <td>{formatDate(event.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}

      {totpResult ? (
        <Modal title={`2FA reseteado: ${totpResult.username}`} onClose={() => setTotpResult(null)}>
          <div className="field"><label>Clave manual</label><div className="address-chip"><span className="mono wrap">{totpResult.secret}</span></div></div>
          <div className="field"><label>URI otpauth</label><div className="address-chip"><span className="mono wrap">{totpResult.url}</span></div></div>
          <div className="field">
            <label>Códigos de recuperación</label>
            <div className="mono small" style={{ display: 'grid', gap: 4 }}>{totpResult.recovery.map((code) => <span key={code}>{code}</span>)}</div>
          </div>
          <div className="alert warn">Entregá estos datos al usuario por un canal seguro. No se volverán a mostrar.</div>
        </Modal>
      ) : null}
    </div>
  );
}
