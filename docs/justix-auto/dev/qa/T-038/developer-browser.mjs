import { chromium } from '@playwright/test';
import { createServer, preview } from 'vite';
import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';

const out = 'docs/justix-auto/dev/qa/T-038/developer-';
const evidence = { checks: [], comparisons: [], routes: [], hashes: {} };
const check = (name, actual, expected = true) => { assert.deepEqual(actual, expected, name); evidence.checks.push(name); };
const servers = []; let browser;
try {
  for (const [port, configFile] of [[4381, false], [4382, 'web/apps/financing/vite.config.ts']]) {
    const server = await createServer({ configFile, server: { host: '127.0.0.1', port, strictPort: true } });
    await server.listen(); servers.push(server);
  }
  const built = await preview({ configFile: 'web/apps/financing/vite.config.ts', preview: { host: '127.0.0.1', port: 4383, strictPort: true } });
  servers.push({ close: () => new Promise(resolve => built.httpServer.close(resolve)) });
  browser = await chromium.launch();
  const shape = page => page.evaluate(() => {
    const rect = e => { const r = e.getBoundingClientRect(), s = getComputedStyle(e); return { x:r.x, y:r.y, width:r.width, height:r.height, font:s.font, smoothing:s.webkitFontSmoothing, color:s.color, background:s.backgroundColor, border:s.border, radius:s.borderRadius }; };
    const side = document.querySelector('aside'), header = document.querySelector('header');
    return { side:rect(side), brand:rect(side.firstElementChild), context:rect(side.children[1]), header:rect(header), account:rect(header.querySelector('label')), select:rect(header.querySelector('select')), nav:[...side.querySelectorAll('nav button')].map(rect), external:rect(side.querySelector('div:last-child > a, div:last-child > button')), notification:rect(header.querySelector('button')), overflow:document.documentElement.scrollWidth > innerWidth };
  });
  for (const width of [1440, 1180, 600]) {
    const contexts = [], pages = [];
    for (const url of ['http://127.0.0.1:4180/finance/', 'http://127.0.0.1:4381/docs/justix-auto/dev/qa/T-038/developer-fixture.html']) {
      const context = await browser.newContext({ viewport:{width,height:1000}, locale:'ru-RU', timezoneId:'Asia/Tashkent', reducedMotion:'reduce' }); contexts.push(context);
      const page = await context.newPage(); pages.push(page); await page.goto(url); await page.locator('aside').waitFor(); await page.evaluate(() => document.fonts.ready);
    }
    const [reference, candidate] = pages;
    await reference.screenshot({ path:`${out}${width}-reference-raw.png`, fullPage:true });
    await candidate.screenshot({ path:`${out}${width}-candidate-raw.png`, fullPage:true });
    // Explicit zero-feature projection: retain raw evidence, hide Overview and
    // demo banner, remove its active navigation state. No Dashboard parity claim.
    await reference.locator('aside nav button').evaluateAll(buttons => buttons.forEach(button => button.classList.remove('active')));
    await reference.addStyleTag({ content:'.page {visibility:hidden}' });
    await reference.screenshot({ path:`${out}${width}-reference-projected.png` });
    await candidate.screenshot({ path:`${out}${width}-candidate-viewport.png` });
    const a = await shape(reference), b = await shape(candidate);
    evidence.comparisons.push({ width, reference:a, candidate:b });
    check(`five disabled navigation controls ${width}`, await candidate.locator('nav button:disabled').count(), 5);
    check(`no feature links ${width}`, await candidate.locator('a').count(), 0);
    check(`empty content ${width}`, await candidate.locator('main').innerText(), '');
    await candidate.keyboard.press('Tab'); check(`disabled controls skipped ${width}`, await candidate.evaluate(() => document.activeElement.tagName), 'BODY');
    await candidate.evaluate(() => window.fixture.render(false)); await candidate.locator('main').waitFor({ state:'detached' });
    check(`session boundary removes all display ${width}`, await candidate.locator('body').innerText(), '');
    for (const context of contexts) await context.close();
  }
  for (const port of [4382,4383]) {
    for (const path of ['/', '/admin/', '/insurance/', '/api/session', '/finance/api/session', '/finance/assets/missing.js', '/finance/missing.css', '/finance/assets/missing', '/finance/%61pi', '/finance/src/missing.ts']) {
      const response = await fetch(`http://127.0.0.1:${port}${path}`, { headers:{ Accept:'text/html' } });
      const body = await response.text(); check(`route rejected ${port}${path}`, response.status === 404 && !response.headers.get('content-type')?.includes('text/html') && !body.includes('finance-root'));
      evidence.routes.push({ port, path, status:response.status, type:response.headers.get('content-type') });
    }
    for (const method of ['POST','PUT','DELETE','PATCH']) { const response = await fetch(`http://127.0.0.1:${port}/finance/applications/one`, {method, headers:{Accept:'text/html'}}); check(`method rejected ${port}${method}`, response.status,404); }
    const json = await fetch(`http://127.0.0.1:${port}/finance/applications/one`, {headers:{Accept:'application/json'}}); check(`non-document rejected ${port}`,json.status,404);
    const page = await browser.newPage(); const errors = []; page.on('pageerror',error=>errors.push(error.message));
    page.on('response', response => { if (response.request().resourceType() === 'script' && response.status() >= 400) errors.push(response.url()); });
    for (const path of ['/finance/', '/finance/applications/one', '/finance/index.html']) {
      const response = await page.goto(`http://127.0.0.1:${port}${path}`); await page.waitForLoadState('networkidle');
      check(`closed default entry ${port}${path}`,{status:response.status(),content:await page.locator('#finance-root').innerHTML()},{status:200,content:''});
    }
    check(`no entry runtime errors ${port}`,errors,[]); await page.close();
  }
  const page = await browser.newPage();
  await page.addInitScript(() => { window.calls = []; window.fetch = () => { window.calls.push('fetch'); throw Error('Unexpected fetch'); }; XMLHttpRequest.prototype.open = function () { window.calls.push('xhr'); throw Error('Unexpected xhr'); }; Storage.prototype.setItem = function () { window.calls.push('storage'); throw Error('Unexpected storage'); }; });
  await page.goto('http://127.0.0.1:4381/docs/justix-auto/dev/qa/T-038/developer-fixture.html?long=1'); await page.locator('aside').waitFor();
  check('no app network or storage writes', await page.evaluate(() => window.calls), []);
  check('no persistent browser data', await page.evaluate(() => [localStorage.length,sessionStorage.length,document.cookie]), [0,0,'']);
  await page.screenshot({path:`${out}long-label.png`}); await page.close();
  for (const file of ['finance/index.html','finance/workspace.js','finance/workspace.css','common.css','styles.css']) evidence.hashes[file] = createHash('sha256').update(await readFile(`docs/justix-auto/mocks/${file}`)).digest('hex');
  for (const { width, reference, candidate } of evidence.comparisons) check(`shell metrics ${width}`,candidate,reference);
  await writeFile(`${out}observations.json`,JSON.stringify(evidence,null,2)+'\n');
  console.log(`${evidence.checks.length} browser checks passed`);
} finally { if (browser) await browser.close(); for (const server of servers.reverse()) await server.close(); }
