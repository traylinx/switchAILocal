/**
 * @typedef {Object} ProviderConfig
 * @property {string} apiKey
 * @property {string} baseUrl
 * @property {string} prefix
 * @property {string} [proxyUrl]
 * @property {string} [modelsUrl]
 * @property {Record<string, string>} [headers]
 * @property {string[]} [excludedModels]
 * @property {ModelAlias[]} [models]
 */

/**
 * @typedef {Object} ModelAlias
 * @property {string} name - Upstream model name
 * @property {string} alias - Client-facing alias
 */

/**
 * @typedef {Object} TestResult
 * @property {boolean} success
 * @property {Object} tests
 * @property {TestDetail} tests.apiKey
 * @property {TestDetail} tests.baseUrl
 * @property {TestDetail} [tests.modelsUrl]
 * @property {TestDetail} [tests.proxy]
 * @property {string} overallMessage
 */

/**
 * @typedef {Object} TestDetail
 * @property {boolean} passed
 * @property {string} message
 * @property {number} [latency]
 * @property {number} [modelCount]
 */

/**
 * @typedef {Object} ValidationError
 * @property {string} field
 * @property {string} message
 */

/**
 * Validates a URL string supporting HTTP/HTTPS and SOCKS5 keys.
 * @param {string} url 
 * @returns {boolean}
 */
export const validateURL = (url) => {
  if (!url) return false;
  try {
    const parsed = new URL(url);
    return ['http:', 'https:', 'socks5:'].includes(parsed.protocol);
  } catch (e) {
    return false;
  }
};

/**
 * Checks if a model name matches an exclusion pattern.
 * Supports: *, prefix-*, *-suffix, *substring*
 * @param {string} modelName 
 * @param {string} pattern 
 * @returns {boolean}
 */
export const matchesPattern = (modelName, pattern) => {
  if (!pattern || !modelName) return false;
  if (pattern === '*') return true;
  if (pattern.endsWith('*') && pattern.startsWith('*')) {
    const substring = pattern.slice(1, -1);
    return modelName.includes(substring);
  }
  if (pattern.endsWith('*')) {
    const prefix = pattern.slice(0, -1);
    return modelName.startsWith(prefix);
  }
  if (pattern.startsWith('*')) {
    const suffix = pattern.slice(1);
    return modelName.endsWith(suffix);
  }
  return modelName === pattern;
};

/**
 * Validates custom headers.
 * @param {Record<string, string>} headers 
 * @returns {boolean}
 */
export const validateHeaders = (headers) => {
  if (!headers) return true;
  return Object.entries(headers).every(([key, value]) => {
    return key.trim().length > 0 && value !== undefined && value !== null;
  });
};
