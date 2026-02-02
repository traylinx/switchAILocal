import React, { useEffect } from 'react';
import { useAppStore } from '../../stores/appStore';
import { Sidebar } from './Sidebar';
import { ReadOnlyIndicator } from '../common/ReadOnlyIndicator';

export function MainLayout({ children }) {
  const theme = useAppStore((state) => state.theme);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);
  return (
    <div style={{
      display: 'flex',
      minHeight: '100vh',
      backgroundColor: 'var(--color-bg-secondary)'
    }}>
      <Sidebar />
      <main style={{
        flex: 1,
        padding: 'var(--space-8)',
        maxWidth: '1200px',
        margin: '0 auto',
        width: '100%',
        overflowY: 'auto'
      }}>
        <div style={{ 
          display: 'flex', 
          justifyContent: 'flex-end', 
          marginBottom: 'var(--space-4)' 
        }}>
          <ReadOnlyIndicator />
        </div>
        {children}
      </main>
    </div>
  );
}
