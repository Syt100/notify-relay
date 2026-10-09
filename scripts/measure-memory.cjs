const fs = require('node:fs');
const http = require('node:http');
const os = require('node:os');
const path = require('node:path');
const {spawn} = require('node:child_process');
const {once} = require('node:events');
const binary = path.resolve(__dirname,'../dist/notify-relay');
const pause = ms => new Promise(r=>setTimeout(r,ms));
const listen = server => new Promise(r=>server.listen(0,'127.0.0.1',r));
const close = server => new Promise(r=>server.close(r));
const fetchJSON = url => new Promise((resolve,reject)=>http.get(url,r=>{let b='';r.on('data',d=>b+=d);r.on('end',()=>{try{resolve(JSON.parse(b))}catch(e){reject(e)}})}).on('error',reject));
async function scenario(count) {
 const dir=fs.mkdtempSync(path.join(os.tmpdir(),'bridge-memory-'));
 const nt=http.createServer((req,res)=>{
  res.writeHead(200,{'content-type':'application/x-ndjson'});
  const time=Math.floor(Date.now()/1000);
  res.write(JSON.stringify({event:'open',time,topic:'notify'})+'\n');
  for(let i=0;i<count;i++)res.write(JSON.stringify({event:'message',id:'m'+String(i).padStart(6,'0'),time,topic:'notify',message:'x'.repeat(1024)})+'\n');
  const ticker=setInterval(()=>res.write(JSON.stringify({event:'keepalive',time,topic:'notify'})+'\n'),1000);
  req.on('close',()=>clearInterval(ticker));
 });
 const wx=http.createServer((req,res)=>{req.resume();res.writeHead(503);res.end('{}')});
 const freePort=http.createServer();await listen(freePort);const port=freePort.address().port;await close(freePort);
 await listen(nt);await listen(wx);
 const child=spawn(binary,[],{env:{...process.env,NTFY_BASE_URL:'http://127.0.0.1:'+nt.address().port,NTFY_TOPICS:'notify',NTFY_TOKEN:'tk_memory_test',WXPUSHER_SPT:'SPT_memory_test',WXPUSHER_API:'http://127.0.0.1:'+wx.address().port,STATE_DB:path.join(dir,'state.bolt'),HEALTH_PORT:String(port),GOMEMLIMIT:'32MiB',GOGC:'50',GOMAXPROCS:'1',LOG_LEVEL:'ERROR',NO_PROXY:'127.0.0.1'},stdio:['ignore','ignore','pipe']});
 let errors='';child.stderr.on('data',d=>errors+=d);
 try {
  let health;const deadline=Date.now()+15000;
  while(Date.now()<deadline){if(child.exitCode!==null)throw Error('Exited: '+errors);try{health=await fetchJSON('http://127.0.0.1:'+port+'/healthz');if(health.pending===count&&health.topics.notify.connected)break}catch{};await pause(100)}
  if(!health||health.pending!==count)throw Error('Scenario not ready');
  await pause(2000);
  health=await fetchJSON('http://127.0.0.1:'+port+'/healthz');
  const parentHostPID=Number(fs.readFileSync('/proc/self/status','utf8').match(/^Pid:\s+(\d+)/m)[1]);
  let status;
  for(const pid of fs.readdirSync('/proc').filter(x=>/^\d+$/.test(x))){
   try{const candidate=fs.readFileSync('/proc/'+pid+'/status','utf8');const pp=Number(candidate.match(/^PPid:\s+(\d+)/m)[1]);const ids=candidate.match(/^NSpid:\s+(.+)$/m)[1].trim().split(/\s+/).map(Number);if(pp===parentHostPID&&ids.at(-1)===child.pid){status=candidate;break}}catch{}
  }
  if(!status)throw Error('Cannot resolve child process status');
  const rssKiB=Number(status.match(/^VmRSS:\s+(\d+)/m)[1]);
  const peakKiB=Number(status.match(/^VmHWM:\s+(\d+)/m)[1]);
  const result={pending:health.pending,payloadBytesEach:count?1024:0,rssKiB,peakKiB,stateBytes:fs.statSync(path.join(dir,'state.bolt')).size};
  child.kill('SIGTERM');const [code,signal]=await once(child,'exit');if(code!==0)throw Error('Shutdown failed '+code+' '+signal+' '+errors);
  return result;
 } finally {
  if(child.exitCode===null)child.kill('SIGKILL');
  nt.closeAllConnections();wx.closeAllConnections();await close(nt);await close(wx);fs.rmSync(dir,{recursive:true,force:true});
 }
}
(async()=>{const results={date:new Date().toISOString(),platform:os.platform(),arch:os.arch(),binaryBytes:fs.statSync(binary).size,scenarios:[]};for(const n of [0,1000])results.scenarios.push(await scenario(n));fs.writeFileSync(path.resolve(__dirname,'../dist/memory-results.json'),JSON.stringify(results,null,2));console.log(JSON.stringify(results,null,2))})().catch(e=>{console.error(e);process.exitCode=1});
