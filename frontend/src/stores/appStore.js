import { create } from 'zustand';

export const useAppStore = create((set) => ({
  // Auth state
  isAuthenticated: !!localStorage.getItem('switchai_management_key'),
  managementKey: localStorage.getItem('switchai_management_key'),
  isConfigured: true, // Default to true, will be updated by AuthGuard
  
  // UI state
  currentView: 'providers',
  theme: localStorage.getItem('switchai_theme') || 
         (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'),
  isLoading: false,
  
  // Actions
  login: (key) => {
    localStorage.setItem('switchai_management_key', key);
    set({ isAuthenticated: true, managementKey: key });
  },
  logout: () => {
    localStorage.removeItem('switchai_management_key');
    set({ isAuthenticated: false, managementKey: null });
    window.location.reload(); // Refresh to clear all sensitive state
  },
  setView: (view) => set({ currentView: view }),
  toggleTheme: () => set((state) => {
    const newTheme = state.theme === 'light' ? 'dark' : 'light';
    localStorage.setItem('switchai_theme', newTheme);
    document.documentElement.setAttribute('data-theme', newTheme);
    return { theme: newTheme };
  }),
  setConfigured: (configured) => set({ isConfigured: configured }),
}));
