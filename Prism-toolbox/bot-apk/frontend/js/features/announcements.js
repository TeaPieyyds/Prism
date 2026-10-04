var dismissedAnnouncements={};
var seenAnnouncements={};
var updateURL='';

async function fetchAnnouncements(){
  if(window.wzActive)return;
  try{var r=await A('GET','/api/announcements');if(r.ok&&r.announcements)showAnnouncements(r.announcements)}catch(e){}
}

function handleVersionCheck(d){
  if(!d.ok)return;
  if(d.signature_valid===false){
    // 服务器判定签名无效：Go 已进入拦截模式，重载首页即显示"应用被篡改"拦截页（关不掉）
    location.href='/';
    return;
  }
  if(d.latest_version_code>d.current_version_code){
    var url=d.update_url||'';
    if(d.force_update){
      updateLocked=true;updateURL=url;
      showAlertModal('必须更新',
        '<p>当前版本 <b>'+escHtml(d.current_version)+'</b> → 最新版本 <b style="color:var(--phx-primary)">'+escHtml(d.latest_version)+'</b></p>'+
        renderMD(d.update_message||'')+
        '<p class="dim">必须更新后才能继续使用</p>',false,function(){downloadUpdate(url)},'下载更新','99999',true)
    }else{
      showAlertModal('发现新版本',
        '<p>当前版本 <b>'+escHtml(d.current_version)+'</b> → 最新版本 <b style="color:var(--phx-primary)">'+escHtml(d.latest_version)+'</b></p>'+
        renderMD(d.update_message||''),true,function(){downloadUpdate(url)},'下载更新','99999')
    }
  }
}

function downloadUpdate(u){
  u=u||updateURL||'';
  if(!u){T('下载链接无效','e');return}
  if(typeof android!=='undefined'&&android.openExternal){
    try{android.openExternal(u)}catch(e){T('跳转失败，请手动打开浏览器下载','e')}
  }else{
    window.location.href=u
  }
}

function checkUpdateLock(){
  if(updateLocked){
    T('请先更新到最新版本再使用此功能','e');
    if(updateURL)showAlertModal('需要更新','<p>检测到强制更新，导入/导出功能已禁用</p><p class="dim">请更新后再使用</p>',false,function(){downloadUpdate()},'下载更新','99999',true);
    return true
  }
  return false
}

function showAlertModal(title,body,canClose,onAction,actionLabel,zIdx,keepOpen){
  var ov=document.createElement('div');ov.className='modal-overlay';
  if(zIdx)ov.style.zIndex=zIdx;
  ov.innerHTML='<div class="modal-box" style="max-height:85vh;display:flex;flex-direction:column">'+
    '<div style="font-size:16px;font-weight:800;margin-bottom:8px;flex-shrink:0">'+title+'</div>'+
    '<div style="font-size:13px;line-height:1.6;overflow-y:auto;flex:1;min-height:0">'+body+'</div>'+
    '<div style="margin-top:10px;display:flex;gap:8px;justify-content:'+(canClose?'flex-end':'center')+';flex-shrink:0;padding-top:8px;border-top:1px solid var(--phx-border-light)">'+
    (canClose?'<button class="btn-s" style="width:auto;margin:0" id="alert-close-ann">关闭</button>':'')+
    (onAction?'<button class="btn act-btn" style="width:auto;margin:0">'+(actionLabel||'确认')+'</button>':'')+
    '</div></div>';
  document.body.appendChild(ov);
  if(canClose){
    var cb=document.getElementById('alert-close-ann');
    if(cb)cb.onclick=function(){closeOverlay(ov)}
  }
  if(onAction){
    var btn=ov.querySelector('.act-btn');
    btn.addEventListener('click',function(){onAction();if(!keepOpen)closeOverlay(ov)})
  }
  ov.onclick=function(e){if(!e.target.closest('.modal-box'))closeOverlay(ov)}
}

function showAnnouncements(list){
  for(var i=0;i<list.length;i++){
    var a=list[i];
    if(seenAnnouncements[a.id])continue;
    seenAnnouncements[a.id]=true;
    if(isDismissedToday(a.id))continue;
    if(a.display_mode==='banner'){showAnnouncementBanner(a)}
    else if(a.display_mode==='modal'||a.display_mode==='modal_once'){
      if(a.display_mode==='modal_once'&&dismissedAnnouncements[a.id])continue;
      showAnnouncementModal(a)
    }else if(a.display_mode==='inline'){showAnnouncementInline(a)}
  }
}

function renderMD(text){
  if(typeof marked!=='undefined'){
    try{
      marked.setOptions({breaks:true,gfm:true,headerIds:false,mangle:false});
      var html=marked.parse(text||'');
      return '<div class="ann-content">'+html+'</div>'
    }catch(e){}
  }
  return '<div class="ann-content">'+escHtml(text||'').replace(/\n/g,'<br>')+'</div>'
}

