const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const domain = vm.runInNewContext(fs.readFileSync(path.join(__dirname,'realization-domain.js'),'utf8') + '\nRealization;');
function fixture() {
  return {
    partners: [{id:'P1',name:'Test Partner',relationship:'active'},{id:'P2',relationship:'outgoing'}],
    vehicles: [{id:'V1',status:'available',warehouse:'Test Warehouse'}, {id:'V2',status:'reserved',warehouse:'Test Warehouse',reserve:'Retail sale'}],
    warehouses: [{name:'Test Warehouse',vin:2}],
    listings:[{vehicleId:'V1',status:'published'}],wholesaleOffers:[],wholesaleOrders:[]
  };
}
const input = {partnerId:'P1',vehicleIds:['V1'],price:100,date:'2026-09-10',title:'Test offer',note:''};
test('offer draft and publication do not reserve inventory',()=>{
  const store=fixture();const offer=domain.saveOffer(store,input);
  assert.equal(offer.status,'draft');domain.publish(store,offer.id);
  assert.equal(offer.status,'published');assert.equal(store.vehicles[0].status,'available');
});
test('only active partners can receive offers or orders',()=>{
  const store=fixture();assert.throws(()=>domain.createOrder(store,{...input,partnerId:'P2'}));
  assert.equal(store.wholesaleOrders.length,0);assert.equal(store.vehicles[0].status,'available');
});
test('publication rejects inventory reserved since draft creation',()=>{
  const store=fixture();const offer=domain.saveOffer(store,input);store.vehicles[0].status='reserved';
  assert.throws(()=>domain.publish(store,offer.id));assert.equal(offer.status,'draft');
});
test('wholesale reservation uses the same vehicle as retail',()=>{
  const store=fixture();domain.createOrder(store,input);
  assert.equal(store.vehicles.filter(v=>v.status==='available').length,0);
  assert.throws(()=>domain.createOrder(store,input));assert.equal(store.wholesaleOrders.length,1);
});
test('invalid batch produces no partial reservation',()=>{
  const store=fixture();assert.throws(()=>domain.createOrder(store,{...input,vehicleIds:['V1','V2']}));
  assert.equal(store.vehicles[0].status,'available');assert.equal(store.wholesaleOrders.length,0);
});
test('cancellation releases the shared stock, but not paid orders',()=>{
  const store=fixture();const first=domain.createOrder(store,input);domain.cancel(store,first);
  assert.equal(first.status,'cancelled');assert.equal(store.vehicles[0].status,'available');
  const second=domain.createOrder(store,input);domain.pay(second,10);
  assert.throws(()=>domain.cancel(store,second));assert.equal(store.vehicles[0].status,'reserved');
});
test('handover requires full payment and updates stock and retail listing once',()=>{
  const store=fixture();const order=domain.createOrder(store,input);
  assert.throws(()=>domain.handover(store,order));assert.throws(()=>domain.pay(order,101));
  domain.pay(order,40);domain.pay(order,60);domain.handover(store,order);
  assert.equal(order.status,'completed');assert.equal(store.vehicles[0].status,'sold');
  assert.equal(store.warehouses[0].vin,1);assert.equal(store.listings[0].status,'sold');
  assert.throws(()=>domain.handover(store,order));assert.equal(store.warehouses[0].vin,1);
});
test('payment accumulates cents without floating point residue',()=>{
  const store=fixture();const order=domain.createOrder(store,{...input,price:.3});
  domain.pay(order,.1);domain.pay(order,.2);assert.equal(order.paid,.3);
});
