import React from 'react';
import { useAppStore } from '../../stores/appStore';
import { 
  ShieldCheck, 
  Network, 
  Settings, 
  Sun,
  Moon,
  Cpu
} from 'lucide-react';

export function Sidebar() {
  const { currentView, setView, theme, toggleTheme } = useAppStore();
  
  const navItems = [
    { id: 'providers', label: 'Providers', icon: Cpu },
    { id: 'routing', label: 'Routing', icon: Network },
    { id: 'settings', label: 'Settings', icon: Settings },
  ];
  
  return (
    <aside style={{
      width: '260px',
      backgroundColor: 'var(--color-bg)',
      borderRight: '1px solid var(--color-border)',
      display: 'flex',
      flexDirection: 'column',
      height: '100vh',
      position: 'sticky',
      top: 0
    }}>
      <div style={{
        padding: 'var(--space-8) var(--space-6)',
        display: 'flex',
        alignItems: 'center',
        gap: 'var(--space-3)'
      }}>
        <div style={{
          width: '32px',
          height: '32px',
          backgroundColor: 'var(--color-primary)',
          borderRadius: '8px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'white'
        }}>
          <ShieldCheck size={20} />
        </div>
        <span style={{ 
          fontWeight: 'var(--font-bold)', 
          fontSize: 'var(--text-lg)',
          letterSpacing: '-0.02em'
        }}>
          switchAILocal
        </span>
      </div>
      
      <nav style={{ flex: 1, padding: '0 var(--space-3)' }}>
        {navItems.map((item) => (
          <button
            key={item.id}
            onClick={() => setView(item.id)}
            style={{
              width: '100%',
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-3)',
              padding: 'var(--space-3) var(--space-4)',
              borderRadius: '8px',
              border: 'none',
              background: currentView === item.id ? 'var(--color-primary-light)' : 'transparent',
              color: currentView === item.id ? 'var(--color-primary)' : 'var(--color-text-secondary)',
              cursor: 'pointer',
              transition: 'all 0.2s',
              marginBottom: 'var(--space-1)',
              textAlign: 'left',
              fontWeight: currentView === item.id ? 'var(--font-semibold)' : 'var(--font-normal)'
            }}
          >
            <item.icon size={18} />
            <span>{item.label}</span>
          </button>
        ))}
      </nav>
      
      <div style={{ padding: 'var(--space-4) var(--space-3)', borderTop: '1px solid var(--color-border)' }}>
        <button
          onClick={toggleTheme}
          style={{
            width: '100%',
            display: 'flex',
            alignItems: 'center',
            gap: 'var(--space-3)',
            padding: 'var(--space-3) var(--space-4)',
            borderRadius: 'var(--radius-md)',
            border: 'none',
            background: 'transparent',
            color: 'var(--color-text-secondary)',
            cursor: 'pointer',
            transition: 'all 0.2s',
            textAlign: 'left'
          }}
        >
          {theme === 'light' ? <Moon size={18} /> : <Sun size={18} />}
          <span>{theme === 'light' ? 'Dark Mode' : 'Light Mode'}</span>
        </button>
      </div>
    </aside>
  );
}
