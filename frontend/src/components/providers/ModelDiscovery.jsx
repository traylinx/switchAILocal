import React, { useState } from 'react';
import { Search, Loader2, AlertCircle, Check } from 'lucide-react';
import { Button } from '../common/Button';
import { apiClient } from '../../api/client';

export function ModelDiscovery({ modelsUrl, apiKey, baseUrl, proxyUrl, headers, onSelectModel }) {
  const [isLoading, setIsLoading] = useState(false);
  const [models, setModels] = useState([]);
  const [error, setError] = useState(null);
  const [searchTerm, setSearchTerm] = useState('');
  const [hasSearched, setHasSearched] = useState(false);

  const handleDiscover = async () => {
    setIsLoading(true);
    setError(null);
    setModels([]);

    try {
      const data = await apiClient.discoverProviderModels({
        modelsUrl,
        apiKey,
        baseUrl,
        proxyUrl,
        headers
      });

      if (!data || !data.models) {
        throw new Error('Failed to fetch models');
      }

      setModels(data.models || []);
      setHasSearched(true);
    } catch (err) {
      console.error('Model discovery failed:', err);
      setError(err.message);
    } finally {
      setIsLoading(false);
    }
  };

  const filteredModels = models.filter(m => 
    m.id.toLowerCase().includes(searchTerm.toLowerCase()) || 
    (m.name && m.name.toLowerCase().includes(searchTerm.toLowerCase()))
  );

  return (
    <div style={{ marginBottom: 'var(--space-4)' }}>
      <label style={{ display: 'block', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-2)' }}>
        Model Discovery
      </label>

      <div style={{ 
        border: '1px solid var(--color-border)', 
        borderRadius: 'var(--radius-md)',
        padding: 'var(--space-4)',
        backgroundColor: 'var(--color-bg-secondary)'
      }}>
        {!hasSearched && !isLoading && !error ? (
          <div style={{ textAlign: 'center', padding: 'var(--space-4)' }}>
            <p style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)', marginBottom: 'var(--space-4)' }}>
              Fetch available models directly from the provider configuration to easily create aliases.
            </p>
            <Button onClick={handleDiscover} disabled={!modelsUrl || !apiKey}>
              Fetch Models
            </Button>
            {!modelsUrl && (
              <p style={{ fontSize: 'var(--text-xs)', color: 'var(--color-text-tertiary)', marginTop: 'var(--space-2)' }}>
                Requires Models URL to be configured.
              </p>
            )}
          </div>
        ) : (
          <div>
            <div style={{ display: 'flex', gap: 'var(--space-2)', marginBottom: 'var(--space-3)' }}>
              <div style={{ position: 'relative', flex: 1 }}>
                <Search size={16} style={{ position: 'absolute', left: '10px', top: '50%', transform: 'translateY(-50%)', color: 'var(--color-text-tertiary)' }} />
                <input
                  type="text"
                  className="input"
                  placeholder="Filter models..."
                  value={searchTerm}
                  onChange={(e) => setSearchTerm(e.target.value)}
                  style={{ paddingLeft: '32px', width: '100%' }}
                />
              </div>
              <Button variant="secondary" onClick={handleDiscover} disabled={isLoading}>
                {isLoading ? <Loader2 size={16} className="spin" /> : 'Refresh'}
              </Button>
            </div>

            {error && (
              <div style={{ 
                padding: 'var(--space-3)', 
                backgroundColor: 'var(--color-error-light)', 
                color: 'var(--color-error)', 
                borderRadius: 'var(--radius-sm)',
                marginBottom: 'var(--space-3)',
                fontSize: 'var(--text-sm)',
                display: 'flex',
                alignItems: 'center',
                gap: 'var(--space-2)'
              }}>
                <AlertCircle size={16} />
                {error}
              </div>
            )}

            {hasSearched && !isLoading && filteredModels.length === 0 && !error && (
              <div style={{ textAlign: 'center', padding: 'var(--space-4)', color: 'var(--color-text-secondary)', fontSize: 'var(--text-sm)' }}>
                No models found matching your criteria.
              </div>
            )}

            <div style={{ 
              maxHeight: '300px', 
              overflowY: 'auto', 
              border: '1px solid var(--color-border)',
              borderRadius: 'var(--radius-sm)',
              backgroundColor: 'var(--color-bg-primary)'
            }}>
              {filteredModels.map(model => (
                <div 
                  key={model.id}
                  onClick={() => onSelectModel(model)}
                  style={{
                    padding: 'var(--space-2) var(--space-3)',
                    borderBottom: '1px solid var(--color-border)',
                    cursor: 'pointer',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    transition: 'background-color 0.1s'
                  }}
                  onMouseOver={(e) => e.currentTarget.style.backgroundColor = 'var(--color-bg-secondary)'}
                  onMouseOut={(e) => e.currentTarget.style.backgroundColor = 'var(--color-bg-primary)'}
                >
                  <div>
                    <div style={{ fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)' }}>{model.id}</div>
                    {model.contextWindow && (
                      <div style={{ fontSize: 'var(--text-xs)', color: 'var(--color-text-tertiary)' }}>
                        Context: {model.contextWindow.toLocaleString()} tokens
                      </div>
                    )}
                  </div>
                  <Button size="sm" variant="secondary" style={{ padding: '4px 8px', height: 'auto', fontSize: '10px' }}>
                    Use
                  </Button>
                </div>
              ))}
            </div>
            <div style={{ marginTop: 'var(--space-2)', fontSize: 'var(--text-xs)', color: 'var(--color-text-tertiary)', textAlign: 'right' }}>
              Showing {filteredModels.length} of {models.length} models
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
