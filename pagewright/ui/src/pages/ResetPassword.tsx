import { getErrorMessage } from '../utils/errors';
import React, { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { validPassword, passwordPolicyMessage } from '../utils/password';
import { apiClient } from '../api/client';
import './Auth.css';

export const ResetPassword: React.FC = () => {
  const [searchParams] = useSearchParams();
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [token] = useState(() => new URLSearchParams(window.location.hash.slice(1)).get('token') || searchParams.get('token') || '');
  const [complete, setComplete] = useState(false);
  useEffect(() => {
    // New links use a fragment (never sent to the server). Also scrub legacy
    // query links from the current history entry; keep the token only in memory.
    window.history.replaceState(null, '', window.location.pathname);
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');

    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }

    if (!validPassword(password)) {
      setError(passwordPolicyMessage);
      return;
    }

    setIsLoading(true);

    try {
      await apiClient.resetPassword({ token, password });
      setPassword('');
      setConfirmPassword('');
      setComplete(true);
    } catch (err: unknown) {
      setError(getErrorMessage(err, 'Failed to reset password'));
    } finally {
      setIsLoading(false);
    }
  };

  if (complete) {
    return <div className="auth-container"><div className="auth-box"><p role="status">Password reset successful. Sign in with your new password.</p><Link to="/login">Login</Link></div></div>;
  }

  if (!token) {
    return (
      <div className="auth-container">
        <div className="auth-box">
          <div className="error-message" role="alert">Invalid reset link</div>
          <Link to="/forgot-password">Request a new reset link</Link>
        </div>
      </div>
    );
  }

  return (
    <div className="auth-container">
      <div className="auth-box">
        <h1>Set New Password</h1>

        <form onSubmit={handleSubmit} className="pure-form pure-form-stacked">
          {error && <div className="error-message" role="alert">{error}</div>}

          <label htmlFor="password">New Password</label>
          <input
            id="password"
            type="password"
            autoComplete="new-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            disabled={isLoading}
            placeholder="At least 8 characters"
          />

          <label htmlFor="confirmPassword">Confirm Password</label>
          <input
            id="confirmPassword"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            required
            disabled={isLoading}
            placeholder="Re-enter password"
          />

          <button type="submit" className="pure-button pure-button-primary" disabled={isLoading}>
            {isLoading ? 'Resetting...' : 'Reset Password'}
          </button>
        </form>
      </div>
    </div>
  );
};
