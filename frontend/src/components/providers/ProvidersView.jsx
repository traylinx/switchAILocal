import React, { useMemo } from 'react';
import { PROVIDERS, ProviderCategories, createDynamicProvider } from '../../utils/providers';
import { ProviderCard } from './ProviderCard';
import { ProviderConfigModal } from './ProviderConfigModal';
import { useConfig } from '../../hooks/useConfig';
import { useConfigStore } from '../../stores/configStore';
import { Spinner } from '../common/Spinner';
import { Plus } from 'lucide-react';

export function ProvidersView() {
  const { config, isLoading } = useConfig();
  const { openConfigModal } = useConfigStore();

  // Merge static providers with dynamic OpenAI-compatible ones
  const allProviders = useMemo(() => {
    if (!config) return PROVIDERS;

    const dynamicProviders = [];
    const openaiCompatArray = config['openai-compatibility'] || [];

    // Create dynamic providers for non-openai entries
    openaiCompatArray.forEach(entry => {
      if (entry.name && entry.name !== 'openai') {
        dynamicProviders.push(createDynamicProvider(entry));
      }
    });

    return [...PROVIDERS, ...dynamicProviders];
  }, [config]);
  
  // Check which providers are connected and compute their metadata
  const providerState = useMemo(() => {
    const connectedSet = new Set();
    const metadataMap = {};

    if (!config) return { connectedSet, metadataMap };

    allProviders.forEach(p => {
      let isConnected = false;
      let metadata = {};
      let lastTested = null; // Could come from store/config if persisted, currently purely runtime connection check? 
                             // Plan checks "lastTested" in ProviderCard. 
                             // We don't stick lastTested in config.yaml usually. 
                             // Maybe local store state? For now, we omit or mock.

      // CLI tools are always "available" if no config needed
      if (p.category === ProviderCategories.CLI && !p.configKey) {
        isConnected = true;
      }
      // OpenCode CLI check
      else if (p.id === 'opencode-cli') {
        if (config.opencode?.enabled) {
          isConnected = true;
          metadata = { baseUrl: config.opencode['base-url'] };
        }
      }
      // Ollama check
      else if (p.id === 'ollama') {
        if (config.ollama?.enabled) {
          isConnected = true;
          metadata = { baseUrl: config.ollama['base-url'] };
        }
      }
      // LM Studio check
      else if (p.id === 'lmstudio') {
        if (config.lmstudio?.enabled) {
          isConnected = true;
          metadata = { baseUrl: config.lmstudio['base-url'] };
        }
      }
      // OpenAI API (official) check
      else if (p.id === 'openai') {
        const openaiCompatArray = config['openai-compatibility'] || [];
        const openaiEntry = openaiCompatArray.find(e => e.name === 'openai');
        if (openaiEntry && openaiEntry['api-key-entries']?.length > 0) {
          isConnected = true;
          metadata = { baseUrl: openaiEntry['base-url'], prefix: openaiEntry.prefix };
        }
      }
      // Dynamic OpenAI Compatible providers
      else if (p.isDynamic && p.originalConfig) {
        isConnected = true;
        metadata = { 
          baseUrl: p.originalConfig['base-url'], 
          prefix: p.originalConfig.prefix || p.originalConfig.name 
        };
      }
      // Standard cloud API providers
      else if (p.configKey && !p.isOpenAICompat) {
        const keys = config[p.configKey];
        if (Array.isArray(keys) && keys.length > 0) {
          const hasKey = keys.some(entry => entry['api-key']);
          if (hasKey) {
            isConnected = true;
            metadata = { 
              prefix: keys[0].prefix || '',
              baseUrl: keys[0]['base-url'] || ''
            };
          }
        }
      }

      if (isConnected) {
        connectedSet.add(p.id);
        metadataMap[p.id] = metadata;
      }
    });

    return { connectedSet, metadataMap };
  }, [config, allProviders]);

  // Group providers by category
  const providersByCategory = useMemo(() => {
    const grouped = {
      [ProviderCategories.CLI]: [],
      [ProviderCategories.LOCAL]: [],
      [ProviderCategories.CLOUD]: [],
    };

    allProviders.forEach(p => {
      grouped[p.category].push(p);
    });

    return grouped;
  }, [allProviders]);

  const handleConnect = (provider) => {
    // Populate simple configuration object for the modal
    const currentConfig = {
       apiKey: '',
       baseUrl: '',
       prefix: '',
       proxyUrl: '',
       modelsUrl: '',
       headers: {},
       excludedModels: [],
       models: []
    };
    
    if (provider.id === 'opencode-cli') {
      currentConfig.baseUrl = config.opencode?.['base-url'] || 'http://host.docker.internal:4096';
    }
    else if (provider.id === 'ollama') {
      const c = config.ollama || {};
      currentConfig.baseUrl = c['base-url'] || 'http://localhost:11434';
      currentConfig.modelsUrl = c['models-url'];
      currentConfig.proxyUrl = c['proxy-url'];
      currentConfig.headers = c.headers;
      currentConfig.excludedModels = c['excluded-models'] || [];
      currentConfig.models = c.models || [];
    }
    else if (provider.id === 'lmstudio') {
      const c = config.lmstudio || {};
      currentConfig.baseUrl = c['base-url'] || 'http://localhost:1234/v1';
      currentConfig.proxyUrl = c['proxy-url'];
      currentConfig.headers = c.headers;
      currentConfig.excludedModels = c['excluded-models'] || [];
      currentConfig.models = c.models || [];
    }
    else if (provider.id === 'openai') {
      const openaiCompatArray = config['openai-compatibility'] || [];
      const entry = openaiCompatArray.find(e => e.name === 'openai');
      if (entry) {
         currentConfig.apiKey = entry['api-key-entries']?.[0]?.['api-key'] || '';
         currentConfig.baseUrl = entry['base-url'] || 'https://api.openai.com/v1';
         currentConfig.prefix = entry.prefix;
         currentConfig.proxyUrl = entry['proxy-url'];
         currentConfig.modelsUrl = entry['models-url'];
         currentConfig.headers = entry.headers;
         currentConfig.excludedModels = entry['excluded-models'] || [];
         currentConfig.models = entry.models || [];
      } else {
         currentConfig.baseUrl = 'https://api.openai.com/v1';
         currentConfig.prefix = 'openai';
      }
    }
    else if (provider.isDynamic && provider.originalConfig) {
      const c = provider.originalConfig;
      currentConfig.apiKey = c['api-key-entries']?.[0]?.['api-key'] || '';
      currentConfig.baseUrl = c['base-url'];
      currentConfig.prefix = c.prefix || c.name;
      currentConfig.proxyUrl = c['proxy-url'];
      currentConfig.modelsUrl = c['models-url'];
      currentConfig.headers = c.headers;
      currentConfig.excludedModels = c['excluded-models'] || [];
      currentConfig.models = c.models || [];
    }
    else if (provider.configKey && !provider.isOpenAICompat) {
       // Standard Cloud Providers
       const keys = config[provider.configKey];
       if (Array.isArray(keys) && keys.length > 0) {
          const entry = keys[0];
          currentConfig.apiKey = entry['api-key'] || '';
          currentConfig.baseUrl = entry['base-url'];
          currentConfig.prefix = entry.prefix;
          currentConfig.proxyUrl = entry['proxy-url'];
          currentConfig.modelsUrl = entry['models-url'];
          currentConfig.headers = entry.headers;
          currentConfig.excludedModels = entry['excluded-models'] || [];
          currentConfig.models = entry.models || [];
       }
    }

    openConfigModal({ ...provider, config: currentConfig });
  };

  const handleAddNewProvider = () => {
    openConfigModal({
      id: 'new-openai-compat',
      displayName: 'New OpenAI Compatible Provider',
      name: 'New Provider', // Internal use
      category: ProviderCategories.CLOUD,
      isOpenAICompat: true,
      isDynamic: true,
      config: {
         apiKey: '',
         baseUrl: '',
         prefix: '',
         proxyUrl: '',
         modelsUrl: '',
         headers: {},
         excludedModels: [],
         models: []
      }
    });
  };

  if (isLoading) return <div style={{ display: 'flex', justifyContent: 'center', padding: '100px' }}><Spinner size="lg" /></div>;

  const renderProviderSection = (category) => {
    const providers = providersByCategory[category];
    if (!providers || providers.length === 0) return null;

    return (
      <section key={category} style={{ marginBottom: 'var(--space-8)' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 'var(--space-4)' }}>
          <h2 style={{ 
            fontSize: 'var(--text-xl)', 
            fontWeight: 'var(--font-semibold)', 
            color: 'var(--color-text-primary)',
          }}>
            {category}
          </h2>
        </div>
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(280px, 1fr))',
          gap: 'var(--space-4)'
        }}>
          {providers.map(p => (
            <ProviderCard 
              key={p.id} 
              provider={{...p, metadata: providerState.metadataMap[p.id]}} 
              isConnected={providerState.connectedSet.has(p.id)}
              onConnect={() => handleConnect(p)}
            />
          ))}
          
          {category === ProviderCategories.CLOUD && (
            <div 
              onClick={handleAddNewProvider}
              style={{
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                padding: 'var(--space-8)',
                background: 'transparent',
                border: '2px dashed var(--color-border)',
                borderRadius: 'var(--radius-md)',
                cursor: 'pointer',
                transition: 'all 0.2s',
                gap: 'var(--space-3)',
                height: '100%',
                minHeight: '200px'
              }}
              onMouseOver={(e) => {
                e.currentTarget.style.borderColor = 'var(--color-primary)';
                e.currentTarget.style.backgroundColor = 'var(--color-bg-secondary)';
              }}
              onMouseOut={(e) => {
                e.currentTarget.style.borderColor = 'var(--color-border)';
                e.currentTarget.style.backgroundColor = 'transparent';
              }}
            >
              <div style={{
                width: '40px',
                height: '40px',
                borderRadius: '50%',
                background: 'var(--color-primary-light)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                color: 'var(--color-primary)'
              }}>
                <Plus size={24} />
              </div>
              <div style={{ textAlign: 'center' }}>
                <h3 style={{ fontSize: 'var(--text-lg)', fontWeight: 'var(--font-semibold)', marginBottom: 'var(--space-1)' }}>Add Provider</h3>
                <p style={{ fontSize: 'var(--text-sm)', color: 'var(--color-text-secondary)' }}>Connect a new OpenAI-compatible API</p>
              </div>
            </div>
          )}
        </div>
      </section>
    );
  };

  return (
    <div>
      <header style={{ marginBottom: 'var(--space-8)' }}>
        <h1 style={{ fontSize: 'var(--text-3xl)', fontWeight: 'var(--font-bold)', marginBottom: 'var(--space-2)' }}>AI Providers</h1>
        <p style={{ color: 'var(--color-text-secondary)' }}>Connect your AI accounts to start routing requests through switchAILocal.</p>
      </header>
      
      {renderProviderSection(ProviderCategories.CLI)}
      {renderProviderSection(ProviderCategories.LOCAL)}
      {renderProviderSection(ProviderCategories.CLOUD)}

      <ProviderConfigModal />
    </div>
  );
}
