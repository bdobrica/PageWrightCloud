import { getErrorMessage } from '../utils/errors';
import { validPassword, passwordPolicyMessage } from '../utils/password';
import React, { useEffect, useState } from 'react';
import { apiClient } from '../api/client';
import { Link, useNavigate } from 'react-router-dom';
import { useAuth } from '../contexts/auth';
import './Auth.css';

export const Register: React.FC = () => {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const { register } = useAuth();
  const navigate = useNavigate();
  const [registrationOpen, setRegistrationOpen] = useState<boolean | null>(null);
  const [policyError, setPolicyError] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setRegistrationOpen(null); setPolicyError(false);
    apiClient.registrationOpen(controller.signal).then(open => {
      if (!controller.signal.aborted) setRegistrationOpen(open);
    }).catch(() => { if (!controller.signal.aborted) setPolicyError(true); });
    return () => controller.abort();
  }, [retry]);

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
      await register({ email, password });
      navigate('/');
    } catch (err: unknown) {
      setError(getErrorMessage(err, 'Registration failed. Please try again.'));
    } finally {
      setIsLoading(false);
    }
  };

  if (registrationOpen !== true) return <div className="auth-container"><div className="auth-box">
    <h1>Account access</h1>
    {policyError ? <><p role="alert">Unable to check account access.</p><button onClick={() => setRetry(n => n + 1)}>Retry</button></>
      : <p role="status">{registrationOpen === null ? 'Checking account access…' : 'Accounts are provisioned by the operator. Contact the site administrator for access.'}</p>}
    <Link to="/login">Login</Link>
  </div></div>;

  return (
    <div className="auth-container">
      <div className="auth-box">
        <h1>Create Account</h1>

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
            placeholder="At least 8 characters"
          />

          <label htmlFor="confirmPassword">Confirm Password</label>
          <input
            id="confirmPassword"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            required
            disabled={isLoading}
            placeholder="Re-enter password"
          />

          <button type="submit" className="pure-button pure-button-primary" disabled={isLoading}>
            {isLoading ? 'Creating account...' : 'Register'}
          </button>
        </form>

        <div className="auth-links">
          <Link to="/login">Already have an account? Login</Link>
        </div>
      </div>
    </div>
  );
};
