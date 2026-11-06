import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider, useAuth } from './lib/auth';
import { MFAProvider } from './lib/mfa';
import { ToastProvider } from './lib/toast';
import { AuthPage } from './pages/AuthPage';
import { TOTPVerifyPage } from './pages/TOTPPage';
import { Shell } from './components/Shell';
import { InstallPrompt } from './components/InstallPrompt';
import { DashboardPage } from './pages/DashboardPage';
import { SendPage } from './pages/SendPage';
import { ReceivePage } from './pages/ReceivePage';
import { TokensPage } from './pages/TokensPage';
import { TokenDetailPage } from './pages/TokenDetailPage';
import { ContractsPage } from './pages/ContractsPage';
import { ContractDetailPage } from './pages/ContractDetailPage';
import { HistoryPage } from './pages/HistoryPage';
import { SettingsPage } from './pages/SettingsPage';
import { AdminPage } from './pages/AdminPage';
import { ApiKeysPage } from './pages/ApiKeysPage';

export function App() {
  return (
    <BrowserRouter>
      <ToastProvider>
        <AuthProvider>
          <MFAProvider>
            <Gate />
            <InstallPrompt />
          </MFAProvider>
        </AuthProvider>
      </ToastProvider>
    </BrowserRouter>
  );
}

function Gate() {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <div className="auth-wrap">
        <div className="auth-card center">
          <div className="brand" style={{ justifyContent: 'center' }}>
            <div className="brand-mark">K</div>
            <div style={{ textAlign: 'left' }}>
              <h1>Kron Wallet</h1>
              <p>Kron Smart Chain</p>
            </div>
          </div>
          <div className="spinner" style={{ margin: '18px auto' }} />
          <div className="muted small">Cargando wallet…</div>
        </div>
      </div>
    );
  }

  if (!user) return <AuthPage />;
  if (user.totp_enabled && !user.mfa_verified) return <TOTPVerifyPage />;

  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<DashboardPage />} />
        <Route path="send" element={<SendPage />} />
        <Route path="receive" element={<ReceivePage />} />
        <Route path="tokens" element={<TokensPage />} />
        <Route path="tokens/:contract" element={<TokenDetailPage />} />
        <Route path="contracts" element={<ContractsPage />} />
        <Route path="contracts/:address" element={<ContractDetailPage />} />
        <Route path="history" element={<HistoryPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route path="api-keys" element={<ApiKeysPage />} />
        <Route path="admin" element={<AdminPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
