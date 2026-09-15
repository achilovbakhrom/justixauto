import assert from 'node:assert/strict';
import {writeFile, readFile} from 'node:fs/promises';
import {spawn, execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {chromium} from '@playwright/test';
import {createServer, preview} from 'vite';

const dir = 'docs/justix-auto/dev/qa/T-036';
const sha = execFileSync('git', ['rev-parse','HEAD'], {encoding:'utf8'}).trim();
assert.equal(sha, '0f6c32843f38eeb204adc37880fc4b964fd4be50');
const result = {sha, comparisons:[], routes:[], failures:[], checks:[]};
let browser; const servers=[];
const mock = spawn(process.execPath, ['tools/serve-mocks.mjs'], {env:{...process.env, MOCK_PORT:'4186'}, stdio:'inherit'});
function check(condition, description) { assert.ok(condition, description); result.checks.push(description); }
const fixture = 'http://127.0.0.1:4206/'+dir+'/independent-fixture.html';
async function metrics(page) {
  return page.evaluate(() => {
    const aside=document.querySelector('.admin-shell aside');
    const header=document.querySelector('.admin-shell main header');
    const measure=e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return {x:r.x,y:r.y,width:r.width,height:r.height,font:s.font,color:s.color,padding:s.padding,background:s.backgroundColor,border:s.border};};
    return {sidebar:measure(aside),brand:measure(aside.firstElementChild),header:measure(header),buttons:[...aside.querySelectorAll('nav button')].map(e=>({label:e.textContent,...measure(e)})),overflow:document.documentElement.scrollWidth>innerWidth};
  });
}
try {
  const general=await createServer({configFile:false,root:process.cwd(),server:{host:'127.0.0.1',port:4206,strictPort:true}}); servers.push(general); await general.listen();
  const dev=await createServer({configFile:'web/apps/admin/vite.config.ts',server:{host:'127.0.0.1',port:4207,strictPort:true}}); servers.push(dev); await dev.listen();
  const built=await preview({configFile:'web/apps/admin/vite.config.ts',preview:{host:'127.0.0.1',port:4208,strictPort:true}}); servers.push({close:()=>new Promise(resolve=>built.httpServer.close(resolve))});
  browser=await chromium.launch({headless:true});
  for(const width of [1440,600,580]) {
    const context=await browser.newContext({viewport:{width,height:1000},locale:'ru-RU',timezoneId:'Asia/Tashkent',reducedMotion:'reduce'});
    const ref=await context.newPage(), app=await context.newPage();
    const errors=[]; app.on('pageerror',e=>errors.push(e.message));
    await app.addInitScript(()=>{
      window.__qaEffects=[];
      Storage.prototype.setItem=function(){window.__qaEffects.push('storage');throw Error('Unexpected storage');};
      window.fetch=function(){window.__qaEffects.push('fetch');throw Error('Unexpected fetch');};
    });
    await ref.goto('http://127.0.0.1:4186/admin/'); await ref.locator('.admin-sidebar').waitFor();
    await ref.screenshot({path:`${dir}/independent-${width}-reference-raw.png`,fullPage:true});
    // Explicit shell-only projection; retained raw screenshot is the actual reference.
    // This does not establish Overview, auth-state or business-page parity.
    await ref.locator('.admin-sidebar button').evaluateAll(nodes=>nodes.forEach(n=>{n.classList.remove('active');n.disabled=true;}));
    await ref.addStyleTag({content:'.admin-page {visibility:hidden}'});
    await app.goto(fixture); await app.locator('main').waitFor();
    const reference=await metrics(ref), candidate=await metrics(app);
    assert.deepEqual(candidate,reference,`shell geometry and styles at ${width}`);
    check(!candidate.overflow,`no shell overflow at ${width}`);
    await ref.screenshot({path:`${dir}/independent-${width}-reference-shell-only.png`,fullPage:true});
    await app.screenshot({path:`${dir}/independent-${width}-candidate-raw.png`,fullPage:true});
    assert.deepEqual(await app.locator('nav button').evaluateAll(nodes=>nodes.map(n=>n.disabled)),Array(8).fill(true));
    check(await app.locator('a,h1,[role=dialog],nav .active').count()===0,`no links, page title, dialog or active feature at ${width}`);
    for(const button of await app.locator('nav button').all()) await button.dispatchEvent('click');
    check(new URL(app.url()).pathname==='/admin/',`disabled controls never navigate at ${width}`);
    await app.keyboard.press('Tab'); await app.keyboard.press('Enter'); await app.keyboard.press('Space');
    check(await app.evaluate(()=>document.activeElement.tagName)==='BODY',`disabled controls excluded from keyboard focus at ${width}`);
    await app.evaluate(()=>{history.pushState(null,'','/admin/companies/qa');dispatchEvent(new PopStateEvent('popstate'));history.pushState(null,'','/admin/users/qa');dispatchEvent(new PopStateEvent('popstate'));});
    await app.goBack(); check(new URL(app.url()).pathname==='/admin/companies/qa',`back at ${width}`);
    await app.goForward(); check(new URL(app.url()).pathname==='/admin/users/qa',`forward at ${width}`);
    await app.goto(fixture+'?long=1'); await app.locator('main').waitFor();
    check(!(await metrics(app)).overflow,`long unbroken synthetic identity no overflow at ${width}`);
    await app.screenshot({path:`${dir}/independent-${width}-long-identity.png`,fullPage:true});
    check((await app.evaluate(()=>window.__qaEffects)).length===0,`no API or storage effects at ${width}`);
    check((await context.cookies()).length===0,`no cookies at ${width}`);
    await app.goto(fixture+'?deny=1'); await app.waitForLoadState('networkidle');
    check(await app.locator('#qa-root').innerHTML()==='',`denied synthetic identity never rendered at ${width}`);
    assert.deepEqual(errors,[]);
    result.comparisons.push({width,height:1000,reference,candidate,projection:'Raw reference retained. Mask business content and remove Overview active state for shell-only geometry/style comparison; no Overview/auth parity claim.'});
    await context.close();
  }
  const context=await browser.newContext(); const page=await context.newPage(); const errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  page.on('response',r=>{if(r.request().resourceType()==='script'&&r.status()>=400)errors.push(r.url()+':'+r.status());});
  for(const port of [4207,4208]) {
    const bare=await context.request.get(`http://127.0.0.1:${port}/admin`,{headers:{Accept:'text/html'}});
    result.routes.push({port,path:'/admin',status:bare.status(),type:bare.headers()['content-type'],note:'Informational: task specifies /admin/ with trailing slash.'});
    for(const path of ['/admin/','/admin/index.html','/admin/users/qa?tab=access','/admin/companies/qa/unknown']) {
      const response=await page.goto(`http://127.0.0.1:${port}${path}`); assert.equal(response.status(),200,`${port}${path}`);
      await page.waitForLoadState('networkidle'); check(await page.locator('#admin-root').innerHTML()==='',`closed entry ${port}${path}`);
      check(await page.locator('main,nav,input,vite-error-overlay').count()===0,`no protected or invented UI ${port}${path}`);
      await page.reload(); await page.waitForLoadState('networkidle'); assert.equal(await page.locator('#admin-root').innerHTML(),'');
    }
    for(const path of ['/','/finance/','/insurance/','/administer/','/api/session','/admin/api','/admin/api/session','/admin/assets/missing.js','/admin/assets/extensionless','/admin/src/missing','/admin/node_modules/missing','/admin/missing.svg','/admin/foo%2Ejs','/admin/foo@bar']) {
      const response=await context.request.get(`http://127.0.0.1:${port}${path}`,{headers:{Accept:'text/html'}});
      const record={port,path,status:response.status(),type:response.headers()['content-type']}; result.routes.push(record);
      assert.equal(response.status(),404,JSON.stringify(record)); assert.ok(!record.type?.includes('text/html'));
    }
    for(const method of ['POST','PUT','DELETE','OPTIONS']) {
      const r=await context.request.fetch(`http://127.0.0.1:${port}/admin/users/qa`,{method,headers:{Accept:'text/html'}});
      check(!r.headers()['content-type']?.includes('text/html'),`no HTML fallback for ${method} on ${port}`);
    }
    const json=await context.request.get(`http://127.0.0.1:${port}/admin/users/qa`,{headers:{Accept:'application/json'}});assert.equal(json.status(),404);
    const head=await context.request.head(`http://127.0.0.1:${port}/admin/users/qa`,{headers:{Accept:'text/html'}});assert.equal(head.status(),200);assert.equal((await head.body()).length,0);
  }
  assert.deepEqual(errors,[]); await context.close();
  const old=JSON.parse(execFileSync('git',['show','6a25973f20c528ef66ecdeb86aefec058e3c06fb:package-lock.json'],{encoding:'utf8'}));
  const current=JSON.parse(await readFile('package-lock.json','utf8'));
  for(const [name,entry] of Object.entries(old.packages)) for(const key of ['version','resolved','integrity']) assert.equal(current.packages[name]?.[key],entry[key],`${name}.${key}`);
  result.newLockEntries=Object.keys(current.packages).filter(name=>!(name in old.packages)); assert.equal(result.newLockEntries.length,7);
  const {ESLint}=await import('eslint'); const lint=new ESLint({overrideConfigFile:'web/eslint.config.js'});
  const config=await lint.calculateConfigForFile('web/apps/admin/src/app/App.tsx'); assert.equal(config.rules['react-refresh/only-export-components'][0],2);
  const messages=await lint.lintText('export function Broken(){return <div/>;} export function util(){ return 1; }',{filePath:'web/apps/admin/src/app/IndependentQA.tsx'});
  check(messages[0].messages.some(m=>m.ruleId==='react-refresh/only-export-components'), 'React Refresh enabled and rejects mixed function exports');
  result.referenceHashes=Object.fromEntries(await Promise.all(['admin/index.html','admin/admin.js','admin/management.js','admin/admin.css','common.css','styles.css','insurance.css'].map(async name=>[name,createHash('sha256').update(await readFile('docs/justix-auto/mocks/'+name)).digest('hex')])));
  assert.equal(execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),sha);
} catch(error) { result.failures.push(error.stack); process.exitCode=1; }
finally {
  await browser?.close(); for(const server of servers.reverse()) await server.close(); mock.kill('SIGTERM');
  await writeFile(`${dir}/independent-results.json`,JSON.stringify(result,null,2)+'\n');
  console.log(JSON.stringify({sha,checks:result.checks.length,comparisons:result.comparisons.length,routes:result.routes.length,failures:result.failures},null,2));
}
