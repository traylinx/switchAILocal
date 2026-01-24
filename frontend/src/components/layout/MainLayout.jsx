import React, { useEffect } from 'react';
import { useAppStore } from '../../stores/appStore';
import { Sidebar } from './Sidebar';

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
        {children}
      </main>
    </div>
  );
}
