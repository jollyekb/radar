import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

export default defineConfig({
  plugins: [tailwindcss(), react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@skyhook-io/k8s-ui': path.resolve(__dirname, '../packages/k8s-ui/src'),
    },
  },
  server: {
    port: 9273,
    proxy: {
      '/api': {
        target: `http://localhost:${process.env.RADAR_PORT || '9280'}`,
        changeOrigin: true,
        ws: true, // WebSocket/SSE support
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Split large vendor chunk to avoid Vite build-import-analysis parse failures
    rolldownOptions: {
      output: {
        manualChunks(id: string) {
          if (id.includes('node_modules/react-dom/') || id.includes('node_modules/react/') || id.includes('node_modules/react-router')) {
            return 'vendor'
          }
          if (id.includes('node_modules/@xyflow/') || id.includes('node_modules/@monaco-editor/') || id.includes('node_modules/@xterm/')) {
            return 'ui'
          }
        },
      },
    },
  },
  // Handle client-side routing - serve index.html for all routes
  appType: 'spa',
})
