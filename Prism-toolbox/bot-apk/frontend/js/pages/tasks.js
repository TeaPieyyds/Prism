async function Rtsk(){
  var r=await A('GET','/api/task/list'),h='<div class="content-inner" style="padding:4px 0">';
  if(!r.ok||!r.tasks||!r.tasks.length){h+='<div class="card"><p style="text-align:center;color:var(--phx-text-secondary);padding:20px;font-size:13px">暂无断点任务</p></div>'}
  else r.tasks.forEach(function(t){
    var tn=t.name&&t.name!='import'&&t.name!='mapart'&&t.name!='export'?t.name:t.type+' #'+t.task_id.slice(0,6);
    var pp={};try{pp=JSON.parse(t.params)}catch(e){}var ppath=pp.path||'';var isMc=ppath.toLowerCase().endsWith('.mcworld');
    var stCl=t.status==='running'?'color:var(--phx-success)':t.status==='paused'?'color:var(--phx-warning)':t.status==='failed'?'color:var(--phx-error)':'color:var(--phx-text-disabled)';
    var stLabel=t.status==='running'?'运行中':t.status==='paused'?'已暂停':t.status==='failed'?'失败':t.status==='done'?'完成':t.status;
    h+='<div class="qcard" data-taskid="'+t.task_id+'" data-path="'+ppath.replace(/"/g,'&quot;')+'" data-ismc="'+(isMc?1:0)+'" data-progress="'+t.progress+'" data-msg="'+(t.message||'')+'" data-status="'+t.status+'" data-name="'+tn+'" onclick="var d=this.dataset;showTaskDetail(d.taskid,d.name,parseFloat(d.progress),d.msg,d.status,d.path,d.ismc==1)">'+
      '<div class="qc-icon" style="font-size:16px">'+(t.status==='running'?ICONS.radio:t.status==='done'?ICONS.check:ICONS.close)+'</div>'+
      '<div class="qc-body"><div class="qc-title">'+tn+'</div><div class="qc-sub">'+(t.message||'')+' · '+Math.round(t.progress*100)+'%</div></div>'+
      '<span style="font-size:10px;font-weight:700;'+stCl+'">'+stLabel+'</span></div>'
  });
  h+='</div>';document.getElementById('tab-tasks').innerHTML=h
}

function showTaskDetail(id,name,prog,msg,st,path,isMc){
  var m=document.createElement('div');m.className='modal-overlay';
  m.innerHTML='<div class="modal-box">'+
    '<div style="font-weight:800;font-size:16px;margin-bottom:10px">'+name+'</div>'+
    '<div class="dim" style="margin-bottom:2px">进度: '+Math.round(prog*100)+'%</div>'+
    '<div class="dim" style="margin-bottom:4px">'+msg+'</div>'+
    '<div style="font-size:12px;font-weight:700;margin-bottom:14px;color:'+(st==='running'?'var(--phx-success)':st==='failed'?'var(--phx-error)':st==='paused'?'var(--phx-warning)':'var(--phx-text-disabled)')+'">状态: '+st+'</div>'+
    (path?'<button class="btn" style="width:100%;margin-bottom:8px" onclick="closeModalOverlay(this);impPath=\''+path+'\';doAnalyze(\''+path+'\','+isMc+')">查看文件</button>':'')+
    (st==='running'?'<button class="btn-d" style="width:100%" onclick="stopT();closeModalOverlay(this)">停止</button>':
     st!=='done'?'<div class="row"><button class="btn" onclick="resT(\''+id+'\');closeModalOverlay(this)">恢复</button><button class="btn-s" onclick="delT(\''+id+'\');closeModalOverlay(this)">删除</button></div>':
     '<button class="btn-s" style="width:100%" onclick="delT(\''+id+'\');closeModalOverlay(this)">删除</button>')+
    '<button class="btn-s" style="width:100%;margin-top:8px" onclick="closeModalOverlay(this)">关闭</button></div>';
  document.body.appendChild(m);m.addEventListener('click',function(e){if(e.target===m)m.remove()})
}
async function resT(id){var r=await A('POST','/api/task/resume',{task_id:id});T(r.ok?'已恢复':r.error,'o');Rtsk()}
async function stopT(){await A('POST','/api/task/stop');Rtsk()}
async function delT(id){await A('POST','/api/task/delete',{task_id:id});Rtsk()}
