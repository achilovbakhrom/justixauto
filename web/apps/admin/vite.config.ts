import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const root = fileURLToPath(new URL('.', import.meta.url));

/** Local development/preview only. Deployment routing is a separate task. */
function prefixGuard(req: IncomingMessage, res: ServerResponse, next: () => void) {
  const path = (req.url ?? '/').split('?')[0]!;
  if ((path !== '/admin' && !path.startsWith('/admin/')) || /^\/admin\/api(?:\/|$)/.test(path)) {
    res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('Not found');
    return;
  }
  next();
}

function htmlFallback(html: (url: string) => Promise<string>) {
  return async (req: IncomingMessage, res: ServerResponse) => {
    // Vite's base middleware has already stripped /admin at this point.
    const path = (req.url ?? '/').split('?')[0]!;
    const acceptsHtml = req.headers.accept?.includes('text/html') || req.headers.accept?.includes('*/*');
    if (!['GET', 'HEAD'].includes(req.method ?? '') || !acceptsHtml
      || (path !== '/index.html' && /[.%\\@]/.test(path))
      || /^\/(?:api|assets|src|node_modules)(?:\/|$)/.test(path)) {
      res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('Not found');
      return;
    }
    try {
      const body = await html(`/admin${req.url ?? '/'}`);
      res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8', 'Cache-Control': 'no-store' });
      res.end(req.method === 'HEAD' ? undefined : body);
    } catch {
      res.writeHead(500, { 'Content-Type': 'text/plain; charset=utf-8' });
      res.end('Application entry unavailable');
    }
  };
}

export default defineConfig({
  root, base: '/admin/', appType: 'custom', plugins: [react(), {
    name: 'admin-prefix-only-html',
    configureServer(server) {
      server.middlewares.use(prefixGuard);
      return () => { server.middlewares.use(htmlFallback(async (url) =>
        server.transformIndexHtml('/index.html', await readFile(`${root}index.html`, 'utf8'), url))); };
    },
    configurePreviewServer(server) {
      server.middlewares.use(prefixGuard);
      return () => { server.middlewares.use(htmlFallback(() => readFile(`${root}dist/index.html`, 'utf8'))); };
    },
  }],
});
