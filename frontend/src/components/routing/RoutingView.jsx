import React, { useState } from 'react';
import useSWR from 'swr';
import { apiClient } from '../../api/client';
import { useStateBoxStatus } from '../../hooks/useStateBoxStatus';
import { Button } from '../common/Button';
import { Card } from '../common/Card';
import { Modal } from '../common/Modal';
import { Spinner } from '../common/Spinner';
import { Trash2, Plus } from 'lucide-react';

export function RoutingView() {
  const { data: mappings, mutate, isLoading } = useSWR(
    '/ampcode/model-mappings',
    () => apiClient.getModelMappings()
  );
  
  const { status: stateBoxStatus } = useStateBoxStatus();
  const isReadOnly = stateBoxStatus?.read_only || false;
  
  const [showModal, setShowModal] = useState(false);
  const [newFrom, setNewFrom] = useState('');
  const [newTo, setNewTo] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  const handleSave = async () => {
    const trimmedFrom = newFrom.trim();
    const trimmedTo = newTo.trim();

    if (!trimmedFrom || !trimmedTo) {
      alert('Model names cannot be empty');
      return;
    }

    // Validation for model names (no spaces, valid characters)
    const modelNameRegex = /^[a-zA-Z0-9\-_\.:\/]+$/;
    if (!modelNameRegex.test(trimmedFrom)) {
      alert('Legacy/Incoming model name contains invalid characters or spaces. Use letters, numbers, and - _ . : / only.');
      return;
    }
    if (!modelNameRegex.test(trimmedTo)) {
      alert('Target model name contains invalid characters or spaces. Use letters, numbers, and - _ . : / only.');
      return;
    }

    // Check for duplicate
    const exists = mappings?.some(m => m.from === trimmedFrom);
    if (exists) {
      alert('A mapping for this model already exists. Please delete it first or use a different name.');
      return;
    }

    setIsSaving(true);
    try {
      const updated = [...(mappings || []), { from: trimmedFrom, to: trimmedTo }];
      await apiClient.updateModelMappings(updated);
      mutate();
      setNewFrom('');
      setNewTo('');
      setShowModal(false);
    } catch (error) {
      alert('Failed to update mappings: ' + error.message);
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async (index) => {
    if (!confirm('Are you sure you want to delete this mapping?')) return;
    
    const updated = mappings.filter((_, i) => i !== index);
    try {
      await apiClient.updateModelMappings(updated);
      mutate(updated);
    } catch (e) {
      alert('Failed to delete mapping');
    }
  };

  if (isLoading) return <div style={{ display: 'flex', justifyContent: 'center', padding: '100px' }}><Spinner size="lg" /></div>;

  return (
    <div>
      <header style={{ 
        marginBottom: 'var(--space-8)', 
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'flex-start' 
      }}>
        <div>
          <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 'var(--font-bold)', marginBottom: 'var(--space-2)' }}>Model Routing</h1>
          <p style={{ color: 'var(--color-text-secondary)' }}>Map model names to available models across providers.</p>
        </div>
        <Button variant="primary" onClick={() => setShowModal(true)} disabled={isReadOnly}>
          <Plus size={18} /> Add Mapping
        </Button>
      </header>
      
      <Card style={{ padding: 0, overflow: 'hidden' }}>
        <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left' }}>
          <thead style={{ backgroundColor: 'var(--color-bg-secondary)', borderBottom: '1px solid var(--color-border)' }}>
            <tr>
              <th style={{ padding: 'var(--space-4) var(--space-6)', fontSize: 'var(--text-xs)', fontWeight: 'var(--font-semibold)', color: 'var(--color-text-secondary)', textTransform: 'uppercase' }}>From Model</th>
              <th style={{ padding: 'var(--space-4) var(--space-6)', fontSize: 'var(--text-xs)', fontWeight: 'var(--font-semibold)', color: 'var(--color-text-secondary)', textTransform: 'uppercase' }}>To Model</th>
              <th style={{ padding: 'var(--space-4) var(--space-6)', textAlign: 'right' }}></th>
            </tr>
          </thead>
          <tbody>
            {!mappings || mappings.length === 0 ? (
              <tr>
                <td colSpan="3" style={{ padding: 'var(--space-12)', textAlign: 'center', color: 'var(--color-text-tertiary)' }}>
                  No model mappings configured.
                </td>
              </tr>
            ) : (
              mappings.map((m, i) => (
                <tr key={i} style={{ borderBottom: '1px solid var(--color-border)' }}>
                  <td style={{ padding: 'var(--space-4) var(--space-6)', fontWeight: 'var(--font-medium)' }}>{m.from}</td>
                  <td style={{ padding: 'var(--space-4) var(--space-6)', fontFamily: 'var(--font-mono)', fontSize: 'var(--text-sm)' }}>{m.to}</td>
                  <td style={{ padding: 'var(--space-4) var(--space-6)', textAlign: 'right' }}>
                    <button 
                      onClick={() => handleDelete(i)}
                      disabled={isReadOnly}
                      style={{ 
                        background: 'none', 
                        border: 'none', 
                        color: 'var(--color-text-tertiary)', 
                        cursor: isReadOnly ? 'not-allowed' : 'pointer',
                        padding: 'var(--space-1)',
                        opacity: isReadOnly ? 0.5 : 1
                      }}
                      onMouseOver={e => !isReadOnly && (e.currentTarget.style.color = 'var(--color-error)')}
                      onMouseOut={e => e.currentTarget.style.color = 'var(--color-text-tertiary)'}
                    >
                      <Trash2 size={16} />
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </Card>

      <Modal
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        title="Add Model Mapping"
        footer={
          <>
            <Button onClick={() => setShowModal(false)}>Cancel</Button>
            <Button variant="primary" onClick={handleSave} disabled={isSaving || isReadOnly || !newFrom || !newTo}>
              {isSaving ? <Spinner size="sm" /> : 'Create Mapping'}
            </Button>
          </>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <div>
            <label style={{ display: 'block', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-2)' }}>
              Legacy / Incoming Model Name
            </label>
            <input
              type="text"
              className="input"
              placeholder="e.g. gpt-4"
              value={newFrom}
              onChange={(e) => setNewFrom(e.target.value)}
              disabled={isSaving || isReadOnly}
            />
          </div>
          <div>
            <label style={{ display: 'block', fontSize: 'var(--text-sm)', fontWeight: 'var(--font-medium)', marginBottom: 'var(--space-2)' }}>
              Target Model (exact provider name)
            </label>
            <input
              type="text"
              className="input"
              placeholder="e.g. claude-3-opus-20240229"
              value={newTo}
              onChange={(e) => setNewTo(e.target.value)}
              disabled={isSaving || isReadOnly}
            />
          </div>
        </div>
      </Modal>
    </div>
  );
}
