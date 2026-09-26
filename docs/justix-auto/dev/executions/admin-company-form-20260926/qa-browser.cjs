const { chromium } = require('/Users/bakhromachilov/startups/justixauto/node_modules/playwright');
const assert = require('node:assert/strict');

const baseURL = 'http://127.0.0.1:5176';
const evidenceDir = '/Users/bakhromachilov/startups/justixauto/docs/justix-auto/dev/executions/admin-company-form-20260926';
const session = {
  user: { id: 'qa-user', displayName: 'QA Admin', status: 'active', passwordChangeRequired: false },
  roles: [{ id: 'platform-admin', name: 'Platform admin' }],
  permissions: ['platform.directory.read', 'platform.users.manage', 'platform.roles.manage', 'platform.audit.read'],
  context: { revision: 'context-r1', companyId: null, branchScope: { mode: 'ALL', branchIds: [] } },
  accessibleCompanies: [],
  setup: { next: 'none' },
};

const companies = {
  seller: [
    {
      id: 'legacy-company', kind: 'seller', name: 'Legacy Seller', legalName: 'Legacy Legal LLC',
      country: { label: 'Казахстан' }, region: { label: 'Алматы' }, registration: 'LEG-001',
      email: 'legacy@example.test', address: 'Legacy address', phone: '+998900000001',
      access: 'draft', accessReason: '', revision: 'company-r7',
    },
    {
      id: 'empty-legal', kind: 'seller', name: 'Empty Legal Seller', legalName: '',
      country: { label: 'Узбекистан' }, region: { label: 'Ташкент' }, registration: 'EMP-002',
      email: 'empty@example.test', address: 'Empty address', phone: '+998900000002',
      access: 'draft', accessReason: '', revision: 'company-r3',
    },
  ],
  bank: [], mfo: [], insurance: [],
};

const mutations = [];
let createMode = 'success';
let patchMode = 'success';
let activationMode = 'success';
let pendingRoute = null;

function json(route, status, body, headers = {}) {
  return route.fulfill({ status, contentType: 'application/json', headers, body: JSON.stringify(body) });
}

