import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { readdir, readFile, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { brotliCompressSync, gzipSync } from 'node:zlib';

const backendProxy = {
  target: 'http://127.0.0.1:9999',
  changeOrigin: false,
  configure(proxy) {
    proxy.on('proxyReq', (proxyReq, req) => {
      if (req.headers.host) {
        proxyReq.setHeader('Host', req.headers.host);
      }
    });
  },
};

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
          if (normalized.includes('node_modules/@xyflow/')) {
            return 'xyflow';
          }
          if (normalized.includes('node_modules/elkjs/')) {
            return 'elk';
          }
        },
      },
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': backendProxy,
      '/sub': backendProxy,
      '/panel/api': backendProxy,
      '/panel/sub': backendProxy,
    },
  },
});

function precompressAssets() {
  let outDir = '';
  return {
    name: 'migate-precompress-assets',
    apply: 'build' as const,
    configResolved(config: { build: { outDir: string } }) {
      outDir = config.build.outDir;
    },
    async writeBundle() {
      for (const filePath of await listPrecompressableFiles(outDir)) {
        const input = await readFile(filePath);
        await writeFile(`${filePath}.gz`, gzipSync(input, { level: 9, mtime: 0 }));
        await writeFile(`${filePath}.br`, brotliCompressSync(input));
      }
    },
  };
}

function shouldPrecompress(fileName: string) {
  return /\.(js|css|html|svg|json)$/.test(fileName);
}

async function listPrecompressableFiles(dir: string): Promise<string[]> {
  const entries = await readdir(dir);
  const files: string[] = [];
  for (const entry of entries) {
    const filePath = join(dir, entry);
    const info = await stat(filePath);
    if (info.isDirectory()) {
      files.push(...await listPrecompressableFiles(filePath));
    } else if (shouldPrecompress(filePath) && !filePath.endsWith('.gz') && !filePath.endsWith('.br')) {
      files.push(filePath);
    }
  }
  return files;
}
