var skinPath='';
var SKIN_SCALE_ITEMS=[{value:'1',label:'1x (极小)'},{value:'2',label:'2x (小型)'},{value:'3',label:'3x'},{value:'4',label:'4x'}];
var SKIN_ARM_ITEMS=[{value:'classic',label:'Classic (3px)'},{value:'slim',label:'Slim (2px)'}];
var SKIN_BLOCK_ITEMS=[{value:'mixed',label:'混合'},{value:'wool',label:'羊毛'},{value:'concrete',label:'混凝土'},{value:'terracotta',label:'陶瓦'}];
var SKIN_ROT_ITEMS=[{value:'0',label:'朝南 (Z+ 默认)'},{value:'90',label:'朝西 (X-)'},{value:'180',label:'朝北 (Z-)'},{value:'270',label:'朝东 (X+)'}];
function showSkinScalePicker(){
  showPicker({title:'选择缩放',items:SKIN_SCALE_ITEMS,selected:document.getElementById('skin-scale').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('skin-scale');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=SKIN_SCALE_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showSkinArmPicker(){
  showPicker({title:'选择手臂类型',items:SKIN_ARM_ITEMS,selected:document.getElementById('skin-arm').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('skin-arm');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=SKIN_ARM_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showSkinRotPicker(){
  showPicker({title:'选择朝向',items:SKIN_ROT_ITEMS,selected:document.getElementById('skin-rot').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('skin-rot');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=SKIN_ROT_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showSkinBlockPicker(){
  showPicker({title:'选择方块组',items:SKIN_BLOCK_ITEMS,selected:document.getElementById('skin-block').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('skin-block');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=SKIN_BLOCK_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function RskinHTML(){
  return '<div class="card"><div class="pick" onclick="doSkinPick()"><i class="pi">' + ICONS.upload + '</i><div id="skin-pick-txt">选择皮肤 PNG 图片</div><div style="font-size:10px;color:var(--phx-text-secondary)">.png</div></div>'+
    '<div id="skin-info" class="gone"></div>'+
    '<div id="skin-preview-area"></div>'+
    '<div id="skin-params" class="gone">'+
    '<label>缩放</label><div class="picker-btn" id="skin-scale" data-value="2" onclick="showSkinScalePicker()"><span class="picker-label">2x (小型)</span><span class="picker-arrow">▾</span></div>'+
    '<label>手臂类型</label><div class="picker-btn" id="skin-arm" data-value="classic" onclick="showSkinArmPicker()"><span class="picker-label">Classic (3px)</span><span class="picker-arrow">▾</span></div>'+
    '<label>方块组</label><div class="picker-btn" id="skin-block" data-value="mixed" onclick="showSkinBlockPicker()"><span class="picker-label">混合</span><span class="picker-arrow">▾</span></div>'+
    '<label>放置坐标</label><div class="row"><input id="skin-x" type="number" placeholder="X"><input id="skin-y" type="number" placeholder="Y"><input id="skin-z" type="number" placeholder="Z"></div>'+
    '<label>朝向</label><div class="picker-btn" id="skin-rot" data-value="0" onclick="showSkinRotPicker()"><span class="picker-label">朝南 (Z+ 默认)</span><span class="picker-arrow">▾</span></div>'+
    '<label>速度（方块/秒）</label><input id="skin-spd" type="number" value="'+cfgSpeed+'"></div>'+
    '<div class="row"><button class="btn gone" id="skin-go" onclick="doSkinBuild()">开始建造</button>'+
    '<button class="btn-d gone" id="skin-stop" onclick="doSkinStop()">停止</button></div>'+
    '<div class="pg gone" id="skin-pg"><div id="skin-bar"></div></div></div>'
}

async function doSkinPick(){
  var p=useBuiltinPicker?await fpicker('skin'):await pick('image');if(!p)return;skinPath=p;
  document.getElementById('skin-pick-txt').innerHTML='<span class="spin"></span> 分析中...';
  var isOnline = p.indexOf('online://') === 0;
  var r;
  if(isOnline){
    var playerName = p.replace('online://','');
    r=await A('GET','/api/skin/online-preview?player='+encodeURIComponent(playerName));
  } else {
    r=await A('GET','/api/skin/preview?path='+encodeURIComponent(p));
  }
  if(!r.ok){T('分析失败: '+r.error,'e');document.getElementById('skin-pick-txt').textContent='选择皮肤 PNG 图片';return}
  document.getElementById('skin-pick-txt').textContent=isOnline?('在线: '+r.player):p.split('/').pop();
  // 自动检测手臂类型
  if(r.detected_arm_type){
    var sa=document.getElementById('skin-arm');
    if(sa&&r.detected_arm_type!==sa.getAttribute('data-value')){
      sa.setAttribute('data-value',r.detected_arm_type);
      sa.querySelector('.picker-label').textContent=r.detected_arm_type==='slim'?'Slim (2px)':'Classic (3px)'
    }
  }
  document.getElementById('skin-info').classList.remove('gone');
  document.getElementById('skin-info').innerHTML=
    '<div class="stats"><div class="s"><div class="n">'+r.blockCount+'</div><div class="l">方块</div></div>'+
    '<div class="s"><div class="n">'+r.width+'×'+r.height+'×'+r.length+'</div><div class="l">尺寸</div></div></div>';
  document.getElementById('skin-params').classList.remove('gone');
  document.getElementById('skin-go').classList.remove('gone');
  document.getElementById('skin-go').textContent='开始建造';
  document.getElementById('skin-go').disabled=false;
  // 3D 预览（使用当前设置）— 在线玩家皮肤暂不支持 3D 预览
  if(!isOnline){
    var sk = document.getElementById('skin-scale'), sa = document.getElementById('skin-arm'), sb = document.getElementById('skin-block');
    var vd=await A('GET','/api/skin/voxel?path='+encodeURIComponent(p)+'&scale='+(sk?sk.getAttribute('data-value'):2)+'&arm_type='+(sa?sa.getAttribute('data-value'):'classic')+'&block_set='+(sb?sb.getAttribute('data-value'):'mixed')+'&outer_thickness=1&rotation='+(document.getElementById('skin-rot')?document.getElementById('skin-rot').getAttribute('data-value'):0)+'');
    if(vd.ok&&vd.voxel&&!vd.voxel.too_large){
      var vc=document.createElement('div');vc.className='viewer';vc.id='viewer';
      document.getElementById('skin-preview-area').appendChild(vc);
      setTimeout(function(){if(typeof THREE!=='undefined')show3D(vc,vd.voxel)},200)
    }
  }
}

async function doSkinBuild(){
  if(checkUpdateLock())return;
  var btn=document.getElementById('skin-go');
  btn.disabled=true;btn.innerHTML='<span class="spin"></span> 启动中...';
  if(!skinPath){btn.innerHTML='开始建造';btn.disabled=false;T('请先选择皮肤文件','e');return}
  var isOnline = skinPath.indexOf('online://') === 0;
  var fd=new FormData();
  if(isOnline){
    fd.append('player',skinPath.replace('online://',''));
  } else {
    fd.append('path',skinPath);
  }
  fd.append('scale',document.getElementById('skin-scale').getAttribute('data-value')||2);
  fd.append('arm_type',document.getElementById('skin-arm').getAttribute('data-value')||'classic');
  fd.append('block_set',document.getElementById('skin-block').getAttribute('data-value')||'mixed');
  fd.append('rotation',document.getElementById('skin-rot').getAttribute('data-value')||0);
  fd.append('x',document.getElementById('skin-x').value||0);
  fd.append('y',document.getElementById('skin-y').value||0);
  fd.append('z',document.getElementById('skin-z').value||0);
  fd.append('speed',Math.min(Math.max(parseInt(document.getElementById('skin-spd').value)||9500,1),20000));
  var url = isOnline ? '/api/skin/online-build' : '/api/skin/build';
  var r=await fetch(url,{method:'POST',body:fd}).then(function(x){return x.json()});
  if(r.ok){btn.innerHTML='开始建造';btn.disabled=false;T('皮肤雕像任务已启动','o')}
  else{T('启动失败: '+r.error,'e');btn.innerHTML='开始建造';btn.disabled=false}
}

function doSkinStop(){A('POST','/api/task/stop');T('已停止')}
