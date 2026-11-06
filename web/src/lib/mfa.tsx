import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { setMFAHandler } from './api';
import { useAuth } from './auth';
import { useToast } from './toast';
import { Modal } from '../components/Modal';

type Mode = 'verify' | 'setup';

type PendingMFA = {
  mode: Mode;
  resolve: () => void;
  reject: (error: Error) => void;
  secret?: string;
  otpauthURL?: string;
};

type MFAContextValue = {
  requestMFA: (mode?: Mode) => Promise<void>;
};

const MFAContext = createContext<MFAContextValue | null>(null);

export function MFAProvider({ children }: { children: ReactNode }) {
  const { user, setupTOTP, verifyTOTP, useRecovery } = useAuth();
  const toast = useToast();
  const [pending, setPending] = useState<PendingMFA | null>(null);
  const [code, setCode] = useState('');
  const [recoveryMode, setRecoveryMode] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const requestMFA = useCallback((mode?: Mode) => {
    return new Promise<void>((resolve, reject) => {
      if (!user) {
        reject(new Error('Sesión no iniciada'));
        return;
      }
      const nextMode = mode ?? (user.totp_enabled ? 'verify' : 'setup');
      setCode('');
      setError('');
      setRecoveryMode(false);
      setPending({ mode: nextMode, resolve, reject });
      if (nextMode === 'setup') {
        setBusy(true);
        setupTOTP()
          .then((data) => {
            setPending((current) => current ? { ...current, secret: data.totp_secret, otpauthURL: data.otpauth_url } : current);
          })
          .catch((err: Error) => {
            setError(err.message);
            setPending((current) => {
              current?.reject(err);
              return null;
            });
          })
          .finally(() => setBusy(false));
      }
    });
  }, [user, setupTOTP]);

  useEffect(() => {
    setMFAHandler(async (codeName) => {
      const mode: Mode = codeName === 'totp_setup_required' || !user?.totp_enabled ? 'setup' : 'verify';
      await requestMFA(mode);
    });
    return () => setMFAHandler(null);
  }, [requestMFA, user?.totp_enabled]);

  const close = useCallback(() => {
    if (pending) pending.reject(new Error('Operación cancelada'));
    setPending(null);
    setCode('');
    setError('');
  }, [pending]);

  const submit = useCallback(async () => {
    if (!pending) return;
    setError('');
    setBusy(true);
    try {
      if (recoveryMode) {
        await useRecovery(code);
      } else {
        await verifyTOTP(code);
      }
      pending.resolve();
      setPending(null);
      setCode('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Código inválido');
    } finally {
      setBusy(false);
    }
  }, [pending, recoveryMode, code, useRecovery, verifyTOTP]);

  const value = useMemo(() => ({ requestMFA }), [requestMFA]);

  return (
    <MFAContext.Provider value={value}>
      {children}
      {pending ? (
        <Modal
          title={pending.mode === 'setup' ? 'Configurá 2FA' : 'Confirmá tu identidad'}
          subtitle={pending.mode === 'setup'
            ? 'Escaneá el código con Google Authenticator, Authy o similar.'
            : 'Ingresá el código de 6 dígitos de tu app autenticadora.'}
          onClose={close}
        >
          {pending.mode === 'setup' ? (
            <>
              {pending.otpauthURL ? (
                <div className="qr-box mb">
                  <QRCodeSVG value={pending.otpauthURL} size={172} bgColor="#ffffff" fgColor="#0b0d15" level="M" />
                </div>
              ) : (
                <div className="skeleton mb" style={{ height: 172 }} />
              )}
              {pending.secret ? (
                <div className="field">
                  <label>Clave manual</label>
                  <div className="address-chip"><span className="mono wrap">{pending.secret}</span></div>
                </div>
              ) : null}
            </>
          ) : null}
          {recoveryMode ? (
            <div className="field">
              <label>Uno de tus códigos de recuperación</label>
              <input className="input mono" value={code} onChange={(event) => setCode(event.target.value)} placeholder="XXXX-XXXX-XXXX-XXXX" autoFocus />
            </div>
          ) : (
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
                autoFocus
              />
            </div>
          )}
          {error ? <div className="alert error mb">{error}</div> : null}
          <div className="btn-group">
            <button className="btn wide" disabled={busy || code.length < 6} onClick={submit}>
              {busy ? <span className="spinner" /> : null}
              {pending.mode === 'setup' ? 'Activar 2FA' : 'Verificar'}
            </button>
          </div>
          <div className="center mt">
            <button className="btn ghost small" onClick={() => { setRecoveryMode((v) => !v); setCode(''); setError(''); }}>
              {recoveryMode ? 'Usar app autenticadora' : 'Usar código de recuperación'}
            </button>
          </div>
        </Modal>
      ) : null}
    </MFAContext.Provider>
  );
}

export function useMFA(): MFAContextValue {
  const value = useContext(MFAContext);
  if (!value) throw new Error('useMFA must be used inside MFAProvider');
  return value;
}
