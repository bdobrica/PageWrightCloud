import React, { type ReactNode } from 'react';
import { Link, useNavigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';
import './Layout.css';

interface LayoutProps {
  children: ReactNode;
  sidebar?: ReactNode;
}

export const Layout: React.FC<LayoutProps> = ({ children, sidebar }) => {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();

  const handleLogout = () => {
    logout();
    navigate('/login');
  };

  const isActive = (path: string) => location.pathname === path;

  return (
    <div className="layout">
      <header className="header">
        <div className="header-content">
          <div className="logo">
            <Link to="/">PageWright</Link>
          </div>
          {user && (
            <nav className="nav">
              <Link to="/" className={isActive('/') ? 'active' : ''}>
                Dashboard
              </Link>
              <Link to="/profile" className={isActive('/profile') ? 'active' : ''}>
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
        {sidebar && <aside className="sidebar">{sidebar}</aside>}
        <main className="content">{children}</main>
      </div>
    </div>
  );
};
