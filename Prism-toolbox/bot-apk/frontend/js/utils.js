var allLogs=[];

var ICONS = {
  check: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  x: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>',
  dot: '<svg width="10" height="10" viewBox="0 0 10 10"><circle cx="5" cy="5" r="4" fill="currentColor"/></svg>',
  dotEmpty: '<svg width="10" height="10" viewBox="0 0 10 10"><circle cx="5" cy="5" r="3.5" fill="none" stroke="currentColor" stroke-width="1.5"/></svg>',
  musicNote: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>',
  arrowRight: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>',
  arrowLeft: '<span style="display:inline-block;width:10px;height:10px;border-left:3px solid var(--phx-text);border-bottom:3px solid var(--phx-text);transform:rotate(45deg)"></span>',
  loop: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>',
  trash: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  warning: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
  close: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>',
  expand: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 3 21 3 21 9"/><polyline points="9 21 3 21 3 15"/><line x1="21" y1="3" x2="14" y2="10"/><line x1="3" y1="21" x2="10" y2="14"/></svg>',
  folder: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>',
  file: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg>',
  upload: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg>',
  radio: '<svg width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="8" cy="8" r="3" fill="currentColor"/></svg>',
  parentDir: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/><polyline points="5 12 12 5 19 12"/><line x1="12" y1="5" x2="12" y2="19"/></svg>',
  prev: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="19 20 9 12 19 4 19 20"/><line x1="5" y1="19" x2="5" y2="5"/></svg>',
  stop: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>',
  next: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 4 15 12 5 20 5 4"/><line x1="19" y1="5" x2="19" y2="19"/></svg>',
  piano: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="4" width="20" height="16" rx="2"/><line x1="6" y1="6" x2="6" y2="10"/><line x1="8" y1="6" x2="8" y2="10"/><line x1="10" y1="6" x2="10" y2="10"/><line x1="14" y1="6" x2="14" y2="10"/><line x1="16" y1="6" x2="16" y2="10"/><line x1="18" y1="6" x2="18" y2="10"/><line x1="12" y1="6" x2="12" y2="18"/><rect x="4" y="10" width="16" height="10" rx="1"/></svg>',
  plus: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
  search: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>',
  sort: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l3-3 3 3"/><path d="M6 6v12"/><path d="M15 15l3 3 3-3"/><path d="M18 18V6"/></svg>',
  globe: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>'
};

function T(m,t){
  var e=document.getElementById('toast');
  if(!e)return;
  e.textContent=m;e.className='toast'+(t?' '+t:'')+' show';
  clearTimeout(e._t);
  e._t=setTimeout(function(){e.classList.remove('show')},2200)
}

function L(b,l,t){b.disabled=l;b.innerHTML=l?'<span class="spin"></span>'+t:t}

function U(conn,isOP,server){
  var d=document.getElementById('gdot');
  if(!d)return;
  if(conn===false||conn===0)d.className='cb-dot';
  else if(conn) d.className='cb-dot'+(isOP?' on':' wait')
  updateConnStatus(conn,isOP,server);
}

function updateConnStatus(conn,isOP,server){
  var st=document.getElementById('conn-st');
  if(!st)return;
  var sv=document.getElementById('conn-srv');
  if(!conn){
    st.style.color='var(--phx-text-disabled)';st.innerHTML=ICONS.dotEmpty+' 未连接';
    if(sv)sv.textContent='';
  } else if(isOP){
    st.style.color='var(--phx-success)';st.innerHTML=ICONS.dot+' 已连接(OP)';
    if(sv)sv.textContent=server||'';
  } else {
    st.style.color='var(--phx-warning)';st.innerHTML=ICONS.dot+' 已连接';
    if(sv)sv.textContent=server||'';
  }
}

function I(n,d){var e=document.getElementById(n);if(!e)return d;var v=parseInt(e.value);return isNaN(v)?d:v}

