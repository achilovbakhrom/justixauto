async (page) => {
  const dir = '/Users/bakhromachilov/startups/justixauto/.worktrees/T-034/docs/justix-auto/dev/qa/T-034';
  const evidence = { baseCommit:'fb744c830fa2228c16d41ef695d953299d140045', testedTree:'developer fix1 before handoff commit', date:'2026-09-15', captures:[], checks:[], errors:[] };
  const ensure = (condition, label) => { if (!condition) throw new Error(label); evidence.checks.push(label); };
  page.on('pageerror', error => evidence.errors.push(error.message));
  const capture = async (kind, scope, width, selector) => {
    await page.setViewportSize({width,height:1000});
    const values = await page.locator(selector).evaluate(el => {
      const s=getComputedStyle(el), r=el.getBoundingClientRect();
      const header=el.querySelector('header,.modal-header,.ins-dialog-head,.admin-modal-head');
      const controls=Array.from(el.querySelectorAll('h2,header button,.modal-close,label,input')).slice(0,5).map(control=>{
        const cs=getComputedStyle(control),cr=control.getBoundingClientRect();
        return {tag:control.tagName,text:control.textContent,className:control.className,width:cr.width,height:cr.height,fontSize:cs.fontSize,lineHeight:cs.lineHeight,border:cs.border,borderRadius:cs.borderRadius,padding:cs.padding,color:cs.color};
      });
      return {controls,x:r.x,width:r.width,right:r.right,height:r.height,radius:s.borderRadius,font:s.fontFamily,background:s.backgroundColor,padding:header?getComputedStyle(header).padding:null};
    });
    ensure(values.x>=0 && values.right<=width+.1, `${kind} ${scope} fits ${width}px`);
    if(kind==='react') {
      const [title,close,label,input]=values.controls;
      const scoped=scope==='ins-workspace'||scope==='admin-shell';
      ensure(title.fontSize===(scoped?'23px':scope==='finance-workspace'?'22px':'18px'),scope+' title size '+width);
      ensure(title.lineHeight===(scoped?'33.35px':'24px'),scope+' title line-height '+width);
      ensure(close.height===(scoped?38:36),scope+' close height '+width);
      ensure(close.fontSize===(scope==='admin-shell'?'20px':scope==='finance-workspace'?'24px':'14px'),scope+' close font '+width);
      ensure(scope==='dealer-shell'?close.text==='':close.text==='×',scope+' close artwork '+width);
      if(scoped) {
        ensure(close.border==='1px solid rgb(185, 193, 206)' && close.padding==='0px 14px',scope+' outlined close '+width);
        ensure(label.fontSize==='14px' && label.color==='rgb(82, 96, 120)',scope+' field label '+width);
        ensure(input.border==='1px solid rgb(203, 213, 225)' && input.borderRadius==='7px',scope+' field border/radius '+width);
      }
      if(scope==='admin-shell') ensure(input.padding==='11px 12px' && Math.abs(input.height-44.3)<.05,scope+' native input padding/height '+width);
    }
    evidence.captures.push({kind,scope,viewport:{width,height:1000},url:page.url(),selector,...values});
    await page.screenshot({path:`${dir}/developer-fix1-${kind}-${scope}-${width}.png`,scale:'css'});
  };
  const fixtures = [
    ['dealer-shell','/', '.modal',async()=>page.getByRole('button',{name:'Создать заказ',exact:true}).click()],
    ['finance-workspace','/finance/', '.modal',async()=>{await page.getByRole('button',{name:'Программы',exact:true}).click();await page.getByRole('button',{name:'Создать программу',exact:true}).click();}],
    ['ins-workspace','/insurance/', '.ins-dialog',async()=>{await page.getByRole('button',{name:'Открыть',exact:true}).nth(1).click();await page.getByRole('button',{name:'Запросить сведения',exact:true}).click();}],
    ['admin-shell','/admin/', '.admin-modal',async()=>{await page.getByRole('button',{name:'Компании',exact:true}).click();await page.getByRole('button',{name:'+ Добавить компанию',exact:true}).click();}]
  ];
  for (const [scope,path,selector,open] of fixtures) {
    await page.setViewportSize({width:1440,height:1000});
    await page.goto(`http://127.0.0.1:4194${path}`); await open();
    for(const width of [1440,600]) await capture('reference',scope,width,selector);
    await page.goto(`http://127.0.0.1:4195/docs/justix-auto/dev/qa/T-034/developer-fixture.html?scope=${scope}`);
    await page.getByRole('button',{name:'Открыть форму',exact:true}).click();
    for(const width of [1440,600]) await capture('react',scope,width,'.jx-dialog');
    ensure(await page.locator(`.jx-dialog-overlay.${scope} > [role=dialog]`).count()===1, `${scope} portal keeps scope`);
    await page.getByRole('button',{name:'Сохранить',exact:true}).focus();
    await page.keyboard.press('Tab');
    ensure(await page.getByRole('button',{name:'Закрыть',exact:true}).evaluate(e=>e===document.activeElement),`${scope} Tab wraps`);
    await page.keyboard.press('Shift+Tab');
    ensure(await page.getByRole('button',{name:'Сохранить',exact:true}).evaluate(e=>e===document.activeElement),`${scope} reverse Tab wraps`);
    await page.getByRole('textbox',{name:'Название',exact:true}).click();
    ensure(await page.getByRole('dialog').count()===1,`${scope} inside click preserves dialog`);
    await page.getByRole('button',{name:'Сохранить',exact:true}).click();
    const input=page.getByRole('textbox',{name:'Название',exact:true});
    ensure(await input.getAttribute('aria-invalid')==='true',`${scope} invalid marked`);
    ensure(await input.evaluate(e=>e.getAttribute('aria-describedby').split(' ').every(id=>document.getElementById(id))) ,`${scope} descriptions exist`);
    ensure(await page.getByRole('alert').textContent()==='Введите название',`${scope} validation announced`);
    await input.fill('Independent fixture');
    await page.getByRole('button',{name:'Отмена',exact:true}).click();
    await page.getByRole('dialog').waitFor({state:'detached'});
    await page.waitForFunction(()=>document.activeElement?.textContent==='Открыть форму');
    ensure(await page.getByTestId('writes').textContent()==='Synthetic writes: 0',`${scope} cancel no writes and restores focus`);
    for (const state of ['Dirty','Processing']) {
      await page.getByRole('checkbox',{name:`${state} guard`}).check();
      await page.getByRole('button',{name:'Открыть форму',exact:true}).click();
      await input.fill('Preserved draft'); await page.keyboard.press('Escape');
      ensure(await page.getByRole('dialog').count()===1,`${scope} ${state} Escape veto`);
      ensure(await input.inputValue()==='Preserved draft',`${scope} ${state} retains input`);
      await page.mouse.click(2,2);
      ensure(await page.getByRole('dialog').count()===1,`${scope} ${state} outside veto`);
      if(state==='Processing') {
        ensure(await page.getByRole('button',{name:'Сохранить',exact:true}).isDisabled(),`${scope} processing submit disabled`);
        ensure(await page.getByRole('button',{name:'Закрыть',exact:true}).isDisabled(),`${scope} processing close disabled`);
        await input.press('Enter');
        ensure(await page.getByTestId('writes').textContent()==='Synthetic writes: 0',`${scope} processing Enter no write`);
      } else {
        await page.getByRole('button',{name:'Закрыть',exact:true}).click();
        ensure(await page.getByRole('dialog').count()===1,`${scope} dirty close veto`);
      }
      await page.goto(`http://127.0.0.1:4195/docs/justix-auto/dev/qa/T-034/developer-fixture.html?scope=${scope}`);
    }
    await page.getByRole('button',{name:'Открыть форму',exact:true}).click();
    await page.keyboard.press('Escape');await page.getByRole('dialog').waitFor({state:'detached'});
    await page.waitForFunction(()=>document.activeElement?.textContent==='Открыть форму');
    ensure(true,`${scope} clean Escape teardown and restore`);
    await page.getByRole('button',{name:'Открыть форму',exact:true}).click();
    await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
    await page.mouse.click(2,2);await page.getByRole('dialog').waitFor({state:'detached'});
    ensure(true,`${scope} clean outside dismiss`);
  }
  ensure(evidence.errors.length===0,'No page exceptions');
  return evidence;
}
