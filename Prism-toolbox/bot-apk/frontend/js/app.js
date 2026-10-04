var curTab='home',tabInited={},tabHistory=['home'];
var activeRenderers=[];
var _savedFormState={};
var _voxelData={};

var cfgSpeed=9500,cfgNewProtocol=false,cfgImportCommands=true,cfgCmdDisabled=false,cfgExcludeFluids=false,cfgExcludeWater=true,cfgExcludeWaterlogged=true,cfgExcludeLava=true,cfgGravity=false,cfgPlatform=false;
var cfgToken='';
var cfgRegionMode='1';
var cfgBotTokens=[];
var subPageOpen=false;
var subPageCache={};

// Preload config
var cfgPreClear=0;(async function preloadCfg(){try{var c=await A('GET','/api/config');if(c.ok&&c.config){cfgToken=c.config.token||'';cfgSpeed=c.config.import_speed||9500;cfgNewProtocol=c.config.use_new_protocol||false;cfgImportCommands=c.config.import_commands!==false;cfgCmdDisabled=c.config.cmd_disabled||false;cfgExcludeFluids=c.config.exclude_fluids||false;cfgExcludeWater=c.config.exclude_water!==false;cfgExcludeWaterlogged=c.config.exclude_waterlogged!==false;cfgExcludeLava=c.config.exclude_lava!==false;cfgGravity=c.config.use_gravity_blocks||false;cfgPlatform=c.config.gravity_platform||false;cfgPreClear=c.config.pre_clear_mode||0;cfgRegionMode=c.config.region_mode||'1';cfgBotTokens=c.config.bot_tokens||[]}}catch(e){}})();
// 启动时自动检查版本更新
setTimeout(function(){A('POST','/api/version/check',{}).then(function(r){if(r.ok)handleVersionCheck(r)}).catch(function(){})},3000);
// 启动时自动检查版本更新

function openSubPage(title, htmlContent){
  var cacheKey = title;
  var sp = document.getElementById('sub-page');
  document.getElementById('bottom-nav').classList.add('gone');
  document.getElementById('conn-bar').classList.add('gone');

  if (subPageCache[cacheKey]) {
    sp.innerHTML = subPageCache[cacheKey];
    // defer restore to run after feature init timeouts (10ms) to avoid override
    setTimeout(restoreSubPageState,20,cacheKey);
  } else {
    var headerHtml = '<div class="sp-header"><button class="sp-back" onclick="closeSubPage()">' + ICONS.arrowLeft + '</button><span class="sp-title">' + title + '</span></div>';
    subPageCache[cacheKey] = headerHtml + '<div class="sp-body">' + htmlContent + '</div>';
    sp.innerHTML = subPageCache[cacheKey];
  }

  sp.className = 'sub-page show';
  subPageOpen = true;
  activeRenderers.forEach(function(kill){try{kill()}catch(e){}});activeRenderers=[];
}

function closeSubPage(){
  // save form state + update cached HTML with current DOM state
  var titleEl=document.querySelector('.sp-title');
  if(titleEl){
    var key=titleEl.textContent;
    var sp=document.getElementById('sub-page');
    var header=sp.querySelector('.sp-header');
    var body=document.querySelector('#sub-page .sp-body');
    if(body){
      // save form values
      _savedFormState[key]=serializeForm(body);
      // update cached HTML with current DOM (preserve file selection, stats, etc.)
      var clone=body.cloneNode(true);
      // remove 3D viewer + canvases from cache (recreated from _voxelData on reopen)
      var vw=clone.querySelector('#viewer');
      if(vw)vw.remove();
      clone.querySelectorAll('canvas').forEach(function(c){c.remove()});
      if(header)subPageCache[key]=header.outerHTML+clone.outerHTML
    }
  }
  document.getElementById('sub-page').className='sub-page';
  document.getElementById('bottom-nav').classList.remove('gone');
  document.getElementById('conn-bar').classList.remove('gone');
  subPageOpen=false;
  // keep cache, kill renderers (performance)
  activeRenderers.forEach(function(kill){try{kill()}catch(e){}});activeRenderers=[];
  // evict stale cache entries (keep last 10)
  var keys=Object.keys(subPageCache);
  if(keys.length>10){
    var keep=keys.slice(-10);
    Object.keys(subPageCache).forEach(function(k){
      if(keep.indexOf(k)<0)delete subPageCache[k]
    });
    Object.keys(_savedFormState).forEach(function(k){
      if(keep.indexOf(k)<0)delete _savedFormState[k]
    });
    Object.keys(_voxelData).forEach(function(k){
      if(keep.indexOf(k)<0)delete _voxelData[k]
    })
  }
  if(curTab==='home')refreshHomeTasks();
}

function serializeForm(root){
  var st={};
  root.querySelectorAll('input,select,textarea').forEach(function(el){
    if(!el.id)return;
    if(el.type==='checkbox')st[el.id]=el.checked;
    else st[el.id]=el.value;
  });
  return st
}

