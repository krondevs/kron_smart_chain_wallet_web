import { useState } from 'react';
import { useAuth } from '../lib/auth';
import { useToast } from '../lib/toast';

export function AuthPage() {
  const { login, register } = useAuth();
  const toast = useToast();
  const [mode, setMode] = useState<'login' | 'register'>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setError('');
    if (mode === 'register' && password !== confirm) {
      setError('Las contraseñas no coinciden.');
      return;
    }
    setBusy(true);
    try {
      if (mode === 'login') {
        const result = await login(username.trim(), password);
        toast.push(result.mfa_required ? 'Ingresá tu código 2FA' : 'Sesión iniciada', 'success');
      } else {
        await register(username.trim(), password);
        toast.push('Cuenta creada. 2FA es opcional y podés activarlo en Seguridad.', 'success');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'No se pudo completar la operación.');
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
            <h1>Kron Wallet</h1>
            <p>Billetera custodial · Kron Smart Chain</p>
          </div>
        </div>

        <div className="auth-tabs">
          <button className={mode === 'login' ? 'active' : ''} onClick={() => { setMode('login'); setError(''); }}>Ingresar</button>
          <button className={mode === 'register' ? 'active' : ''} onClick={() => { setMode('register'); setError(''); }}>Crear cuenta</button>
        </div>

        <form onSubmit={submit}>
          <div className="field">
            <label>Usuario</label>
            <input
              className="input"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              placeholder="tu_usuario"
              minLength={3}
              maxLength={32}
              required
            />
            <span className="hint">3-32 caracteres: letras, números, punto, guion o guion bajo.</span>
          </div>
          <div className="field">
            <label>Contraseña</label>
            <input
              className="input"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              placeholder="Mínimo 12 caracteres"
              minLength={12}
              required
            />
          </div>
          {mode === 'register' ? (
            <div className="field">
              <label>Repetir contraseña</label>
              <input
                className="input"
                type="password"
                value={confirm}
                onChange={(event) => setConfirm(event.target.value)}
                autoComplete="new-password"
                minLength={12}
                required
              />
            </div>
          ) : null}

          {error ? <div className="alert error mb">{error}</div> : null}

          <button className="btn wide" type="submit" disabled={busy}>
            {busy ? <span className="spinner" /> : null}
            {mode === 'login' ? 'Ingresar' : 'Crear mi wallet'}
          </button>
        </form>

        <div className="alert warn mt">
          <strong>Importante:</strong> esta es una wallet custodial. Tus claves las administra el servidor.
          No hay recuperación por email. 2FA es opcional: podés activarlo desde Seguridad cuando quieras.
        </div>
      </div>
    </div>
  );
}
