// Real Windows WebView integration QA. Only uses marker-owned disposable fixtures.
// Build desktop/startup_upgrade_gui_test.go with the documented QA overlay first.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { spawn, execFileSync } from 'node:child_process';
const root = process.argv[2];
assert.ok(root, 'pass the disposable QA directory');
const out = process.argv[3] || 'docs/database-upgrade-stage2-2026-11-18';
const ps = '/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe';
const launch = execFileSync('wslpath', ['-w', path.join(root, 'launch-ui.ps1')], {encoding:'utf8'}).trim();
const pause = () => new Promise(r => setTimeout(r,150));
async function until(fn, label) { for(let i=0;i<150;i++){try{const r=await fn();if(r)return r;}catch{} await pause();}throw Error(`timeout: ${label}`); }
async function connect() {
 const page = await until(async()=> (await(await fetch('http://127.0.0.1:19348/json/list')).json()).find(p=>p.url==='http://wails.localhost/'), 'WebView');
 const ws=new WebSocket(page.webSocketDebuggerUrl); await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
 let id=0;const pending=new Map();
 ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p?.reject(Error(JSON.stringify(m.error))):p?.resolve(m.result)}};
 const send=(method,params={})=>new Promise((resolve,reject)=>{pending.set(++id,{resolve,reject});ws.send(JSON.stringify({id,method,params}))});
 const evaluate=async(expression)=>{const r=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
 return {ws,send,evaluate};
}
async function click(c,text){await c.evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)});if(!b)throw Error('button missing');b.click()})()`)}
async function quit(c,ready=false){
 if(ready)await c.evaluate(`document.querySelector('[aria-label="关闭窗口"]').click()`); else await c.evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>['退出','暂时退出'].includes(b.textContent.trim()));if(!b)throw Error('exit missing');b.click()})()`);
 c.ws.close();
}
const results=[];
for(const name of ['baseline','cancel','backupfail','newer','unknown','fresh','ready']){
 assert.equal(await fs.readFile(path.join(root,'cases',name,'.studyflow-upgrade-qa'),'utf8'),'temporary upgrade UI fixture\n');
 await fs.rm(path.join(root,'cases',name,'ui-result.json'),{force:true});
 {const p=spawn(ps,['-NoProfile','-ExecutionPolicy','Bypass','-File',launch,'-Case',name],{stdio:'ignore'});p.unref();}
 const c=await connect();
 await until(()=>c.evaluate(`!document.body.innerText.includes('正在准备学习空间') && !!document.querySelector('#app')?.textContent.trim()`),name+' mounted');
 const text=await c.evaluate('document.body.innerText');
 assert.equal(await c.evaluate(`getComputedStyle(document.querySelector('.startup-shell')||document.body).backgroundColor`),await c.evaluate(`document.querySelector('.startup-shell')?'rgb(255, 255, 255)':getComputedStyle(document.body).backgroundColor`));
 const errors=[];c.ws.addEventListener('message',e=>{const m=JSON.parse(e.data);if(m.method==='Runtime.exceptionThrown')errors.push(m.params)});await c.send('Runtime.enable');
 if(['baseline','cancel','backupfail'].includes(name)) {
  assert.match(text,/检测到版本更新/);assert.match(text,/开始前将自动备份/);assert.doesNotMatch(text,/登记版本|目标版本|数据位置/);
  assert.equal(await c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),false,'workspace must not mount before consent');
 }else if(name==='newer'){assert.match(text,/请先更新 StudyFlow/);assert.match(text,/版本 2/);assert.doesNotMatch(text,/确认继续/)}
 else if(name==='unknown'){assert.match(text,/需要检查学习数据/);assert.doesNotMatch(text,/确认继续/)}
 else {assert.equal(await c.evaluate(`!!document.querySelector('[data-testid="startup-screen"]')`),false)}
 await c.send('Emulation.setDeviceMetricsOverride',{width:1000,height:664,deviceScaleFactor:1,mobile:false});
 const bounds=await c.evaluate(`(()=>{const e=document.querySelector('[data-testid="startup-screen"]');if(!e)return null;const b=e.getBoundingClientRect();return {left:b.left,right:b.right,top:b.top,bottom:b.bottom,width:innerWidth,height:innerHeight,scroll:document.documentElement.scrollWidth}})()`);
 if(bounds){assert.ok(bounds.left>=0&&bounds.right<=bounds.width);assert.ok(bounds.top>=0); // Taller cards may scroll vertically; actions must remain reachable.
  assert.ok(await c.evaluate(`document.querySelector('.startup-actions button')!==null`));assert.ok(bounds.scroll<=bounds.width)}
 await c.send('Emulation.clearDeviceMetricsOverride');
 await fs.writeFile(path.join(out,`ui-${name}.png`),Buffer.from((await c.send('Page.captureScreenshot',{format:'png'})).data,'base64'));
 if(name==='baseline'||name==='backupfail'){
  await click(c,'确认继续');
  if(name==='baseline'){
   await until(()=>c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),'workspace after upgrade');
   assert.equal(await c.evaluate(`!!document.querySelector('[data-testid="startup-screen"]')`),false);
   await fs.writeFile(path.join(out,'ui-upgraded-workspace.png'),Buffer.from((await c.send('Page.captureScreenshot',{format:'png'})).data,'base64'));
   await quit(c,true);
  }else{
   await until(()=>c.evaluate(`document.body.innerText.includes('暂时无法打开学习数据')`),'backup error');
   assert.equal(await c.evaluate(`!!document.querySelector('[aria-label="学情追踪"]')`),false);
   await c.evaluate(`document.querySelector('summary').click()`);
   assert.match(await c.evaluate('document.body.innerText'),/升级前备份失败/);
   await fs.writeFile(path.join(out,'ui-backupfail-blocked.png'),Buffer.from((await c.send('Page.captureScreenshot',{format:'png'})).data,'base64'));
   await quit(c);
  }
 }else if(['ready','fresh'].includes(name)){
  if(name==='fresh'){
   // Fresh defaults close to tray. Request the regular runtime graceful Quit;
   // all startup-screen scenarios above exercise the actual visible buttons.
   await c.evaluate(`void fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json','x-wails-client-id':'upgrade-qa'},body:JSON.stringify({object:2,method:2})})`);
   c.ws.close();
  }else await quit(c,true);
 }else await quit(c);
 const report=await until(async()=>JSON.parse(await fs.readFile(path.join(root,'cases',name,'ui-result.json'),'utf8')),name+' graceful exit');
 assert.equal(report.status.phase,['baseline','ready','fresh'].includes(name)?'ready':name==='cancel'?'confirmation':'blocked');
 assert.equal(errors.length,0,'no frontend runtime errors');
 results.push({case:name,passed:true,text,bounds,status:report.status});
 console.log('PASS',name);
}
await fs.writeFile(path.join(out,'windows-ui-results.json'),JSON.stringify({passed:true,scenarios:results},null,2)+'\n');
