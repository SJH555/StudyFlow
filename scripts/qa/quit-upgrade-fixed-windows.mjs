// Request the same Wails runtime Quit used by the visible startup Exit button.
// Only for the fixed acceptance overlay on port 19349; never send WM_CLOSE to
// Wails' hidden mainthread window, whose default handler destroys dispatch.
const page=(await(await fetch('http://127.0.0.1:19349/json/list')).json())[0];
const ws=new WebSocket(page.webSocketDebuggerUrl);await new Promise(r=>ws.onopen=r);
ws.send(JSON.stringify({id:1,method:'Runtime.evaluate',params:{expression:`void fetch('/wails/runtime',{method:'POST',headers:{'Content-Type':'application/json','x-wails-client-id':'fixed-qa'},body:JSON.stringify({object:2,method:2})})`}}));
await new Promise(r=>setTimeout(r,100));ws.close();
let closed=false;
for(let i=0;i<200;i++){try{const p=await(await fetch('http://127.0.0.1:19349/json/list')).json();if(!p.length){closed=true;break}}catch{closed=true;break}await new Promise(r=>setTimeout(r,100))}
if(!closed)throw Error('Runtime graceful Quit timed out');
console.log('PASS: actual Wails runtime Application.Quit completed normally');
