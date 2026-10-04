async function A(m,p,b){
  var c=new AbortController(),t=setTimeout(function(){c.abort()},300000);
  try{
    var o={method:m,headers:{'Content-Type':'application/json'},signal:c.signal};
    if(b)o.body=JSON.stringify(b);
    var r=await fetch(p,o);clearTimeout(t);return r.json()
  }catch(e){clearTimeout(t);return{ok:false,error:e.name==='AbortError'?'超时':e.message}}
}
async function APrism(m,p,b){
  var c=new AbortController(),t=setTimeout(function(){c.abort()},15000);
  try{
    var o={method:m,headers:{'Content-Type':'application/json'},signal:c.signal};
    if(b)o.body=JSON.stringify(b);
    var r=await fetch('/api/prism-proxy'+p,o);clearTimeout(t);return r.json()
  }catch(e){clearTimeout(t);return{ok:false,error:e.name==='AbortError'?'超时':e.message}}
}
