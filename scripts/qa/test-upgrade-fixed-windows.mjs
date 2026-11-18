// Acceptance against the fixed Windows directory's real production main and
// instance guard. Uses a build overlay only to enable CDP; no test DB/controller.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
const action=process.argv[2];assert.ok(['cancel','confirm','ready'].includes(action));
const out='docs/database-upgrade-stage3-2026-11-18';
const pause=()=>new Promise(r=>setTimeout(r,100));
async function until(fn,label){for(let i=0;i<200;i++){try{const r=await fn();if(r)return r}catch{}await pause()}throw Error('timeout '+label)}
const page=await until(async()=>(await(await fetch('http://127.0.0.1:19349/json/list')).json()).find(p=>p.url==='http://wails.localhost/'),'WebView');
const ws=new WebSocket(page.webSocketDebuggerUrl);await new Promise((r,j)=>{ws.onopen=r;ws.onerror=j});
let id=0;const pending=new Map();const errors=[];
ws.onmessage=e=>{const m=JSON.parse(e.data);if(m.id){const p=pending.get(m.id);pending.delete(m.id);m.error?p.reject(Error(JSON.stringify(m.error))):p.resolve(m.result)}if(m.method==='Runtime.exceptionThrown')errors.push(m.params)};
const send=(method,params={})=>new Promise((resolve,reject)=>{pending.set(++id,{resolve,reject});ws.send(JSON.stringify({id,method,params}))});
const evaluate=async expression=>{const r=await send('Runtime.evaluate',{expression,returnByValue:true,awaitPromise:true});if(r.exceptionDetails)throw Error(JSON.stringify(r.exceptionDetails));return r.result.value};
async function shot(name){await fs.writeFile(`${out}/${name}.png`,Buffer.from((await send('Page.captureScreenshot',{format:'png'})).data,'base64'))}
async function clickButton(text){await evaluate(`(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent.trim()===${JSON.stringify(text)});if(!b)throw Error('missing button');b.click()})()`)}
await send('Runtime.enable');
const report={action,passed:false};
try{
 await until(()=>evaluate(`!!document.querySelector('#app')?.textContent.trim()&&!document.body.innerText.includes('正在准备学习空间')`),'mounted');
 if(action!=='ready'){
  assert.match(await evaluate('document.body.innerText'),/检测到版本更新/);
  assert.equal(await evaluate(`getComputedStyle(document.querySelector('.startup-shell')).backgroundColor`),'rgb(255, 255, 255)');
  assert.equal(await evaluate(`!!document.querySelector('[aria-label="主导航"]')`),false);
  await shot('fixed-'+action+'-prompt');
  if(action==='cancel'){
   await clickButton('暂时退出');
   await until(async()=>{try{return !(await(await fetch('http://127.0.0.1:19349/json/list')).json()).length}catch{return true}},'normal quit');
  }else{
   await clickButton('确认继续');
   await until(()=>evaluate(`!!document.querySelector('[aria-label="主导航"]')`),'workspace');
  }
 }else{
  assert.equal(await evaluate(`!!document.querySelector('[data-testid="startup-screen"]')`),false);
  await until(()=>evaluate(`!!document.querySelector('[aria-label="主导航"]')`),'same-version workspace');
 }
 if(action!=='cancel'){
  report.navigation=await evaluate(`Array.from(document.querySelectorAll('[aria-label="主导航"] [aria-label], [aria-label="设置导航"] [aria-label]')).map(e=>e.getAttribute('aria-label'))`);
  assert.ok(report.navigation.includes('学情追踪'));
  // Real read-only navigation. No acceptance, data editing, seeding or settings save.
  await evaluate(`document.querySelector('[aria-label="学情追踪"]').click()`);
  await until(()=>evaluate(`!!document.querySelector('#current-goals-title')`),'goal view');
  assert.equal(await evaluate(`!!document.querySelector('.goals-sort-button')`),true);
  await shot('fixed-'+action+'-workspace');
  report.goalsSortVisible=true;
 }
 assert.equal(errors.length,0);report.passed=true;
 await fs.writeFile(`${out}/fixed-${action}-results.json`,JSON.stringify(report,null,2)+'\n');
 console.log('PASS fixed application',action);
}finally{ws.close()}
