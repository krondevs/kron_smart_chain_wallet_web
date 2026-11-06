import { useEffect, useState } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { useAuth } from '../lib/auth';
import { downloadText } from '../lib/format';
import { useToast } from '../lib/toast';
import { CopyButton } from '../components/common';

export function TOTPSetupPage() {
  const { user, onboarding, setupTOTP, verifyTOTP, logout } = useAuth();
  const toast = useToast();
  const [secret, setSecret] = useState(onboarding?.totp_secret ?? '');
  const [otpauth, setOtpauth] = useState(onboarding?.otpauth_url ?? '');
  const [recovery, setRecovery] = useState<string[]>(onboarding?.recovery_codes ?? []);
  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(!onboarding?.totp_secret);

  useEffect(() => {
    if (secret) return;
    let alive = true;
    setLoading(true);
    setupTOTP()
      .then((data) => {
        if (!alive) return;
        setSecret(data.totp_secret ?? '');
        setOtpauth(data.otpauth_url ?? '');
      })
      .catch((err: Error) => {
        if (alive) setError(err.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => { alive = false; };
  }, [secret, setupTOTP]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      await verifyTOTP(code);
      toast.push('2FA activado', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Código inválido');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth-wrap">
      <div className="auth-card">
        <div className="brand">
          <div className="brand-mark">K</div>
          <div>
            <h1>Configurá 2FA</h1>
            <p>Protegé tu cuenta y tus fondos</p>
          </div>
        </div>

        <div className="alert warn mb">
          2FA agrega una capa extra de seguridad. Es opcional y podés activarlo o desactivarlo desde Seguridad.
        </div>

        {loading ? <div className="skeleton mb" style={{ height: 190 }} /> : null}
        {otpauth ? (
          <div className="qr-box mb">
            <QRCodeSVG value={otpauth} size={180} bgColor="#ffffff" fgColor="#0b0d15" level="M" />
          </div>
        ) : null}
        {secret ? (
          <div className="field">
            <label>Clave manual</label>
            <div className="address-chip"><span className="mono wrap">{secret}</span></div>
            <span className="hint">Escaneá el QR o ingresá esta clave en Google Authenticator, Authy o similar.</span>
          </div>
        ) : null}

        {recovery.length > 0 ? (
          <div className="card" style={{ padding: 14, marginBottom: 14 }}>
            <div className="spread mb">
              <strong>Códigos de recuperación</strong>
              <button className="btn ghost small" onClick={() => downloadText('kron-recovery-codes.txt', recovery.join('\n'))}>Descargar</button>
            </div>
            <div className="alert warn small">Guardalos ahora. No se vuelven a mostrar y sin ellos la recuperación requiere soporte.</div>
            <div className="mono small" style={{ display: 'grid', gap: 5, marginTop: 10 }}>
              {recovery.map((item) => <span key={item}>{item}</span>)}
            </div>
            <div className="mt"><CopyButton text={recovery.join('\n')} label="Copiar códigos" /></div>
          </div>
        ) : null}

        <form onSubmit={submit}>
          <div className="field">
            <label>Código de 6 dígitos</label>
            <input
              className="input mono"
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              value={code}
              onChange={(event) => setCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder="000000"
              required
            />
          </div>
          {error ? <div className="alert error mb">{error}</div> : null}
          <button className="btn wide" type="submit" disabled={busy || code.length !== 6}>
            {busy ? <span className="spinner" /> : null}
            Activar 2FA
          </button>
        </form>

        <div className="center mt">
          <span className="faint small">Usuario: {user?.username} · </span>
          <button className="btn ghost small" onClick={logout}>Cerrar sesión</button>
        </div>
      </div>
    </div>
  );
}

export function TOTPVerifyPage() {
  const { user, verifyTOTP, useRecovery, logout } = useAuth();
  const toast = useToast();
  const [code, setCode] = useState('');
  const [recoveryMode, setRecoveryMode] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    setBusy(true);
    try {
      if (recoveryMode) await useRecovery(code);
      else await verifyTOTP(code);
      toast.push('Identidad verificada', 'success');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Código inválido');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="auth-wrap">
      <div className="auth-card">
        <div className="brand">
          <div className="brand-mark">K</div>
          <div>
            <h1>Verificación 2FA</h1>
            <p>Hola, {user?.username}</p>
          </div>
        </div>
        <form onSubmit={submit}>
          <div className="field">
            <label>{recoveryMode ? 'Código de recuperación' : 'Código de 6 dígitos'}</label>
            <input
              className="input mono"
              inputMode={recoveryMode ? 'text' : 'numeric'}
              autoComplete="one-time-code"
              maxLength={recoveryMode ? 24 : 6}
              value={code}
              onChange={(event) => setCode(recoveryMode ? event.target.value.toUpperCase() : event.target.value.replace(/\D/g, '').slice(0, 6))}
              placeholder={recoveryMode ? 'XXXX-XXXX-XXXX-XXXX' : '000000'}
              autoFocus
              required
            />
          </div>
          {error ? <div className="alert error mb">{error}</div> : null}
          <button className="btn wide" type="submit" disabled={busy || code.length < 6}>
            {busy ? <span className="spinner" /> : null}
            Verificar
          </button>
        </form>
        <div className="center mt">
          <button className="btn ghost small" onClick={() => { setRecoveryMode((v) => !v); setCode(''); setError(''); }}>
            {recoveryMode ? 'Usar app autenticadora' : 'Usar código de recuperación'}
          </button>
        </div>
        <div className="center mt">
          <button className="btn ghost small" onClick={logout}>Cerrar sesión</button>
        </div>
      </div>
    </div>
  );
}
