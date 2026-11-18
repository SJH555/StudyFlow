// Real Windows application interruptions during actual upgrade SQL, then restart.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import {spawn,execFileSync} from 'node:child_process';
const root=process.argv[2];assert.ok(root);
const out='docs/database-upgrade-interruption-2026-11-18';
const ps='/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe';
const win=p=>execFileSync('wslpath',['-w',p],{encoding:'utf8'}).trim();
const pause=()=>new Promise(r=>setTimeout(r,100));
async function until(fn,label){for(let i=0;i<250;i++){try{const v=await fn();if(v)return v}catch{}await pause()}throw Error('timeout: '+label)}
async function connect(){
 const page=await until(async()=> (await(await fetch('http://127.0.0.1:19348/json/list')).json()).find(p=>p.url==='http://wails.localhost/'),'WebView');
 const ws=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
 let id=0;const pending=new Map();
 ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p?.reject(Error(JSON.stringify(m.error))):p?.resolve(m.result)}};
 const send=(method,params={})=>new Promise((resolve,reject)=>{pending.set(++id,{resolve,reject});ws.send(JSON.stringify({id,method,params}))});
 const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
 return {ws,send,evaluate};
}
async function text(c){return c.evaluate('document.body.innerText')}
async function mouseClick(c,selector){
 const b=await c.evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});if(!e)throw Error('missing control');const b=e.getBoundingClientRect();return{x:b.x+b.width/2,y:b.y+b.height/2}})()`);
 await c.send('Input.dispatchMouseEvent',{type:'mousePressed',button:'left',clickCount:1,...b});
 await c.send('Input.dispatchMouseEvent',{type:'mouseReleased',button:'left',clickCount:1,...b});
}
async function launch(name){
 const dir=path.join(root,'cases',name);
 await fs.rm(path.join(dir,'ui-result.json'),{force:true});await fs.rm(path.join(dir,'checkpoint.txt'),{force:true});await fs.rm(path.join(dir,'process-id.txt'),{force:true});
 const p=spawn(ps,['-NoProfile','-ExecutionPolicy','Bypass','-File',win(path.join(root,'launch-ui.ps1')),'-Case',name],{stdio:'ignore'});p.unref();
 await until(async()=>Number(await fs.readFile(path.join(dir,'process-id.txt'),'utf8')),'owned process');
 const c=await connect();await until(async()=>{const s=await text(c);return s.includes('检测到版本更新')||await c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`)},'startup settled');return c;
}
async function closed(){await until(async()=>{try{const r=await fetch('http://127.0.0.1:19348/json/list');return !(await r.json()).length}catch{return true}},'WebView closed')}
async function killOwned(name){execFileSync(ps,['-NoProfile','-ExecutionPolicy','Bypass','-File',win(path.join(root,'kill-owned.ps1')),'-Case',name],{encoding:'utf8'});await closed()}
async function report(name){return until(async()=>JSON.parse(await fs.readFile(path.join(root,'cases',name,'ui-result.json'),'utf8')),'graceful exit report')}
function verify(name,version){return JSON.parse(execFileSync(ps,['-NoProfile','-ExecutionPolicy','Bypass','-File',win(path.join(root,'verify-native.ps1')),'-Case',name,'-Version',String(version)],{encoding:'utf8'}))}
async function shot(c,name){await fs.writeFile(path.join(out,name+'.png'),Buffer.from((await c.send('Page.captureScreenshot',{format:'png'})).data,'base64'))}
await fs.writeFile(path.join(root,'kill-owned.ps1'),`param([string]$Case)
$ErrorActionPreference='Stop'
$root=Join-Path $PSScriptRoot "cases\\$Case"
if ((Get-Content -LiteralPath (Join-Path $root '.studyflow-upgrade-qa') -Raw) -ne "temporary upgrade UI fixture\n") { throw 'not owned' }
$processId=[int](Get-Content -LiteralPath (Join-Path $root 'process-id.txt'))
$p=Get-Process -Id $processId
if ($p.Path -ne (Join-Path $PSScriptRoot 'upgrade-ui.exe')) { throw 'wrong process' }
Stop-Process -Id $processId -Force
`);
await fs.writeFile(path.join(root,'verify-native.ps1'),`param([string]$Case,[string]$Version)
$ErrorActionPreference='Stop'
$fixture=Join-Path $PSScriptRoot "cases\\$Case"
$outFile=Join-Path $fixture 'native-verify.json'
$errFile=Join-Path $fixture 'native-verify-error.txt'
$p=Start-Process -FilePath (Join-Path $PSScriptRoot 'upgrade-ui.exe') -ArgumentList @('--verify',$fixture,$Version) -WorkingDirectory $PSScriptRoot -RedirectStandardOutput $outFile -RedirectStandardError $errFile -Wait -PassThru
if ($p.ExitCode -ne 0) { Get-Content -LiteralPath $errFile; exit 1 }
Get-Content -LiteralPath $outFile -Raw
`);
const results=[];
for(const journal of ['delete','wal'])for(const mode of ['close','exit','kill','repeat','committed','sqlerror','full']){
 const name=mode+'-'+journal,dir=path.join(root,'cases',name),checks=[];
 assert.equal(await fs.readFile(path.join(dir,'.studyflow-upgrade-qa'),'utf8'),'temporary upgrade UI fixture\n');
 let c=await launch(name);
 assert.match(await text(c),/学习记录和设置会保留，开始前将自动备份/);
 assert.equal(await c.evaluate(`getComputedStyle(document.querySelector('.startup-shell')).backgroundColor`),'rgb(255, 255, 255)');
 await mouseClick(c,'.startup-actions button:last-child');
 if(['sqlerror','full'].includes(mode)){
  await until(async()=>(await text(c)).includes('暂时无法打开学习数据'),'failure screen');
  await c.evaluate(`document.querySelector('summary').click()`);
  const detail=await text(c);assert.match(detail,mode==='sqlerror'?/QA injected SQL function error/:/SQLITE_FULL|database or disk is full/i);
  assert.equal(await c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),false);
  checks.push(verify(name,2));await shot(c,'failure-'+name);
  await mouseClick(c,'.startup-actions button');await report(name);c.ws.close();await closed();
 }else{
  const phase=mode==='committed'?'after-commit':'in-sql';
  await until(async()=>(await fs.readFile(path.join(dir,'checkpoint.txt'),'utf8'))===phase,'SQL checkpoint '+name);
  assert.match(await text(c),/正在适配新版本，请稍候/);assert.match(await text(c),/完成后将自动进入应用，请勿关闭程序/);
  assert.equal(await c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),false);
  const sidecar=path.join(dir,'data/study.db'+(journal==='wal'?'-wal':'-journal'));
  if(mode!=='committed')assert.ok((await fs.stat(sidecar)).size>0,'dirty database sidecar before interruption');
  if(mode==='close'||mode==='exit'){
   await shot(c,'in-progress-'+name);
   await mouseClick(c,mode==='close'?'[aria-label="关闭窗口"]':'.startup-actions button');
   const r=await report(name);assert.equal(r.status.phase,'blocked');c.ws.close();await closed();checks.push(verify(name,2));
  }else{await killOwned(name);c.ws.close();assert.equal(await fs.stat(path.join(dir,'ui-result.json')).then(()=>true,()=>false),false,'forced kill must bypass cleanup')}
 }
 // Real startup must recover SQLite and inspect the persisted version before
 // opening any business UI. Do not open the hot DB with our verifier first.
 await fs.writeFile(path.join(dir,'resume'),'');
 c=await launch(name);
 if(mode==='committed'){
  assert.equal(await c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),true);checks.push(verify(name,3));
 }else{
  assert.match(await text(c),/检测到版本更新/);checks.push(verify(name,2));
  if(mode==='repeat'){
   await mouseClick(c,'.startup-actions button:last-child');
   await until(async()=>(await fs.readFile(path.join(dir,'checkpoint.txt'),'utf8'))==='in-sql','second interrupted attempt');
   await killOwned(name);c.ws.close();c=await launch(name);checks.push(verify(name,2));
  }
  await fs.writeFile(path.join(dir,'resume'),'');await fs.writeFile(path.join(dir,'release'),'');
  await mouseClick(c,'.startup-actions button:last-child');
  await until(()=>c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),'resume into workspace');checks.push(verify(name,3));
 }
 await shot(c,'recovered-'+name);
 await mouseClick(c,'[aria-label="关闭窗口"]');await report(name);c.ws.close();await closed();
 // Also restart the completed DB: no reapplication or duplicate version rows.
 await fs.writeFile(path.join(dir,'resume'),'');
 c=await launch(name);assert.equal(await c.evaluate(`!!document.querySelector('[data-testid="startup-screen"]')`),false);checks.push(verify(name,3));
 await mouseClick(c,'[aria-label="关闭窗口"]');await report(name);c.ws.close();await closed();
 results.push({case:name,passed:true,checks});await fs.writeFile(path.join(out,'windows-interruption-results.json'),JSON.stringify({passed:results.length===14,scenarios:results},null,2)+'\n');
 console.log('PASS',name);
}
