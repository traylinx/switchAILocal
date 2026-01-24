import { create } from 'zustand';
import { apiClient } from '../api/client';

export const useConfigStore = create((set, get) => ({
  config: null,
  
  // UI State
  activeProvider: null,
  isConfigModalOpen: false,
  configDraft: null,

  // Actions
  setConfig: (config) => set({ config }),
  
  updateConfigLocally: (updates) => set((state) => ({
    config: state.config ? { ...state.config, ...updates } : updates
  })),

  setActiveProvider: (provider) => set({ activeProvider: provider }),

  openConfigModal: (provider) => {
    // Create a draft from the provider's current config/metadata
    // This allows editing without modifying the active config until save
    const draft = {
      apiKey: '',
      baseUrl: '',
      prefix: '',
      proxyUrl: '',
      modelsUrl: '',
      headers: {},
      excludedModels: [],
      models: [],
      ...provider.config, // Assuming provider has a config object populated
      // fallback mapping if provider.config isn't perfectly structured yet
    };

    // Specific field mapping if needed (e.g. from flattened view model)
    if (provider.metadata) {
      if (provider.metadata.baseUrl) draft.baseUrl = provider.metadata.baseUrl;
      if (provider.metadata.prefix) draft.prefix = provider.metadata.prefix;
    }

    set({ 
      activeProvider: provider, 
      isConfigModalOpen: true, 
      configDraft: draft 
    });
  },

  closeConfigModal: () => set({ 
    activeProvider: null, 
    isConfigModalOpen: false, 
    configDraft: null 
  }),

  updateConfigDraft: (updates) => set((state) => ({
    configDraft: { ...state.configDraft, ...updates }
  })),

  // Async Actions
  // Async Actions
  saveProviderConfig: async (providerId, configUpdate) => {
    const state = get();
    // Refresh latest config to avoid stale overwrites
    // In a real app we might want to fetch fresh config first, but here we use what we have in store (synced via SWR usually)
    // Actually, calling apiClient.getConfig() might be safer.
    // Let's assume state.config is reasonably fresh or we accept optimistic updates.
    
    const currentConfig = state.config || {};
    const updates = {};
    const provider = state.activeProvider;

    if (!provider) return;

    try {
        //Logic ported from ProvidersView.jsx
        
        // OpenCode CLI
        if (provider.id === 'opencode-cli') {
            updates.opencode = {
              ...currentConfig.opencode,
              enabled: true,
              'base-url': configUpdate.baseUrl || 'http://host.docker.internal:4096',
            };
            // Preserve other fields if any
        }
        // Ollama
        else if (provider.id === 'ollama') {
            updates.ollama = {
              ...currentConfig.ollama,
              enabled: true, // Auto-enable on save
              'base-url': configUpdate.baseUrl || 'http://localhost:11434',
              'models-url': configUpdate.modelsUrl || `${configUpdate.baseUrl || 'http://localhost:11434'}/api/tags`,
              // Add advanced fields
              'proxy-url': configUpdate.proxyUrl,
              headers: configUpdate.headers,
              'excluded-models': configUpdate.excludedModels,
              models: configUpdate.models,
            };
        }
        // LM Studio
        else if (provider.id === 'lmstudio') {
            updates.lmstudio = {
              ...currentConfig.lmstudio,
              enabled: true,
              'base-url': configUpdate.baseUrl || 'http://localhost:1234/v1',
              'proxy-url': configUpdate.proxyUrl,
              headers: configUpdate.headers,
              'excluded-models': configUpdate.excludedModels,
              models: configUpdate.models,
            };
        }
        // Official OpenAI API
        else if (provider.id === 'openai') {
            const openaiCompatArray = [...(currentConfig['openai-compatibility'] || [])];
            const openaiIndex = openaiCompatArray.findIndex(e => e.name === 'openai');
            
            const openaiEntry = {
              ...(openaiIndex >= 0 ? openaiCompatArray[openaiIndex] : {}),
              name: 'openai',
              prefix: 'openai',
              'base-url': configUpdate.baseUrl || 'https://api.openai.com/v1',
              'api-key-entries': [{ 'api-key': configUpdate.apiKey }],
              'proxy-url': configUpdate.proxyUrl,
              'models-url': configUpdate.modelsUrl,
              headers: configUpdate.headers,
              'excluded-models': configUpdate.excludedModels,
              models: configUpdate.models,
            };

            if (openaiIndex >= 0) {
              openaiCompatArray[openaiIndex] = openaiEntry;
            } else {
              openaiCompatArray.push(openaiEntry);
            }
            updates['openai-compatibility'] = openaiCompatArray;
        }
        // New or Edit Dynamic OpenAI Compatible provider
        else if (provider.isOpenAICompat && provider.isDynamic) {
             const openaiCompatArray = [...(currentConfig['openai-compatibility'] || [])];
             
             // If activeProvider was 'new...', we are creating
             // But configUpdate should contain the name/prefix from the modal
             const newName = configUpdate.name || provider.name || configUpdate.prefix;
             const newPrefix = configUpdate.prefix || newName;

             const newEntry = {
               name: newName,
               prefix: newPrefix,
               'base-url': configUpdate.baseUrl,
               'api-key-entries': [{ 'api-key': configUpdate.apiKey }],
               'proxy-url': configUpdate.proxyUrl,
               'models-url': configUpdate.modelsUrl,
               headers: configUpdate.headers,
               'excluded-models': configUpdate.excludedModels,
               models: configUpdate.models,
             };

             // Are we editing an existing one?
             const existingIndex = openaiCompatArray.findIndex(
               e => e.name === provider.originalConfig?.name
             );
             if (existingIndex >= 0 && provider.id !== 'new-openai-compat') {
                 openaiCompatArray[existingIndex] = newEntry;
             } else {
                 openaiCompatArray.push(newEntry);
             }
             updates['openai-compatibility'] = openaiCompatArray;
        }
        // Standard cloud API providers (Anthropic, Gemini, etc.)
        else if (provider.configKey && !provider.isOpenAICompat) {
             // These usually have a specific array structure like claude-api-key: [{api-key: ...}]
             // We need to check if they support other fields in the backend/yaml struct.
             // Assuming they do support proxy/headers etc at the top level of the entry? 
             // config.yaml structure for compatible providers:
             // claude:
             //   - api-key: ...
             //     proxy-url: ...
             
             const currentEntries = [...(currentConfig[provider.configKey] || [])];
             const newEntry = {
                 'api-key': configUpdate.apiKey,
                 'base-url': configUpdate.baseUrl,
                 prefix: configUpdate.prefix,
                 'proxy-url': configUpdate.proxyUrl,
                 'models-url': configUpdate.modelsUrl,
                 headers: configUpdate.headers,
                 'excluded-models': configUpdate.excludedModels,
                 models: configUpdate.models,
             };
             
             if (currentEntries.length > 0) {
                 // Update first entry? usually single entry supported for these in UI
                 currentEntries[0] = newEntry;
             } else {
                 currentEntries.push(newEntry);
             }
             updates[provider.configKey] = currentEntries;
        }

        await apiClient.updateConfig({ ...currentConfig, ...updates });
        // Update local state if needed, but SWR usually handles revalidation
        // or we can optimistic update via setConfig
        state.setConfig({ ...currentConfig, ...updates });

    } catch (error) {
        console.error("Failed to save config:", error);
        throw error; // Let component handle UI feedback
    }
  },

  testConnection: async (config) => {
    // Call the new test endpoint (to be implemented in client.js)
    return apiClient.testProviderConnection(config);
  },

  discoverModels: async (config) => {
    // Call the new discover endpoint (to be implemented in client.js)
    return apiClient.discoverProviderModels(config);
  }
}));
