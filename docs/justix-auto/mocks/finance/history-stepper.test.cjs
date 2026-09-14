const test=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const H=vm.runInNewContext(fs.readFileSync(path.join(__dirname,'history-stepper.js'),'utf8')+'\nFinanceHistory');
test('separates date and compound actor without losing comment separators',()=>{
  const e=H.parse('Запрошены сведения: Доход · взнос · 09.09.2026, 13:28:00 · Азиза · Демо-банк');
  assert.equal(e.title,'Запрошены сведения');assert.equal(e.detail,'Доход · взнос');
  assert.equal(e.date,'09.09.2026, 13:28:00');assert.equal(e.actor,'Азиза · Демо-банк');
  assert.equal(e.tone,'warning');
});
test('ISO timestamps supported and unstructured legacy entries retained without invented metadata',()=>{
  assert.equal(H.parse('Расчёт сохранён · 2026-09-09T13:28:00.123Z · Продавец').actor,'Продавец');
  const e=H.parse('Демонстрационный расчёт · Пример, не реальная заявка');
  assert.equal(e.title,'Демонстрационный расчёт · Пример, не реальная заявка');assert.equal(e.date,'');assert.equal(e.actor,'');
});
test('semantic list preserves original order, all entries, and exactly one latest marker without mutating data',()=>{
  const events=Object.freeze(['Условия согласованы · 09.09.2026, 13:00:00 · Продавец','Отправлено положительное решение · 09.09.2026, 12:00:00 · Банк','Расчёт сохранён']);
  const html=H.render(events);
  assert.match(html,/<ol[^>]*reversed/);assert.equal((html.match(/<li /g)||[]).length,3);
  assert.equal((html.match(/Последнее событие/g)||[]).length,1);
  assert.ok(html.indexOf('Условия согласованы')<html.indexOf('Отправлено положительное решение'));
  assert.match(html,/value="3"/);assert.match(html,/value="1"/);
  assert.equal(events.length,3);
});
test('escapes all user content including comments and actor',()=>{
  const html=H.render(['Отказ: <img src=x onerror=alert(1)> · 09.09.2026, 13:00:00 · <script>']);
  assert.ok(!html.includes('<img'));assert.ok(!html.includes('<script>'));
  assert.match(html,/&lt;img/);assert.match(html,/event-danger/);
});
test('empty and single-event history do not invent future steps',()=>{
  assert.equal(H.render(),'<p class="interaction-history-empty">Событий пока нет.</p>');
  const html=H.render(['Заявка отправлена']);
  assert.equal((html.match(/<li /g)||[]).length,1);assert.match(html,/value="1"/);
  assert.ok(!html.includes('interaction-meta'));
});
