import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// 说明：
// - 构建产物输出到 web/dist
// - 静态资源目录固定为 dist/static，方便 Go 控制器用 r.Static("/static", ...) 直接托管
// - 开发模式下，/api、/metrics、/healthz 会代理到 8080 端口的控制器（与 listen_addr 默认值一致）
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: "dist",
    assetsDir: "static",
    // 生产环境不发布源码映射，显著减小静态目录体积，也避免暴露源码结构。
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("node_modules/element-plus") || id.includes("node_modules/@element-plus")) return "element-plus";
          if (id.includes("node_modules/vue") || id.includes("node_modules/@vue") || id.includes("node_modules/vue-router")) return "vue-vendor";
          if (id.includes("node_modules/dayjs")) return "dayjs";
        },
      },
    },
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/metrics": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/healthz": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
});
