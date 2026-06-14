import { defineConfig } from 'vite';
import type { OutputAsset, OutputChunk } from 'rollup';
import react from '@vitejs/plugin-react';
import { brotliCompressSync, gzipSync } from 'node:zlib';

export default defineConfig({
  plugins: [react(), precompressAssets()],
  base: './',
  build: {
    outDir: '../internal/web/static/dist',
    emptyOutDir: true,
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          const normalized = id.replace(/\\/g, '/');
          if (!normalized.includes('node_modules')) {
            return;
          }
          if (normalized.includes('node_modules/react/') || normalized.includes('node_modules/react-dom/') || normalized.includes('node_modules/react-router-dom/')) {
            return 'react';
          }
          if (normalized.includes('node_modules/@tanstack/react-query/')) {
            return 'query';
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
      '/panel/api': 'http://127.0.0.1:9999',
      '/panel/sub': 'http://127.0.0.1:9999',
    },
  },
});

function precompressAssets() {
  return {
    name: 'migate-precompress-assets',
    apply: 'build' as const,
    generateBundle(_options: unknown, bundle: Record<string, OutputAsset | OutputChunk>) {
      for (const [fileName, asset] of Object.entries(bundle)) {
        if (!shouldPrecompress(fileName)) continue;
        const source = asset.type === 'asset' ? asset.source : asset.code;
        if (source == null) continue;
        const input = typeof source === 'string' ? Buffer.from(source) : Buffer.from(source);
        this.emitFile({ type: 'asset', fileName: `${fileName}.gz`, source: gzipSync(input, { level: 9 }) });
        this.emitFile({ type: 'asset', fileName: `${fileName}.br`, source: brotliCompressSync(input) });
      }
    },
  };
}

function shouldPrecompress(fileName: string) {
  return /\.(js|css|html|svg|json)$/.test(fileName);
}
