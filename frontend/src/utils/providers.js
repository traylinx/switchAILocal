/**
 * Provider categories matching the README structure
 */
export const ProviderCategories = {
  CLI: 'CLI Tools (Use Your Paid Subscriptions)',
  LOCAL: 'Local LLM APIs',
  CLOUD: 'Cloud LLM APIs',
};

/**
 * Provider definitions with proper categorization
 */
export const PROVIDERS = [
  // CLI Tools - Use local subscriptions/defaults
  {
    id: 'gemini-cli',
    name: 'Google Gemini',
    prefix: 'geminicli:',
    category: ProviderCategories.CLI,
    description: 'Uses your local Gemini CLI authentication. Requires `gemini` in your PATH.',
    status: 'Ready',
    configKey: null,
  },
  {
    id: 'claude-cli',
    name: 'Anthropic Claude',
    prefix: 'claudecli:',
    category: ProviderCategories.CLI,
    description: 'Uses your local Claude CLI authentication. Requires `claude` in your PATH.',
    status: 'Ready',
    configKey: null,
  },
  {
    id: 'codex-cli',
    name: 'OpenAI Codex',
    prefix: 'codex:',
    category: ProviderCategories.CLI,
    description: 'Uses your local Codex environment. Requires `codex` in your PATH.',
    status: 'Ready',
    configKey: null,
  },
  {
    id: 'vibe-cli',
    name: 'Mistral Vibe',
    prefix: 'vibe:',
    category: ProviderCategories.CLI,
    description: 'Uses your local Vibe CLI authentication. Requires `vibe` in your PATH.',
    status: 'Ready',
    configKey: null,
  },
  {
    id: 'opencode-cli',
    name: 'OpenCode',
    prefix: 'opencode:',
    category: ProviderCategories.CLI,
    description: 'Connect to your local OpenCode server or CLI.',
    status: 'Ready',
    configKey: 'opencode',
  },

  // Local Models
  {
    id: 'ollama',
    name: 'Ollama',
    prefix: 'ollama:',
    category: ProviderCategories.LOCAL,
    configKey: 'ollama',
    description: 'Connect to your local Ollama instance.',
  },
  {
    id: 'lmstudio',
    name: 'LM Studio',
    prefix: 'lmstudio:',
    category: ProviderCategories.LOCAL,
    configKey: 'lmstudio',
    description: 'Connect to LM Studio running locally.',
  },

  // Cloud APIs
  {
    id: 'switchai',
    name: 'Traylinx switchAI',
    prefix: 'switchai:',
    category: ProviderCategories.CLOUD,
    configKey: 'switchai-api-key',
    description: 'Access curated models via your switchAI key.',
  },
  {
    id: 'google',
    name: 'Google AI Studio',
    prefix: 'gemini:',
    category: ProviderCategories.CLOUD,
    configKey: 'gemini-api-key',
    description: 'Direct connection to Google\'s API.',
  },
  {
    id: 'anthropic',
    name: 'Anthropic API',
    prefix: 'claude:',
    category: ProviderCategories.CLOUD,
    configKey: 'claude-api-key',
    description: 'Direct connection to Anthropic\'s API.',
  },
  {
    id: 'openai',
    name: 'OpenAI API',
    prefix: 'openai:',
    category: ProviderCategories.CLOUD,
    configKey: 'openai-compatibility',
    configName: 'openai',
    description: 'Official OpenAI API endpoint.',
    isOpenAICompat: true,
  },
];

/**
 * Creates a dynamic provider from openai-compatibility config entry
 */
export function createDynamicProvider(configEntry) {
  return {
    id: `openai-compat-${configEntry.name}`,
    name: configEntry.name.charAt(0).toUpperCase() + configEntry.name.slice(1),
    prefix: `${configEntry.prefix || configEntry.name}:`,
    category: ProviderCategories.CLOUD,
    configKey: 'openai-compatibility',
    description: `OpenAI-compatible endpoint: ${configEntry['base-url']}`,
    isOpenAICompat: true,
    isDynamic: true,
    originalConfig: configEntry,
  };
}
