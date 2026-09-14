const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const photos = vm.runInNewContext(fs.readFileSync(path.join(__dirname,'vehicle-photos.js'),'utf8')+'\nVehiclePhotos;', {document:{addEventListener(){}},icon:()=>''});
test('existing image initializes exterior once; interior stays empty',()=>{
  const vehicle={id:'V1',image:'car.svg'};
  assert.equal(photos.groups(vehicle).exterior.length,1);
  assert.equal(photos.groups(vehicle).interior.length,0);
  photos.section(vehicle);photos.section(vehicle);
  assert.equal(vehicle.photos.exterior.length,1);
});
test('missing image creates two empty categories',()=>{
  const vehicle={id:'V2'};
  assert.equal(photos.groups(vehicle).exterior.length,0);
  assert.equal(photos.groups(vehicle).interior.length,0);
});
test('adding interior does not alter exterior or status evidence',()=>{
  const vehicle={id:'V3',image:'car.svg',statusPhotos:2};
  photos.add(vehicle,'interior',[{name:'Seats',url:'seats.png'}]);
  assert.equal(vehicle.photos.interior[0].url,'seats.png');
  assert.equal(vehicle.photos.exterior[0].url,'car.svg');
  assert.equal(vehicle.statusPhotos,2);
  assert.match(photos.section(vehicle),/aria-selected="true" data-photo-tab="interior"/);
});
test('gallery safely escapes filenames',()=>{
  const vehicle={id:'V4'};
  photos.add(vehicle,'exterior',[{name:'<script>test</script>',url:'car.png'}]);
  assert.ok(!photos.section(vehicle).includes('<script>'));
  assert.match(photos.section(vehicle),/&lt;script&gt;/);
});
