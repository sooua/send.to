// @ts-check
import { defineConfig } from 'astro/config';

import react from '@astrojs/react';
import tailwindcss from '@tailwindcss/vite';

// Go 后端地址（开发模式下用于代理 API 请求）
const GO_BACKEND = 'http://localhost:18080';

/**
 * Vite 插件：将非 GET 请求（PUT/POST/DELETE）代理到 Go 后端
 * 开发模式下，Astro 处理页面路由，Go 后端处理文件 API
 */
function apiProxy() {
  return {
    name: 'api-proxy',
    configureServer(server) {
      server.middlewares.use(async (req, res, next) => {
        // 只代理非 GET/HEAD 请求（上传、删除等 API 操作）
        if (req.method && !['GET', 'HEAD'].includes(req.method)) {
          try {
            const url = new URL(req.url || '/', GO_BACKEND);
            const headers = { ...req.headers, host: url.host };

            const proxyReq = await import('node:http').then(http =>
              http.request(url, {
                method: req.method,
                headers,
              }, (proxyRes) => {
                res.writeHead(proxyRes.statusCode || 502, proxyRes.headers);
                proxyRes.pipe(res);
              })
            );

            proxyReq.on('error', () => {
              res.writeHead(502, { 'Content-Type': 'text/plain' });
              res.end('Go backend not running. Start it with:\n  go run main.go --provider local --basedir /tmp/send.to --listener :18080');
            });

            req.pipe(proxyReq);
          } catch {
            res.writeHead(502, { 'Content-Type': 'text/plain' });
            res.end('Proxy error');
          }
          return;
        }
        next();
      });
    }
  };
}

// https://astro.build/config
export default defineConfig({
  integrations: [react()],

  vite: {
    plugins: [tailwindcss(), apiProxy()]
  }
});