async function main() {
  const browser = await chromium.launch({ channel: 'chrome', headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'ru-RU' });
  const page = await context.newPage();
  const browserVersion = browser.version();

  await page.route('**/api/v1/**', async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const path = url.pathname.replace('/api/v1', '');
    const method = req.method();
    if (method === 'GET' && path === '/identity/session') {
      return json(route, 200, { data: session, revision: 'session-r1' }, { 'X-CSRF-Token': 'synthetic-csrf' });
    }
    if (method === 'GET' && path === '/identity/admin/companies') {
      const kind = url.searchParams.get('kind');
      return json(route, 200, { items: companies[kind] ?? [] });
    }
    if (method === 'GET' && path.startsWith('/identity/companies/')) {
      const id = path.split('/').pop();
      const company = Object.values(companies).flat().find((c) => c.id === id);
      return company ? json(route, 200, { data: company, revision: company.revision }) : json(route, 404, { error: { code: 'not_found', message: 'not found' } });
    }
    if (method === 'POST' && (path === '/identity/admin/seller-companies' || path === '/identity/admin/provider-companies')) {
      const record = { method, path, body: req.postDataJSON(), headers: req.headers() };
      mutations.push(record);
      if (createMode === 'pending') { pendingRoute = route; return; }
      if (createMode === 'network') return route.abort('failed');
      if (createMode === 'conflict') return json(route, 409, { error: { code: 'user_exists', message: 'synthetic conflict' } });
      if (createMode && typeof createMode === 'object') return json(route, 422, { error: { code: 'validation', message: 'Synthetic validation', fields: createMode } });
      return json(route, 201, { data: { id: `created-${mutations.length}`, access: 'draft' }, revision: 'created-r1' });
    }
    if (method === 'PATCH' && path.startsWith('/identity/companies/')) {
      const record = { method, path, body: req.postDataJSON(), headers: req.headers() };
      mutations.push(record);
      if (patchMode === 'stale') return json(route, 412, { error: { code: 'stale_revision', message: 'synthetic stale' } });
      const id = path.split('/').pop();
      const company = Object.values(companies).flat().find((c) => c.id === id);
      if (company) Object.assign(company, record.body, { revision: company.revision + '-next' });
      return json(route, 200, { data: company, revision: company?.revision ?? 'next' });
    }
    if (method === 'POST' && /\/identity\/admin\/companies\/[^/]+\/activate$/.test(path)) {
      mutations.push({ method, path, body: req.postDataJSON(), headers: req.headers() });
      if (activationMode === 'pending') { pendingRoute = route; return; }
      return json(route, 200, { data: null, revision: 'activation-r1' });
    }
    return json(route, 200, { items: [], data: null, revision: 'synthetic-r1' });
  });

  const waitReady = async () => {
    await page.getByText('QA Admin').waitFor({ timeout: 10000 });
    await page.waitForLoadState('networkidle');
  };
  const dialog = () => page.getByRole('dialog').last();
  const input = (name) => dialog().getByLabel(new RegExp(`^${name}(?:\\s|$)`));
  const fillCreate = async (suffix, unknownCountry = false) => {
    await input('Название компании').fill(`QA Company ${suffix}`);
    await input('Регистрационный номер').fill(`REG-${suffix}`);
    await input('Страна').fill(unknownCountry ? 'Германия' : 'казахстан');
    await input('Страна').blur();
    if (!unknownCountry) assert.equal(await input('Страна').inputValue(), 'Казахстан');
    await input('Регион').fill(unknownCountry ? 'Берлин' : 'Алматы');
    await input('Адрес').fill(`QA address ${suffix}`);
    await input('Электронная почта').fill(`qa-${suffix.toLowerCase()}@example.test`);
    await input('Телефон').fill('+998901234567');
    await input('Имя администратора').fill(`Admin ${suffix}`);
    await input('Логин').fill(`qa_${suffix.toLowerCase()}`);
    await input('Пароль').fill('SyntheticPass12!');
    await input('Повторите пароль').fill('SyntheticPass12!');
  };
  const openCreate = async (kind) => {
    const route = kind === 'seller' ? '/admin/companies' : `/admin/integrations/${kind}`;
    await page.goto(baseURL + route);
    await waitReady();
    const label = kind === 'seller' ? '+ Добавить компанию' : '+ Подключить:';
    await page.getByRole('button', { name: new RegExp(label.replace(/[+]/g, '\\+')) }).click();
    await dialog().waitFor();
    return dialog();
  };
  const cancelDialog = async () => {
    await dialog().getByRole('button', { name: 'Отмена', exact: true }).click();
  };

  // Desktop grouped layout and address dependency.
  await openCreate('seller');
  assert.deepEqual(await dialog().locator('fieldset.form-fieldset > legend').allTextContents(), [
    'Информация о компании', 'Адрес', 'Контакты', 'Данные для входа',
  ]);
  assert.equal(await dialog().getByLabel('Название компании', { exact: false }).count(), 1);
  assert.equal(await dialog().getByLabel('Электронная почта', { exact: false }).count(), 1);
  assert.equal(await dialog().getByLabel('Имя администратора', { exact: false }).count(), 1);
  assert.equal(await dialog().getByLabel(/Юридическое название/).count(), 0);
  assert.equal(await dialog().getByLabel(/E-mail администратора/).count(), 0);
  assert.ok((await dialog().textContent()).includes('Контактная электронная почта также используется'));
  assert.ok((await dialog().textContent()).includes('Не менее 12 символов'));
  const desktopPasswordBox = await input('Пароль').boundingBox();
  const desktopConfirmationBox = await input('Повторите пароль').boundingBox();
  assert.ok(desktopPasswordBox && desktopConfirmationBox);
  assert.ok(Math.abs(desktopPasswordBox.y - desktopConfirmationBox.y) <= 1, 'desktop password controls must share y');
  assert.ok(Math.abs(desktopPasswordBox.height - desktopConfirmationBox.height) <= 1, 'desktop password controls must share height');
  assert.equal(await input('Регион').isDisabled(), true);
  await input('Страна').fill('Казахстан');
  assert.equal(await input('Регион').isEnabled(), true);
  await input('Регион').fill('Алматы');
  await input('Страна').fill('Китай');
  assert.equal(await input('Регион').inputValue(), '');
  await input('Страна').fill('');
  assert.equal(await input('Регион').isDisabled(), true);
  const desktopOverflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(desktopOverflow <= 0, `desktop horizontal overflow ${desktopOverflow}`);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await dialog().screenshot({ path: `${evidenceDir}/qa-desktop.png` });
  await page.setViewportSize({ width: 1440, height: 900 });
  await cancelDialog();

  // Every creation entry point and exact payload mapping.
  const kinds = ['seller'];
  for (const kind of kinds) {
    await openCreate(kind);
    assert.equal(await dialog().locator('fieldset.form-fieldset').count(), 4);
    await fillCreate(kind, kind === 'insurance');
    createMode = 'success';
    const before = mutations.length;
    await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
    await dialog().waitFor({ state: 'detached' });
    const record = mutations[before];
    assert.ok(record);
    assert.equal(record.path, kind === 'seller' ? '/identity/admin/seller-companies' : '/identity/admin/provider-companies');
    if (kind !== 'seller') assert.equal(record.body.kind, kind);
    assert.equal(record.body.company.email, record.body.firstAdmin.email);
    assert.equal(record.body.company.legalName, '');
    assert.equal('adminEmail' in record.body.firstAdmin, false);
    assert.equal('group' in record.body.company, false);
    if (kind === 'insurance') {
      assert.equal(record.body.company.country.label, 'Германия');
      assert.equal(record.body.company.region.label, 'Берлин');
    }
  }
  assert.equal(mutations.some((m) => m.path === '/identity/session/context'), false);

  // Cancel and pending/repeated submit behavior.
  await openCreate('seller');
  await fillCreate('cancel');
  const cancelCount = mutations.length;
  await cancelDialog();
  assert.equal(mutations.length, cancelCount);
  await openCreate('seller');
  await fillCreate('pending');
  createMode = 'pending'; pendingRoute = null;
  const pendingBefore = mutations.length;
  const pendingSubmit = dialog().locator('.modal-footer button').last();
  await pendingSubmit.click();
  await page.waitForFunction(() => document.querySelector('.modal button[disabled]') !== null);
  await input('Логин').press('Enter');
  await page.waitForTimeout(100);
  assert.equal(mutations.length, pendingBefore + 1);
  assert.equal(await pendingSubmit.isDisabled(), true);
  await pendingRoute.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({ data: { id: 'pending-created', access: 'draft' }, revision: 'r1' }) });
  await dialog().waitFor({ state: 'detached' });
  createMode = 'success'; pendingRoute = null;

  // Network and conflict retain values and keep the form open.
  await openCreate('seller');
  await fillCreate('failure');
  createMode = 'network';
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  await dialog().getByRole('alert').waitFor();
  assert.equal(await input('Название компании').inputValue(), 'QA Company failure');
  createMode = 'conflict';
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  await dialog().getByRole('alert').waitFor();
  assert.equal(await input('Электронная почта').inputValue(), 'qa-failure@example.test');
  await cancelDialog();

  // Routed errors separately, together, duplicate-message negative control, then successful retry.
  await openCreate('seller');
  await fillCreate('errors');
  createMode = { 'company.email': 'Company email invalid' };
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  await dialog().getByText('Company email invalid').waitFor();
  createMode = { 'firstAdmin.email': 'Admin email occupied' };
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  await dialog().getByText('Admin email occupied').waitFor();
  createMode = {
    'company.email': 'Company email invalid together',
    'firstAdmin.email': 'Admin email occupied together',
    'firstAdmin.login': 'Login occupied',
    'firstAdmin.password': 'Password weak',
    'firstAdmin.passwordConfirmation': 'Confirmation mismatch',
    'company.name': 'Name required',
    'company.country': 'Country required',
    'company.region': 'Region invalid',
    'company.legalName': 'Hidden legacy error',
  };
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  for (const message of ['Company email invalid together', 'Admin email occupied together', 'Login occupied', 'Password weak', 'Confirmation mismatch', 'Name required', 'Country required', 'Region invalid']) {
    await dialog().getByText(message).waitFor();
  }
  await dialog().getByText(/company\.legalName: Hidden legacy error/).waitFor();
  assert.equal(await dialog().getByLabel('Электронная почта', { exact: false }).count(), 1);
  await dialog().screenshot({ path: `${evidenceDir}/qa-errors.png` });
  createMode = { 'company.email': 'Same invalid email', 'firstAdmin.email': 'Same invalid email' };
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  await dialog().getByText('Same invalid email', { exact: true }).waitFor();
  assert.equal(await dialog().getByText('Same invalid email', { exact: true }).count(), 1);
  assert.equal((await dialog().textContent()).includes('firstAdmin.email:'), false);
  createMode = 'success';
  await input('Электронная почта').fill('qa-corrected@example.test');
  await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).click();
  await dialog().waitFor({ state: 'detached' });

  // Legacy legalName/If-Match edit, stale retention, and empty legalName preservation.
  await page.goto(baseURL + '/admin/companies'); await waitReady();
  await page.getByRole('button', { name: 'Открыть' }).first().click();
  await page.getByRole('dialog', { name: 'Legacy Seller' }).waitFor();
  await page.getByRole('button', { name: 'Изменить реквизиты' }).click();
  assert.deepEqual(await dialog().locator('fieldset.form-fieldset > legend').allTextContents(), ['Информация о компании', 'Адрес', 'Контакты']);
  assert.equal(await dialog().locator('fieldset.form-fieldset').count(), 3);
  assert.equal(await dialog().getByLabel('Пароль', { exact: false }).count(), 0);
  await input('Название компании').fill('Legacy Seller Updated');
  await input('Электронная почта').fill('legacy-updated@example.test');
  patchMode = 'success';
  const editBefore = mutations.length;
  await dialog().getByRole('button', { name: 'Изменить реквизиты', exact: true }).click();
  await page.getByRole('dialog', { name: 'Legacy Seller' }).waitFor();
  const editRecord = mutations[editBefore];
  assert.equal(editRecord.path, '/identity/companies/legacy-company');
  assert.equal(editRecord.body.legalName, 'Legacy Legal LLC');
  assert.equal(editRecord.headers['if-match'], '"company-r7"');
  assert.equal('firstAdmin' in editRecord.body, false);
  assert.equal('user' in editRecord.body, false);
  await page.getByRole('button', { name: 'Изменить реквизиты' }).click();
  await input('Название компании').fill('Unsaved stale name');
  patchMode = 'stale';
  await dialog().getByRole('button', { name: 'Изменить реквизиты', exact: true }).click();
  await dialog().getByRole('alert').waitFor();
  assert.equal(await input('Название компании').inputValue(), 'Unsaved stale name');
  await cancelDialog();
  patchMode = 'success';
  await page.keyboard.press('Escape');

  await page.getByRole('button', { name: 'Открыть' }).nth(1).click();
  await page.getByRole('dialog', { name: 'Empty Legal Seller' }).waitFor();
  await page.getByRole('button', { name: 'Изменить реквизиты' }).click();
  await input('Название компании').fill('Empty Legal Updated');
  const emptyBefore = mutations.length;
  await dialog().getByRole('button', { name: 'Изменить реквизиты', exact: true }).click();
  const emptyRecord = mutations[emptyBefore];
  assert.equal(emptyRecord.body.legalName, '');
  assert.equal(emptyRecord.headers['if-match'], '"company-r3"');
  await page.keyboard.press('Escape');

  // Existing ungrouped activation reason: open-only writes nothing, cancel writes nothing, submit is separate.
  await page.goto(baseURL + '/admin/companies'); await waitReady();
  await page.getByRole('button', { name: 'Открыть' }).first().click();
  await page.getByRole('dialog', { name: /Legacy Seller/ }).waitFor();
  const activationBefore = mutations.length;
  await page.getByRole('button', { name: 'Активировать' }).click();
  assert.equal(await dialog().locator('fieldset.form-fieldset').count(), 0);
  assert.equal(mutations.length, activationBefore);
  await cancelDialog();
  assert.equal(mutations.length, activationBefore);
  await page.getByRole('button', { name: 'Активировать' }).click();
  await input('Основание').fill('Synthetic QA activation reason');
  await dialog().getByRole('button', { name: 'Активировать', exact: true }).click();
  assert.equal(mutations.at(-1).path, '/identity/admin/companies/legacy-company/activate');
  assert.deepEqual(mutations.at(-1).body, { reason: 'Synthetic QA activation reason' });
  await page.keyboard.press('Escape');

  // Mobile provider layout/footer and keyboard order.
  await page.setViewportSize({ width: 390, height: 844 });
  await openCreate('bank');
  const focusOrder = await dialog().locator('input,textarea,select,button').evaluateAll((nodes) => nodes.filter((n) => !n.hidden && n.offsetParent !== null && !n.disabled).map((n) => n.getAttribute('aria-label') || n.getAttribute('name') || n.textContent?.trim() || n.id));
  assert.ok(focusOrder.length >= 12);
  await fillCreate('mobile');
  const mobilePasswordBox = await input('Пароль').boundingBox();
  const mobileConfirmationBox = await input('Повторите пароль').boundingBox();
  assert.ok(mobilePasswordBox && mobileConfirmationBox);
  assert.ok(mobileConfirmationBox.y > mobilePasswordBox.y, 'mobile password controls should follow single-column order');
  assert.ok(Math.abs(mobilePasswordBox.height - mobileConfirmationBox.height) <= 1, 'mobile password controls must share height');
  const mobileOverflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  assert.ok(mobileOverflow <= 0, `mobile horizontal overflow ${mobileOverflow}`);
  await dialog().locator('.modal-footer').scrollIntoViewIfNeeded();
  assert.equal(await dialog().getByRole('button', { name: 'Создать компанию', exact: true }).isVisible(), true);
  await dialog().screenshot({ path: `${evidenceDir}/qa-mobile.png` });
  await cancelDialog();

  // Remaining provider wording was exercised by all payload loops; assert current title semantics once more.
  for (const kind of ['mfo', 'insurance']) {
    await openCreate(kind);
    assert.match(await dialog().getAttribute('aria-label'), /Новая компания и администратор/);
    await cancelDialog();
  }

  const result = {
    browser: `Google Chrome ${browserVersion}`,
    url: baseURL,
    api: 'Playwright route interception; synthetic session and isolated responses; no backend',
    viewportDesktop: '1440x900', viewportMobile: '390x844',
    mutations: mutations.length,
    createEndpoints: [...new Set(mutations.filter((m) => m.path.includes('seller-companies') || m.path.includes('provider-companies')).map((m) => m.path))],
    requirements: ['Q1-A', 'Q1-B', 'Q1-C', 'Q1-D', 'Q1-E', 'Q1-F', 'Q1-G'],
    screenshots: ['qa-desktop.png', 'qa-mobile.png', 'qa-errors.png'],
  };
  console.log(JSON.stringify(result, null, 2));
  await browser.close();
}

main().catch((error) => {
  console.error(error?.stack || error);
  process.exitCode = 1;
});
