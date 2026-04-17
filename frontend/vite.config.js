import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const apiTarget = env.VITE_API_PROXY || 'http://127.0.0.1:8088'
  return {
    plugins: [vue()],
    server: {
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
        },
      },
    },
    build: {
      // Split the vendor bundle so returning visitors only re-download the
      // app chunks that actually changed. element-plus is the big one;
      // isolating it lets the browser cache it across deployments.
      rollupOptions: {
        output: {
          manualChunks: (id) => {
            if (!id.includes('node_modules')) return
            if (id.includes('element-plus') || id.includes('@element-plus')) return 'vendor-element-plus'
            if (id.includes('@vue') || id.includes('/vue/') || id.includes('vue-router') || id.includes('pinia')) return 'vendor-vue'
            if (id.includes('axios')) return 'vendor-axios'
            return 'vendor'
          },
        },
      },
      chunkSizeWarningLimit: 800,
    },
  }
})
