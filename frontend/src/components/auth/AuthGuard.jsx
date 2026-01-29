import React, { useEffect, useState } from 'react';
import { useAppStore } from '../../stores/appStore';
import { LoginScreen } from './LoginScreen';
import { SetupScreen } from './SetupScreen';

export function AuthGuard({ children }) {
  const { isAuthenticated, isConfigured, login, setConfigured } = useAppStore();
  const [isCheckingStatus, setIsCheckingStatus] = useState(true);
  
  useEffect(() => {
    const checkStatus = async () => {
      try {
        // 1. Check setup status
        const statusRes = await fetch('/v0/management/setup-status');
        if (statusRes.ok) {
          const data = await statusRes.json();
          setConfigured(data.isConfigured);
        }

        // 2. Check for URL parameter ?key=
        const params = new URLSearchParams(window.location.search);
        const keyFromUrl = params.get('key');
        
        if (keyFromUrl) {
          login(keyFromUrl);
          const newUrl = window.location.pathname + window.location.hash;
          window.history.replaceState({}, document.title, newUrl);
          setIsCheckingStatus(false);
          return;
        }
        
        // 3. Check if we're on localhost and can bypass auth
        const isLocalhost = window.location.hostname === 'localhost' || 
                           window.location.hostname === '127.0.0.1' ||
                           window.location.hostname === '::1';
        
        if (isLocalhost) {
          try {
            const response = await fetch('/v0/management/config', {
              headers: { 'Authorization': 'Bearer localhost-bypass-check' }
            });
            
            if (response.ok || response.headers.get('X-Management-Auth') === 'localhost-bypass') {
              login('localhost-bypass');
            }
          } catch (err) {
            console.error('Localhost bypass check failed:', err);
          }
        }
      } catch (err) {
        console.error('Failed to fetch setup status:', err);
      } finally {
        setIsCheckingStatus(false);
      }
    };

    checkStatus();
  }, [login, setConfigured]);
  
  if (isCheckingStatus) {
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
  
  if (!isConfigured) {
    return <SetupScreen />;
  }
  
  if (!isAuthenticated) {
    return <LoginScreen />;
  }
  
  return children;
}
