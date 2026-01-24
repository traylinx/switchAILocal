import React, { useState, useEffect } from 'react';
import { Plus, X } from 'lucide-react';
import { Button } from './Button';

export function KeyValueInput({ label, pairs = {}, onChange, placeholder = { key: 'Key', value: 'Value' } }) {
  const [internalPairs, setInternalPairs] = useState([]);

  // Sync internal state with props when pairs change
  useEffect(() => {
    const entries = Object.entries(pairs);
    if (entries.length === 0) {
      setInternalPairs([{ key: '', value: '' }]);
    } else {
      setInternalPairs(entries.map(([key, value]) => ({ key, value })));
    }
  }, [pairs]);

  const handleChange = (index, field, value) => {
    const newPairs = [...internalPairs];
    newPairs[index][field] = value;
    setInternalPairs(newPairs);

    // Notify parent of valid changes
    const validPairs = newPairs.reduce((acc, pair) => {
      if (pair.key.trim()) {
        acc[pair.key.trim()] = pair.value;
      }
      return acc;
    }, {});
    
    // Only fire onChange if we have valid data or empty
    // Deboucing could be added here if needed, but for now strict sync
    onChange(validPairs);
  };

  const handleAdd = () => {
    setInternalPairs([...internalPairs, { key: '', value: '' }]);
  };

  const handleRemove = (index) => {
    const newPairs = internalPairs.filter((_, i) => i !== index);
    setInternalPairs(newPairs.length ? newPairs : [{ key: '', value: '' }]);
    
    const validPairs = newPairs.reduce((acc, pair) => {
      if (pair.key.trim()) {
        acc[pair.key.trim()] = pair.value;
      }
      return acc;
    }, {});
    onChange(validPairs);
  };

  return (
    <div style={{ marginBottom: 'var(--space-4)' }}>
      <label style={{ display: 'block', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-2)' }}>
        {label}
      </label>
      
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        {internalPairs.map((pair, index) => (
          <div key={index} style={{ display: 'flex', gap: 'var(--space-2)' }}>
            <input
              type="text"
              className="input"
              placeholder={placeholder.key}
              value={pair.key}
              onChange={(e) => handleChange(index, 'key', e.target.value)}
              style={{ flex: 1 }}
            />
            <input
              type="text"
              className="input"
              placeholder={placeholder.value}
              value={pair.value}
              onChange={(e) => handleChange(index, 'value', e.target.value)}
              style={{ flex: 1 }}
            />
            <Button 
              variant="secondary" 
              onClick={() => handleRemove(index)}
              style={{ padding: 'var(--space-2)', minWidth: 'auto' }}
              title="Remove header"
            >
              <X size={16} />
            </Button>
          </div>
        ))}
      </div>

      <Button 
        variant="secondary" 
        onClick={handleAdd}
        style={{ marginTop: 'var(--space-2)', width: '100%', borderStyle: 'dashed' }}
      >
        <Plus size={16} style={{ marginRight: 'var(--space-2)' }} />
        Add Header
      </Button>
    </div>
  );
}
