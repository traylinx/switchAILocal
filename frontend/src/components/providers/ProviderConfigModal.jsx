import React, { useState, useEffect } from 'react';
import { useConfigStore } from '../../stores/configStore';
import { useStateBoxStatus } from '../../hooks/useStateBoxStatus';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { KeyValueInput } from '../common/KeyValueInput';
import { StringArrayInput } from '../common/StringArrayInput';
import { ModelAliasInput } from '../common/ModelAliasInput';
import { ModelDiscovery } from './ModelDiscovery';
import { validateURL } from '../../utils/validation';
import { Loader2, CheckCircle2, AlertTriangle, XCircle, Info } from 'lucide-react';

export function ProviderConfigModal() {
  const { 
    isConfigModalOpen, 
    closeConfigModal, 
    activeProvider, 
    configDraft, 
    updateConfigDraft, 
    saveProviderConfig,
    testConnection
  } = useConfigStore();

  const { status: stateBoxStatus } = useStateBoxStatus();
  const isReadOnly = stateBoxStatus?.read_only || false;

  const [activeTab, setActiveTab] = useState('basic');
  const [testResult, setTestResult] = useState(null);
  const [isTesting, setIsTesting] = useState(false);
  const [errors, setErrors] = useState({});
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    if (isConfigModalOpen) {
      setActiveTab('basic');
      setTestResult(null);
      setErrors({});
      setIsSaving(false);
    }
  }, [isConfigModalOpen]);

  if (!isConfigModalOpen || !configDraft || !activeProvider) return null;

  const validate = () => {
    const newErrors = {};
    if (configDraft.baseUrl && !validateURL(configDraft.baseUrl)) {
      newErrors.baseUrl = "Invalid URL format. Include http:// or https://";
    }
    if (configDraft.proxyUrl && !validateURL(configDraft.proxyUrl)) {
      newErrors.proxyUrl = "Invalid URL format. Include protocol (e.g. socks5://)";
    }
    if (configDraft.modelsUrl && !validateURL(configDraft.modelsUrl)) {
      newErrors.modelsUrl = "Invalid URL format. Include http:// or https://";
    }
    
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSave = async () => {
    if (!validate()) return;
    setIsSaving(true);
    try {
      await saveProviderConfig(activeProvider.id, configDraft);
      closeConfigModal();
    } catch (e) {
      console.error("Save failed:", e);
      // Could show toast here if we had a toast system
    } finally {
      setIsSaving(false);
    }
  };

  const handleTest = async () => {
    setIsTesting(true);
    setTestResult(null);
    try {
      const result = await testConnection({ ...configDraft, type: activeProvider.type });
      setTestResult(result);
    } catch (e) {
      setTestResult({ 
        success: false, 
        overallMessage: e.message || "Connection test failed", 
        tests: {} 
      });
    } finally {
      setIsTesting(false);
    }
  };

  const renderTestResult = (key, label) => {
    if (!testResult || !testResult.tests || !testResult.tests[key]) return null;
    const test = testResult.tests[key];
    
    return (
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', fontSize: 'var(--text-sm)', marginTop: '4px' }}>
        {test.passed ? <CheckCircle2 size={14} color="var(--color-success)" /> : <XCircle size={14} color="var(--color-error)" />}
        <span style={{ color: test.passed ? 'var(--color-text-secondary)' : 'var(--color-error)' }}>
          {label}: {test.message}
        </span>
      </div>
    );
  };

  const footer = (
    <>
      <div style={{ marginRight: 'auto', display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
        <Button variant="secondary" onClick={handleTest} disabled={isTesting}>
          {isTesting ? <Loader2 size={16} className="spin" /> : 'Test Connection'}
        </Button>
        {testResult && (
           <span style={{ 
             fontSize: 'var(--text-sm)', 
             color: testResult.success ? 'var(--color-success)' : 'var(--color-error)',
             display: 'flex', 
             alignItems: 'center', 
             gap: '4px' 
           }}>
             {testResult.success ? <CheckCircle2 size={16} /> : <AlertTriangle size={16} />}
             {testResult.overallMessage}
           </span>
        )}
      </div>
      <Button variant="secondary" onClick={closeConfigModal} disabled={isSaving}>
        Cancel
      </Button>
      <Button variant="primary" onClick={handleSave} disabled={isSaving || isTesting || isReadOnly}>
        {isSaving ? <Loader2 size={16} className="spin" /> : 'Save Changes'}
      </Button>
    </>
  );

  return (
    <Modal 
      isOpen={isConfigModalOpen} 
      onClose={closeConfigModal} 
      title={`Configure ${activeProvider.displayName || activeProvider.id}`}
      footer={footer}
    >
      <div style={{ borderBottom: '1px solid var(--color-border)', marginBottom: 'var(--space-6)' }}>
        <div style={{ display: 'flex', gap: 'var(--space-6)' }}>
          <button
            onClick={() => setActiveTab('basic')}
            style={{
              padding: 'var(--space-3) 0',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'basic' ? '2px solid var(--color-primary)' : '2px solid transparent',
              color: activeTab === 'basic' ? 'var(--color-text-primary)' : 'var(--color-text-secondary)',
              fontWeight: activeTab === 'basic' ? 'var(--font-semibold)' : 'normal',
              cursor: 'pointer'
            }}
          >
            Basic Settings
          </button>
          <button
            onClick={() => setActiveTab('advanced')}
            style={{
              padding: 'var(--space-3) 0',
              background: 'none',
              border: 'none',
              borderBottom: activeTab === 'advanced' ? '2px solid var(--color-primary)' : '2px solid transparent',
              color: activeTab === 'advanced' ? 'var(--color-text-primary)' : 'var(--color-text-secondary)',
              fontWeight: activeTab === 'advanced' ? 'var(--font-semibold)' : 'normal',
              cursor: 'pointer'
            }}
          >
            Advanced
          </button>
        </div>
      </div>

      {testResult && !testResult.success && (
        <div style={{ 
          padding: 'var(--space-3)', 
          backgroundColor: 'var(--color-error-light)', 
          borderRadius: 'var(--radius-sm)',
          marginBottom: 'var(--space-4)'
        }}>
          {renderTestResult('apiKey', 'API Key')}
          {renderTestResult('baseUrl', 'Base URL')}
          {renderTestResult('proxy', 'Proxy')}
          {renderTestResult('modelsUrl', 'Models URL')}
        </div>
      )}

      {activeTab === 'basic' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <div>
            <label className="label">API Key</label>
            <input
              type="password"
              className="input"
              value={configDraft.apiKey || ''}
              onChange={(e) => updateConfigDraft({ apiKey: e.target.value })}
              placeholder={activeProvider.id === 'openai' ? 'sk-...' : 'API Key'}
            />
            <p className="help-text">Leave empty if not required or using env variables.</p>
          </div>

          <div>
            <label className="label">Base URL</label>
            <input
              type="text"
              className={`input ${errors.baseUrl ? 'input-error' : ''}`}
              value={configDraft.baseUrl || ''}
              onChange={(e) => updateConfigDraft({ baseUrl: e.target.value })}
              placeholder="https://api.openai.com/v1"
            />
            {errors.baseUrl && <p className="error-text">{errors.baseUrl}</p>}
          </div>

          <div>
            <label className="label">Prefix</label>
            <input
              type="text"
              className="input"
              value={configDraft.prefix || ''}
              onChange={(e) => updateConfigDraft({ prefix: e.target.value })}
              placeholder="openai"
              disabled={activeProvider.static} // Some providers might strictly enforce prefix? usually editable.
            />
            <p className="help-text">Prefix used in model requests (e.g. <code>prefix:model-name</code>).</p>
          </div>
        </div>
      )}

      {activeTab === 'advanced' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-6)' }}>
          <div style={{ padding: 'var(--space-4)', backgroundColor: 'var(--color-bg-secondary)', borderRadius: 'var(--radius-md)' }}>
            <h3 style={{ fontSize: 'var(--text-sm)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-3)' }}>Network</h3>
            
            <div style={{ marginBottom: 'var(--space-4)' }}>
              <label className="label">Proxy URL</label>
              <input
                type="text"
                className={`input ${errors.proxyUrl ? 'input-error' : ''}`}
                value={configDraft.proxyUrl || ''}
                onChange={(e) => updateConfigDraft({ proxyUrl: e.target.value })}
                placeholder="socks5://user:pass@host:port"
              />
              {errors.proxyUrl && <p className="error-text">{errors.proxyUrl}</p>}
              <p className="help-text">Overrides global proxy settings for this provider.</p>
            </div>

            <KeyValueInput 
              label="Custom Headers" 
              pairs={configDraft.headers || {}}
              onChange={(headers) => updateConfigDraft({ headers })} 
            />
          </div>

          <div style={{ padding: 'var(--space-4)', backgroundColor: 'var(--color-bg-secondary)', borderRadius: 'var(--radius-md)' }}>
             <h3 style={{ fontSize: 'var(--text-sm)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-3)' }}>Models</h3>

             <div style={{ marginBottom: 'var(--space-4)' }}>
                <label className="label">Models Discovery URL</label>
                <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
                  <input
                    type="text"
                    className={`input ${errors.modelsUrl ? 'input-error' : ''}`}
                    value={configDraft.modelsUrl || ''}
                    onChange={(e) => updateConfigDraft({ modelsUrl: e.target.value })}
                    placeholder="https://api.openai.com/v1/models"
                    style={{ flex: 1 }}
                  />
                </div>
                {errors.modelsUrl && <p className="error-text">{errors.modelsUrl}</p>}
             </div>

             <ModelDiscovery 
                modelsUrl={configDraft.modelsUrl}
                apiKey={configDraft.apiKey}
                baseUrl={configDraft.baseUrl}
                proxyUrl={configDraft.proxyUrl}
                headers={configDraft.headers}
                onSelectModel={(model) => {
                  // Pre-fill alias input or add to list?
                  // Plan: "allow selection for alias creation"
                  // I'll implement a simple add to aliases list if not exists
                  const newAlias = { name: model.id, alias: model.id };
                  const currentModels = configDraft.models || [];
                  if (!currentModels.find(m => m.name === newAlias.name)) {
                     updateConfigDraft({ models: [...currentModels, newAlias] });
                  }
                }}
             />

             <ModelAliasInput
                aliases={configDraft.models || []}
                onChange={(models) => updateConfigDraft({ models })}
                availableModels={[]} // discovery populates this? or separate state?
             />

             <StringArrayInput
               label="Excluded Models"
               values={configDraft.excludedModels || []}
               onChange={(excludedModels) => updateConfigDraft({ excludedModels })}
               placeholder="wildcards supported (e.g. text-*)"
               helpText="Models matching these patterns will be hidden."
             />
          </div>
        </div>
      )}
    </Modal>
  );
}
