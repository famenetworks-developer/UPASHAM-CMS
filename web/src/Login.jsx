import React, { useState } from 'react';
import { UserRound, LockKeyhole, Eye, EyeOff, ShieldCheck, ArrowRight } from 'lucide-react';
import './login.css';

export default function Login({ onLogin }) {
  const [email, setEmail] = useState('admin@upashamplus.com');
  const [password, setPassword] = useState('');
  const [visible, setVisible] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const response = await fetch('/api/auth/login', {
        method: 'POST', credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
      const result = await response.json();
      if (!response.ok) throw new Error(result.error || 'Unable to sign in. Please try again.');
      onLogin(result);
    } catch (error) {
      setError(error.message);
    } finally { setBusy(false); }
  }
  return <main className="ocean-login">
    <section className="ocean-login-content" aria-labelledby="login-title">
      <div className="ocean-logo-stage">
        <span className="ocean-lamp" aria-hidden="true" />
        <span className="ocean-light" aria-hidden="true" />
        <img src={`${import.meta.env.BASE_URL}login-logo.png`} alt="Upasham Plus — Doctors from far, not so far" />
      </div>
      <h1 id="login-title">Upasham Plus</h1>
      <p className="ocean-login-subtitle">Sign in to manage your care platform</p>
      <form onSubmit={submit} className="ocean-login-form" aria-label="Admin login">
        {error && <p className="ocean-login-error" role="alert">{error}</p>}
        <div className="ocean-input-stack">
          <label className="ocean-field ocean-email">
            <UserRound aria-hidden="true" />
            <span className="ocean-sr-only">Email address</span>
            <input type="email" value={email} onChange={e => setEmail(e.target.value)} autoComplete="username" placeholder="Email address" required />
          </label>
          <label className="ocean-field ocean-password">
            <LockKeyhole aria-hidden="true" />
            <span className="ocean-sr-only">Password</span>
            <input type={visible ? 'text' : 'password'} value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" placeholder="Password" required />
            <button type="button" className="ocean-reveal" aria-label={visible ? 'Hide password' : 'Show password'} aria-pressed={visible} onClick={() => setVisible(v => !v)}>{visible ? <EyeOff /> : <Eye />}</button>
          </label>
        </div>
        <button className="ocean-submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in securely'}<ArrowRight size={17} aria-hidden="true" /></button>
        <p className="ocean-security"><ShieldCheck size={15} aria-hidden="true" /> Secure access. Thoughtful care.</p>
      </form>
      <p className="ocean-login-footer">UPASHAM PLUS <span>·</span> CONTROL CENTRE</p>
    </section>
  </main>;
}
