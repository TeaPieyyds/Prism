async function Rhome(){
  try{
    document.getElementById('tab-home').innerHTML='<div class="content-inner"><div style="text-align:center;padding:36px"><span class="spin" style="color:var(--phx-primary)"></span><span class="dim"> 加载中...</span></div></div>';
    var s=await A('GET','/api/bot/status'),t=await A('GET','/api/task/list');
    U(s.ok&&s.connected, s.ok&&s.is_op, s.ok?s.server:'');

    var features=[
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>`,t:'建筑导入',d:'导入 mcstructure',act:function(){openSubPage('建筑导入',RimpHTML())}},
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/><rect x="3" y="3" width="16" height="6" rx="1"/></svg>`,t:'建筑导出',d:'导出 mcstructure',act:function(){openSubPage('建筑导出',RexpHTML())}},
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="16" height="16" rx="2"/><line x1="3" y1="9" x2="19" y2="9"/><line x1="9" y1="3" x2="9" y2="19"/></svg>`,t:'地图画',d:'图片转像素画',act:function(){openSubPage('地图画',RmaHTML())}},
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>`,t:'音乐',d:'MIDI 播放 · 钢琴',act:function(){openSubPage('音乐',RmusicHTML())}},
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>`,t:'皮肤雕像',d:'皮肤图→方块雕像',act:function(){openSubPage('皮肤雕像',RskinHTML())}},
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="10" cy="10" r="7"/><line x1="21" y1="21" x2="15" y2="15"/></svg>`,t:'搜索',d:'租赁服 / 玩家',act:function(){openSubPage('搜索',RsearchHTML())}},
      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="16" height="16" rx="2"/><path d="M7 3v4M15 3v4M3 11h16M3 7h16"/><circle cx="11" cy="14" r="2"/><circle cx="16" cy="14" r="2"/><circle cx="6" cy="14" r="2"/></svg>`,t:'生成预览图',d:'建筑文件→多角度预览图',act:function(){if(typeof previewCheckPermission==='function'){previewCheckPermission().then(function(ok){if(ok){delete subPageCache['生成预览图'];openSubPage('生成预览图',RpreviewHTML())}})}else{delete subPageCache['生成预览图'];openSubPage('生成预览图',RpreviewHTML())}}},
	      {i:`<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>`,t:'其他',d:'更多功能',act:function(){openSubPage('其他',RothersHTML())}},
    ];

    // Grid card layout (参考 HOSDLA)
    var h='<div class="grid-2" style="margin-top:4px">';
    for(var i=0;i<features.length;i++){
      var f=features[i];
      h+='<div class="grid-card" onclick="('+f.act.toString()+')()">'+
        '<div class="gc-icon">'+f.i+'</div>'+
        '<div class="gc-title">'+f.t+'</div>'+
        '<div class="gc-sub">'+f.d+'</div></div>'
    }
    h+='</div>';

    // Task list
    h+='<div class="tsk-section" style="margin-top:14px"><div class="tsk-header">任务列表</div>';
    if(t.ok&&t.tasks&&t.tasks.length){
      for(var ti=0;ti<t.tasks.length;ti++){
        var tk=t.tasks[ti];
        var tIcon={'import':'<svg width="28" height="28" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>','export':'<svg width="28" height="28" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/><rect x="3" y="3" width="16" height="6" rx="1"/></svg>','mapart':'<svg width="28" height="28" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="16" height="16" rx="2"/><line x1="3" y1="9" x2="19" y2="9"/><line x1="9" y1="3" x2="9" y2="19"/></svg>','skin':'<svg width="28" height="28" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>'}[tk.type]||'·';
        var tSt=tk.status||'unknown';
        var tCl=tSt==='running'?'running':tSt==='done'?'done':tSt==='failed'?'failed':'paused';
        var tLb=tSt==='running'?'运行中':tSt==='done'?'完成':tSt==='failed'?'失败':'暂停';
        var tName=tk.name||tk.type||'未知';
        var tMeta=tk.message||'';
        if(tk.progress!=null)tMeta='进度: '+(tk.progress*100).toFixed(0)+'%';
        h+='<div class="tsk-item">'+
          '<div class="tsk-icon">'+tIcon+'</div>'+
          '<div class="tsk-body"><div class="tsk-name">'+escHtml(tName)+'</div>'+
          '<div class="tsk-meta">'+escHtml(tMeta)+'</div></div>'+
          '<span class="tsk-status '+tCl+'">'+tLb+'</span>';
        if(tSt==='running'||tSt==='paused'){
          h+='<button class="tsk-act" onclick="resumeTask(\''+tk.task_id+'\')">恢复</button>'
        }
        h+='</div>'
      }
    } else {
      h+='<div class="tsk-empty">暂无任务</div>'
    }
    h+='</div></div>';

    document.getElementById('tab-home').innerHTML=h;
    if(!window.wzActive) fetchAnnouncements()
  }catch(e){
    document.getElementById('tab-home').innerHTML='<div class="content-inner"><div class="card" style="text-align:center;color:var(--phx-error)">错误: '+e.message+'</div></div>'
  }
}

// ── Search (Originally in main index.html) ──
var scMode='server';
function setScMode(m){
  scMode=m;
  document.querySelectorAll('.sc-tab').forEach(function(b){
    b.style.background='transparent';b.style.color='var(--phx-text-secondary)';b.style.borderColor='var(--phx-border-light)'
  });
  var sel=document.querySelector('.sc-tab[data-mode="'+m+'"]');
  if(sel){sel.style.background='var(--phx-primary-bg)';sel.style.color='var(--phx-primary)';sel.style.borderColor='var(--phx-primary)'}
  var hint=document.getElementById('sc-hint');
  if(hint)hint.textContent=m==='server'?'租赁服搜索：按名称搜索网易我的世界租赁服，显示在线人数/版本/点赞等':'玩家搜索：按昵称搜索网易我的世界玩家，显示等级/在线状态等'
}

function qEsc(s){return String(s).replace(/'/g,"\\'").replace(/"/g,'&quot;')}

function accountCard(a,activeId){
  var isAct=a.id===activeId;
  var srcName=a.source==='web'?'网页':a.source==='guest'?'游客':a.source==='email'?'邮箱':a.source==='phone'?'手机':a.source;
  return'<div style="display:flex;align-items:center;gap:10px;padding:8px 0;border-bottom:1px solid var(--phx-border-light)">'+
    (a.avatar_image_url?'<img src="'+a.avatar_image_url+'" style="width:32px;height:32px;border-radius:8px;flex-shrink:0">':'<div style="width:32px;height:32px;border-radius:8px;background:var(--phx-border-light);flex-shrink:0"></div>')+
    '<div style="flex:1;min-width:0"><div style="font-size:13px;font-weight:700;color:'+(isAct?'var(--phx-primary)':'var(--phx-text)')+'">'+escHtml(a.display_name)+
    (a.is_vip?' <span style="font-size:9px;color:var(--phx-warning)">VIP</span>':'')+'</div>'+
    '<div class="dim" style="font-size:9px">'+srcName+' &nbsp;Lv.'+a.growth_level+' &nbsp;UID '+a.uid+'</div></div>'+
    (isAct?'<span style="font-size:10px;font-weight:700;color:var(--phx-primary)">使用中</span>':
    '<button class="btn-s" style="width:auto;margin:0;padding:6px 12px;font-size:11px" onclick="doSwitchAcc('+a.id+',\''+qEsc(a.display_name)+'\')">切换</button>')+
    '</div>'
}

async function doSwitchAcc(id,name){
  var r=await APrism('POST','/api/accounts/'+id+'/switch',{});
  if(r.ok){
    T('已切换到: '+name,'o');
    document.querySelectorAll('.modal-overlay').forEach(function(x){x.remove()});
    if(curTab==='settings')Rcfg();else Rhome()
  }else T('切换失败: '+r.error,'e')
}

function showAccountModal(){
  var zIdx = window.wzActive ? '10001' : '999';
  APrism('GET','/api/accounts').then(function(r){
    if(!r.ok||!r.accounts){T('加载失败: '+(r.error||'未知'),'e');return}
    var seen={},list=[];
    r.accounts.forEach(function(a){
      if(seen[a.uid]){
        var existing=list[seen[a.uid]-1];
        if(!a.is_shared&&existing.is_shared)list[seen[a.uid]-1]=a
      }else{
        seen[a.uid]=list.length+1;list.push(a)
      }
    });
    var privates=list.filter(function(a){return!a.is_shared});
    var shared=list.filter(function(a){return a.is_shared});
    var activeId=0;
    for(var i=0;i<list.length;i++){if(list[i].is_active){activeId=list[i].id;break}}
    var m=document.createElement('div');m.className='modal-overlay';m.style.zIndex=zIdx;
    var h='<div class="modal-box" style="max-width:360px;max-height:85vh;display:flex;flex-direction:column">'+
      '<div style="font-size:15px;font-weight:800;margin-bottom:8px;flex-shrink:0">切换账号</div>'+
      '<div style="overflow-y:auto;flex:1;min-height:0">';
    if(privates.length){h+='<div class="dim" style="font-size:9px;font-weight:700;margin:6px 0 4px;color:var(--phx-text-secondary)">私有账号</div>'+
      privates.map(function(a){return accountCard(a,activeId)}).join('')}
    if(shared.length){h+='<div class="dim" style="font-size:9px;font-weight:700;margin:6px 0 4px;color:var(--phx-text-secondary)">共享账号</div>'+
      shared.map(function(a){return accountCard(a,activeId)}).join('')}
    h+='</div><button class="btn-s" style="width:auto;margin:12px 0 0;padding:8px 16px" onclick="this.closest(\'.modal-overlay\').remove()">关闭</button></div>';
    m.innerHTML=h;document.body.appendChild(m)
  })
}
