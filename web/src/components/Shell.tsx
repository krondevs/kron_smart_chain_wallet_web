import { NavLink, Outlet, useNavigate } from 'react-router-dom';
import { useAuth } from '../lib/auth';

const navItems = [
  { to: '/', label: 'Inicio', icon: '⌂', end: true },
  { to: '/send', label: 'Enviar', icon: '↑' },
  { to: '/receive', label: 'Recibir', icon: '↓' },
  { to: '/tokens', label: 'Tokens', icon: '◈' },
  { to: '/contracts', label: 'Contratos', icon: '▤' },
];

export function Shell() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();

  const onLogout = async () => {
    await logout();
    navigate('/');
  };

  return (
    <div className="app-shell with-side">
      <aside className="side-nav">
        <div className="brand">
          <div className="brand-mark">K</div>
          <div>
            <h1>Kron Wallet</h1>
            <p>Kron Smart Chain</p>
          </div>
        </div>
        {navItems.map((item) => (
          <NavLink key={item.to} to={item.to} end={item.end} className={({ isActive }) => isActive ? 'active' : ''}>
            <span className="nav-icon">{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
        <NavLink to="/history" className={({ isActive }) => isActive ? 'active' : ''}>
          <span className="nav-icon">≡</span>
          Historial
        </NavLink>
        <NavLink to="/settings" className={({ isActive }) => isActive ? 'active' : ''}>
          <span className="nav-icon">⚙</span>
          Seguridad
        </NavLink>
        <NavLink to="/api-keys" className={({ isActive }) => isActive ? 'active' : ''}>
          <span className="nav-icon">⚿</span>
          API Keys
        </NavLink>
        {user?.role === 'admin' ? (
          <NavLink to="/admin" className={({ isActive }) => isActive ? 'active' : ''}>
            <span className="nav-icon">◆</span>
            Admin
          </NavLink>
        ) : null}
        <div className="spacer" />
        <button className="btn ghost small" onClick={onLogout}>Cerrar sesión</button>
      </aside>

      <header className="topbar">
        <div className="brand">
          <div className="brand-mark">K</div>
          <div>
            <h1>Kron Wallet</h1>
            <p>Mainnet</p>
          </div>
        </div>
        <div className="topbar-actions">
          <span className="badge green"><span>●</span> Mainnet</span>
          <span className="badge accent">{user?.username}</span>
          <button className="btn ghost small" onClick={onLogout} title="Cerrar sesión">Salir</button>
        </div>
      </header>

      <main>
        <Outlet />
      </main>

      <nav className="bottom-nav" aria-label="Navegación principal">
        {navItems.map((item) => (
          <NavLink key={item.to} to={item.to} end={item.end} className={({ isActive }) => isActive ? 'active' : ''}>
            <span className="nav-icon">{item.icon}</span>
            {item.label}
          </NavLink>
        ))}
      </nav>
    </div>
  );
}
