import React, { useState, useEffect } from 'react';
import { useConfig } from '../../hooks/useConfig';
import { useStateBoxStatus } from '../../hooks/useStateBoxStatus';
import { Card } from '../common/Card';
import { Toggle } from '../common/Toggle';
import { Button } from '../common/Button';
import { Spinner } from '../common/Spinner';
import { Server, Info, Terminal } from 'lucide-react';

export function SettingsView() {
  const { config, updateConfig, isLoading } = useConfig();
  const { status: stateBoxStatus } = useStateBoxStatus();
  const [proxyUrl, setProxyUrl] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  const isReadOnly = stateBoxStatus?.read_only || false;

  useEffect(() => {
    if (config?.['proxy-url']) {
      setProxyUrl(config['proxy-url']);
    }
  }, [config]);

  const handleToggleDebug = async () => {
    try {
      await updateConfig({ debug: !config.debug });
    } catch (e) {
      alert('Failed to update debug mode');
    }
  };

  const handleSaveProxy = async () => {
    setIsSaving(true);
    try {
      await updateConfig({ 'proxy-url': proxyUrl });
    } catch (e) {
      alert('Failed to save proxy URL');
    } finally {
      setIsSaving(false);
    }
  };

  if (isLoading) return <div style={{ display: 'flex', justifyContent: 'center', padding: '100px' }}><Spinner size="lg" /></div>;

  return (
    <div>
      <header style={{ marginBottom: 'var(--space-8)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 'var(--font-bold)', marginBottom: 'var(--space-2)' }}>System Settings</h1>
        <p style={{ color: 'var(--color-text-secondary)' }}>Configure global proxy behavior and view system information.</p>
      </header>

      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-6)' }}>
        {/* Core Settings */}
        <Card>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-6)' }}>
            <Terminal size={20} color="var(--color-primary)" />
            <h2 style={{ fontSize: 'var(--text-lg)', fontWeight: 'var(--font-semibold)' }}>Debugging & Logs</h2>
          </div>
          
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <p style={{ fontWeight: 'var(--font-medium)' }}>Debug Mode</p>
              <p style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>Enable verbose logging in the CLI console.</p>
            </div>
            <Toggle active={!!config?.debug} onChange={handleToggleDebug} disabled={isReadOnly} />
          </div>
        </Card>

        {/* Network Settings */}
        <Card>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-6)' }}>
            <Server size={20} color="var(--color-primary)" />
            <h2 style={{ fontSize: 'var(--text-lg)', fontWeight: 'var(--font-semibold)' }}>Network</h2>
          </div>
          
          <div style={{ marginBottom: 'var(--space-4)' }}>
            <label style={{ display: 'block', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-2)' }}>
              Proxy URL (External access)
            </label>
            <div style={{ display: 'flex', gap: 'var(--space-3)' }}>
              <input
                type="text"
                className="input"
                placeholder="https://example.com"
                value={proxyUrl}
                onChange={(e) => setProxyUrl(e.target.value)}
                disabled={isSaving || isReadOnly}
              />
              <Button variant="primary" onClick={handleSaveProxy} disabled={isSaving || isReadOnly || proxyUrl === config?.['proxy-url']}>
                {isSaving ? <Spinner size="sm" /> : 'Update'}
              </Button>
            </div>
          </div>
        </Card>

        {/* System Info (Read-only) */}
        <Card style={{ backgroundColor: 'var(--color-bg-tertiary)', borderColor: 'transparent' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', marginBottom: 'var(--space-6)' }}>
            <Info size={20} color="var(--color-text-secondary)" />
            <h2 style={{ fontSize: 'var(--text-lg)', fontWeight: 'var(--font-semibold)', color: 'var(--color-text-secondary)' }}>System Information</h2>
          </div>
          
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-4)', fontSize: 'var(--text-sm)' }}>
            <div>
              <p style={{ color: 'var(--color-text-tertiary)', textTransform: 'uppercase', fontSize: '10px', fontWeight: 'var(--font-bold)', marginBottom: 'var(--space-1)' }}>Server Host</p>
              <p style={{ fontFamily: 'var(--font-mono)' }}>{config?.host || '---'}</p>
            </div>
            <div>
              <p style={{ color: 'var(--color-text-tertiary)', textTransform: 'uppercase', fontSize: '10px', fontWeight: 'var(--font-bold)', marginBottom: 'var(--space-1)' }}>Server Port</p>
              <p style={{ fontFamily: 'var(--font-mono)' }}>{config?.port || '---'}</p>
            </div>
            <div>
              <p style={{ color: 'var(--color-text-tertiary)', textTransform: 'uppercase', fontSize: '10px', fontWeight: 'var(--font-bold)', marginBottom: 'var(--space-1)' }}>TLS Status</p>
              <p>{config?.tls ? '🔒 Enabled' : '🔓 Disabled'}</p>
            </div>
          </div>
        </Card>
      </div>
    </div>
  );
}
