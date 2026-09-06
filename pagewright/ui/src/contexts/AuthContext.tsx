import React, { useEffect, useState, type ReactNode } from 'react';
import { apiClient } from '../api/client';
import type { User, AuthResponse, LoginRequest, RegisterRequest } from '../types/api';

import { AuthContext } from './auth';
import { clearDrafts } from '../api/drafts';

function restoreUser(): User | null {
  const token = localStorage.getItem('token');
  const stored = localStorage.getItem('user');
  if (!token || !stored) return null;
  try {
    const user: unknown = JSON.parse(stored);
    if (user && typeof user === 'object' &&
        'id' in user && typeof user.id === 'string' &&
        'email' in user && typeof user.email === 'string' &&
        'created_at' in user && typeof user.created_at === 'string') {
      return user as User;
    }
  } catch { /* Invalid saved session is discarded. */ }
  localStorage.removeItem('token');
  localStorage.removeItem('user');
  return null;
}

export const AuthProvider: React.FC<{ children: ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<User | null>(restoreUser);
  const isLoading = false;
  useEffect(() => {
    const changed = (event: StorageEvent) => {
      if (event.key === 'user' || event.key === 'token' || event.key === null) setUser(restoreUser());
    };
    window.addEventListener('storage', changed);
    return () => window.removeEventListener('storage', changed);
  }, []);

  const login = async (data: LoginRequest) => {
    const response: AuthResponse = await apiClient.login(data);
    localStorage.setItem('token', response.token);
    localStorage.setItem('user', JSON.stringify(response.user));
    setUser(response.user);
  };

  const register = async (data: RegisterRequest) => {
    const response: AuthResponse = await apiClient.register(data);
    localStorage.setItem('token', response.token);
    localStorage.setItem('user', JSON.stringify(response.user));
    setUser(response.user);
  };

  const logout = () => {
    try { clearDrafts(sessionStorage); } catch { /* Browser storage may be unavailable. */ }
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    setUser(null);
  };

  return (
    <AuthContext.Provider value={{ user, isAuthenticated: !!user, isLoading, login, register, logout }}>
      {children}
    </AuthContext.Provider>
  );
};
