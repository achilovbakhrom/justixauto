import { chromium } from '@playwright/test';
import { createServer, preview } from 'vite';
import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { request } from 'node:http';

// Node fetch supplies Sec-Fetch-Mode: cors even when a caller supplies another
// mode. Raw HTTP is required for exact navigation/legacy metadata probes.
const probe = (url, options = {}) => new Promise((resolve, reject) => {
  const req = request(url, options, res => {
    let body = ''; res.setEncoding('utf8'); res.on('data', chunk => { body += chunk; });
    res.on('end', () => resolve({ status:res.statusCode, headers:{get:name => res.headers[name.toLowerCase()] ?? null}, text:async () => body }));
  }); req.on('error', reject); req.end();
});

const out = 'docs/justix-auto/dev/qa/T-039/developer-';
const evidence = { checks: [], comparisons: [], routes: [], hashes: {} };
const check = (name, actual, expected = true) => { assert.deepEqual(actual, expected, name); evidence.checks.push(name); };
const servers = []; let browser;
try {
  for (const [port, configFile] of [[4391, false], [4392, 'web/apps/insurance/vite.config.ts']]) {
    const server = await createServer({ configFile, server: { host: '127.0.0.1', port, strictPort: true } });
    await server.listen(); servers.push(server);
  }
  const built = await preview({ configFile: 'web/apps/insurance/vite.config.ts', preview: { host: '127.0.0.1', port: 4393, strictPort: true } });
  servers.push({ close: () => new Promise(resolve => built.httpServer.close(resolve)) });
  browser = await chromium.launch();
  const shape = page => page.evaluate(() => {
    const rect = e => { const r = e.getBoundingClientRect(), s = getComputedStyle(e); return { x:r.x, y:r.y, width:r.width, height:r.height, font:s.font, smoothing:s.webkitFontSmoothing, color:s.color, background:s.backgroundColor, border:s.border, radius:s.borderRadius }; };
    const side = document.querySelector('aside'), header = document.querySelector('header');
    return { side:rect(side), brand:rect(side.firstElementChild), header:rect(header), account:rect(header.querySelector('label')), select:rect(header.querySelector('select')), nav:[...side.querySelectorAll('nav button')].map(rect), overflow:document.documentElement.scrollWidth > innerWidth };
  });
  for (const width of [1440, 900, 600]) {
    const contexts = [], pages = [];
    for (const url of ['http://127.0.0.1:4180/insurance/', 'http://127.0.0.1:4391/docs/justix-auto/dev/qa/T-039/developer-fixture.html']) {
      const context = await browser.newContext({ viewport:{width,height:1000}, locale:'ru-RU', timezoneId:'Asia/Tashkent', reducedMotion:'reduce' }); contexts.push(context);
      const page = await context.newPage(); pages.push(page); await page.goto(url); await page.locator('aside').waitFor(); await page.evaluate(() => document.fonts.ready);
    }
    const [reference, candidate] = pages;
    await reference.screenshot({ path:`${out}${width}-reference-raw.png`, fullPage:true });
    await candidate.screenshot({ path:`${out}${width}-candidate-raw.png`, fullPage:true });
    // Explicit zero-feature projection: retain raw evidence, hide Overview and
    // demo banner, remove its active navigation state. No Dashboard parity claim.
    await reference.locator('aside nav button').evaluateAll(buttons => buttons.forEach(button => button.classList.remove('active')));
    await reference.addStyleTag({ content:'.page {display:none}' });
    await reference.screenshot({ path:`${out}${width}-reference-projected.png` });
    await candidate.screenshot({ path:`${out}${width}-candidate-viewport.png` });
    const a = await shape(reference), b = await shape(candidate);
    evidence.comparisons.push({ width, reference:a, candidate:b });
    check(`three disabled navigation controls ${width}`, await candidate.locator('nav button:disabled').count(), 3);
    check(`no feature links ${width}`, await candidate.locator('a').count(), 0);
    check(`empty content ${width}`, await candidate.locator('main > section').innerText(), '');
    await candidate.keyboard.press('Tab'); check(`disabled controls skipped ${width}`, await candidate.evaluate(() => document.activeElement.tagName), 'BODY');
    await candidate.evaluate(() => window.fixture.render(false)); await candidate.locator('main').waitFor({ state:'detached' });
    check(`session boundary removes all display ${width}`, await candidate.locator('body').innerText(), '');
    for (const context of contexts) await context.close();
  }
  for (const port of [4392,4393]) {
    for (const path of ['/insurance/applications/missing-resource', '/insurance/index.html', '/insurance/index%2ehtml']) {
      for (const headers of [
        { Accept:'*/*' }, { Accept:'application/json' }, { Accept:'text/html;q=0' },
        { Accept:'text/html', 'Sec-Fetch-Dest':'empty', 'Sec-Fetch-Mode':'cors' },
        { Accept:'text/html', 'Sec-Fetch-Dest':'script', 'Sec-Fetch-Mode':'no-cors' },
      ]) {
        const response = await probe(`http://127.0.0.1:${port}${path}`, {headers});
        check(`non-document intent rejected ${port}${path}${JSON.stringify(headers)}`, response.status === 404 && !response.headers.get('content-type')?.includes('text/html'));
        evidence.routes.push({port,path,headers,status:response.status,type:response.headers.get('content-type')});
      }
    }
    const head = await probe(`http://127.0.0.1:${port}/insurance/deep/route`, {method:'HEAD', headers:{Accept:'text/html;q=0.8', 'Sec-Fetch-Mode':'navigate', 'Sec-Fetch-Dest':'document'}});
    check(`positive HTML HEAD accepted ${port}`, [head.status,await head.text()], [200,'']);
    for (const path of ['/', '/admin/', '/finance/', '/api/session', '/insurance/api/session', '/insurance/assets/missing.js', '/insurance/missing.css', '/insurance/assets/missing', '/insurance/%61pi', '/insurance/src/missing.ts']) {
      const response = await probe(`http://127.0.0.1:${port}${path}`, { headers:{ Accept:'text/html' } });
      const body = await response.text(); check(`route rejected ${port}${path}`, response.status === 404 && !response.headers.get('content-type')?.includes('text/html') && !body.includes('insurance-root'));
      evidence.routes.push({ port, path, status:response.status, type:response.headers.get('content-type') });
    }
    for (const method of ['POST','PUT','DELETE','PATCH']) { const response = await probe(`http://127.0.0.1:${port}/insurance/applications/one`, {method, headers:{Accept:'text/html'}}); check(`method rejected ${port}${method}`, response.status,404); }
    const json = await probe(`http://127.0.0.1:${port}/insurance/applications/one`, {headers:{Accept:'application/json'}}); check(`non-document rejected ${port}`,json.status,404);
    const page = await browser.newPage(); const errors = []; page.on('pageerror',error=>errors.push(error.message));
    page.on('response', response => { if (response.request().resourceType() === 'script' && response.status() >= 400) errors.push(response.url()); });
    for (const path of ['/insurance/', '/insurance/applications/one', '/insurance/index.html']) {
      const response = await page.goto(`http://127.0.0.1:${port}${path}`); await page.waitForLoadState('networkidle');
      check(`closed default entry ${port}${path}`,{status:response.status(),content:await page.locator('#insurance-root').innerHTML()},{status:200,content:''});
    }
    for (const path of ['/insurance/missing-resource', '/insurance/index.html', '/insurance/index%2ehtml']) {
      const response = await page.evaluate(async path => { const r = await fetch(path); return {status:r.status,type:r.headers.get('content-type'),body:await r.text()}; }, path);
      check(`actual browser wildcard fetch rejected ${port}${path}`, response.status === 404 && !response.type?.includes('text/html') && !response.body.includes('insurance-root'));
    }
    check(`no entry runtime errors ${port}`,errors,[]); await page.close();
  }
  const page = await browser.newPage();
  await page.addInitScript(() => { window.calls = []; window.fetch = () => { window.calls.push('fetch'); throw Error('Unexpected fetch'); }; XMLHttpRequest.prototype.open = function () { window.calls.push('xhr'); throw Error('Unexpected xhr'); }; Storage.prototype.setItem = function () { window.calls.push('storage'); throw Error('Unexpected storage'); }; });
  await page.goto('http://127.0.0.1:4391/docs/justix-auto/dev/qa/T-039/developer-fixture.html?long=1'); await page.locator('aside').waitFor();
  check('no app network or storage writes', await page.evaluate(() => window.calls), []);
  check('no persistent browser data', await page.evaluate(() => [localStorage.length,sessionStorage.length,document.cookie]), [0,0,'']);
  await page.screenshot({path:`${out}long-label.png`}); await page.close();
  for (const file of ['insurance/index.html','insurance/workspace.js','insurance.css','common.css','styles.css']) evidence.hashes[file] = createHash('sha256').update(await readFile(`docs/justix-auto/mocks/${file}`)).digest('hex');
  for (const { width, reference, candidate } of evidence.comparisons) check(`shell metrics ${width}`,candidate,reference);
  await writeFile(`${out}observations.json`,JSON.stringify(evidence,null,2)+'\n');
  console.log(`${evidence.checks.length} browser checks passed`);
} finally { if (browser) await browser.close(); for (const server of servers.reverse()) await server.close(); }
