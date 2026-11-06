import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, clearCSRF, ensureCSRF, setUnauthorizedHandler, type User } from './api';

export type Onboarding = {
  totp_secret?: string;
  otpauth_url?: string;
  recovery_codes?: string[];
};

type MeResponse = {
  user: User;
  wallet: { address: string };
};

type LoginResponse = {
  user: User;
  mfa_required: boolean;
  totp_enabled: boolean;
};

type RegisterResponse = {
  user: User;
  wallet: { address: string };
  mfa_required: boolean;
};

type AuthContextValue = {
  user: User | null;
  loading: boolean;
  onboarding: Onboarding | null;
  login: (username: string, password: string) => Promise<LoginResponse>;
  register: (username: string, password: string) => Promise<RegisterResponse>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  verifyTOTP: (code: string) => Promise<void>;
  setupTOTP: () => Promise<Onboarding>;
  disableTOTP: (password: string, code: string) => Promise<void>;
  useRecovery: (code: string) => Promise<void>;
  clearOnboarding: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [onboarding, setOnboarding] = useState<Onboarding | null>(null);

  const refresh = useCallback(async () => {
    try {
      const data = await api.get<MeResponse>('/api/v1/auth/me');
      setUser({ ...data.user, wallet_address: data.wallet?.address ?? data.user.wallet_address });
    } catch {
      setUser(null);
    }
  }, []);

  useEffect(() => {
    setUnauthorizedHandler(() => setUser(null));
    return () => setUnauthorizedHandler(null);
  }, []);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        await ensureCSRF();
        const data = await api.get<{ user: User | null; wallet?: { address?: string } }>('/api/v1/auth/session');
        if (alive) {
          setUser(data.user
            ? { ...data.user, wallet_address: data.wallet?.address ?? data.user.wallet_address }
            : null);
        }
      } catch {
        if (alive) setUser(null);
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, []);

  const login = useCallback(async (username: string, password: string) => {
    const data = await api.post<LoginResponse>('/api/v1/auth/login', { username, password });
    setUser({ ...data.user, mfa_verified: false });
    return data;
  }, []);

  const register = useCallback(async (username: string, password: string) => {
    const data = await api.post<RegisterResponse>('/api/v1/auth/register', { username, password });
    setUser({ ...data.user, wallet_address: data.wallet?.address ?? data.user.wallet_address, mfa_verified: true });
    setOnboarding(null);
    return data;
  }, []);

  const logout = useCallback(async () => {
    try {
      await api.post('/api/v1/auth/logout');
    } catch {
      // Ignore a stale session; the local state must still be cleared.
    }
    setUser(null);
    setOnboarding(null);
    clearCSRF();
    try {
      await ensureCSRF();
    } catch {
      // The login page will retry on its first action.
    }
  }, []);

  const verifyTOTP = useCallback(async (code: string) => {
    await api.post('/api/v1/auth/totp/verify', { code });
    await refresh();
    setOnboarding(null);
  }, [refresh]);

  const setupTOTP = useCallback(async () => {
    const data = await api.post<{ secret: string; otpauth_url: string; recovery_codes: string[] }>('/api/v1/auth/totp/setup');
    const next = { totp_secret: data.secret, otpauth_url: data.otpauth_url, recovery_codes: data.recovery_codes };
    setOnboarding(next);
    return next;
  }, []);

  const disableTOTP = useCallback(async (password: string, code: string) => {
    await api.post('/api/v1/auth/totp/disable', { password, code });
    await refresh();
    setOnboarding(null);
  }, [refresh]);

  const useRecovery = useCallback(async (code: string) => {
    await api.post('/api/v1/auth/recovery', { code });
    await refresh();
  }, [refresh]);

  const value = useMemo<AuthContextValue>(() => ({
    user,
    loading,
    onboarding,
    login,
    register,
    logout,
    refresh,
    verifyTOTP,
    setupTOTP,
    disableTOTP,
    useRecovery,
    clearOnboarding: () => setOnboarding(null),
  }), [user, loading, onboarding, login, register, logout, refresh, verifyTOTP, setupTOTP, disableTOTP, useRecovery]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used inside AuthProvider');
  return value;
}
