import React, { useState } from 'react';
import { useAppStore } from '../../stores/appStore';
import { Button } from '../common/Button';
import { Card } from '../common/Card';
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
      // Verify key by making a test request
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
      backgroundColor: 'var(--color-bg-secondary)'
    }}>
      <Card style={{ width: '100%', maxWidth: '400px', textAlign: 'center' }}>
        <h1 style={{ 
          fontSize: 'var(--text-3xl)', 
          fontWeight: 'var(--font-bold)', 
          marginBottom: 'var(--space-2)',
          color: 'var(--color-primary)'
        }}>
          switchAILocal
        </h1>
        <p style={{ 
          color: 'var(--color-text-secondary)', 
          marginBottom: 'var(--space-8)' 
        }}>
          Management Center
        </p>
        
        {!isLocalhost && (
          <div style={{
            padding: 'var(--space-4)',
            backgroundColor: 'var(--color-warning-light)',
            borderRadius: 'var(--radius-md)',
            marginBottom: 'var(--space-6)',
            textAlign: 'left'
          }}>
            <p style={{ fontSize: 'var(--text-sm)', color: 'var(--color-warning-dark)', fontWeight: 'var(--font-medium)' }}>
              🔒 Remote Access Detected
            </p>
            <p style={{ fontSize: 'var(--text-xs)', color: 'var(--color-warning-dark)', marginTop: 'var(--space-2)' }}>
              You're accessing from <strong>{window.location.hostname}</strong>. 
              A management key is required for security.
            </p>
          </div>
        )}
        
        <form onSubmit={handleLogin}>
          <div style={{ marginBottom: 'var(--space-6)', textAlign: 'left' }}>
            <label style={{ 
              display: 'block', 
              fontSize: 'var(--text-sm)', 
              fontWeight: 'var(--font-medium)',
              marginBottom: 'var(--space-2)'
            }}>
              Management Key
            </label>
            <input
              type="password"
              className="input"
              placeholder="••••••••••••"
              value={key}
              onChange={(e) => setKey(e.target.value)}
              disabled={isLoading}
              autoFocus
            />
            {error && <p className="error-text">{error}</p>}
          </div>
          
          <Button 
            type="submit" 
            variant="primary" 
            style={{ width: '100%' }}
            disabled={isLoading || !key}
          >
            {isLoading ? <Spinner size="sm" /> : 'Login'}
          </Button>
        </form>
        
        <div style={{ 
          marginTop: 'var(--space-8)', 
          padding: 'var(--space-4)',
          backgroundColor: 'var(--color-bg-tertiary)',
          borderRadius: 'var(--radius-md)',
          textAlign: 'left'
        }}>
          <p style={{ fontSize: 'var(--text-xs)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-2)' }}>
            Where to find your key:
          </p>
          <ul style={{ fontSize: 'var(--text-xs)', color: 'var(--color-text-tertiary)', paddingLeft: 'var(--space-4)', margin: 0 }}>
            <li>Check your CLI logs when starting switchAILocal</li>
            <li>Look in your <code style={{ fontFamily: 'var(--font-mono)', backgroundColor: 'var(--color-bg-secondary)', padding: '2px 4px', borderRadius: '3px' }}>config.yaml</code> file</li>
            <li>Set via environment variable: <code style={{ fontFamily: 'var(--font-mono)', backgroundColor: 'var(--color-bg-secondary)', padding: '2px 4px', borderRadius: '3px' }}>MANAGEMENT_PASSWORD</code></li>
          </ul>
        </div>
      </Card>
    </div>
  );
}
