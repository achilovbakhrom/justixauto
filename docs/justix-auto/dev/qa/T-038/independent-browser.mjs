import assert from 'node:assert/strict';
import { chromium } from '@playwright/test';
import { createServer, preview } from 'vite';
import { readFile, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
const prefix='docs/justix-auto/dev/qa/T-038/independent-';
const evidence={sha:execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),checks:[],failures:[],routes:[],comparisons:[],hashes:{}};
assert.equal(evidence.sha,'c1b0b50eed76902bf024fc6c5d9c5e4f032f642b');
function check(name,actual,expected=true) { try { assert.deepEqual(actual,expected); evidence.checks.push(name); } catch { evidence.failures.push({name,actual,expected}); } }
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const oldLock=JSON.parse(execFileSync('git',['show','3d8c5a26c3890eaa358a52281db055c1faf7e2be:package-lock.json'],{encoding:'utf8'}));
const lock=JSON.parse(await readFile('package-lock.json','utf8'));
check('all existing lock package records byte-equivalent JSON',Object.entries(oldLock.packages).every(([name,value])=>JSON.stringify(value)===JSON.stringify(lock.packages[name])));
evidence.lockAdded=Object.keys(lock.packages).filter(name=>!(name in oldLock.packages));
check('exact two workspace lock additions',evidence.lockAdded.sort(),['node_modules/@justixauto/financing','web/apps/financing']);
const servers=[]; let browser;
try {
  for (const [port,configFile] of [[4481,false],[4482,'web/apps/financing/vite.config.ts']]) { const server=await createServer({configFile,server:{host:'127.0.0.1',port,strictPort:true}}); await server.listen(); servers.push(server); }
  const built=await preview({configFile:'web/apps/financing/vite.config.ts',preview:{host:'127.0.0.1',port:4483,strictPort:true}});
  servers.push({close:()=>new Promise(resolve=>built.httpServer.close(resolve))});
  browser=await chromium.launch();
  async function metrics(page) { return page.evaluate(()=>{
    const measure=e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return {rect:[r.x,r.y,r.width,r.height],font:s.font,smoothing:s.webkitFontSmoothing,color:s.color,background:s.backgroundColor,border:s.border,radius:s.borderRadius,overflow:s.overflow};};
    const side=document.querySelector('aside'),head=document.querySelector('header');
    const seller=[...side.querySelectorAll('a, button')].find(node=>node.textContent.includes('Кабинет продавца'));
    return {side:measure(side),brand:measure(side.children[0]),organization:measure(side.children[1]),header:measure(head),select:measure(head.querySelector('select')),navigation:[...side.querySelectorAll('nav button')].map(measure),seller:measure(seller),notification:measure(head.querySelector('button')),scrollWidth:document.documentElement.scrollWidth,bodyFont:getComputedStyle(document.body).font};
  }); }
  for (const width of [1440,1180,600]) {
    const contexts=[];const pages=[];
    for (const url of ['http://127.0.0.1:4180/finance/','http://127.0.0.1:4481/docs/justix-auto/dev/qa/T-038/independent-fixture.html']) {
      const context=await browser.newContext({viewport:{width,height:1000},locale:'ru-RU',timezoneId:'Asia/Tashkent',reducedMotion:'reduce'});contexts.push(context);
      const page=await context.newPage(); pages.push(page); await page.goto(url); await page.locator('aside').waitFor(); await page.evaluate(()=>document.fonts.ready);
    }
    const [ref,app]=pages;
    await ref.screenshot({path:`${prefix}${width}-reference-raw.png`,fullPage:true});
    await app.screenshot({path:`${prefix}${width}-candidate-raw.png`,fullPage:true});
    await ref.locator('nav button.active').evaluateAll(nodes=>nodes.forEach(node=>node.classList.remove('active')));
    await ref.addStyleTag({content:'.page { visibility:hidden; }'});
    const refPng=await ref.screenshot({path:`${prefix}${width}-reference-projected.png`});
    const appPng=await app.screenshot({path:`${prefix}${width}-candidate-viewport.png`});
    check(`projected viewport PNG exact equality ${width}`,sha(appPng),sha(refPng));
    const reference=await metrics(ref),candidate=await metrics(app);evidence.comparisons.push({width,reference,candidate});check(`geometry and computed styles ${width}`,candidate,reference);
    check(`sidebar breakpoint ${width}`,candidate.side.rect[2],width>1180?232:76);
    check(`catalog order ${width}`,await app.locator('nav button').allTextContents(),['Обзор','Заявки','Программы','Партнёры','Настройки']);
    check(`all navigation native disabled ${width}`,await app.locator('nav button:disabled').count(),5);
    check(`no link or active item ${width}`,await app.locator('a, [aria-current], nav button.active').count(),0);
    check(`no invented page ${width}`,await app.locator('main').innerText(),'');
    const before=app.url(); await app.locator('nav button').first().dispatchEvent('click'); await app.keyboard.press('Tab');await app.keyboard.press('Enter');check(`disabled keyboard/click cannot navigate ${width}`,app.url(),before);check(`disabled controls skipped by Tab ${width}`,await app.evaluate(()=>document.activeElement.tagName),'BODY');
    await app.evaluate(()=>window.qa.render(false));await app.locator('main').waitFor({state:'detached'});check(`revocation removes shell and identity ${width}`,await app.locator('body').innerText(),'');
    await app.evaluate(()=>window.qa.render(true,null));await app.locator('main').waitFor();check(`no identity fallback ${width}`,await app.locator('select, header label, aside strong').count(),0);
    await app.evaluate(()=>window.qa.render());await app.locator('select').waitFor();check(`re-admission restores explicit display ${width}`,await app.locator('aside strong').innerText(),'Демо-банк');
    await app.evaluate(()=>window.qa.clients[0].setQueryData(['qa-private'],'synthetic-private'));await app.evaluate(()=>window.qa.remount());await app.waitForFunction(()=>window.qa.clients.length===2);
    check(`mount query client isolation ${width}`,await app.evaluate(()=>window.qa.clients[0]!==window.qa.clients[1]&&window.qa.clients[1].getQueryData(['qa-private'])===undefined));
    for (const context of contexts) await context.close();
  }
  for (const port of [4482,4483]) {
    const cases=[
      ...['/','/admin/','/insurance/','/finances/','/api/session','/finance/api/session','/finance/assets/missing','/finance/missing.js','/finance/src/missing','/finance/node_modules/missing','/finance/%61pi','/finance/missing.svg?x=1'].map(path=>({path,headers:{Accept:'text/html'},want:404})),
      ...['GET','HEAD'].map(method=>({path:'/finance/applications/synthetic',method,headers:{Accept:'text/html'},want:200})),
      ...['POST','PUT','PATCH','DELETE'].map(method=>({path:'/finance/applications/synthetic',method,headers:{Accept:'text/html'},want:404})),
      {path:'/finance/applications/synthetic',headers:{Accept:'application/json'},want:404},
      {path:'/finance/missing-resource',headers:{Accept:'*/*','Sec-Fetch-Dest':'script','Sec-Fetch-Mode':'no-cors'},want:404},
      {path:'/finance/missing-resource',headers:{Accept:'*/*','Sec-Fetch-Dest':'empty','Sec-Fetch-Mode':'cors'},want:404},
    ];
    for (const item of cases) {const response=await fetch(`http://127.0.0.1:${port}${item.path}`,item);const body=await response.text();const actual={port,...item,status:response.status,type:response.headers.get('content-type'),html:body.includes('finance-root')};evidence.routes.push(actual);check(`route ${port} ${item.method??'GET'} ${item.path} ${JSON.stringify(item.headers)}`,response.status,item.want);if(item.want===404)check(`404 no HTML ${evidence.routes.length}`,!actual.type?.includes('text/html')&&!actual.html);}
    const page=await browser.newPage();const errors=[];page.on('pageerror',error=>errors.push(error.message));
    for (const path of ['/finance/','/finance/applications/synthetic','/finance/index.html']) {const response=await page.goto(`http://127.0.0.1:${port}${path}`);await page.waitForLoadState('networkidle');check(`actual entry closed ${port}${path}`,{status:response.status(),html:await page.locator('#finance-root').innerHTML()},{status:200,html:''});}
    check(`default runtime errors ${port}`,errors,[]);
    const actualFetch=await page.evaluate(async()=>{const result=await fetch('/finance/missing-resource');return {status:result.status,type:result.headers.get('content-type'),html:(await result.text()).includes('finance-root')};});
    evidence.routes.push({port,kind:'actual browser fetch with automatic headers',...actualFetch});check(`actual non-document browser fetch rejected ${port}`,actualFetch.status,404);await page.close();
  }
  const context=await browser.newContext();const page=await context.newPage();const dynamicRequests=[];
  page.on('request',request=>{if(['fetch','xhr'].includes(request.resourceType()))dynamicRequests.push(request.url());});
  await page.addInitScript(()=>{window.storageCalls=[];const original=Storage.prototype.setItem;Storage.prototype.setItem=function(...args){window.storageCalls.push(args[0]);return original.apply(this,args);};});
  await page.goto('http://127.0.0.1:4481/docs/justix-auto/dev/qa/T-038/independent-fixture.html');await page.locator('main').waitFor();
  check('no dynamic app requests',dynamicRequests,[]);check('no browser persistence',await page.evaluate(()=>({writes:window.storageCalls,local:localStorage.length,session:sessionStorage.length,cookies:document.cookie})),{writes:[],local:0,session:0,cookies:''});
  await page.evaluate(()=>window.qa.render(true,{...window.qa.display,organizationName:'Очень длинное синтетическое наименование организации для независимой проверки',employeeName:'<script>synthetic</script>'}));
  await page.locator('header').getByText('<script>synthetic</script>',{exact:true}).waitFor();check('injected text escaped',await page.locator('header script').count(),0);await page.screenshot({path:`${prefix}long-label.png`,fullPage:true});await context.close();
  for(const file of ['finance/index.html','finance/workspace.js','finance/workspace.css','common.css','styles.css'])evidence.hashes[file]=sha(await readFile(`docs/justix-auto/mocks/${file}`));
} finally {if(browser)await browser.close();for(const server of servers.reverse())await server.close();await writeFile(`${prefix}observations.json`,JSON.stringify(evidence,null,2)+'\n');}
console.log(JSON.stringify({checks:evidence.checks.length,failures:evidence.failures},null,2));if(evidence.failures.length)process.exitCode=1;
