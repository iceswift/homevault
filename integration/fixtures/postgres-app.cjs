// Synthetic PostgreSQL-backed HTTP application; not Saan or production auth.
const {createRequire} = require('node:module');
const {createServer} = require('node:http');
const {readFileSync} = require('node:fs');
const {randomBytes,createHash,timingSafeEqual} = require('node:crypto');
const {Pool} = createRequire('/app/package.json')('pg');
const pool = new Pool({host:'127.0.0.1',port:5432,user:'postgres',database:'recovery',max:2});
const sessions = new Set();
createServer(async (req,res) => {
  try {
    if(req.method==='GET' && req.url==='/health') {
      const {rows}=await pool.query('SELECT marker FROM proof');
      res.end(JSON.stringify({status:rows.length===1?'ok':'failed'}));
    } else if(req.method==='GET' && req.url==='/media') {
      res.end(readFileSync('/data/media/proof.txt'));
    } else if(req.method==='POST' && req.url==='/login') {
      let body='';for await (const chunk of req) {body+=chunk;if(body.length>1024)throw Error('limit');}
      const {rows}=await pool.query('SELECT password_hash FROM accounts WHERE username=$1',['fixture']);
      const actual=createHash('sha256').update(body).digest();
      if(rows.length!==1 || !timingSafeEqual(actual,Buffer.from(rows[0].password_hash,'hex'))) {res.writeHead(401);return res.end('{}');}
      const token=randomBytes(24).toString('hex');sessions.add(token);
      res.setHeader('Set-Cookie',`session=${token}; HttpOnly; SameSite=Strict; Path=/`);
      res.end('{"authenticated":true}');
    } else if(req.method==='GET' && req.url==='/dashboard') {
      const token=(req.headers.cookie||'').match(/(?:^|;\s*)session=([a-f0-9]+)/)?.[1];
      if(!sessions.has(token)){res.writeHead(401);return res.end('{}');}
      const {rows}=await pool.query('SELECT marker FROM proof');
      res.end(JSON.stringify({marker:rows[0].marker}));
    } else {res.writeHead(404);res.end('{}');}
  } catch {res.writeHead(500);res.end('{}');}
}).listen(8000,'127.0.0.1');
