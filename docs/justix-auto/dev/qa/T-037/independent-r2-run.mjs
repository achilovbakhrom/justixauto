import {chromium} from '@playwright/test';
import {createServer, preview} from 'vite';
import assert from 'node:assert/strict';
import {readFile, writeFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
const out = 'docs/justix-auto/dev/qa/T-037/independent-r2-';
const expected = 'd984616c9d63760540f0127c6145c4aff87fa81f';
assert.equal(execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),expected);
const observations = {sha: expected, checks: [], routes: [], metrics: [], hashes: {}};
const check = (name, actual, expected = true) => {assert.deepEqual(actual, expected, name); observations.checks.push(name);};
const servers = []; let browser;
try {
  for (const [port, configFile] of [[4236, false],[4237,'web/apps/realization/vite.config.ts']]) {
    const server = await createServer({configFile, server: {host:'127.0.0.1', port, strictPort:true}}); await server.listen(); servers.push(server);
  }
  const built = await preview({configFile:'web/apps/realization/vite.config.ts',preview:{host:'127.0.0.1',port:4238,strictPort:true}});
  servers.push({close:()=>new Promise(resolve=>built.httpServer.close(resolve))});
  browser = await chromium.launch();
  for (const file of ['app.js','realization.js','common.css','styles.css','realization.css']) observations.hashes[file] = createHash('sha256').update(await readFile(`docs/justix-auto/mocks/${file}`)).digest('hex');
  const shape = page => page.evaluate(() => {
    const rect = e => {const r=e.getBoundingClientRect(); const c=getComputedStyle(e); return {x:r.x,y:r.y,width:r.width,height:r.height,font:c.font,fontSmoothing:c.webkitFontSmoothing,color:c.color,background:c.backgroundColor,border:c.border,radius:c.borderRadius};};
    const side=document.querySelector('.dealer-shell aside'), header=document.querySelector('.dealer-shell header');
    return {side:rect(side), brand:rect(side.firstElementChild), header:rect(header), controls:[...header.children].filter(e=>e.tagName!=='STYLE').map(rect), nav:[...side.querySelectorAll('nav button')].map(e=>({label:e.textContent,rect:rect(e),icon:rect(e.querySelector('svg')),text:rect(e.querySelector('span'))})), external:[...side.querySelectorAll(':scope > div:last-child > a,:scope > div:last-child > button')].map(rect), overflow:document.documentElement.scrollWidth>innerWidth};
  });
  for (const width of [1440,1180,600]) {
    const contexts=[]; const pages=[];
    for (const url of ['http://127.0.0.1:4180/','http://127.0.0.1:4236/docs/justix-auto/dev/qa/T-037/independent-r2-fixture.html']) {
      const context=await browser.newContext({viewport:{width,height:1000},locale:'ru-RU',timezoneId:'Asia/Tashkent',colorScheme:'light',reducedMotion:'reduce'}); contexts.push(context);
      const page=await context.newPage(); pages.push(page); await page.clock.setFixedTime(new Date('2026-09-15T12:00:00Z')); await page.goto(url); await page.locator('.dealer-shell aside').waitFor(); await page.evaluate(()=>document.fonts.ready);
    }
    const [ref,app]=pages;
    await ref.screenshot({path:`${out}${width}-reference-raw.png`,fullPage:true});
    await app.screenshot({path:`${out}${width}-candidate-raw.png`,fullPage:true});
    await ref.locator('aside nav button').evaluateAll(buttons=>buttons.forEach(b=>{b.classList.remove('active');b.disabled=true;}));
    await ref.addStyleTag({content:'.page {visibility:hidden}'});
    await ref.screenshot({path:`${out}${width}-reference-projected.png`});
    await app.screenshot({path:`${out}${width}-candidate-viewport.png`});
    const reference=await shape(ref), candidate=await shape(app);
    check(`actual source antialiased ${width}`,candidate.side.fontSmoothing,"antialiased");
    // Reference anchors are replaced by disabled Buttons. Border box and type
    // are compared; browser anchor background is not an application state.
    for (const metrics of [reference,candidate]) for(const item of metrics.external) delete item.background;
    check(`shell geometry and style ${width}`,candidate,reference); observations.metrics.push({width,reference,candidate});
    check(`eleven disabled native nav controls ${width}`,await app.locator('aside nav button:disabled').count(),11);
    check(`zero active links ${width}`,await app.locator('a').count(),0);
    check(`empty feature content ${width}`,await app.locator('main').innerText(),'');
    await app.keyboard.press('Tab'); check(`disabled controls excluded from tab ${width}`,await app.evaluate(()=>document.activeElement.tagName),'BODY');
    for(const ctx of contexts) await ctx.close();
  }
  const ctx=await browser.newContext(); const page=await ctx.newPage(); const errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.addInitScript(()=>{window.audit=[]; for(const name of ['fetch']) window[name]=()=>{window.audit.push(name);throw Error(name);}; XMLHttpRequest.prototype.open=function(){window.audit.push('xhr');throw Error('xhr');}; Storage.prototype.setItem=function(){window.audit.push('storage');throw Error('storage');};});
  await page.goto('http://127.0.0.1:4236/docs/justix-auto/dev/qa/T-037/independent-r2-fixture.html'); await page.locator('main').waitFor();
  await page.evaluate(()=>window.qa.clients[0].setQueryData(['synthetic-private'],{company:'qa-one'}));
  await page.evaluate(()=>window.qa.gate(false)); await page.locator('main').waitFor({state:'detached'});
  check('gate removes identity from document',await page.locator('body').innerText(),'');
  await page.evaluate(()=>window.qa.gate(true)); await page.locator('main').waitFor();
  check('same mount keeps one query context',await page.evaluate(()=>window.qa.clients.length),1);
  await page.evaluate(()=>window.qa.remount()); await page.waitForFunction(()=>window.qa.clients.length===2);
  check('new mount cannot read old private query',await page.evaluate(()=>window.qa.clients[1].getQueryData(['synthetic-private'])===undefined));
  for(const path of ['/admin','/admin/users','/finance','/finance/apps','/insurance','/insurance/apps','/api','/api/session']) {
    await page.evaluate(path=>window.qa.navigate(path),path); await page.locator('main').waitFor({state:'detached'}); check(`client rejects ${path}`,await page.locator('main').count(),0);
  }
  await page.evaluate(()=>window.qa.navigate('/vehicles/synthetic')); await page.locator('main').waitFor();
  await page.evaluate(()=>window.qa.display(null)); await page.waitForFunction(()=>!document.body.innerText.includes('Tashkent Motors'));
  check('missing display context invents no identity',await page.locator('body').innerText().then(t=>!/Tashkent|Все филиалы|RU/.test(t)));
  check('no runtime API or persistence calls',await page.evaluate(()=>window.audit),[]);
  check('no browser cache persistence',await page.evaluate(()=>[localStorage.length,sessionStorage.length]),[0,0]);
  check('no cookies',await ctx.cookies(),[]); check('no uncaught browser errors',errors,[]);
  await ctx.close();
  for(const port of [4237,4238]) {
    for(const path of ['/admin','/admin/','/admin/deep','/finance','/finance/','/insurance','/insurance/','/api','/api/','/api/session','/assets/no-file','/assets/missing.js','/missing.css','/src/missing.ts','/node_modules/missing','/%61dmin/','/admin%2fusers']) {
      const r=await fetch(`http://127.0.0.1:${port}${path}`,{headers:{Accept:'text/html'}}),body=await r.text();
      check(`non-HTML rejected route ${port}${path}`,r.status===404&&!r.headers.get('content-type')?.includes('text/html')&&!body.includes('realization-root'));
      observations.routes.push({port,path,status:r.status,type:r.headers.get('content-type')});
    }
    for(const method of ['POST','PUT','DELETE','PATCH']) {const r=await fetch(`http://127.0.0.1:${port}/vehicles/deep`,{method,headers:{Accept:'text/html'}});check(`${method} never gets fallback ${port}`,r.status,404);}
    const json=await fetch(`http://127.0.0.1:${port}/vehicles/deep`,{headers:{Accept:'application/json'}});check(`JSON request never gets fallback ${port}`,json.status,404);
    const head=await fetch(`http://127.0.0.1:${port}/vehicles/deep`,{method:'HEAD',headers:{Accept:'text/html'}});check(`HEAD document ${port}`,{status:head.status,body:await head.text()},{status:200,body:''});
    const c=await browser.newContext();const p=await c.newPage();const loadErrors=[];p.on('pageerror',e=>loadErrors.push(e.message));p.on('response',r=>{if(r.request().resourceType()==='script'&&r.status()>=400)loadErrors.push(r.url());});
    for(const path of ['/','/vehicles/deep?tab=details','/index.html']) {const r=await p.goto(`http://127.0.0.1:${port}${path}`);await p.waitForLoadState('networkidle');check(`closed actual entry ${port}${path}`,{status:r.status(),text:await p.locator('#realization-root').innerHTML()},{status:200,text:''});await p.reload();await p.waitForLoadState('networkidle');check(`closed reload ${port}${path}`,await p.locator('#realization-root').innerHTML(),'');}
    check(`entry scripts load without errors ${port}`,loadErrors,[]);await c.close();
  }
  const baseline=JSON.parse(execFileSync('git',['show','60abf5622597aa1af77925fd7968251e0cb81be4:package-lock.json'],{encoding:'utf8'}));
  const current=JSON.parse(await readFile('package-lock.json','utf8'));
  check('all existing lock entries unchanged',Object.entries(baseline.packages).every(([k,v])=>JSON.stringify(v)===JSON.stringify(current.packages[k])));
  check('only two workspace lock entries added',Object.keys(current.packages).filter(k=>!Object.hasOwn(baseline.packages,k)).sort(),['node_modules/@justixauto/realization','web/apps/realization']);
  for(const file of execFileSync('git',['ls-files','web/apps/realization'],{encoding:'utf8'}).trim().split('\n')) observations.hashes[file]=createHash('sha256').update(await readFile(file)).digest('hex');
  await writeFile(`${out}observations.json`,JSON.stringify(observations,null,2)+'\n');
  console.log(`PASS ${observations.checks.length} independent assertions; ${observations.routes.length} route observations; three visual comparisons.`);
} finally {if(browser) await browser.close(); for(const server of servers.reverse()) await server.close();}
