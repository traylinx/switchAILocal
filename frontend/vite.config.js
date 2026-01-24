import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { viteSingleFile } from 'vite-plugin-singlefile';

export default defineConfig({
  plugins: [
    react(),
    viteSingleFile({
      removeViteModuleLoader: true,
    }),
  ],
  build: {
    outDir: '../static',
    emptyOutDir: false,
    assetsInlineLimit: 100000000, // Inline everything
    cssCodeSplit: false,
  },
});
