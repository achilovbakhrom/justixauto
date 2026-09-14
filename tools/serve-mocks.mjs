import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../docs/justix-auto/mocks');
const manifest=JSON.parse(await fs.readFile(path.join(root,'manifest.json'),'utf8'));
const allowed=new Set(manifest.files.filter(f=>f.kind==='runtime').map(f=>f.path));
const types={'.html':'text/html; charset=utf-8','.js':'text/javascript; charset=utf-8','.css':'text/css; charset=utf-8','.svg':'image/svg+xml','.png':'image/png','.jpg':'image/jpeg','.jpeg':'image/jpeg','.webp':'image/webp'};
const port=Number(process.env.MOCK_PORT||4180);
http.createServer(async(req,res)=>{
  try{
    if(!['GET','HEAD'].includes(req.method)){res.writeHead(405);return res.end();}
    let rel=decodeURIComponent(new URL(req.url,'http://localhost').pathname).replace(/^\//,'');
    if(!rel||rel.endsWith('/'))rel+='index.html';
    if(!allowed.has(rel)){res.writeHead(404);return res.end('Not found');}
    const data=await fs.readFile(path.join(root,rel));
    res.writeHead(200,{'Content-Type':types[path.extname(rel)]||'application/octet-stream','Cache-Control':'no-store','X-Content-Type-Options':'nosniff'});
    res.end(req.method==='HEAD'?undefined:data);
  }catch{res.writeHead(400);res.end('Bad request');}
}).listen(port,'127.0.0.1',()=>console.log(`Mock reference only: http://127.0.0.1:${port}/ (finance/, insurance/, admin/). No backend.`));
