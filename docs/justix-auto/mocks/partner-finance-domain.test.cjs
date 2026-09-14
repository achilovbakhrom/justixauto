const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const finance = vm.runInNewContext(fs.readFileSync(path.join(__dirname, 'partner-finance-domain.js'), 'utf8') + '\nPartnerFinance;');
const input = { providerType: 'bank', providerId: 'demo-bank', programId: 'bank-standard', price: 25400, downPayment: 5080, months: 24, firstDue: '2026-10-05', vehicleId: 'V1', customerId: 'C1', branch: 'Branch', owner: 'Manager', reserveUntil: '2026-09-10' };
function fixture() {
  return { company: 'Seller', companies: [{ id: 'COM1', name: 'Seller' }, { id: 'COM2', name: 'Other' }], branches: [{ companyId: 'COM1', name: 'Branch' }],
    sales: [], vehicles: [{ id: 'V1', status: 'available', model: 'BYD Song', vin: 'TESTVIN', price: '25 400 USD' }], customers: [{ id: 'C1', name: 'Customer' }], leads: [] };
}
test('fixed markup, down payment and exact schedule reconcile', () => {
  const { calculation: c } = finance.quote(input);
  assert.equal(c.priceCents, 2540000); assert.equal(c.markupCents, 457200);
  assert.equal(c.totalCents, 2997200); assert.equal(c.deferredCents, 2489200);
  assert.equal(c.monthlyCents, 103716); assert.equal(c.lastPaymentCents, 103732);
  assert.equal(c.schedule.length, 24); assert.equal(c.schedule.at(-1).balanceCents, 0);
  assert.equal(c.schedule.reduce((n, row) => n + row.paymentCents, 0) + c.downCents, c.totalCents);
});
test('term selects its own whole-term markup and not an annual interest rate', () => {
  const first = finance.quote({ ...input, months: 12 }).calculation;
  const second = finance.quote({ ...input, months: 36 }).calculation;
  assert.equal(first.markupBps, 1000); assert.equal(second.markupBps, 2500);
  assert.equal(first.markupCents, 254000); assert.equal(second.markupCents, 635000);
  assert.equal(second.annualRate, undefined);
});
test('cross-provider or cross-type programs and unsupported terms are rejected', () => {
  assert.throws(() => finance.quote({ ...input, providerType: 'mfo' }));
  assert.throws(() => finance.quote({ ...input, programId: 'mfo-murabaha' }));
  assert.throws(() => finance.quote({ ...input, months: 60 }));
  assert.throws(() => finance.quote({ ...input, providerId: '' }));
});
test('MFO has independent offers and program minimum deposit', () => {
  const mfo = { ...input, providerType: 'mfo', providerId: 'demo-mfo', programId: 'mfo-murabaha', downPayment: 7620 };
  assert.equal(finance.quote(mfo).calculation.markupBps, 2000);
  assert.throws(() => finance.quote({ ...mfo, downPayment: 5080 }));
  assert.throws(() => finance.quote({ ...input, programId: 'bank-large-deposit' }));
});
test('invalid/empty amounts, minimum deposit, cost and currency precision', () => {
  for (const downPayment of ['', -1, 100, NaN, Infinity, 25400, 99999999999999]) assert.throws(() => finance.quote({ ...input, downPayment }));
  for (const price of ['', 0, -1, Infinity]) assert.throws(() => finance.quote({ ...input, price }));
  const c = finance.quote({ ...input, price: 25400.01, downPayment: 5080.01 }).calculation;
  assert.equal(c.schedule.reduce((sum, row) => sum + row.paymentCents, 0) + c.downCents, c.totalCents);
});
test('month-end calendar keeps anchor and leap years without overflow', () => {
  assert.equal(finance.dueDate('2028-01-31', 1), '2028-02-29');
  assert.equal(finance.dueDate('2028-01-31', 2), '2028-03-31');
  assert.equal(finance.dueDate('2026-12-31', 2), '2027-02-28');
  for (const firstDue of ['', '2026-02-30', 'nonsense', '1800-01-01']) assert.throws(() => finance.quote({ ...input, firstDue }));
});
test('zero markup and irregular cents still reconcile for every supported term', () => {
  for (const months of [12, 24, 36, 48, 60]) {
    const c = finance.calculate({ price: 12345.67, downPayment: 2345.66, markupBps: 0, months, firstDue: '2026-10-31' });
    assert.equal(c.markupCents, 0);
    assert.equal(c.schedule.reduce((sum, row) => sum + row.paymentCents, 0), c.deferredCents);
    assert.ok(c.schedule.every(row => Number.isInteger(row.paymentCents) && row.balanceCents >= 0));
  }
});
test('save reserves shared VIN once, snapshot belongs to sale, editing preserves identity', () => {
  const store = fixture();
  const sale = finance.save(store, input);
  assert.equal(store.vehicles[0].status, 'reserved'); assert.equal(sale.type, 'partner-finance');
  assert.equal(sale.financing, null); assert.equal(sale.paid, '0 USD');
  assert.equal(sale.partnerFinance.status, 'draft');
  assert.throws(() => finance.save(store, input)); assert.equal(store.sales.length, 1);
  const updated = finance.save(store, { ...input, months: 12 }, sale.id);
  assert.equal(updated.id, sale.id); assert.equal(updated.partnerFinance.calculation.months, 12);
  assert.equal(store.sales.length, 1);
});
test('invalid save is atomic and rejects wrong company/branch/customer', () => {
  for (const change of [{ downPayment: 0 }, { customerId: 'missing' }, { branch: 'Other' }, { owner: '' }, { reserveUntil: '' }, { providerType: 'mfo' }]) {
    const store = fixture(); assert.throws(() => finance.save(store, { ...input, ...change }));
    assert.equal(store.sales.length, 0); assert.equal(store.vehicles[0].status, 'available');
  }
});
test('currency is not silently relabelled or converted to USD', () => {
  const store = fixture(); store.vehicles[0].price = '300 000 000 UZS';
  assert.throws(() => finance.save(store, input));
  assert.equal(store.vehicles[0].status, 'available');
});
test('demo submission requires explicit confirmation, cannot repeat or edit after send', () => {
  const store = fixture(); const sale = finance.save(store, input);
  assert.throws(() => finance.submitDemo(store, sale, false));
  finance.submitDemo(store, sale, true);
  assert.equal(sale.partnerFinance.status, 'submitted-demo'); assert.equal(sale.paid, '0 USD');
  assert.throws(() => finance.submitDemo(store, sale, true));
  assert.throws(() => finance.save(store, input, sale.id));
});
test('changed company or reservation cannot edit/submit another sale', () => {
  const store = fixture(); const sale = finance.save(store, input);
  store.company = 'Other'; assert.throws(() => finance.save(store, input, sale.id));
  assert.throws(() => finance.submitDemo(store, sale, true));
  store.company = 'Seller'; store.vehicles[0].reserve = 'Another sale';
  assert.throws(() => finance.save(store, input, sale.id));
  assert.throws(() => finance.submitDemo(store, sale, true));
});
