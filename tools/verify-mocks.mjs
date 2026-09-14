import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import vm from 'node:vm';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../docs/justix-auto/mocks');
const manifest=JSON.parse(await fs.readFile(path.join(root,'manifest.json'),'utf8'));
const resolveLocal=(file,ref)=>{
  if(/^(?:[a-z]+:|#|\/\/)/i.test(ref))return null;
  const target=path.resolve(path.dirname(path.join(root,file)),ref.split(/[?#]/)[0]);
  if(!target.startsWith(root+path.sep))throw Error('Outside mock root: '+ref);
  return target;
};
for(const file of manifest.files){
  const data=await fs.readFile(path.join(root,file.path));
  if(createHash('sha256').update(data).digest('hex')!==file.sha256)throw Error('Hash mismatch: '+file.path);
  if(file.path.endsWith('.js'))new vm.Script(data.toString(),{filename:file.path});
  if(file.path.endsWith('.html'))for(const match of data.toString().matchAll(/(?:src|href)="([^"]+)"/g)){
    const target=resolveLocal(file.path,match[1]);if(target)await fs.access(target);
  }
  if(file.path.endsWith('.css'))for(const match of data.toString().matchAll(/url\(["']?([^\s)'";]+)["']?\)/g)){
    const target=resolveLocal(file.path,match[1]);if(target)await fs.access(target);
  }
}
for(const entry of manifest.entries)await fs.access(path.join(root,entry));
console.log(`PASS: ${manifest.files.length} hashes, JS syntax, local HTML/CSS references and four app entry points.`);
