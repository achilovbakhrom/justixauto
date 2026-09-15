async page => {
 const evidence={reviewedCommit:'6321f190fdd8edc31b65ad8455cf96b405aaf23f',checks:[],states:[]};
 const check=(value,label)=>{if(!value)throw Error(label);evidence.checks.push(label)};
 for(const scope of ['dealer-shell','finance-workspace','ins-workspace','admin-shell']){
  await page.setViewportSize({width:320,height:800});
  await page.goto('http://127.0.0.1:4195/docs/justix-auto/dev/qa/T-034/developer-fixture.html?scope='+scope);
  await page.getByRole('button',{name:'Открыть форму',exact:true}).click();
  await page.locator('.jx-dialog-title').evaluate(e=>{e.textContent='Очень длинное синтетическое название формы для проверки переноса и доступности';});
  const rects=await page.locator('.jx-dialog').evaluate(e=>{
   const title=e.querySelector('h2').getBoundingClientRect(),close=e.querySelector('.jx-dialog-close').getBoundingClientRect(),r=e.getBoundingClientRect();
   return {x:r.x,right:r.right,width:r.width,titleRight:title.right,closeX:close.x,clientWidth:e.clientWidth,scrollWidth:e.scrollWidth};
  });
  check(rects.x>=0&&rects.right<=320,scope+' 320px fits');
  check(rects.titleRight<=rects.closeX,scope+' long title no close overlap');
  check(rects.scrollWidth<=rects.clientWidth,scope+' no horizontal dialog overflow');
  const input=page.getByRole('textbox',{name:'Название',exact:true});
  await input.focus();
  const focused=await input.evaluate(e=>({outline:getComputedStyle(e).outline,border:getComputedStyle(e).borderColor}));
  check(!focused.outline.startsWith('0px')&&!focused.outline.includes('none'),scope+' input focus outline visible');
  await page.getByRole('button',{name:'Сохранить',exact:true}).focus();await page.keyboard.press('Tab');
  const closeFocus=await page.getByRole('button',{name:'Закрыть',exact:true}).evaluate(e=>({focused:document.activeElement===e,outline:getComputedStyle(e).outline}));
  check(closeFocus.focused&&!closeFocus.outline.includes('none'),scope+' keyboard close focus visible');
  await page.screenshot({path:'/Users/bakhromachilov/startups/justixauto/.worktrees/T-034/docs/justix-auto/dev/qa/T-034/r2-narrow-'+scope+'.png',scale:'css'});
  await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'detached'});
  await page.waitForFunction(()=>document.activeElement?.textContent==='Открыть форму');
  check(await page.getByTestId('writes').textContent()==='Synthetic writes: 0',scope+' narrow Escape zero writes and restores');
  evidence.states.push({scope,viewport:{width:320,height:800},rects,focused,closeFocus});
 }
 return evidence;
}
