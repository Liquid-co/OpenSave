import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte()],
  test: {
    // Existing presentation assertions are English, independent of the host OS.
    setupFiles: ['./src/test/setup.js']
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // The default warning is about download time on the web. This bundle is
    // embedded in the app and read from disk, so size matters far less.
    chunkSizeWarningLimit: 1500
  }
});
