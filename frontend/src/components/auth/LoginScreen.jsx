import React, { useState } from 'react';
import { useAppStore } from '../../stores/appStore';
import { Button } from '../common/Button';
import { Spinner } from '../common/Spinner';

export function LoginScreen() {
  const [key, setKey] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const login = useAppStore((state) => state.login);
  
  const isLocalhost = window.location.hostname === 'localhost' || 
                     window.location.hostname === '127.0.0.1' ||
                     window.location.hostname === '::1';
  
  const handleLogin = async (e) => {
    e.preventDefault();
    if (!key) return;
    
    setIsLoading(true);
    setError('');
    
    try {
      const response = await fetch('/v0/management/config', {
        headers: { 'Authorization': `Bearer ${key}` }
      });
      
      if (response.ok) {
        login(key);
      } else {
        setError('Invalid management key');
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
      background: 'radial-gradient(circle at top left, var(--mui-primary-dark), #000, #000)',
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
          color: 'rgba(255, 255, 255, 0.5)', 
          fontSize: '14px',
          fontWeight: '500',
          marginBottom: 'var(--space-10)',
          textTransform: 'uppercase',
          letterSpacing: '0.1em'
        }}>
          Management Center
        </p>
        
        {!isLocalhost && (
          <div style={{
            padding: 'var(--space-4)',
            background: 'rgba(245, 158, 11, 0.1)',
            border: '1px solid rgba(245, 158, 11, 0.2)',
            borderRadius: 'var(--radius-md)',
            marginBottom: 'var(--space-8)',
            textAlign: 'left'
          }}>
            <p style={{ fontSize: '13px', color: '#fbbf24', fontWeight: '600', display: 'flex', alignItems: 'center', gap: '8px' }}>
              <span>🔒</span> Remote Access Mode
            </p>
            <p style={{ fontSize: '12px', color: 'rgba(251, 191, 36, 0.8)', marginTop: '4px', lineHeight: '1.4' }}>
              Accessed via <strong>{window.location.hostname}</strong>. Key authentication required.
            </p>
          </div>
        )}
        
        <form onSubmit={handleLogin} style={{ textAlign: 'left' }}>
          <div style={{ marginBottom: 'var(--space-6)' }}>
            <label style={{ 
              display: 'block', 
              fontSize: '13px', 
              fontWeight: '600',
              color: 'rgba(255, 255, 255, 0.7)',
              marginBottom: 'var(--space-2)',
              marginLeft: '2px'
            }}>
              Management Key
            </label>
            <input
              type="password"
              placeholder="••••••••••••"
              value={key}
              onChange={(e) => setKey(e.target.value)}
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
            {error && <p style={{ color: '#ef4444', fontSize: '12px', marginTop: '8px', fontWeight: '500' }}>{error}</p>}
            {isLocalhost && error === 'Invalid management key' && (
              <div style={{ marginTop: '12px', textAlign: 'center' }}>
                <button
                  type="button"
                  onClick={async () => {
                    if (confirm('Verify you are on localhost: This will reset your management key configuration. Continue?')) {
                      try {
                         await fetch('/v0/management/reset', { method: 'POST' });
                         window.location.reload();
                      } catch(e) { alert('Reset failed'); }
                    }
                  }}
                  style={{
                    background: 'none',
                    border: 'none',
                    color: 'rgba(239, 68, 68, 0.8)',
                    fontSize: '12px',
                    textDecoration: 'underline',
                    cursor: 'pointer'
                  }}
                >
                  Reset Configuration (Localhost Only)
                </button>
              </div>
            )}
          </div>
          
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
              cursor: 'pointer',
              transition: 'transform 0.1s active'
            }}
            disabled={isLoading || !key}
          >
            {isLoading ? <Spinner size="sm" /> : 'Enter Dashboard'}
          </Button>
        </form>
        
        <div style={{ 
          marginTop: 'var(--space-10)', 
          paddingTop: 'var(--space-6)',
          borderTop: '1px solid rgba(255, 255, 255, 0.08)',
          textAlign: 'left'
        }}>
          <p style={{ fontSize: '12px', fontWeight: '600', color: 'rgba(255, 255, 255, 0.4)', marginBottom: 'var(--space-3)' }}>
            QUICK HELP
          </p>
          <ul style={{ 
            fontSize: '12px', 
            color: 'rgba(255, 255, 255, 0.5)', 
            listStyle: 'none',
            padding: 0, 
            margin: 0,
            display: 'flex',
            flexDirection: 'column',
            gap: '8px'
          }}>
            <li style={{ display: 'flex', gap: '8px' }}>
              <span style={{ color: 'var(--mui-primary-light)' }}>•</span>
              Check CLI logs on startup
            </li>
            <li style={{ display: 'flex', gap: '8px' }}>
              <span style={{ color: 'var(--mui-primary-light)' }}>•</span>
              Verify <code>config.yaml</code> secret key
            </li>
            <li style={{ display: 'flex', gap: '8px' }}>
              <span style={{ color: 'var(--mui-primary-light)' }}>•</span>
              Check <code>MANAGEMENT_PASSWORD</code> env
            </li>
          </ul>
        </div>
      </div>
    </div>
  );
}