function showAnnouncementBanner(a){
  var old=document.getElementById('ann-banner');if(old)old.remove();
  var sevCl=a.severity==='critical'?'border-left:4px solid var(--phx-error)':a.severity==='warning'?'border-left:4px solid var(--phx-warning)':'border-left:4px solid #4a90d9';
  var b=document.createElement('div');b.id='ann-banner';
  b.style.cssText=sevCl+';background:var(--phx-bg-card);padding:12px 16px;margin:0 16px 10px;border-radius:var(--phx-radius-sm);box-shadow:var(--phx-shadow);position:relative;max-height:200px;overflow-y:auto;font-size:12px';
  b.innerHTML='<div style="font-weight:800;margin-bottom:6px;color:var(--phx-text);font-size:14px">'+escHtml(a.title)+'</div>'+renderMD(a.content)+
    (a.can_dismiss?'<button style="position:absolute;top:6px;right:6px;background:none;border:none;font-size:16px;cursor:pointer;color:var(--phx-text-secondary);padding:2px 6px;margin:0" onclick="dismissToday(\''+a.id+'\');this.parentElement.remove()">✕</button>':'');
  var home=document.getElementById('tab-home');
  if(home)home.insertBefore(b,home.firstChild)
}

function showAnnouncementModal(a){
  var ov=document.createElement('div');ov.className='modal-overlay';
  var sevIcon=a.severity==='critical'?ICONS.warning:a.severity==='warning'?ICONS.warning:'';
  var sevCl=a.severity==='critical'?'color:var(--phx-error)':a.severity==='warning'?'color:var(--phx-warning)':'color:#4a90d9';
  ov.innerHTML='<div class="modal-box" style="max-width:380px;max-height:85vh;display:flex;flex-direction:column">'+
    '<div style="font-size:16px;font-weight:800;margin-bottom:8px;flex-shrink:0;'+sevCl+'">'+sevIcon+' '+escHtml(a.title)+'</div>'+
    '<div style="overflow-y:auto;flex:1;min-height:0">'+renderMD(a.content)+'</div>'+
    (a.can_dismiss?
      '<div style="display:flex;gap:6px;margin-top:10px;flex-shrink:0;padding-top:8px;border-top:1px solid var(--phx-border-light)">'+
      '<button class="btn-s" style="flex:1;margin:0;font-size:12px;padding:10px 8px" onclick="closeModalOverlay(this)">临时关闭</button>'+
      '<button class="btn-s" style="flex:1;margin:0;font-size:12px;padding:10px 8px" onclick="dismissToday(\''+a.id+'\');closeModalOverlay(this)">今日不再显示</button>'+
      '</div>':
      (a.severity==='critical'?'<div class="dim" style="margin-top:10px;text-align:center;font-size:11px;flex-shrink:0">此公告不可关闭</div>':''))+
    '</div>';
  document.body.appendChild(ov);
  if(a.can_dismiss)ov.addEventListener('click',function(e){if(e.target===ov)closeOverlay(ov)});
  setTimeout(function(){
    ov.querySelectorAll('.ann-content a').forEach(function(link){
      link.setAttribute('target','_blank');link.setAttribute('rel','noopener noreferrer');
      link.addEventListener('click',function(e){e.preventDefault();window.open(link.href,'_blank')})
    });
    ov.querySelectorAll('.ann-content img').forEach(function(img){
      img.style.cssText='max-width:100%;border-radius:8px;cursor:pointer;margin:8px 0';
      img.addEventListener('click',function(){showImagePreview(img.src)})
    })
  },100)
}

function showAnnouncementInline(a){
  var home=document.getElementById('tab-home');if(!home)return;
  var sevCl=a.severity==='critical'?'border-left:4px solid var(--phx-error)':a.severity==='warning'?'border-left:4px solid var(--phx-warning)':'border-left:4px solid #4a90d9';
  var card=document.createElement('div');
  card.style.cssText=sevCl+';background:var(--phx-bg-card);padding:14px 16px;margin-bottom:10px;border-radius:var(--phx-radius);box-shadow:var(--phx-shadow);font-size:12px';
  card.innerHTML='<div style="font-weight:800;margin-bottom:6px;color:var(--phx-text);font-size:14px">'+escHtml(a.title)+'</div>'+renderMD(a.content);
  home.appendChild(card)
}

function dismissToday(id){
  try{localStorage.setItem('ann_dismiss_'+id,new Date().toDateString())}catch(e){}
}
function isDismissedToday(id){
  try{return localStorage.getItem('ann_dismiss_'+id)===new Date().toDateString()}catch(e){return false}
}

function showImagePreview(src){
  var ov=document.createElement('div');
  ov.style.cssText='position:fixed;inset:0;background:rgba(0,0,0,.92);z-index:9999;display:flex;align-items:center;justify-content:center;cursor:pointer';
  ov.innerHTML='<img src="'+src+'" style="max-width:95vw;max-height:95vh;object-fit:contain;border-radius:8px">';
  ov.addEventListener('click',function(){ov.style.opacity='0';setTimeout(function(){ov.remove()},260)});
  document.body.appendChild(ov)
}

function showIntegrityAlert(d){
  showAlertModal('完整性警告','<div style="color:var(--phx-error)"><b>资源校验失败</b></div><p>'+escHtml(d.message||'检测到资源被篡改')+'</p><p class="dim">请从官方渠道 prism.adblanlu.qzz.io 重新下载</p>',false)
}