function escHtml(s){return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;')}

function formatSize(bytes){
  if(!bytes)return'';
  if(bytes<1024)return bytes+'B';
  if(bytes<1048576)return(bytes/1024).toFixed(1)+'KB';
  return(bytes/1048576).toFixed(1)+'MB'
}

function formatTime(ts){
  if(!ts)return'';
  var d=new Date(ts*1000);
  return d.getFullYear()+'-'+(d.getMonth()+1)+'-'+d.getDate()+' '+d.getHours()+':'+('0'+d.getMinutes()).slice(-2)
}

function C(i,t,d,b,cl){
  return'<div class="qcard" onclick="('+cl.toString()+')()">'+
    '<div class="qc-icon">'+i+'</div>'+
    '<div class="qc-body"><div class="qc-title">'+t+
    (b?' <span class="qc-badge">'+b+'</span>':'')+
    '</div><div class="qc-sub">'+d+'</div></div>'+
    '<span class="qc-arrow">' + ICONS.arrowRight + '</span></div>'
}

var _mcColors={
  '0':'#000000','1':'#0000AA','2':'#00AA00','3':'#00AAAA',
  '4':'#AA0000','5':'#AA00AA','6':'#FFAA00','7':'#AAAAAA',
  '8':'#555555','9':'#5555FF','a':'#55FF55','b':'#55FFFF',
  'c':'#FF5555','d':'#FF55FF','e':'#FFFF55','f':'#FFFFFF'
};

function parseMCColor(text){
  if(!text)return'';
  var out='',i=0,spans=[];
  while(i<text.length){
    if(text[i]==='§'&&i+1<text.length){
      var code=text[i+1];i+=2;
      if(code==='r'){out+=spans.reverse().map(function(s){return'</'+s+'>'}).join('');spans=[];continue}
      if(code==='l'){out+='<b>';spans.push('b');continue}
      if(code==='o'){out+='<i>';spans.push('i');continue}
      if(code==='m'){out+='<s>';spans.push('s');continue}
      if(code==='n'){out+='<u>';spans.push('u');continue}
      if(code==='k')continue;
      var cl=_mcColors[code];
      if(cl){out+=spans.reverse().map(function(s){return'</'+s+'>'}).join('');spans=[];out+='<span style="color:'+cl+'">'}
    }else{
      out+=escHtml(text[i]);i++
    }
  }
  out+=spans.reverse().map(function(s){return'</'+s+'>'}).join('');
  return out
}

function parseRawtext(str){
  try{var j=JSON.parse(str);if(j.rawtext){var parts=[];j.rawtext.forEach(function(r){if(r.text)parts.push(r.text)});return parts.join('')}}catch(e){}
  return str
}
function renderLogLine(msg){
  var raw=parseRawtext(msg);
  var m=String(raw);
  if(m.indexOf('§')>=0)return parseMCColor(m);
  var cl='';
  if(m.indexOf('[✓]')>=0||m.indexOf('完成')>=0||m.indexOf('成功')>=0)cl='cl-ok';
  else if(m.indexOf('[✗]')>=0||m.indexOf('失败')>=0||m.indexOf('错误')>=0)cl='cl-err';
  return'<span class="'+cl+'">'+escHtml(m)+'</span>'
}

function soundLabel(v){
  var map={'note.harp':'竖琴','note.bass':'贝斯','note.bd':'鼓','note.snare':'小鼓','note.hat':'踩镲','note.bell':'钟琴','note.flute':'长笛','note.chime':'管钟','note.guitar':'吉他','note.xylophone':'木琴','note.iron_xylophone':'铁马林巴','note.cow_bell':'牛铃','note.didgeridoo':'迪吉里杜','note.bit':'方波','note.banjo':'班卓','note.pling':'电钢'};
  return map[v]||v.replace('note.','')
}

var fw=null;
window.__onFilePicked__=function(p){if(fw){fw(p);fw=null}}

function pick(t){
  return new Promise(function(r){
    if(typeof android!=='undefined'&&android.pickFile){fw=r;android.pickFile(t||'building')}
    else{var i=document.createElement('input');i.type='file';i.accept=t==='image'?'.png,.jpg,.jpeg':'.mcstructure,.schematic,.schem,.mcworld,.bdx';i.onchange=async function(){var f=i.files[0];if(!f){r('');return}var d=new FormData();d.append('file',f);try{var x=await fetch('/api/file/upload',{method:'POST',body:d});var j=await x.json();r(j.ok?j.path:'')}catch(e){r('')}};i.click()}
  })
}

function fpicker(type) {
  return new Promise(function(resolve) {
    // 内置文件选择器（仅 building / skin 类型）
    if (useBuiltinPicker && typeof showFilePicker === 'function' && (type === 'building' || type === 'skin')) {
      // 检查权限
      if (typeof previewCheckPermission === 'function') {
        previewCheckPermission().then(function(ok) {
          if (!ok) { resolve(''); return; }
          showFilePicker('/storage/emulated/0', type, function(path) {
            resolve(path);
          });
        });
      } else {
        showFilePicker('/storage/emulated/0', type, function(path) {
          resolve(path);
        });
      }
      return;
    }
    // 其他类型（如 image）回退到原生 / HTML 文件选择器
    if (typeof android !== 'undefined' && android.pickFile) {
      fw = function(path) { resolve(path); };
      android.pickFile(type || 'building');
    } else {
      pick(type).then(resolve);
    }
  });
}

// ── 优雅关闭弹窗（动画结束后移除 DOM）──
function closeOverlay(el,delay){
  el.classList.add('closing');
  setTimeout(function(){el.remove()},delay||350)
}
// 内联 onclick 用：关闭底部弹出选择器
function closeBsOverlay(el){closeOverlay(el.closest('.bs-overlay'))}
// 内联 onclick 用：关闭模态弹窗
function closeModalOverlay(el){closeOverlay(el.closest('.modal-overlay'))}

// ── 自定义确认弹窗（替代原生 confirm）──
function showPrompt(opts){
  var ov=document.createElement('div');ov.className='modal-overlay';
  ov.innerHTML='<div class="modal-box" style="max-width:340px">'+
    (opts.title?'<div style="font-size:15px;font-weight:800;margin-bottom:8px">'+opts.title+'</div>':'')+
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin-bottom:12px">'+(opts.message||'')+'</div>'+
    '<input id="prompt-input" type="text" placeholder="'+(opts.placeholder||'')+'" value="'+(opts.default||'')+'" style="width:100%;padding:10px;border:2px solid var(--phx-border);border-radius:8px;background:var(--phx-bg);font-size:14px;color:var(--phx-text);box-sizing:border-box;margin-bottom:8px;outline:none">'+
    '<div style="display:flex;gap:6px">'+
      '<button class="btn-s" style="flex:1;margin:0" id="prompt-cancel">取消</button>'+
      '<button class="btn" style="flex:1;margin:0" id="prompt-confirm">'+(opts.confirmText||'确定')+'</button>'+
    '</div></div>';
  document.body.appendChild(ov);
  document.getElementById('prompt-cancel').onclick=function(){closeOverlay(ov)};
  document.getElementById('prompt-confirm').onclick=function(){var v=document.getElementById('prompt-input').value;closeOverlay(ov);(opts.onConfirm||function(){})(v)};
  ov.onclick=function(e){if(e.target===ov)closeOverlay(ov)}
}

function showConfirm(opts){
  var overlay=document.createElement('div');overlay.className='modal-overlay';
  var html='<div class="modal-box" style="max-width:340px;text-align:center">'+
    (opts.title?'<div style="font-size:15px;font-weight:800;margin-bottom:10px;color:var(--phx-text)">'+opts.title+'</div>':'')+
    '<div style="font-size:13px;color:var(--phx-text-secondary);line-height:1.6;margin-bottom:16px;white-space:pre-line">'+opts.message+'</div>'+
    '<div style="display:flex;gap:8px">'+
    '<button class="bs-btn bs-btn-cancel" style="flex:1">'+(opts.cancelText||'取消')+'</button>'+
    '<button class="bs-btn bs-btn-confirm" style="flex:1">'+(opts.confirmText||'确定')+'</button></div></div>';
  overlay.innerHTML=html;
  document.body.appendChild(overlay);
  overlay.querySelector('.bs-btn-cancel').addEventListener('click',function(){closeOverlay(overlay)});
  overlay.querySelector('.bs-btn-confirm').addEventListener('click',function(){closeOverlay(overlay);if(opts.onConfirm)opts.onConfirm()});
  overlay.addEventListener('click',function(e){if(!e.target.closest('.modal-box'))closeOverlay(overlay)})
}


// showAlertModal 弹出信息弹窗
function showAlertModal(title, msg, closeable, onClose, extraButtons, zIndex) {
  var ov = document.createElement("div"); ov.className = "modal-overlay";
  if (zIndex) ov.style.zIndex = zIndex;
  var closeBtn = closeable ? '<button class="btn-s" style="width:auto;padding:4px 8px;margin:0;font-size:11px" id="alert-close-btn">\u2716</button>' : "";
  ov.innerHTML = '<div class="modal-box" style="max-width:360px">' +
    '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:8px">' +
    '<div style="font-size:15px;font-weight:800">' + title + '</div>' + closeBtn + '</div>' +
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin-bottom:12px;line-height:1.5">' + msg + '</div>' +
    (extraButtons || "") +
    '<button class="btn" style="width:100%;margin:0" id="alert-dismiss-btn">\u5173\u95ed</button>' +
    '</div>';
  document.body.appendChild(ov);
  if(closeable){
    var cb=document.getElementById('alert-close-btn');
    if(cb)cb.onclick=function(){closeOverlay(ov);if(onClose)onClose()}
  }
  var db=document.getElementById('alert-dismiss-btn');
  if(db)db.onclick=function(){closeOverlay(ov);if(onClose)onClose()};
  ov.onclick=function(e){if(e.target===ov)closeOverlay(ov)}
}
