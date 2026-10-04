var _consoleRendered = 0;

function Rconsole(){
  document.getElementById('tab-terminal').innerHTML=
    '<div class="content-inner" style="display:flex;flex-direction:column;height:calc(100vh - 160px)">'+
    '<div class="console-log" id="console-log" style="flex:1;overflow-y:auto;margin-bottom:8px"></div>'+
    '<div style="flex-shrink:0;display:flex;gap:8px;padding:8px;background:var(--phx-bg-card);border-radius:var(--phx-radius);border:2px solid var(--phx-border-light)">'+
    '<input id="console-input" type="text" placeholder="输入消息或 /命令" style="flex:1;margin:0" onkeydown="if(event.key===\'Enter\')sendConsole()">'+
    '<button class="btn" id="console-send" onclick="sendConsole()" style="width:auto;margin:0;flex-shrink:0;padding:11px 16px;font-size:13px">发送</button>'+
    '</div>'+
    '<div style="font-size:10px;color:var(--phx-text-secondary);margin-top:4px">输入文字 → 聊天 &nbsp;|&nbsp; /开头 → 指令（自动以机器人身份执行）</div>'+
    '</div>';
  var log=document.getElementById('console-log');
  if(log) appendConsoleLogs(log);
  setTimeout(function(){var el=document.getElementById('console-input');if(el)el.focus()},100)
}

function appendConsoleLogs(log){
  if(!log) return;
  if(allLogs.length===0){
    if(!log.children.length) log.innerHTML='<div style="color:#888;text-align:center;padding:40px 0;font-size:12px">▍控制台就绪</div>';
    return;
  }
  // 清除占位提示
  if(log.children.length===1 && log.children[0].textContent==='▍控制台就绪') log.innerHTML='';
  // 追加增量日志
  for(var i=_consoleRendered;i<allLogs.length;i++){
    var e=allLogs[i];
    var d=document.createElement('div');
    d.style.padding='2px 0';
    d.style.borderBottom='1px solid rgba(255,255,255,.04)';
    d.style.fontFamily="'SF Mono','Fira Code',monospace";
    d.style.fontSize='12px';
    d.style.lineHeight='1.5';
    if(e.cl==='e'){d.innerHTML='<span style="color:#ff5555">'+renderLogLine(e.msg)+'</span>'}
    else if(e.tp==='console'){
      if(e.cl==='cmd')d.innerHTML='<span style="color:#ffaa00;font-weight:700">'+escHtml(e.msg)+'</span>';
      else if(e.cl==='chat')d.innerHTML='<span style="color:#aaa">'+escHtml(e.msg)+'</span>';
      else d.innerHTML=renderLogLine(e.msg);
    }else{d.innerHTML=renderLogLine(e.msg);if(e.cl==='o')d.style.color='var(--phx-success)';else if(e.cl==='e')d.style.color='var(--phx-error)'}
    log.appendChild(d);
  }
  _consoleRendered=allLogs.length;
  log.scrollTop=log.scrollHeight
}

async function sendConsole(){
  var inp=document.getElementById('console-input');
  if(!inp)return;
  var text=inp.value.trim();
  if(!text)return;
  var sendBtn=document.getElementById('console-send');
  if(sendBtn){sendBtn.disabled=true;sendBtn.innerHTML='<span class="spin"></span>'}
  inp.value='';
  inp.focus();
  var isCmd=text[0]==='/';
  addConsoleLine((isCmd?'[指令] ':'[聊天] ')+text,isCmd?'cmd':'chat');
  var r=await A('POST','/api/bot/console',{input:text});
  if(sendBtn){sendBtn.disabled=false;sendBtn.textContent='发送'}
  if(!r.ok){addConsoleLine('[错误] '+(r.error||'发送失败'),'e');return}
  // 显示命令执行结果（直接走 HTTP 响应，不走 SSE 避免洪水）
  if(r.output && r.output.length>0){
    for(var i=0;i<r.output.length;i++){addConsoleLine('▸ '+r.output[i],'o')}
  }else if(r.success===false){
    addConsoleLine('[失败] 命令执行失败','e')
  }else if(r.note){
    addConsoleLine('ⓘ '+r.note,'o')
  }
}

function addConsoleLine(msg,cl){
  allLogs.push({msg:msg,cl:cl,tp:'console'});
  var l=document.getElementById('console-log');
  if(!l)return;
  var d=document.createElement('div');
  d.style.padding='2px 0';
  d.style.borderBottom='1px solid rgba(255,255,255,.04)';
  d.style.fontFamily="'SF Mono','Fira Code',monospace";
  d.style.fontSize='12px';
  d.style.lineHeight='1.5';
  if(cl==='e'){d.innerHTML='<span style="color:#ff5555">'+renderLogLine(msg)+'</span>'}
  else if(cl==='cmd'){d.innerHTML='<span style="color:#ffaa00;font-weight:700">'+escHtml(msg)+'</span>'}
  else if(cl==='chat'){d.innerHTML='<span style="color:#aaa">'+escHtml(msg)+'</span>'}
  else{d.innerHTML=renderLogLine(msg)}
  l.appendChild(d);
  l.scrollTop=l.scrollHeight
}
