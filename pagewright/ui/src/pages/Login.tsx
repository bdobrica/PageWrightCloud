import { getErrorMessage } from '../utils/errors';
import React, { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/auth';
import './Auth.css';

export const Login: React.FC = () => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const { login } = useAuth();
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setIsLoading(true);

    try {
      await login({ email, password });
      navigate('/');
    } catch (err: unknown) {
      setError(getErrorMessage(err, 'Login failed. Please check your credentials.'));
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="auth-container">
      <div className="auth-box">
        <h1>Login to PageWright</h1>

        <form onSubmit={handleSubmit} className="pure-form pure-form-stacked">
          {error && <div className="error-message">{error}</div>}

          <label htmlFor="email">Email</label>
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            disabled={isLoading}
            placeholder="your@email.com"
          />

          <label htmlFor="password">Password</label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            disabled={isLoading}
            placeholder="••••••••"
          />

          <button type="submit" className="pure-button pure-button-primary" disabled={isLoading}>
            {isLoading ? 'Logging in...' : 'Login'}
          </button>
        </form>

        <div className="auth-links">
          <Link to="/forgot-password">Forgot Password?</Link>
          <Link to="/register">Don't have an account? Register</Link>
        </div>

        <p>Sign in with email and password. Google sign-in is not available in this MVP.</p>
      </div>
    </div>
  );
};
