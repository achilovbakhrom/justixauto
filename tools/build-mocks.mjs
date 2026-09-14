import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { minify as jsMinify } from 'terser';
import { minify as htmlMinify } from 'html-minifier-terser';
import CleanCSS from 'clean-css';

const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source=process.argv[2];
if(!source)throw Error('Usage: npm run mocks:build -- /absolute/path/to/original/prototype');
const sourceRoot=await fs.realpath(source);
const output=path.join(root,'docs/justix-auto/mocks');
if(sourceRoot===output||sourceRoot.startsWith(output+path.sep))throw Error('Source must be the original, not the generated bundle.');
try{await fs.stat(output);throw Error('Output already exists. Preserve/review it before rebuilding; this command never overwrites a bundle.');}catch(error){if(error.code!=='ENOENT')throw error;}
const entries=['index.html','finance/index.html','insurance/index.html','admin/index.html'];
for(const entry of entries)await fs.access(path.join(sourceRoot,entry));
const digest=b=>createHash('sha256').update(b).digest('hex');
const files=[];
async function walk(dir,relative=''){
  for(const item of (await fs.readdir(dir,{withFileTypes:true})).sort((a,b)=>a.name.localeCompare(b.name))){
    if(item.name.startsWith('.')||item.name==='node_modules')continue;
    const rel=path.posix.join(relative,item.name);
    if(item.isSymbolicLink())throw Error('Unexpected symlink: '+rel);
    if(item.isDirectory()){await walk(path.join(dir,item.name),rel);continue;}
    if(!/\.(html|js|css|svg|png|jpg|jpeg|webp|woff2?|cjs)$/.test(rel))continue;
    const original=await fs.readFile(path.join(dir,item.name));let result=original;
    if(rel.endsWith('.js')){
      // Cross-file classic scripts deliberately retain their names and execution order.
      const minified=await jsMinify(original.toString(),{compress:false,mangle:{toplevel:false},keep_fnames:true,keep_classnames:true,format:{comments:/@license|@preserve|^!/}});
      result=Buffer.from(minified.code+'\n');
    }else if(rel.endsWith('.css')){
      const minified=new CleanCSS({level:1,rebase:false}).minify(original.toString());
      if(minified.errors.length)throw Error(rel+': '+minified.errors.join('; '));
      result=Buffer.from(minified.styles+'\n');
    }else if(rel.endsWith('.html')){
      result=Buffer.from(await htmlMinify(original.toString(),{collapseWhitespace:true,conservativeCollapse:true,removeComments:true,minifyJS:false,minifyCSS:false})+'\n');
    }
    await fs.mkdir(path.dirname(path.join(output,rel)),{recursive:true});
    await fs.writeFile(path.join(output,rel),result,{flag:'wx'});
    files.push({path:rel,sourceBytes:original.length,bytes:result.length,sourceSha256:digest(original),sha256:digest(result),kind:rel.endsWith('.test.cjs')?'regression-test':'runtime'});
  }
}
await walk(sourceRoot);
const manifest={schema:1,source:sourceRoot,entries,tools:{terser:'5.44.0',cleanCss:'5.3.3',htmlMinifierTerser:'7.2.0'},policy:'Per-file minification with local variable mangling only; no JS compression, top-level/property/function/class name mangling; shared files and script order retained; originals untouched.',files};
await fs.writeFile(path.join(output,'manifest.json'),JSON.stringify(manifest,null,2)+'\n',{flag:'wx'});
const sum=key=>files.reduce((n,f)=>n+f[key],0);
console.log(JSON.stringify({files:files.length,sourceBytes:sum('sourceBytes'),outputBytes:sum('bytes'),savedPercent:((1-sum('bytes')/sum('sourceBytes'))*100).toFixed(1),output},null,2));
