import React, { useState, type ReactNode } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/auth';
import './Layout.css';

interface LayoutProps {
  children: ReactNode;
  sidebar?: ReactNode;
}

export const Layout: React.FC<LayoutProps> = ({ children, sidebar }) => {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [versionsOpen, setVersionsOpen] = useState(() => !window.matchMedia('(max-width: 768px)').matches);

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const isActive = (path: string) => location.pathname === (path === '/' ? '/dashboard' : path);

  return (
    <div className="layout">
      <a className="skip-link" href="#main-content">Skip to main content</a>
      <header className="header">
        <div className="header-content">
          <div className="logo">
            <Link to="/">PageWright</Link>
          </div>
          {user && (
            <nav className="nav" aria-label="Main navigation">
              <Link to="/" className={isActive('/') ? 'active' : ''} aria-current={isActive('/') ? 'page' : undefined}>
                Dashboard
              </Link>
              <Link to="/profile" className={isActive('/profile') ? 'active' : ''} aria-current={isActive('/profile') ? 'page' : undefined}>
                Profile
              </Link>
              <button onClick={handleLogout} className="logout-btn">
                Logout
              </button>
            </nav>
          )}
        </div>
      </header>

      <div className="main-container">
        {sidebar && <aside className="sidebar" aria-label="Site versions">
          <details open={versionsOpen} onToggle={event => setVersionsOpen(event.currentTarget.open)}>
            <summary>Browse versions</summary>
            {sidebar}
          </details>
        </aside>}
        <main id="main-content" tabIndex={-1} className="content">{children}</main>
      </div>
    </div>
  );
};
