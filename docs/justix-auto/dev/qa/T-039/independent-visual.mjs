import assert from 'node:assert/strict';
import {chromium} from '@playwright/test';
import {createServer} from 'vite';
import {readFile,writeFile} from 'node:fs/promises';
import {createHash} from 'node:crypto';
import {execFileSync} from 'node:child_process';
const prefix='docs/justix-auto/dev/qa/T-039/independent-';
const evidence={sha:execFileSync('git',['rev-parse','HEAD'],{encoding:'utf8'}).trim(),checks:[],failures:[],comparisons:[],hashes:{}};
assert.equal(evidence.sha,'022f663a7d492b0a1c4276be1f4bd0dac78929a1');
function check(name,actual,expected=true){try{assert.deepEqual(actual,expected);evidence.checks.push(name);}catch{evidence.failures.push({name,actual,expected});}}
const metrics=page=>page.evaluate(()=>{
 const rect=e=>{const r=e.getBoundingClientRect(),s=getComputedStyle(e);return {x:r.x,y:r.y,w:r.width,h:r.height,font:s.font,smoothing:s.webkitFontSmoothing,color:s.color,background:s.backgroundColor,border:s.border,radius:s.borderRadius,position:s.position};};
 const aside=document.querySelector('aside'),header=document.querySelector('header');
 return {aside:rect(aside),brand:rect(aside.firstElementChild),header:rect(header),label:rect(header.querySelector('label')),select:rect(header.querySelector('select')),nav:[...aside.querySelectorAll('nav button')].map(rect),grid:getComputedStyle(aside.parentElement).gridTemplateColumns,navDisplay:getComputedStyle(aside.querySelector('nav')).display,overflow:document.documentElement.scrollWidth>innerWidth};
});
let server,browser;
try{
 server=await createServer({configFile:false,server:{host:'127.0.0.1',port:4791,strictPort:true}});await server.listen();browser=await chromium.launch();
 for(const width of [1440,900,600]){
  const contexts=[],pages=[];
  for(const url of ['http://127.0.0.1:4180/insurance/','http://127.0.0.1:4791/docs/justix-auto/dev/qa/T-039/independent-fixture.html']){
   const context=await browser.newContext({viewport:{width,height:1000},locale:'ru-RU',timezoneId:'Asia/Tashkent',reducedMotion:'reduce'});contexts.push(context);
   const page=await context.newPage();pages.push(page);await page.goto(url);await page.locator('aside').waitFor();await page.evaluate(()=>document.fonts.ready);
  }
  const [reference,candidate]=pages;
  await reference.screenshot({path:`${prefix}${width}-reference-raw.png`,fullPage:true});await candidate.screenshot({path:`${prefix}${width}-candidate-raw.png`,fullPage:true});
  await reference.locator('aside nav button').evaluateAll(items=>items.forEach(item=>item.classList.remove('active')));
  await reference.addStyleTag({content:'.page {display:none}'});
  await reference.screenshot({path:`${prefix}${width}-reference-projected.png`});await candidate.screenshot({path:`${prefix}${width}-candidate-viewport.png`});
  const a=await metrics(reference),b=await metrics(candidate);evidence.comparisons.push({width,reference:a,candidate:b});check(`all shell geometry/typography ${width}`,b,a);
  check(`sidebar is normal grid content ${width}`,b.aside.position,'static');
  if(width>600)check(`sidebar breakpoint width ${width}`,b.aside.w,width>900?248:180);else check('horizontal mobile navigation',b.navDisplay,'flex');
  check(`exact disabled menu ${width}`,await candidate.locator('nav button').evaluateAll(items=>items.map(e=>({label:e.textContent,disabled:e.disabled}))),['Обзор','Заявки','Настройки'].map(label=>({label,disabled:true})));
  check(`no links/registered page ${width}`,{links:await candidate.locator('a').count(),page:await candidate.locator('main>section').innerText()},{links:0,page:''});
  check(`current organization only ${width}`,await candidate.locator('select').evaluate(e=>({disabled:e.disabled,options:e.options.length,value:e.value})),{disabled:true,options:1,value:'current'});
  await candidate.keyboard.press('Tab');check(`disabled controls skipped by keyboard ${width}`,await candidate.evaluate(()=>document.activeElement.tagName),'BODY');
  await candidate.locator('nav button').first().dispatchEvent('click');await candidate.keyboard.press('Enter');check(`no action/navigation ${width}`,new URL(candidate.url()).pathname,'/insurance/');
  await candidate.evaluate(()=>window.fixture.render(false));await candidate.locator('aside').waitFor({state:'detached'});check(`revoked injected boundary blank ${width}`,await candidate.locator('body').innerText(),'');
  await candidate.evaluate(()=>window.fixture.render(true,null));await candidate.locator('aside').waitFor();check(`admitted without identity defaults ${width}`,await candidate.locator('header').innerText(),'');
  await candidate.evaluate(()=>window.fixture.render());await candidate.locator('header strong').waitFor();
  await candidate.evaluate(()=>{window.firstClient=window.fixture.cache();window.firstClient.setQueryData(['qa-private'],{synthetic:'private'});window.fixture.remount();});
  await candidate.locator('aside').waitFor();check(`remount private cache isolation ${width}`,await candidate.evaluate(()=>({different:window.firstClient!==window.fixture.cache(),empty:window.fixture.cache().getQueryData(['qa-private'])===undefined})),{different:true,empty:true});
  for(const context of contexts)await context.close();
 }
 const context=await browser.newContext({viewport:{width:600,height:1000}});const page=await context.newPage();
 await page.addInitScript(()=>{window.calls=[];window.fetch=()=>{window.calls.push('fetch');throw Error('Unexpected fetch');};XMLHttpRequest.prototype.open=function(){window.calls.push('xhr');throw Error('Unexpected xhr');};Storage.prototype.setItem=function(){window.calls.push('storage');throw Error('Unexpected storage');};});
 await page.goto('http://127.0.0.1:4791/docs/justix-auto/dev/qa/T-039/independent-fixture.html');await page.locator('aside').waitFor();
 await page.evaluate(()=>window.fixture.render(true,Object.freeze({organizationName:'Синтетическая страховая организация с очень длинным названием для проверки переноса',accountLabel:'<img src=x onerror=alert(1)> — синтетический сотрудник'})));
 await page.waitForFunction(()=>document.querySelector('header strong')?.textContent.startsWith('Синтетическая'));
 await page.screenshot({path:`${prefix}long-label.png`,fullPage:true});check('injected display text escaped',await page.locator('header img').count(),0);
 check('no dynamic application network or persistence',await page.evaluate(()=>window.calls),[]);
 check('no persisted browser state',await page.evaluate(()=>[localStorage.length,sessionStorage.length,document.cookie]),[0,0,'']);
 evidence.longLabel=await metrics(page);await context.close();
 for(const name of ['insurance/index.html','insurance/workspace.js','insurance.css','common.css','styles.css'])evidence.hashes[name]=createHash('sha256').update(await readFile(`docs/justix-auto/mocks/${name}`)).digest('hex');
}finally{if(browser)await browser.close();if(server)await server.close();await writeFile(`${prefix}observations.json`,JSON.stringify(evidence,null,2)+'\n');}
console.log(JSON.stringify({checks:evidence.checks.length,failures:evidence.failures},null,2));if(evidence.failures.length)process.exitCode=1;
