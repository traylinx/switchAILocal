import React, { useEffect, useState } from 'react';
import { useAppStore } from '../../stores/appStore';
import { LoginScreen } from './LoginScreen';

export function AuthGuard({ children }) {
  const { isAuthenticated, login } = useAppStore();
  const [isCheckingLocalhost, setIsCheckingLocalhost] = useState(true);
  
  useEffect(() => {
    // 1. Check for URL parameter ?key=
    const params = new URLSearchParams(window.location.search);
    const keyFromUrl = params.get('key');
    
    if (keyFromUrl) {
      // 2. Store key and trigger login
      login(keyFromUrl);
      
      // 3. Clean URL without refreshing page
      const newUrl = window.location.pathname + window.location.hash;
      window.history.replaceState({}, document.title, newUrl);
      setIsCheckingLocalhost(false);
      return;
    }
    
    // 4. Check if we're on localhost and can bypass auth
    const isLocalhost = window.location.hostname === 'localhost' || 
                       window.location.hostname === '127.0.0.1' ||
                       window.location.hostname === '::1';
    
    if (isLocalhost) {
      // Try to access config without auth to see if localhost bypass is enabled
      fetch('/v0/management/config', {
        headers: { 'Authorization': 'Bearer localhost-bypass-check' }
      })
      .then(response => {
        if (response.ok || response.headers.get('X-Management-Auth') === 'localhost-bypass') {
          // Localhost bypass is enabled, auto-login with special token
          login('localhost-bypass');
          setIsCheckingLocalhost(false);
        } else {
          // Localhost bypass not enabled, show login
          setIsCheckingLocalhost(false);
        }
      })
      .catch(() => {
        // Network error, show login
        setIsCheckingLocalhost(false);
      });
    } else {
      setIsCheckingLocalhost(false);
    }
  }, [login]);
  
  if (isCheckingLocalhost) {
    return (
      <div style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        backgroundColor: 'var(--color-bg-secondary)'
      }}>
        <div style={{ textAlign: 'center' }}>
          <div style={{ 
            width: '40px', 
            height: '40px', 
            border: '3px solid var(--color-border)',
            borderTopColor: 'var(--color-primary)',
            borderRadius: '50%',
            animation: 'spin 1s linear infinite',
            margin: '0 auto 16px'
          }} />
          <p style={{ color: 'var(--color-text-secondary)' }}>Loading...</p>
        </div>
      </div>
    );
  }
  
  if (!isAuthenticated) {
    return <LoginScreen />;
  }
  
  return children;
}
