import { fileURLToPath, URL } from 'node:url';
import { defineConfig, loadEnv } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

// This proxy belongs to the UI. Browser clients use /api on their own origin;
// authentication headers and WebSocket query tokens pass through unchanged.
export default defineConfig(({ mode }) => {
  const environment = loadEnv(mode, process.cwd(), 'HARNESS_');
  const target = environment.HARNESS_API_URL || 'http://127.0.0.1:18080';
  const proxy = {
    '/api': {
      target,
      changeOrigin: true,
      ws: true,
      rewrite: (path: string) => path.replace(/^\/api(?=\/|$)/, '') || '/',
    },
  };
  return {
    plugins: [tailwindcss(), svelte()],
    resolve: { alias: { $lib: fileURLToPath(new URL('./src/lib', import.meta.url)) } },
    server: { port: 5174, strictPort: true, proxy },
    preview: { port: 4173, strictPort: true, proxy },
  };
});
