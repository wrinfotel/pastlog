import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  build: { target: 'es2022', sourcemap: false },
  server: { port: 5178, strictPort: true },
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'] },
});
