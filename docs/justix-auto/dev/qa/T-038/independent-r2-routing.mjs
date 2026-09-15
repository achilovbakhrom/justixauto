import assert from 'node:assert/strict';
import { request } from 'node:http';
import { createServer, preview } from 'vite';
import { chromium } from '@playwright/test';
import { readFile, writeFile } from 'node:fs/promises';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
const prefix='docs/justix-auto/dev/qa/T-038/independent-r2-';
const evidence={sha:execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),checks:[],failures:[],routes:[],browser:[]};
assert.equal(evidence.sha,'fba510c0c5220207b1f470eb2008132097a27a54');
const hash=b=>createHash('sha256').update(b).digest('hex');
function check(name,actual,expected=true){try{assert.deepEqual(actual,expected);evidence.checks.push(name);}catch{evidence.failures.push({name,actual,expected});}}
check('reviewed lock byte identical',hash(await readFile('package-lock.json')),hash(execFileSync('git',['show','c1b0b50eed76902bf024fc6c5d9c5e4f032f642b:package-lock.json'])));
const preserved=JSON.parse(await readFile('docs/justix-auto/dev/qa/T-038/developer-r2-preserved-hashes.json','utf8'));
evidence.preservedArtifactHashes=preserved;
for(const [file,expected] of Object.entries(preserved))check(`preserved bytes ${file}`,hash(await readFile(file)),expected);
// Raw HTTP intentionally avoids Node fetch's automatically injected cors mode.
function raw(port,path,headers={},method='GET'){return new Promise((resolve,reject)=>{const req=request({host:'127.0.0.1',port,path,method,headers},res=>{const parts=[];res.on('data',b=>parts.push(b));res.on('end',()=>resolve({status:res.statusCode,type:res.headers['content-type']??'',body:Buffer.concat(parts).toString()}));});req.on('error',reject);req.end();});}
const servers=[];let browser;
try{
 browser=await chromium.launch();
 let port=4680;
 for(const [app,base,rootId] of [['admin','/admin/','admin-root'],['realization','/','realization-root'],['financing','/finance/','finance-root']]){
  for(const mode of ['dev','preview']){
   const currentPort=port++;const configFile=`web/apps/${app}/vite.config.ts`;
   if(mode==='dev'){const server=await createServer({configFile,server:{host:'127.0.0.1',port:currentPort,strictPort:true}});await server.listen();servers.push(server);}
   else {const server=await preview({configFile,preview:{host:'127.0.0.1',port:currentPort,strictPort:true}});servers.push({close:()=>new Promise(resolve=>server.httpServer.close(resolve))});}
   const label=`${app} ${mode}`;const deep=`${base}applications/synthetic`;
   const probe=async(path,headers,want=404,method='GET')=>{const result=await raw(currentPort,path,headers,method);const row={app,mode,port:currentPort,path,headers,method,want,status:result.status,type:result.type,entry:result.body.includes(rootId),bytes:result.body.length};evidence.routes.push(row);check(`${label} ${method} ${path} ${JSON.stringify(headers)} status`,result.status,want);if(want===404)check(`${label} reject has no HTML ${evidence.routes.length}`,!result.type.includes('text/html')&&!row.entry);if(want===200){check(`${label} document type ${evidence.routes.length}`,result.type.includes('text/html'));check(`${label} document body ${evidence.routes.length}`,method==='HEAD'?result.body==='':row.entry);}return result;};
   for(const path of [base,deep,`${base}index.html`])for(const method of ['GET','HEAD'])await probe(path,{Accept:'text/html'},200,method);
   for(const headers of [{Accept:'text/html; q=0.1'},{Accept:'application/json, text/html; charset=utf-8; q=0.8'},{Accept:'TEXT/HTML'},{Accept:'text/html','Sec-Fetch-Dest':'document','Sec-Fetch-Mode':'navigate'},{Accept:'text/html','Sec-Fetch-Dest':'iframe','Sec-Fetch-Mode':'navigate'},{Accept:'text/html','Sec-Fetch-Dest':'frame','Sec-Fetch-Mode':'navigate'}])await probe(deep,headers,200);
   for(const headers of [{},{Accept:'*/*'},{Accept:'application/json'},{Accept:'text/html; q=0, */*; q=1'},{Accept:'text/html; q=0.000'},{Accept:'text/html; q=invalid'},{Accept:'text/html; q=1.1'},{Accept:'text/html','Sec-Fetch-Dest':'empty','Sec-Fetch-Mode':'cors'},{Accept:'text/html','Sec-Fetch-Dest':'script','Sec-Fetch-Mode':'no-cors'},{Accept:'text/html','Sec-Fetch-Dest':'image'},{Accept:'text/html','Sec-Fetch-Dest':'document','Sec-Fetch-Mode':'cors'},{Accept:'*/*','Sec-Fetch-Dest':'document','Sec-Fetch-Mode':'navigate'}])await probe(deep,headers);
   for(const file of ['index.html','%69ndex.html','index%2ehtml','%69ndex%2E%68tml','missing.html']){
    for(const suffix of ['', '?v=1', '?html-proxy&index=0.js&extra=1'])await probe(`${base}${file}${suffix}`,{Accept:'*/*','Sec-Fetch-Dest':'empty','Sec-Fetch-Mode':'cors'});
   }
   for(const method of ['POST','PUT','PATCH','DELETE'])for(const path of [deep,`${base}index.html`])await probe(path,{Accept:'text/html'},404,method);
   // Vite CORS middleware handles OPTIONS before custom guards. Its empty
   // preflight response is allowed; the invariant here is no entry HTML.
   for(const path of [deep,`${base}index.html`]){const r=await raw(currentPort,path,{Accept:'text/html'},'OPTIONS');evidence.routes.push({app,mode,path,method:'OPTIONS',...r});check(`${label} OPTIONS empty preflight ${path}`,{status:r.status,body:r.body},{status:204,body:''});check(`${label} OPTIONS no HTML ${path}`,!r.type.includes('text/html'));}
   const siblings=app==='realization'?['/admin/','/finance/','/insurance/']:['/',app==='admin'?'/finance/':'/admin/','/insurance/'];
   for(const path of [...siblings,'/api/session',`${base}api/session`,`${base}assets/missing`,`${base}src/missing`,`${base}node_modules/missing`,`${base}missing.js`,`${base}missing.css`,`${base}missing.svg`,`${base}%61pi/session`,`${base}malformed%ZZ`])await probe(path,{Accept:'text/html'});
   const context=await browser.newContext();const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
   const scripts=[];page.on('response',r=>{if(r.request().resourceType()==='script')scripts.push({url:r.url(),status:r.status()});});
   for(const path of [base,deep,`${base}index.html`]){const r=await page.goto(`http://127.0.0.1:${currentPort}${path}`);await page.waitForLoadState('networkidle');check(`${label} live navigation ${path}`,r.status(),200);check(`${label} closed entry ${path}`,await page.locator(`#${rootId}`).innerHTML(),'');}
   check(`${label} runtime errors`,errors,[]);check(`${label} actual entry scripts loaded`,scripts.length>0&&scripts.every(s=>s.status===200||s.status===304));
   for(const path of [`${base}missing-fetch`,`${base}index.html`,`${base}index%2ehtml`])for(const explicitHtml of [false,true]){
    const r=await page.evaluate(async({path,explicitHtml})=>{const r=await fetch(path,explicitHtml?{headers:{Accept:'text/html'}}:{});return {status:r.status,type:r.headers.get('content-type'),body:await r.text()};},{path,explicitHtml});evidence.browser.push({app,mode,kind:'real fetch',path,explicitHtml,...r});check(`${label} real fetch ${path} ${explicitHtml}`,r.status,404);check(`${label} real fetch no HTML ${path} ${explicitHtml}`,!r.type.includes('text/html'));
   }
   const scriptPath=`${base}missing-script`;const responsePromise=page.waitForResponse(r=>new URL(r.url()).pathname===scriptPath);
   await page.evaluate(path=>new Promise(resolve=>{const s=document.createElement('script');s.src=path;s.onload=s.onerror=()=>{s.remove();resolve();};document.body.append(s);}),scriptPath);
   const response=await responsePromise;evidence.browser.push({app,mode,kind:'real script insertion',status:response.status(),headers:await response.request().allHeaders(),type:response.headers()['content-type']});check(`${label} real script rejected`,response.status(),404);
   const iframePath=`${base}iframe/deep`;await page.evaluate(path=>{const frame=document.createElement('iframe');frame.src=path;document.body.append(frame);},iframePath);await page.waitForFunction(()=>document.querySelector('iframe')?.contentDocument?.readyState==='complete');
   check(`${label} real iframe navigation`,await page.locator('iframe').contentFrame().locator(`#${rootId}`).count(),1);
   // Dev's exact module proxy must return JS; malformed/preview proxy may never leak HTML.
   for(const suffix of ['?html-proxy&index=0.js','?html-proxy&index=999.js','?html-proxy&index=0.js&x=1']){
    const r=await raw(currentPort,`${base}index.html${suffix}`,{Accept:'*/*','Sec-Fetch-Dest':'script','Sec-Fetch-Mode':'cors'});evidence.routes.push({app,mode,kind:'proxy boundary',suffix,...r});check(`${label} proxy is never HTML ${suffix}`,!r.type.includes('text/html')&&!r.body.includes(`id="${rootId}"`));if(mode==='preview'||suffix.endsWith('&x=1'))check(`${label} unapproved proxy rejected ${suffix}`,r.status,404);else if(suffix.includes('index=0.js'))check(`${label} valid module proxy JS`,r.status===200&&r.type.includes('javascript'));
   }
   await context.close();
  }
 }
}finally{if(browser)await browser.close();for(const server of servers.reverse())await server.close();await writeFile(`${prefix}routing-final.json`,JSON.stringify(evidence,null,2)+'\n');}
console.log(JSON.stringify({checks:evidence.checks.length,failures:evidence.failures,routes:evidence.routes.length,browser:evidence.browser.length},null,2));if(evidence.failures.length)process.exitCode=1;
