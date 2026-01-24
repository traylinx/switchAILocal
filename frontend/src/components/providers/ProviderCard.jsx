import React from 'react';
import { Card } from '../common/Card';
import { Button } from '../common/Button';
import { CheckCircle2, CircleDashed, Terminal, Server, Cloud } from 'lucide-react';
import { ProviderCategories } from '../../utils/providers';

export function ProviderCard({ provider, isConnected, onConnect }) {
  const getCategoryIcon = () => {
    switch (provider.category) {
      case ProviderCategories.CLI:
        return <Terminal size={16} />;
      case ProviderCategories.LOCAL:
        return <Server size={16} />;
      case ProviderCategories.CLOUD:
        return <Cloud size={16} />;
      default:
        return null;
    }
  };

  const isCLITool = provider.category === ProviderCategories.CLI && !provider.configKey;

  return (
    <Card style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 'var(--space-3)' }}>
        <div style={{ flex: 1 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', marginBottom: 'var(--space-1)' }}>
            <span style={{ color: 'var(--color-text-tertiary)' }}>
              {getCategoryIcon()}
            </span>
            <h3 style={{ fontSize: 'var(--text-lg)', fontWeight: 'var(--font-semibold)' }}>{provider.name}</h3>
          </div>
          <div style={{ display: 'flex', gap: 'var(--space-2)', marginBottom: 'var(--space-2)' }}>
            <span style={{ 
              fontSize: '10px', 
              padding: '2px 6px', 
              borderRadius: '4px', 
              backgroundColor: 'var(--color-bg-secondary)', 
              color: 'var(--color-text-secondary)',
              border: '1px solid var(--color-border)'
            }}>
              {provider.category === ProviderCategories.CLI ? 'Native' : (provider.category === ProviderCategories.CLOUD ? 'API Key' : 'Local')}
            </span>
            {(provider.metadata?.prefix || provider.prefix) && (
               <span style={{ 
                fontSize: '10px', 
                padding: '2px 6px', 
                borderRadius: '4px', 
                backgroundColor: 'var(--color-bg-secondary)', 
                color: 'var(--color-text-secondary)',
                border: '1px solid var(--color-border)',
                fontFamily: 'var(--font-mono)'
              }}>
                {provider.metadata?.prefix || provider.prefix}
              </span>
            )}
          </div>
        </div>
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: 'var(--space-1)',
          padding: 'var(--space-1) var(--space-2)',
          borderRadius: '20px',
          fontSize: 'var(--text-xs)',
          fontWeight: 'var(--font-medium)',
          backgroundColor: isConnected ? 'var(--color-primary-light)' : 'var(--color-bg-tertiary)',
          color: isConnected ? 'var(--color-primary)' : 'var(--color-text-secondary)',
          whiteSpace: 'nowrap',
        }}>
          {isConnected ? <CheckCircle2 size={12} /> : <CircleDashed size={12} />}
          {isConnected ? 'Ready' : 'Setup'}
        </div>
      </div>
      
      {provider.description && (
        <p style={{ 
          fontSize: 'var(--text-sm)', 
          color: 'var(--color-text-secondary)', 
          marginBottom: 'var(--space-2)',
        }}>
          {provider.description}
        </p>
      )}

      {isConnected && (provider.configKey || provider.id === 'openai') && (
        <div style={{
          fontSize: 'var(--text-xs)',
          color: 'var(--color-text-tertiary)',
          background: 'var(--color-primary-light)',
          padding: 'var(--space-2) var(--space-3)',
          borderRadius: 'var(--radius-sm)',
          marginBottom: 'var(--space-4)',
          border: '1px solid var(--color-primary-transparent)',
          fontFamily: 'var(--font-mono)',
          display: 'flex',
          flexDirection: 'column',
          gap: '2px'
        }}>
          {provider.category === ProviderCategories.LOCAL && (
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span style={{ opacity: 0.6 }}>URL</span>
              <span style={{ fontWeight: 'var(--font-semibold)', color: 'var(--color-primary)' }}>{provider.metadata?.baseUrl || 'default'}</span>
            </div>
          )}
          {provider.category === ProviderCategories.CLOUD && provider.metadata?.baseUrl && (
            <div style={{ display: 'flex', justifyContent: 'space-between' }}>
              <span style={{ opacity: 0.6 }}>API</span>
              <span style={{ fontWeight: 'var(--font-semibold)', color: 'var(--color-primary)', fontSize: '10px' }}>{provider.metadata.baseUrl.replace('https://', '').replace('/v1', '')}</span>
            </div>
          )}
          {provider.category === ProviderCategories.CLOUD && !provider.metadata?.baseUrl && (
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ opacity: 0.6 }}>STATUS</span>
              <span style={{ fontWeight: 'var(--font-semibold)', color: 'var(--color-primary)', display: 'flex', alignItems: 'center', gap: '4px' }}>
                <CheckCircle2 size={10} /> Active
              </span>
            </div>
          )}
        </div>
      )}

      {isConnected && provider.category === ProviderCategories.CLI && (
        <div style={{
          fontSize: 'var(--text-xs)',
          color: 'var(--color-text-tertiary)',
          background: 'var(--color-bg-secondary)',
          padding: 'var(--space-2)',
          borderRadius: 'var(--radius-sm)',
          marginBottom: 'var(--space-4)',
          border: '1px solid var(--color-border)',
          fontFamily: 'var(--font-mono)'
        }}>
          Command: {provider.id === 'claude-cli' ? 'claude' : (provider.id === 'gemini-cli' ? 'gemini' : (provider.id === 'vibe-cli' ? 'vibe' : (provider.id === 'codex-cli' ? 'codex' : 'opencode')))}
        </div>
      )}

      {provider.status?.lastTested && (
        <div style={{ fontSize: '10px', color: 'var(--color-text-tertiary)', marginBottom: 'var(--space-2)', textAlign: 'center' }}>
           Tested: {new Date(provider.status.lastTested).toLocaleString()}
        </div>
      )}
      
      <Button 
        variant={isConnected ? 'secondary' : 'primary'} 
        style={{ width: '100%', marginTop: 'auto', borderRadius: 'var(--radius-md)' }}
        onClick={onConnect}
      >
        Configure
      </Button>
    </Card>
  );
}
