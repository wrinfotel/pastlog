import { defineConfig, type Plugin } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { fileURLToPath } from 'node:url';

// PASTLOG_UI_MOCK=1 swaps the generated Wails bindings for src/mock, so
// `vite dev` renders every view with fixture data in a plain browser
// (design iteration / screenshots). Never active in a real build.
function uiMock(): Plugin {
  const mockRoot = fileURLToPath(new URL('./src/mock', import.meta.url)).replace(/\\/g, '/');
  return {
    name: 'pastlog-ui-mock',
    enforce: 'pre',
    resolveId(source) {
      if (/wailsjs[\\/]+go[\\/]+app[\\/]+App$/.test(source)) return `${mockRoot}/app.ts`;
      if (/wailsjs[\\/]+go[\\/]+main[\\/]+guiBridge$/.test(source)) return `${mockRoot}/guiBridge.ts`;
      if (/wailsjs[\\/]+runtime[\\/]+runtime$/.test(source)) return `${mockRoot}/runtime.ts`;
      return null;
    },
  };
}
const mock = process.env.PASTLOG_UI_MOCK === '1';

export default defineConfig({
  plugins: [svelte(), ...(mock ? [uiMock()] : [])],
  build: { target: 'es2022', sourcemap: false, emptyOutDir: false },
  server: { port: 5178, strictPort: true },
  test: { environment: 'jsdom', include: ['src/**/*.test.ts'] },
});
