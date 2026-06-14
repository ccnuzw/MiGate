import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../internal/web/static/dist',
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return;
          }
          if (id.includes('/react/') || id.includes('/react-dom/') || id.includes('/react-router-dom/')) {
            return 'react';
          }
          if (id.includes('/@tanstack/react-query/')) {
            return 'query';
          }
          if (id.includes('/react-hook-form/') || id.includes('/@hookform/resolvers/') || id.includes('/zod/')) {
            return 'forms';
          }
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://127.0.0.1:9999',
      '/sub': 'http://127.0.0.1:9999',
    },
  },
});
