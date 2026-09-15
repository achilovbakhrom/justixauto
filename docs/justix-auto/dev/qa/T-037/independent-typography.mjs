import { chromium } from '@playwright/test';
import { createServer } from 'vite';
import { writeFile } from 'node:fs/promises';
const prefix='docs/justix-auto/dev/qa/T-037/independent-';
const server=await createServer({configFile:false,server:{host:'127.0.0.1',port:4236,strictPort:true}});
await server.listen(); let browser;
try {
  browser=await chromium.launch(); const context=await browser.newContext({viewport:{width:1440,height:1000},locale:'ru-RU',timezoneId:'Asia/Tashkent'});
  const records=[];
  for(const url of ['http://127.0.0.1:4180/','http://127.0.0.1:4236/docs/justix-auto/dev/qa/T-037/independent-fixture.html']) {
    const page=await context.newPage();await page.goto(url);await page.locator('.dealer-shell aside').waitFor();await page.evaluate(()=>document.fonts.ready);
    const styles=await page.evaluate(()=>Object.fromEntries(['body','aside nav button span','header button'].map(selector=>{const c=getComputedStyle(document.querySelector(selector));return [selector,{font:c.font,fontSmoothing:c.webkitFontSmoothing,textRendering:c.textRendering}];})));
    records.push({url,styles});
    if(url.includes('4236')) {
      // Diagnostic only: browser style injection demonstrates the omitted
      // reference declaration; task source remains at the reviewed SHA.
      await page.addStyleTag({content:'body {-webkit-font-smoothing: antialiased}'});
      await page.screenshot({path:`${prefix}1440-candidate-smoothing-diagnostic.png`});
    }
  }
  await writeFile(`${prefix}typography.json`,JSON.stringify(records,null,2)+'\n');console.log(JSON.stringify(records,null,2));
} finally {if(browser) await browser.close();await server.close();}
