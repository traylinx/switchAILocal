import React, { useState } from 'react';
import { Plus, X } from 'lucide-react';
import { Button } from './Button';

export function ModelAliasInput({ aliases = [], onChange, availableModels = [] }) {
  const [newAlias, setNewAlias] = useState({ name: '', alias: '' });

  const handleAdd = () => {
    if (newAlias.name && newAlias.alias) {
      onChange([...aliases, newAlias]);
      setNewAlias({ name: '', alias: '' });
    }
  };

  const handleRemove = (index) => {
    onChange(aliases.filter((_, i) => i !== index));
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter') {
      handleAdd();
    }
  };

  return (
    <div style={{ marginBottom: 'var(--space-4)' }}>
      <label style={{ display: 'block', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-2)' }}>
        Model Aliases
      </label>
      
      <div style={{ 
        border: '1px solid var(--color-border)', 
        borderRadius: 'var(--radius-md)',
        overflow: 'hidden'
      }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 'var(--text-sm)' }}>
          <thead style={{ backgroundColor: 'var(--color-bg-secondary)', textAlign: 'left' }}>
            <tr>
              <th style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-medium)', width: '45%' }}>Upstream Name</th>
              <th style={{ padding: 'var(--space-2) var(--space-3)', fontWeight: 'var(--font-medium)', width: '45%' }}>Alias</th>
              <th style={{ padding: 'var(--space-2) var(--space-3)', width: '10%' }}></th>
            </tr>
          </thead>
          <tbody>
            {aliases.map((item, index) => (
              <tr key={index} style={{ borderTop: '1px solid var(--color-border)' }}>
                <td style={{ padding: 'var(--space-2) var(--space-3)' }}>{item.name}</td>
                <td style={{ padding: 'var(--space-2) var(--space-3)' }}>{item.alias}</td>
                <td style={{ padding: 'var(--space-2) var(--space-3)', textAlign: 'center' }}>
                  <button
                    onClick={() => handleRemove(index)}
                    style={{
                      background: 'none',
                      border: 'none',
                      cursor: 'pointer',
                      color: 'var(--color-text-secondary)',
                    }}
                    title="Remove alias"
                  >
                    <X size={16} />
                  </button>
                </td>
              </tr>
            ))}
            
            <tr style={{ borderTop: '1px solid var(--color-border)', backgroundColor: 'var(--color-bg-primary)' }}>
              <td style={{ padding: 'var(--space-2)' }}>
                {availableModels.length > 0 ? (
                  <input
                    type="text"
                    list="available-models"
                    className="input"
                    placeholder="gpt-4"
                    value={newAlias.name}
                    onChange={(e) => setNewAlias({ ...newAlias, name: e.target.value })}
                    style={{ width: '100%' }}
                  />
                ) : (
                  <input
                    type="text"
                    className="input"
                    placeholder="gpt-4"
                    value={newAlias.name}
                    onChange={(e) => setNewAlias({ ...newAlias, name: e.target.value })}
                    style={{ width: '100%' }}
                  />
                )}
                {availableModels.length > 0 && (
                  <datalist id="available-models">
                    {availableModels.map(model => (
                      <option key={model} value={model} />
                    ))}
                  </datalist>
                )}
              </td>
              <td style={{ padding: 'var(--space-2)' }}>
                <input
                  type="text"
                  className="input"
                  placeholder="gpt-4-alias"
                  value={newAlias.alias}
                  onChange={(e) => setNewAlias({ ...newAlias, alias: e.target.value })}
                  onKeyDown={handleKeyDown}
                  style={{ width: '100%' }}
                />
              </td>
              <td style={{ padding: 'var(--space-2)', textAlign: 'center' }}>
                <Button 
                  size="sm" 
                  variant="secondary"
                  onClick={handleAdd}
                  disabled={!newAlias.name || !newAlias.alias}
                  style={{ minWidth: 'auto', padding: 'var(--space-2)' }}
                >
                  <Plus size={16} />
                </Button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}
