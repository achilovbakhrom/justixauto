import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import type { IncomingMessage, ServerResponse } from 'node:http';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

const root = fileURLToPath(new URL('.', import.meta.url));
// Local Go API (cmd/api); override with JUSTIX_API.
const api = process.env.JUSTIX_API ?? 'http://127.0.0.1:8080';

function isDocumentRequest(req: IncomingMessage): boolean {
  // Wildcards advertise acceptable bytes, not document navigation. Legacy
  // clients may omit Fetch Metadata; when present it must agree with HTML.
  const acceptsHtml = req.headers.accept?.split(',').some(value => {
    const [mediaType, ...parameters] = value.split(';').map(part => part.trim().toLowerCase());
    const quality = parameters.find(parameter => parameter.startsWith('q='))?.slice(2);
    return mediaType === 'text/html' && (quality === undefined || (Number(quality) > 0 && Number(quality) <= 1));
  });
  const destination = req.headers['sec-fetch-dest'];
  const mode = req.headers['sec-fetch-mode'];
  return Boolean(acceptsHtml
    && (destination === undefined || ['document', 'iframe', 'frame'].includes(String(destination)))
    && (mode === undefined || mode === 'navigate'));
}

/** Local development/preview only. Deployment routing is a separate task. */
function prefixGuard(req: IncomingMessage, res: ServerResponse, next: () => void, allowHtmlProxy = false) {
  const path = (req.url ?? '/').split('?')[0]!;
  // API calls go to the Go server through the dev/preview proxy.
  if (path.startsWith('/api/')) { next(); return; }
  // Preview's static middleware can serve an explicit HTML file before the
  // fallback. Apply the same intent rule there, including encoded filenames.
  let decodedPath = '';
  try { decodedPath = decodeURIComponent(path); } catch { /* Fallback rejects malformed paths. */ }
  // Vite's dev-only inline-module proxy returns JavaScript, not entry HTML.
  const htmlProxy = allowHtmlProxy && /\?html-proxy&index=\d+\.js$/.test(req.url ?? '');
  if (!htmlProxy && /\.html$/i.test(decodedPath) && (!['GET', 'HEAD'].includes(req.method ?? '') || !isDocumentRequest(req))) {
    res.writeHead(404, { 'Content-Type': 'text/plain; charset=utf-8' });
    res.end('Not found');
    return;
  }
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
    if (!['GET', 'HEAD'].includes(req.method ?? '') || !isDocumentRequest(req)
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
  root, base: '/admin/', appType: 'custom',
  server: { proxy: { '/api': api } }, preview: { proxy: { '/api': api } }, plugins: [react(), {
    name: 'admin-prefix-only-html',
    configureServer(server) {
      server.middlewares.use((req, res, next) => prefixGuard(req, res, next, true));
      return () => { server.middlewares.use(htmlFallback(async (url) =>
        server.transformIndexHtml('/index.html', await readFile(`${root}index.html`, 'utf8'), url))); };
    },
    configurePreviewServer(server) {
      server.middlewares.use(prefixGuard);
      return () => { server.middlewares.use(htmlFallback(() => readFile(`${root}dist/index.html`, 'utf8'))); };
    },
  }],
});
