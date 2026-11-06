import { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { api, type SessionInfo } from '../lib/api';
import { useAuth, type Onboarding } from '../lib/auth';
import { useMFA } from '../lib/mfa';
import { useToast } from '../lib/toast';
import { CopyButton, ErrorBanner, Loading } from '../components/common';
import { Modal } from '../components/Modal';
import { QRCodeSVG } from 'qrcode.react';
import { downloadText, formatAmount, formatDate, shorten } from '../lib/format';

type AddressEntry = { id: number; label: string; address: string; created_at: string };

export function SettingsPage() {
  const { user, logout, setupTOTP, verifyTOTP, disableTOTP } = useAuth();
  const { requestMFA } = useMFA();
  const toast = useToast();
  const navigate = useNavigate();
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [entries, setEntries] = useState<AddressEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordBusy, setPasswordBusy] = useState(false);

  const [label, setLabel] = useState('');
  const [address, setAddress] = useState('');
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [totpMode, setTotpMode] = useState<'enable' | 'disable' | null>(null);
  const [totpData, setTotpData] = useState<Onboarding | null>(null);
  const [totpCode, setTotpCode] = useState('');
  const [totpPassword, setTotpPassword] = useState('');
  const [totpBusy, setTotpBusy] = useState(false);
  const [totpError, setTotpError] = useState('');

  const load = useCallback(async () => {
    setError('');
    try {
      const [sessionData, bookData] = await Promise.all([
        api.get<{ sessions: SessionInfo[] }>('/api/v1/auth/sessions'),
        api.get<{ entries: AddressEntry[] }>('/api/v1/address-book'),
      ]);
      setSessions(sessionData.sessions ?? []);
      setEntries(bookData.entries ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cargar la configuración.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { load(); }, [load]);

  const changePassword = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    if (newPassword !== confirmPassword) {
      setError('Las contraseñas nuevas no coinciden.');
      return;
    }
    setPasswordBusy(true);
    try {
      await api.post('/api/v1/auth/password', { current_password: currentPassword, new_password: newPassword });
      toast.push('Contraseña cambiada. Iniciá sesión de nuevo.', 'success');
      await logout();
      navigate('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo cambiar la contraseña.');
    } finally {
      setPasswordBusy(false);
    }
  };

  const revoke = async (id: number) => {
    try {
      await api.del(`/api/v1/auth/sessions/${id}`);
      toast.push('Sesión revocada', 'success');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo revocar la sesión.');
    }
  };

  const addEntry = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    try {
      await api.post('/api/v1/address-book', { label, address });
      setLabel('');
      setAddress('');
      toast.push('Contacto agregado', 'success');
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo agregar el contacto.');
    }
  };

  const removeEntry = async (id: number) => {
    try {
      await api.del(`/api/v1/address-book/${id}`);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo borrar el contacto.');
    }
  };

  const openEnableTOTP = async () => {
    setTotpMode('enable');
    setTotpData(null);
    setTotpCode('');
    setTotpPassword('');
    setTotpError('');
    setTotpBusy(true);
    try {
      const data = await setupTOTP();
      setTotpData(data);
    } catch (err) {
      setTotpError(err instanceof Error ? err.message : 'No se pudo iniciar 2FA.');
    } finally {
      setTotpBusy(false);
    }
  };

  const confirmEnableTOTP = async () => {
    setTotpBusy(true);
    setTotpError('');
    try {
      await verifyTOTP(totpCode);
      toast.push('2FA activado', 'success');
      setTotpMode(null);
      setTotpData(null);
      setTotpCode('');
    } catch (err) {
      setTotpError(err instanceof Error ? err.message : 'Código inválido.');
    } finally {
      setTotpBusy(false);
    }
  };

  const openDisableTOTP = () => {
    setTotpMode('disable');
    setTotpCode('');
    setTotpPassword('');
    setTotpError('');
  };

  const confirmDisableTOTP = async () => {
    setTotpBusy(true);
    setTotpError('');
    try {
      await disableTOTP(totpPassword, totpCode);
      toast.push('2FA desactivado', 'success');
      setTotpMode(null);
      setTotpCode('');
      setTotpPassword('');
      setRecoveryCodes([]);
    } catch (err) {
      setTotpError(err instanceof Error ? err.message : 'No se pudo desactivar 2FA.');
    } finally {
      setTotpBusy(false);
    }
  };

  const reauth = async () => {
    try {
      await requestMFA('verify');
      toast.push('Reautenticación correcta', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo verificar.');
    }
  };

  const regenerateRecovery = async () => {
    if (!window.confirm('¿Generar códigos nuevos? Los anteriores dejarán de funcionar.')) return;
    try {
      const data = await api.post<{ recovery_codes: string[] }>('/api/v1/auth/recovery-codes/regenerate');
      setRecoveryCodes(data.recovery_codes);
      toast.push('Códigos regenerados', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudieron regenerar los códigos.');
    }
  };

  if (loading) return <div className="page"><Loading /></div>;

  return (
    <div className="page">
      <div className="page-head">
        <div>
          <h2>Seguridad y ajustes</h2>
          <p>Cuenta, 2FA, sesiones y contactos</p>
        </div>
      </div>

      <ErrorBanner message={error} />

      <div className="grid two">
        <div className="card">
          <div className="card-head"><h3>Cuenta</h3><span className="badge accent">{user?.username}</span></div>
          <div className="list">
            <div className="list-item"><span className="muted">Rol</span><span>{user?.role}</span></div>
            <div className="list-item"><span className="muted">2FA</span><span className={user?.totp_enabled ? 'badge green' : 'badge'}>{user?.totp_enabled ? 'Activo' : 'Deshabilitado'}</span></div>
            <div className="list-item"><span className="muted">Límite por tx</span><span>{user?.per_tx_limit === 0 ? 'Sin límite' : `${formatAmount((user?.per_tx_limit ?? 0) / 1e8, 8)} KSC`}</span></div>
            <div className="list-item"><span className="muted">Límite diario</span><span>{user?.daily_limit === 0 ? 'Sin límite' : `${formatAmount((user?.daily_limit ?? 0) / 1e8, 8)} KSC`}</span></div>
            <div className="list-item"><span className="muted">Creada</span><span>{formatDate(user?.created_at)}</span></div>
          </div>
          <div className="btn-group mt">
            {user?.totp_enabled ? (
              <>
                <button className="btn secondary small" onClick={reauth}>Reautenticar 2FA</button>
                <button className="btn danger small" onClick={openDisableTOTP}>Deshabilitar 2FA</button>
              </>
            ) : (
              <button className="btn secondary small" onClick={openEnableTOTP}>Habilitar 2FA</button>
            )}
            <Link to="/api-keys" className="btn secondary small">API Keys</Link>
            {user?.role === 'admin' ? <Link to="/admin" className="btn secondary small">Panel admin</Link> : null}
          </div>
        </div>

        <div className="card">
          <div className="card-head"><h3>Cambiar contraseña</h3></div>
          <form onSubmit={changePassword}>
            <div className="field"><label>Contraseña actual</label><input className="input" type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} autoComplete="current-password" required /></div>
            <div className="field"><label>Nueva contraseña</label><input className="input" type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} autoComplete="new-password" minLength={12} required /></div>
            <div className="field"><label>Repetir nueva</label><input className="input" type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} autoComplete="new-password" minLength={12} required /></div>
            <button className="btn wide" type="submit" disabled={passwordBusy}>{passwordBusy ? <span className="spinner" /> : null} Cambiar contraseña</button>
          </form>
          <div className="alert warn mt small">Al cambiar la contraseña se cierran todas las sesiones, incluida la actual.</div>
        </div>
      </div>

      <div className="card mt">
        <div className="card-head"><h3>Sesiones activas</h3><button className="btn ghost small" onClick={load}>Actualizar</button></div>
        <div className="list">
          {sessions.map((session) => (
            <div className="list-item" key={session.id}>
              <div className="main">
                <div className="title">{session.current ? 'Este dispositivo' : shorten(session.user_agent || 'Dispositivo', 32, 12)}</div>
                <div className="sub">{session.ip} · {formatDate(session.created_at)}</div>
              </div>
              <button className="btn danger small" onClick={() => revoke(session.id)}>Revocar</button>
            </div>
          ))}
          {sessions.length === 0 ? <div className="alert">No hay sesiones activas.</div> : null}
        </div>
      </div>

      <div className="card mt">
        <div className="card-head"><h3>Address book</h3></div>
        <form onSubmit={addEntry} className="grid two" style={{ alignItems: 'end' }}>
          <div className="field"><label>Etiqueta</label><input className="input" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Alice" required /></div>
          <div className="field"><label>Dirección</label><input className="input mono" value={address} onChange={(e) => setAddress(e.target.value)} placeholder="K..." required /></div>
          <button className="btn" type="submit">Agregar contacto</button>
        </form>
        <div className="list mt">
          {entries.map((entry) => (
            <div className="list-item" key={entry.id}>
              <div className="main"><div className="title">{entry.label}</div><div className="sub mono">{entry.address}</div></div>
              <button className="btn danger small" onClick={() => removeEntry(entry.id)}>Borrar</button>
            </div>
          ))}
          {entries.length === 0 ? <div className="alert">Sin contactos guardados.</div> : null}
        </div>
      </div>

      <div className="card mt">
        <h3 style={{ marginTop: 0 }}>Recuperación</h3>
        <p className="muted small">
          Sin email, la recuperación de contraseña se hace por el administrador después de verificar tu identidad.
          {user?.totp_enabled ? ' Los códigos de recuperación son de un solo uso; guardalos en un lugar seguro.' : ' Los códigos de recuperación se generan cuando activás 2FA.'}
        </p>
        <div className="btn-group mt">
          {user?.totp_enabled ? <button className="btn secondary small" onClick={regenerateRecovery}>Regenerar códigos de recuperación</button> : null}
          <button className="btn danger small" onClick={async () => { await logout(); navigate('/'); }}>Cerrar sesión</button>
        </div>
      </div>

      {totpMode === 'enable' ? (
        <Modal
          title="Activar 2FA"
          subtitle="Escaneá el QR con tu app autenticadora y confirmá el código."
          onClose={() => !totpBusy && setTotpMode(null)}
        >
          {totpError ? <div className="alert error mb">{totpError}</div> : null}
          {totpBusy && !totpData ? <div className="skeleton mb" style={{ height: 190 }} /> : null}
          {totpData?.otpauth_url ? (
            <div className="qr-box mb">
              <QRCodeSVG value={totpData.otpauth_url} size={180} bgColor="#ffffff" fgColor="#0b0d15" level="M" />
            </div>
          ) : null}
          {totpData?.totp_secret ? (
            <div className="field">
              <label>Clave manual</label>
              <div className="address-chip"><span className="mono wrap">{totpData.totp_secret}</span></div>
              <CopyButton text={totpData.totp_secret} label="Copiar clave" />
            </div>
          ) : null}
          {totpData?.recovery_codes && totpData.recovery_codes.length > 0 ? (
            <div className="card mb" style={{ padding: 12 }}>
              <div className="spread mb"><strong>Códigos de recuperación</strong><button className="btn ghost small" onClick={() => downloadText('kron-recovery-codes.txt', totpData.recovery_codes!.join('\n'))}>Descargar</button></div>
              <div className="alert warn small mb">Guardalos ahora. No se vuelven a mostrar.</div>
              <div className="mono small" style={{ display: 'grid', gap: 4 }}>
                {totpData.recovery_codes.map((code) => <span key={code}>{code}</span>)}
              </div>
              <div className="mt"><CopyButton text={totpData.recovery_codes.join('\n')} label="Copiar códigos" /></div>
            </div>
          ) : null}
          <div className="field">
            <label>Código de 6 dígitos</label>
            <input
              className="input mono"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={totpCode}
              onChange={(event) => setTotpCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="000000"
            />
          </div>
          <div className="btn-group">
            <button className="btn secondary" disabled={totpBusy} onClick={() => setTotpMode(null)}>Cancelar</button>
            <button className="btn" disabled={totpBusy || totpCode.length !== 6} onClick={confirmEnableTOTP}>{totpBusy ? <span className="spinner" /> : null} Activar 2FA</button>
          </div>
        </Modal>
      ) : null}

      {totpMode === 'disable' ? (
        <Modal
          title="Deshabilitar 2FA"
          subtitle="Confirmá con tu contraseña y un código actual."
          onClose={() => !totpBusy && setTotpMode(null)}
        >
          <div className="alert warn mb">Al deshabilitar 2FA, las operaciones quedarán protegidas solo por tu contraseña.</div>
          {totpError ? <div className="alert error mb">{totpError}</div> : null}
          <div className="field">
            <label>Contraseña actual</label>
            <input className="input" type="password" value={totpPassword} onChange={(event) => setTotpPassword(event.target.value)} autoComplete="current-password" />
          </div>
          <div className="field">
            <label>Código de 6 dígitos</label>
            <input className="input mono" inputMode="numeric" maxLength={6} value={totpCode} onChange={(event) => setTotpCode(event.target.value.replace(/\D/g, '').slice(0, 6))} placeholder="000000" />
          </div>
          <div className="btn-group">
            <button className="btn secondary" disabled={totpBusy} onClick={() => setTotpMode(null)}>Cancelar</button>
            <button className="btn danger" disabled={totpBusy || !totpPassword || totpCode.length !== 6} onClick={confirmDisableTOTP}>{totpBusy ? <span className="spinner" /> : null} Deshabilitar</button>
          </div>
        </Modal>
      ) : null}

      {recoveryCodes.length > 0 ? (
        <Modal title="Nuevos códigos de recuperación" subtitle="Guardalos ahora. No se volverán a mostrar." onClose={() => setRecoveryCodes([])}>
          <div className="alert warn mb">Reemplazan a los anteriores. Cada código sirve una sola vez.</div>
          <div className="mono small" style={{ display: 'grid', gap: 5 }}>
            {recoveryCodes.map((code) => <span key={code}>{code}</span>)}
          </div>
          <div className="btn-group mt">
            <button className="btn secondary" onClick={() => navigator.clipboard.writeText(recoveryCodes.join('\n'))}>Copiar</button>
            <button className="btn secondary" onClick={() => downloadText('kron-recovery-codes.txt', recoveryCodes.join('\n'))}>Descargar</button>
            <button className="btn" onClick={() => setRecoveryCodes([])}>Listo</button>
          </div>
        </Modal>
      ) : null}
    </div>
  );
}
