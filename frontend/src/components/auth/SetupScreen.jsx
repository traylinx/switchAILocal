import React, { useState } from 'react';
import { useAppStore } from '../../stores/appStore';
import { Button } from '../common/Button';
import { Spinner } from '../common/Spinner';

export function SetupScreen() {
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const { setConfigured, login } = useAppStore();

  const handleSetup = async (e) => {
    e.preventDefault();
    if (!password) return;
    if (password !== confirmPassword) {
      setError('Passwords do not match');
      return;
    }
    if (password.length < 8) {
      setError('Password must be at least 8 characters long');
      return;
    }

    setIsLoading(true);
    setError('');

    try {
      const response = await fetch('/v0/management/initialize', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ secret: password })
      });

      if (response.ok) {
        setConfigured(true);
        login(password);
      } else {
        const data = await response.json();
        setError(data.message || 'Failed to initialize management secret');
      }
    } catch (err) {
      setError('Failed to connect to server. Ensure switchAILocal is running.');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div style={{
      minHeight: '100vh',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: 'var(--space-4)',
      background: 'radial-gradient(circle at bottom right, var(--mui-primary-dark), #000, #000)',
      fontFamily: 'var(--font-sans)',
      color: '#fff'
    }}>
      <div style={{
        width: '100%',
        maxWidth: '440px',
        padding: 'var(--space-10)',
        borderRadius: 'var(--radius-xl)',
        background: 'rgba(255, 255, 255, 0.03)',
        backdropFilter: 'blur(20px)',
        border: '1px solid rgba(255, 255, 255, 0.1)',
        boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.5)',
        textAlign: 'center',
        position: 'relative',
        overflow: 'hidden'
      }}>
        {/* Decorative Top Line */}
        <div style={{
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          height: '2px',
          background: 'linear-gradient(90deg, transparent, var(--mui-primary-main), transparent)'
        }} />

        <h1 style={{ 
          fontSize: '32px', 
          fontWeight: '800', 
          marginBottom: 'var(--space-1)',
          letterSpacing: '-0.025em',
          background: 'linear-gradient(to bottom, #fff, #aaa)',
          WebkitBackgroundClip: 'text',
          WebkitTextFillColor: 'transparent'
        }}>
          switchAILocal
        </h1>
        <p style={{ 
          color: 'rgba(133, 51, 255, 1)', 
          fontSize: '14px',
          fontWeight: '700',
          marginBottom: 'var(--space-10)',
          textTransform: 'uppercase',
          letterSpacing: '0.1em'
        }}>
          First-Time Setup
        </p>

        <div style={{
          padding: 'var(--space-4)',
          background: 'rgba(133, 51, 255, 0.1)',
          border: '1px solid rgba(133, 51, 255, 0.2)',
          borderRadius: 'var(--radius-md)',
          marginBottom: 'var(--space-8)',
          textAlign: 'left'
        }}>
          <p style={{ fontSize: '13px', color: 'var(--mui-primary-light)', fontWeight: '600' }}>
            🔒 Secure Your Management Center
          </p>
          <p style={{ fontSize: '12px', color: 'rgba(255, 255, 255, 0.6)', marginTop: '4px', lineHeight: '1.4' }}>
            Choose a management key. This protects your configuration and API keys.
          </p>
        </div>

        <form onSubmit={handleSetup} style={{ textAlign: 'left' }}>
          <div style={{ marginBottom: 'var(--space-4)' }}>
            <label style={{ 
              display: 'block', 
              fontSize: '13px', 
              fontWeight: '600',
              color: 'rgba(255, 255, 255, 0.7)',
              marginBottom: 'var(--space-2)',
              marginLeft: '2px'
            }}>
              New Management Key
            </label>
            <input
              type="password"
              placeholder="••••••••••••"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={isLoading}
              autoFocus
              style={{
                width: '100%',
                padding: 'var(--space-3) var(--space-4)',
                background: 'rgba(255, 255, 255, 0.05)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                borderRadius: 'var(--radius-md)',
                color: '#fff',
                fontSize: '15px',
                outline: 'none',
                transition: 'all 0.2s ease',
                boxSizing: 'border-box'
              }}
              onFocus={(e) => {
                e.target.style.background = 'rgba(255, 255, 255, 0.08)';
                e.target.style.borderColor = 'var(--mui-primary-main)';
                e.target.style.boxShadow = '0 0 0 3px rgba(133, 51, 255, 0.2)';
              }}
              onBlur={(e) => {
                e.target.style.background = 'rgba(255, 255, 255, 0.05)';
                e.target.style.borderColor = 'rgba(255, 255, 255, 0.1)';
                e.target.style.boxShadow = 'none';
              }}
            />
          </div>

          <div style={{ marginBottom: 'var(--space-8)' }}>
            <label style={{ 
              display: 'block', 
              fontSize: '13px', 
              fontWeight: '600',
              color: 'rgba(255, 255, 255, 0.7)',
              marginBottom: 'var(--space-2)',
              marginLeft: '2px'
            }}>
              Confirm Key
            </label>
            <input
              type="password"
              placeholder="••••••••••••"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              disabled={isLoading}
              style={{
                width: '100%',
                padding: 'var(--space-3) var(--space-4)',
                background: 'rgba(255, 255, 255, 0.05)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                borderRadius: 'var(--radius-md)',
                color: '#fff',
                fontSize: '15px',
                outline: 'none',
                transition: 'all 0.2s ease',
                boxSizing: 'border-box'
              }}
              onFocus={(e) => {
                e.target.style.background = 'rgba(255, 255, 255, 0.08)';
                e.target.style.borderColor = 'var(--mui-primary-main)';
                e.target.style.boxShadow = '0 0 0 3px rgba(133, 51, 255, 0.2)';
              }}
              onBlur={(e) => {
                e.target.style.background = 'rgba(255, 255, 255, 0.05)';
                e.target.style.borderColor = 'rgba(255, 255, 255, 0.1)';
                e.target.style.boxShadow = 'none';
              }}
            />
            {error && <p style={{ color: '#ef4444', fontSize: '12px', marginTop: '8px', fontWeight: '500' }}>{error}</p>}
          </div>

          <div style={{ display: 'flex', gap: '12px', flexDirection: 'column' }}>
            <Button 
              type="submit" 
              variant="primary" 
              style={{ 
                width: '100%', 
                height: '44px',
                fontSize: '15px',
                fontWeight: '600',
                backgroundColor: 'var(--mui-primary-main)',
                border: 'none',
                borderRadius: 'var(--radius-md)',
                cursor: 'pointer'
              }}
              disabled={isLoading || !password || !confirmPassword}
            >
              {isLoading ? <Spinner size="sm" /> : 'Set Key & Initialize'}
            </Button>

            <button
              type="button"
              onClick={async () => {
                setIsLoading(true);
                try {
                  const res = await fetch('/v0/management/skip', { method: 'POST' });
                  if (res.ok) {
                    setConfigured(true);
                    login('disabled'); // special token
                  } else {
                    setError('Failed to skip setup');
                  }
                } catch (e) {
                  setError('Connection failed');
                } finally {
                  setIsLoading(false);
                }
              }}
              style={{
                background: 'transparent',
                border: '1px solid rgba(255, 255, 255, 0.1)',
                color: 'rgba(255, 255, 255, 0.5)',
                padding: '10px',
                borderRadius: 'var(--radius-md)',
                fontSize: '13px',
                cursor: 'pointer',
                transition: 'all 0.2s'
              }}
              className="hover:bg-white/5"
            >
              Continue without Password (Localhost Only)
            </button>
          </div>
        </form>
        
        <p style={{ 
          marginTop: 'var(--space-6)', 
          fontSize: '11px', 
          color: 'rgba(255, 255, 255, 0.3)',
          lineHeight: '1.5'
        }}>
          Initialization will update your <code>config.yaml</code> file.<br/>
          Make sure you have write permissions.
        </p>
      </div>
    </div>
  );
}