function restoreSubPageState(key){
  // restore form values
  var sv=_savedFormState[key];
  if(sv){
    Object.keys(sv).forEach(function(id){
      var el=document.getElementById(id);
      if(!el)return;
      if(el.type==='checkbox')el.checked=sv[id];
      else el.value=sv[id]
    })
    // re-run conditional toggles
    try{
      toggleSub('imp-cmds','imp-cmd-sub');
      toggleSub('imp-exclude-fluids','imp-fluid-sub');
      toggleDimInput();
      toggleGravitySub();
      toggleMaRelief()
    }catch(e){}
  }
  // recreate 3D preview if voxel data exists
  var vd=_voxelData[key];
  if(vd&&typeof show3D==='function'){
    var vw=document.getElementById('viewer');
    // if viewer was removed from cached HTML on close, re-create it
    if(!vw){
      var info=document.getElementById('imp-info')||document.getElementById('ma-info')||document.getElementById('skin-preview-area');
      if(info){
        vw=document.createElement('div');vw.className='viewer';vw.id='viewer';
        info.appendChild(vw)
      }
    }
    if(vw){
      vw.innerHTML='';
      requestAnimationFrame(function(){
        if(typeof THREE!=='undefined')show3D(vw,vd)
      })
    }
  }
  // restore task-running UI if a task is active
  if(window._currentTask){
    var ct=window._currentTask;
    var t2p={import:'imp',export:'exp',mapart:'md',skin:'skin'};
    var pr=t2p[ct.type];
    if(pr){
      var go=document.getElementById(pr+'-go');
      var stop=document.getElementById(pr+'-stop');
      var pg=document.getElementById(pr+'-pg');
      var lc=document.getElementById(pr+'-log-card');
      if(go)go.classList.add('gone');
      if(stop)stop.classList.remove('gone');
      if(pg)pg.classList.remove('gone');
      if(lc)lc.classList.remove('gone');
      // restore progress bar if known
      if(ct.progress>0){
        var bar=document.getElementById(pr+'-bar');
        if(bar)bar.style.width=(ct.progress*100)+'%'
      }
    }
  }
  // restore marquee state if on marquee page
  if(document.getElementById('mq-text')&&typeof restoreMarqueeState==='function'){
    restoreMarqueeState()
  }
  // restore mcfunction state if on mcfunction page
  if(document.getElementById('mcfn-wrap')&&typeof restoreMcfunctionState==='function'){
    restoreMcfunctionState()
  }
}

function refreshHomeTasks(){
  var section=document.querySelector('#tab-home .tsk-section');
  if(!section)return;
  A('GET','/api/task/list').then(function(t){
    if(!t.ok||!t.tasks)return;
    var h='<div class="tsk-header">任务列表</div>';
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
    var s=document.querySelector('#tab-home .tsk-section');
    if(s){var hdr=s.querySelector('.tsk-header');if(hdr)hdr.outerHTML='<div class="tsk-header">任务列表</div>';s.innerHTML=h}
  })
}

function resumeTask(id){
  A('POST','/api/task/resume',{task_id:id}).then(function(r){
    if(r.ok){T('任务已恢复','o');refreshHomeTasks()}else T('恢复失败: '+r.error,'e')
  })
}

// Tab navigation
document.querySelectorAll('.nav button').forEach(function(b){
  b.addEventListener('click',function(){
    var t=b.getAttribute('data-tab');
    if(t!==curTab)tabHistory.push(t);
    tabTo(t);
  })
});

function tabTo(t){
  document.querySelectorAll('.nav button').forEach(function(b){b.classList.remove('active')});
  document.querySelectorAll('.content').forEach(function(c){c.classList.remove('show')});
  var btn=document.querySelector('.nav button[data-tab="'+t+'"]');
  if(btn)btn.classList.add('active');
  var tab=document.getElementById('tab-'+t);
  if(tab)tab.classList.add('show');
  curTab=t;
  activeRenderers.forEach(function(kill){try{kill()}catch(e){}});activeRenderers=[];
  if(t!=='settings'){if(typeof _cfgRefreshTimer!=='undefined'){clearInterval(_cfgRefreshTimer);_cfgRefreshTimer=null}if(typeof _toolboxPollTimer!=='undefined'){clearInterval(_toolboxPollTimer);_toolboxPollTimer=null}}
  if(!tabInited[t]){tabInited[t]=true;if(t==='home')Rhome();if(t==='tasks')Rtsk();if(t==='terminal')Rconsole();if(t==='settings')Rcfg()}
}

// Android back button
window.__onBackPressed__=function(){
  if(subPageOpen){closeSubPage();return'true'}
  if(tabHistory.length>1){
    tabHistory.pop();
    tabTo(tabHistory[tabHistory.length-1]);
    return'true'
  }
  return'exit'
};
// 初始化首页
tabTo('home');
