import { chromium } from '@playwright/test';
import { createServer, preview } from 'vite';
import assert from 'node:assert/strict';
import { readFile, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { request as httpRequest } from 'node:http';

// Node fetch adds Sec-Fetch-Mode:cors itself. Raw HTTP is required to test
// the intentionally supported legacy client with absent Fetch Metadata.
const request = (url, options = {}) => new Promise((resolve,reject) => {
  const req = httpRequest(url,options,response => {
    const chunks=[]; response.on('data',chunk=>chunks.push(chunk));
    response.on('end',()=>resolve({status:response.statusCode,headers:{get:name=>response.headers[name]},text:async()=>Buffer.concat(chunks).toString()}));
    response.on('error',reject);
  }); req.on('error',reject);req.end();
});

const out = 'docs/justix-auto/dev/qa/T-038/developer-r2-routing.json';
const result = { checks: [], responses: [], browserRequests: [] };
const check = (name, actual, expected = true) => { assert.deepEqual(actual,expected,name); result.checks.push(name); };
const servers = []; let browser;
try {
  browser = await chromium.launch();
  for (const [app,base,rootId,firstPort] of [['admin','/admin/','admin-root',4590],['realization','/','realization-root',4592],['financing','/finance/','finance-root',4594]]) {
    const configFile = `web/apps/${app}/vite.config.ts`;
    const dev = await createServer({configFile,server:{host:'127.0.0.1',port:firstPort,strictPort:true}}); await dev.listen(); servers.push(dev);
    const built = await preview({configFile,preview:{host:'127.0.0.1',port:firstPort+1,strictPort:true}}); servers.push({close:()=>new Promise(resolve=>built.httpServer.close(resolve))});
    for (const port of [firstPort,firstPort+1]) {
      const origin = `http://127.0.0.1:${port}`;
      const negative = [
        ['wildcard',{Accept:'*/*'}], ['JSON',{Accept:'application/json'}],
        ['zero HTML',{Accept:'text/html;q=0, */*'}], ['wrong media',{Accept:'text/html-fragment'}],
        ['script',{Accept:'*/*','Sec-Fetch-Dest':'script','Sec-Fetch-Mode':'no-cors'}],
        ['fetch',{Accept:'*/*','Sec-Fetch-Dest':'empty','Sec-Fetch-Mode':'cors'}],
        ['HTML fetch',{Accept:'text/html','Sec-Fetch-Dest':'empty','Sec-Fetch-Mode':'cors'}],
        ['HTML script',{Accept:'text/html','Sec-Fetch-Dest':'script','Sec-Fetch-Mode':'no-cors'}],
        ['image',{Accept:'text/html','Sec-Fetch-Dest':'image'}],
        ['inconsistent mode',{Accept:'text/html','Sec-Fetch-Dest':'document','Sec-Fetch-Mode':'cors'}],
      ];
      for (const [name,headers] of negative) {
        for (const resource of ['missing-resource','index.html','%69ndex%2ehtml']) {
          const response = await request(`${origin}${base}${resource}`,{headers}),body=await response.text();
          check(`${app}:${port} ${name} ${resource} rejected`,response.status===404&&!response.headers.get('content-type')?.includes('text/html')&&!body.includes(rootId));
          result.responses.push({app,port,name,resource,status:response.status,type:response.headers.get('content-type')});
        }
      }
      for (const headers of [{Accept:'text/html'}, {Accept:'text/html;q=0.8,application/xhtml+xml;q=0.9,*/*;q=0.1','Sec-Fetch-Dest':'document','Sec-Fetch-Mode':'navigate'}, {Accept:'text/html','Sec-Fetch-Dest':'iframe','Sec-Fetch-Mode':'navigate'}]) {
        for (const method of ['GET','HEAD']) {
          const response=await request(`${origin}${base}applications/synthetic`,{method,headers}),body=await response.text();
          check(`${app}:${port} document ${method} ${JSON.stringify(headers)}`,response.status===200&&response.headers.get('content-type')?.includes('text/html')&&(method==='HEAD'?body==='':body.includes(rootId)));
        }
      }
      const forbidden = app==='realization' ? ['/admin/','/finance/','/insurance/','/api/session'] : ['/', '/api/session', `${base}api/session`, app==='admin'?'/finance/':'/admin/'];
      for (const path of [...forbidden,`${base}assets/missing`,`${base}missing.js`,`${base}%61pi`]) {
        const response=await request(origin+path,{headers:{Accept:'text/html'}});check(`${app}:${port} prefix/assets ${path}`,response.status===404&&!response.headers.get('content-type')?.includes('text/html'));
      }
      for (const method of ['POST','PUT','PATCH','DELETE']) {const response=await request(`${origin}${base}applications/synthetic`,{method,headers:{Accept:'text/html'}});check(`${app}:${port} reject ${method}`,response.status,404);}
      const context=await browser.newContext(); const page=await context.newPage(); const errors=[];
      page.on('pageerror',error=>errors.push(error.message));
      page.on('response',response=>{if(response.request().resourceType()==='script'&&response.status()>=400&&!response.url().includes('missing-resource'))errors.push(response.url());});
      for (const path of [base,`${base}applications/synthetic`,`${base}index.html`]) {
        const response=await page.goto(origin+path);await page.waitForLoadState('networkidle');check(`${app}:${port} actual navigation ${path}`,response.status()===200&&(await page.locator(`#${rootId}`).innerHTML())==='');
      }
      check(`${app}:${port} actual entry scripts loaded`,errors,[]);
      for(const accept of [null,'text/html']) {
        for(const resource of ['missing-resource','index.html']) {
          const response=await page.evaluate(async({base,accept,resource})=>{const response=await fetch(`${base}${resource}`,accept?{headers:{Accept:accept}}:{});return{status:response.status,type:response.headers.get('content-type'),body:await response.text()};},{base,accept,resource});
          check(`${app}:${port} Chromium fetch ${resource} ${accept}`,response.status===404&&!response.type?.includes('text/html')&&!response.body.includes(rootId));
          result.browserRequests.push({app,port,kind:'fetch',accept,resource,...response});
        }
      }
      const pending=page.waitForResponse(response=>response.url()===`${origin}${base}missing-resource`&&response.request().resourceType()==='script');
      const loaded=page.evaluate(base=>new Promise(resolve=>{const script=document.createElement('script');script.src=`${base}missing-resource`;script.onload=()=>{script.remove();resolve(true);};script.onerror=()=>{script.remove();resolve(false);};document.head.append(script);}),base);
      const response=await pending;check(`${app}:${port} Chromium script rejected`,response.status()===404&&!response.headers()['content-type']?.includes('text/html'));check(`${app}:${port} missing script not executed`,await loaded,false);
      result.browserRequests.push({app,port,kind:'script',status:response.status(),requestHeaders:await response.request().allHeaders()});
      await context.close();
    }
  }
  const preserved=JSON.parse(await readFile('docs/justix-auto/dev/qa/T-038/developer-r2-preserved-hashes.json','utf8'));
  for (const [path,hash] of Object.entries(preserved)) check(`preserved ${path}`,createHash('sha256').update(await readFile(path)).digest('hex'),hash);
  await writeFile(out,JSON.stringify(result,null,2)+'\n');console.log(`${result.checks.length} routing/preservation checks passed`);
} finally { if(browser) await browser.close();for(const server of servers.reverse())await server.close(); }
