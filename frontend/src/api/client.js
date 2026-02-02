const API_BASE = '/v0/management';

class APIClient {
  constructor() {
    this.baseURL = API_BASE;
  }
  
  getAuthHeader() {
    const key = localStorage.getItem('switchai_management_key');
    return key ? { 'Authorization': `Bearer ${key}` } : {};
  }
  
  async request(endpoint, options = {}) {
    const url = `${this.baseURL}${endpoint}`;
    const headers = {
      'Content-Type': 'application/json',
      ...this.getAuthHeader(),
      ...options.headers,
    };
    
    const response = await fetch(url, { ...options, headers });
    
    if (response.status === 401) {
      // Trigger logout
      localStorage.removeItem('switchai_management_key');
      window.location.reload();
      throw new Error('Unauthorized');
    }
    
    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || 'Request failed');
    }
    
    // Some endpoints might return empty/204
    if (response.status === 204 || response.headers.get('content-length') === '0') {
      return null;
    }

    try {
      return await response.json();
    } catch (e) {
      return null;
    }
  }
  
  // Endpoints
  getConfig() {
    return this.request('/config');
  }
  
  updateConfig(config) {
    return this.request('/config.yaml', {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  }
  
  getApiKeys() {
    return this.request('/api-keys');
  }
  
  addApiKey(key) {
    return this.request('/api-keys', {
      method: 'POST',
      body: JSON.stringify(key),
    });
  }
  
  deleteApiKey(keyId) {
    return this.request(`/api-keys/${keyId}`, {
      method: 'DELETE',
    });
  }
  
  getModelMappings() {
    return this.request('/ampcode/model-mappings').then(res => res?.['model-mappings'] || []);
  }
  
  updateModelMappings(mappings) {
    return this.request('/ampcode/model-mappings', {
      method: 'PUT',
      body: JSON.stringify({ value: mappings }),
    });
  }

  testProviderConnection(config) {
    return this.request('/providers/test', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  }

  discoverProviderModels(config) {
    return this.request('/providers/discover-models', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  }

  getStateBoxStatus() {
    return this.request('/state-box/status');
  }
}

export const apiClient = new APIClient();
