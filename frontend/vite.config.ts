import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { VitePWA } from 'vite-plugin-pwa'
import { fileURLToPath } from 'node:url'

export default defineConfig({
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      strategies: 'injectManifest',
      srcDir: 'src',
      filename: 'sw.ts',
      registerType: 'prompt',
      injectRegister: false,
      pwaAssets: { config: true, overrideManifestIcons: true },
      manifest: {
        name: 'HomeOps',
        short_name: 'HomeOps',
        description: 'Household sprints, chores, meals and budget',
        theme_color: '#1d4ed8',
        background_color: '#f6f7f9',
        display: 'standalone',
        orientation: 'portrait',
        start_url: '/',
        scope: '/',
        id: '/',
      },
      injectManifest: {
        globPatterns: ['**/*.{js,css,html,svg,png,ico,woff2}'], // the plugin adds the manifest itself
        // Only precache the Latin font subsets; others load on demand if ever needed.
        globIgnores: ['**/inter-{cyrillic,cyrillic-ext,greek,greek-ext,vietnamese}-*'],
      },
      devOptions: { enabled: true, type: 'module' },
    }),
  ],
  server: {
    port: 5173,
    proxy: { '/api': { target: 'http://localhost:8080', changeOrigin: false } },
  },
  build: { target: 'es2022', chunkSizeWarningLimit: 900 },
})
