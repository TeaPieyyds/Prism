var impPath='';

// ── 底部弹出选择器通用函数 ──
function showPicker(opts){
  var overlay=document.createElement('div');overlay.className='bs-overlay';
  var itemsHtml='';
  for(var i=0;i<opts.items.length;i++){
    var item=opts.items[i];
    var sel=item.value===opts.selected;
    itemsHtml+='<div class="bs-option'+(sel?' active':'')+'" data-value="'+item.value+'">'+
      '<div class="bs-dot"></div><span class="bs-label">'+item.label+'</span></div>'
  }
  overlay.innerHTML=
    '<div class="bs-sheet">'+
    (opts.title?'<div class="bs-title">'+opts.title+'</div>':'')+
    '<div class="bs-body">'+itemsHtml+'</div>'+
    '<div class="bs-footer">'+
    '<button class="bs-btn bs-btn-cancel">取消</button>'+
    '<button class="bs-btn bs-btn-confirm">确定</button></div></div>';
  document.body.appendChild(overlay);
  var selVal=opts.selected;
  overlay.querySelectorAll('.bs-option').forEach(function(opt){
    opt.addEventListener('click',function(){
      overlay.querySelectorAll('.bs-option').forEach(function(o){o.classList.remove('active')});
      this.classList.add('active');selVal=this.getAttribute('data-value')
    })
  });
  overlay.querySelector('.bs-btn-cancel').addEventListener('click',function(){closeOverlay(overlay)});
  overlay.querySelector('.bs-btn-confirm').addEventListener('click',function(){
    closeOverlay(overlay);if(opts.onConfirm)opts.onConfirm(selVal)
  });
  overlay.addEventListener('click',function(e){if(!e.target.closest('.bs-sheet'))closeOverlay(overlay)})
}

var ROT_ITEMS=[{value:'0',label:'0°'},{value:'90',label:'90°'},{value:'180',label:'180°'},{value:'270',label:'270°'}];
var DIM_ITEMS=[{value:'overworld',label:'主世界'},{value:'nether',label:'下界'},{value:'the_end',label:'末地'},{value:'dm',label:'DM'}];
var REGION_ITEMS=[{value:'1',label:'单区块 16×16（当前）'},{value:'2',label:'2×2 小区 32×32'},{value:'3',label:'4×4 小区 64×64'}];
var PRECLEAR_ITEMS=[{value:'0',label:'关闭'},{value:'1',label:'一格 — 清空建筑矩形空间'},{value:'2',label:'竖柱 — 基点到高度上限'},{value:'3',label:'区块 — 清空整区块列'}];
var PRECLEAR_SHORT={'0':'关闭','1':'一格','2':'竖柱','3':'区块'};
var EXP_ITEMS=[{value:'mcstructure',label:'MCStructure'},{value:'schematic',label:'Schematic'},{value:'mcworld',label:'MCWorld 世界文件'}];

function showRotPicker(){
  showPicker({title:'选择旋转',items:ROT_ITEMS,selected:document.getElementById('imp-rot-picker').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('imp-rot-picker');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=ROT_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showDimPicker(){
  showPicker({title:'选择维度',items:DIM_ITEMS,selected:document.getElementById('imp-dim-picker').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('imp-dim-picker');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=DIM_ITEMS.find(function(i){return i.value===v}).label;
      toggleDimInput()
    }})
}
function showRegionPicker(){
  showPicker({title:'选择导入粒度',items:REGION_ITEMS,selected:document.getElementById('imp-region-picker').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('imp-region-picker');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=REGION_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showPreclearPicker(){
  showPicker({title:'选择预清空模式',items:PRECLEAR_ITEMS,selected:document.getElementById('imp-preclear-picker').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('imp-preclear-picker');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=PRECLEAR_SHORT[v]||'关闭'
    }})
}
function showExpFmtPicker(){
  showPicker({title:'选择导出格式',items:EXP_ITEMS,selected:document.getElementById('exp-fmt-picker').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('exp-fmt-picker');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=EXP_ITEMS.find(function(i){return i.value===v}).label;
      onExpFmtChange()
    }})
}

function RimpHTML(){
  var html='<div class="card"><div class="pick" onclick="doPick()"><i class="pi">' + ICONS.upload + '</i><div id="imp-pick-txt">点击选择建筑文件</div><div style="font-size:10px;color:var(--phx-text-secondary)">.mcstructure .schematic .schem .bdx .mcworld</div></div>'+
    '<div class="mode-cap gone" id="imp-mode-cap">'+
    '<div class="mode-cap-thumb" id="imp-mode-thumb"></div>'+
    '<button class="mode-cap-btn active" data-mode="import" onclick="setImpMode(\'import\')">导入模式</button>'+
    (cfgBotTokens.length>0?'<button class="mode-cap-btn" data-mode="multi" onclick="setImpMode(\'multi\')">多机器人</button>':'')+
    '<button class="mode-cap-btn" data-mode="repair" onclick="setImpMode(\'repair\')">修补模式</button>'+
    '</div>'+
    '<div id="imp-info" class="gone"></div>'+
    '<div id="imp-mcworld-params" class="gone">'+
    '<label>起点坐标</label><div class="row"><input id="imp-x1" type="number" placeholder="X"><input id="imp-y1" type="number" placeholder="Y"><input id="imp-z1" type="number" placeholder="Z"></div>'+
    '<label>终点坐标</label><div class="row"><input id="imp-x2" type="number" placeholder="X"><input id="imp-y2" type="number" placeholder="Y"><input id="imp-z2" type="number" placeholder="Z"></div>'+
    '<button class="btn" id="imp-reanalyze" onclick="doReanalyze()" style="width:auto;padding:8px 14px">重新解析</button></div>'+
    '<div id="imp-params" class="gone">'+
    // ── 常规 ──
    '<label>放置坐标</label><div class="row"><input id="imp-x" type="number" placeholder="X"><input id="imp-y" type="number" placeholder="Y"><input id="imp-z" type="number" placeholder="Z"></div>'+
    '<label>旋转</label><div class="picker-btn" id="imp-rot-picker" data-value="0" onclick="showRotPicker()"><span class="picker-label">0°</span><span class="picker-arrow">▾</span></div>'+
    '<div id="imp-cmd-opts" class="gone qopt">'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-cmds" checked onchange="toggleSub(\'imp-cmds\',\'imp-cmd-sub\')"><span class="tgl"></span> 导入指令</label>'+
    '<div id="imp-cmd-sub" class="qopt-sub">'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-cmd-disabled"><span class="tgl"></span> 关闭状态下导入</label>'+
    '<div class="dim" style="font-size:10px">开启后，循环命令方块始终以红石控制模式导入</div>'+
    '</div></div>'+
    '<label>速度（方块/秒）</label><input id="imp-spd" type="number" value="'+cfgSpeed+'">'+
    // ── 高级（折叠） ──
    '<div id="imp-adv" class="gone" style="margin-top:12px;padding-top:12px;border-top:2px solid var(--phx-border-light)">'+
    '<div id="imp-fluid-opts" class="qopt">'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-exclude-fluids" onchange="toggleSub(\'imp-exclude-fluids\',\'imp-fluid-sub\')"><span class="tgl"></span> 排除流体方块</label>'+
    '<div id="imp-fluid-sub" class="gone qopt-sub">'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-exclude-water" checked><span class="tgl"></span> 排除水</label>'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-exclude-waterlogged" checked><span class="tgl"></span> 排除含水方块</label>'+
    '<div class="dim" style="font-size:10px;margin-bottom:4px">仅放置方块本身，不放置其中的水</div>'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-exclude-lava" checked><span class="tgl"></span> 排除岩浆</label>'+
    '</div></div>'+
    '<div id="imp-deny-opts" class="qopt">'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-deny" onchange="toggleSub(\'imp-deny\',\'imp-deny-sub\')"><span class="tgl"></span> 拒绝方块层</label>'+
    '<div id="imp-deny-sub" class="gone qopt-sub">'+
    '<label>Y偏移</label><input id="imp-deny-offset" type="number" value="-1" style="width:80px">'+
    '<div class="dim" style="font-size:10px">导入坐标下方格数（负=下方）</div>'+
    '</div></div>'+
    '<div id="imp-border-opts" class="qopt">'+
    '<label class="lbl-cb"><input type="checkbox" id="imp-border" onchange="toggleSub(\'imp-border\',\'imp-border-sub\')"><span class="tgl"></span> 边界方块</label>'+
    '<div id="imp-border-sub" class="gone qopt-sub">'+
    '<label>Y坐标</label><input id="imp-border-y" type="number" value="0" style="width:80px">'+
    '</div></div>'+
    '<label>维度</label><div class="picker-btn" id="imp-dim-picker" data-value="overworld" onclick="showDimPicker()"><span class="picker-label">主世界</span><span class="picker-arrow">▾</span></div>'+
    '<div id="imp-dim-dm" class="gone"><label>DM ID</label><input id="imp-dim-id" type="number" value="3" style="width:80px"></div>'+
    '<label>导入粒度</label><div class="picker-btn" id="imp-region-picker" data-value="'+cfgRegionMode+'" onclick="showRegionPicker()"><span class="picker-label">'+(cfgRegionMode==='2'?'2×2 小区 32×32':cfgRegionMode==='3'?'4×4 小区 64×64':'单区块 16×16（当前）')+'</span><span class="picker-arrow">▾</span></div>'+
    // —— 预清空（放在最下面） ——
    '<div id="imp-preclear-opts" class="qopt" style="margin-top:12px;padding-top:12px;border-top:2px solid var(--phx-border-light)">'+
    '<label style="font-weight:700;margin-bottom:4px">预清空</label>'+
    '<div class="picker-btn" id="imp-preclear-picker" data-value="0" onclick="showPreclearPicker()"><span class="picker-label">关闭</span><span class="picker-arrow">▾</span></div>'+
    '<div class="dim" style="font-size:10px;margin-top:4px">导入前先用空气填充目标区域，清空完成后才导入建筑</div>'+
    '</div>'+
    '</div>'+  // end imp-adv
    // ── 展开按钮 ──
    '<button class="btn-s" id="imp-adv-btn" onclick="toggleAdvImp()" style="width:100%;margin-top:10px;display:none">展开高级选项</button></div>'+
    // ── 修补模式参数 ──
    '<div id="imp-repair-params" class="gone">'+
    '<label style="font-weight:700;color:var(--phx-primary);margin-top:8px">修补模式</label>'+
    '<div class="dim" style="font-size:10px;margin-bottom:6px">起始坐标必须与导入时一致</div>'+
    '<label>起始坐标</label><div class="row"><input id="rep-x" type="number" placeholder="X"><input id="rep-y" type="number" placeholder="Y"><input id="rep-z" type="number" placeholder="Z"></div>'+
    '<label>修补区域</label><div class="picker-btn" id="rep-area-picker" data-value="rect" onclick="showRepairAreaPicker()"><span class="picker-label">矩形范围</span><span class="picker-arrow">▾</span></div>'+
    '<div id="rep-rect-params">'+
    '<label>起点</label><div class="row"><input id="rep-x1" type="number" placeholder="X"><input id="rep-y1" type="number" placeholder="Y"><input id="rep-z1" type="number" placeholder="Z"></div>'+
    '<label>终点</label><div class="row"><input id="rep-x2" type="number" placeholder="X"><input id="rep-y2" type="number" placeholder="Y"><input id="rep-z2" type="number" placeholder="Z"></div></div>'+
    '<div id="rep-circle-params" class="gone">'+
    '<label>中心坐标</label><div class="row"><input id="rep-cx" type="number" placeholder="X"><input id="rep-cz" type="number" placeholder="Z"></div>'+
    '<label>半径</label><input id="rep-radius" type="number" placeholder="半径" value="16">'+
    '<button class="btn-s" onclick="fillBotPos()" style="width:auto;padding:8px 14px;margin:4px 0;font-size:12px">使用机器人当前位置</button></div>'+
    '<label>旋转</label><div class="picker-btn" id="rep-rot-picker" data-value="0" onclick="showRepRotPicker()"><span class="picker-label">0°</span><span class="picker-arrow">▾</span></div>'+
    '<label>速度（方块/秒）</label><input id="rep-spd" type="number" value="20">'+
    '<div class="dim" style="font-size:10px">修补模式默认速度 20，建议不超过 50</div>'+
    '<label>维度</label><div class="picker-btn" id="rep-dim-picker" data-value="overworld" onclick="showRepDimPicker()"><span class="picker-label">主世界</span><span class="picker-arrow">▾</span></div>'+
    '<div class="qopt" style="margin-top:8px;padding-top:8px;border-top:2px solid var(--phx-border-light)">'+
    '<label class="lbl-cb"><input type="checkbox" id="rep-nbt-only"><span class="tgl"></span> 仅修补NBT</label>'+
    '<div class="dim" style="font-size:10px;margin-bottom:4px">只修补命令方块/容器/告示牌等NBT数据，跳过普通方块</div>'+
    '<label class="lbl-cb"><input type="checkbox" id="rep-cmds" checked><span class="tgl"></span> 导入指令</label>'+
    '<label class="lbl-cb"><input type="checkbox" id="rep-cmd-disabled"><span class="tgl"></span> 关闭状态下导入</label>'+
    '</div>'+
    '</div>'+
    '<div class="row"><button class="btn gone" id="imp-go" onclick="doImp()">开始导入</button>'+
    '<button class="btn-d gone" id="imp-stop" onclick="doImpStop()">停止导入</button></div>'+
    '<div class="pg gone" id="imp-pg"><div id="imp-bar"></div></div>'+
    '<div class="gone" id="imp-multi-pg" style="margin-top:6px">'+
    '<div class="mp-row"><span class="mp-label">普通</span><div class="mp-bar"><div id="mp-bar-normal" style="width:0%"></div></div><span class="mp-pct" id="mp-pct-normal">0%</span></div>'+
    '<div class="mp-row"><span class="mp-label" style="color:#d473ff">NBT</span><div class="mp-bar"><div id="mp-bar-nbt" style="width:0%;background:#d473ff"></div></div><span class="mp-pct" id="mp-pct-nbt">0%</span></div>'+
    '<div class="mp-row"><span class="mp-label" style="color:#63b3ed">告示牌</span><div class="mp-bar"><div id="mp-bar-sign" style="width:0%;background:#63b3ed"></div></div><span class="mp-pct" id="mp-pct-sign">0%</span></div>'+
    '</div>'+
    '<div class="pg gone" id="clear-pg"><div id="clear-bar" style="background:var(--phx-error)"></div></div>'+
    '<div class="dim gone" id="clear-stat"></div>'+
    '<div class="dim" id="imp-stat"></div></div>'+
    '<div class="card gone" id="imp-log-card"><div class="log" id="imp-log"></div></div>';
  setTimeout(function(){
    document.getElementById('imp-cmds').checked=cfgImportCommands;
    toggleSub('imp-cmds','imp-cmd-sub');
    document.getElementById('imp-exclude-fluids').checked=cfgExcludeFluids;
    toggleSub('imp-exclude-fluids','imp-fluid-sub');
    document.getElementById('imp-exclude-water').checked=cfgExcludeWater;
    document.getElementById('imp-exclude-waterlogged').checked=cfgExcludeWaterlogged;
    document.getElementById('imp-exclude-lava').checked=cfgExcludeLava;
    document.getElementById('imp-spd').value=cfgSpeed;
    document.getElementById('imp-cmd-disabled').checked=cfgCmdDisabled;
    var preclearPicker=document.getElementById('imp-preclear-picker');
    if(preclearPicker){preclearPicker.setAttribute('data-value',String(cfgPreClear));preclearPicker.querySelector('.picker-label').textContent=PRECLEAR_SHORT[String(cfgPreClear)]||'关闭'}
  },10);
  return html
}

function RexpHTML(){
  return '<div class="card"><div style="font-size:16px;font-weight:800;margin-bottom:12px">导出区域</div>'+
    '<label>起点</label><div class="row"><input id="exp-x1" type="number" placeholder="X"><input id="exp-y1" type="number" placeholder="Y"><input id="exp-z1" type="number" placeholder="Z"></div>'+
    '<label>终点</label><div class="row"><input id="exp-x2" type="number" placeholder="X"><input id="exp-y2" type="number" placeholder="Y"><input id="exp-z2" type="number" placeholder="Z"></div>'+
    '<label>格式</label><div class="picker-btn" id="exp-fmt-picker" data-value="mcstructure" onclick="showExpFmtPicker()"><span class="picker-label">MCStructure</span><span class="picker-arrow">▾</span></div>'+
    '<label>文件名</label><input id="exp-name" value="" placeholder="文件名称" oninput="onExpFmtChange()">'+
    '<div class="dim" id="exp-name-preview" style="font-size:11px;margin:4px 0;color:var(--phx-text-secondary)"></div>'+
    '<div class="row"><button class="btn" onclick="doExpGo()" id="exp-go">开始导出</button>'+
    '<button class="btn-d gone" id="exp-stop" onclick="doExpStop()">停止导出</button></div>'+
    '<div class="pg gone" id="exp-pg"><div id="exp-bar"></div></div></div>'+
    '<div class="card gone" id="exp-log-card"><div class="log" id="exp-log"></div></div>'
}

function RsearchHTML(){
  return '<div class="card">'+
    '<div style="display:flex;gap:4px;margin-bottom:12px">'+
    '<button class="btn-s sc-tab" data-mode="server" onclick="setScMode(\'server\')" style="flex:1;margin:0;font-size:12px;padding:8px;background:var(--phx-primary-bg);color:var(--phx-primary);border-color:var(--phx-primary)">&#9776; 租赁服</button>'+
    '<button class="btn-s sc-tab" data-mode="player" onclick="setScMode(\'player\')" style="flex:1;margin:0;font-size:12px;padding:8px">&#128101; 玩家</button>'+
    '</div>'+
    '<div id="sc-hint" style="font-size:11px;color:var(--phx-text-secondary);margin-bottom:8px">租赁服搜索：按名称搜索网易我的世界租赁服</div>'+
    '<div class="row"><input id="sc-query" placeholder="搜索..." style="flex:1" onkeydown="if(event.key===\'Enter\')doSearch()"><button class="btn" onclick="doSearch()" style="width:auto;margin:0;padding:11px 16px;font-size:13px">搜索</button></div>'+
    '<div id="sc-results" style="margin-top:12px"></div></div>'
}

function toggleSub(cbId, subId){
  var sub=document.getElementById(subId);
  var cb=document.getElementById(cbId);
  if(sub)sub.classList.toggle('gone',!cb||!cb.checked)
}
// ═══ 修补模式 ═══
var impMode='import';
function setImpMode(mode){
  impMode=mode;
  var thumb=document.getElementById('imp-mode-thumb');
  var btns=document.querySelectorAll('#imp-mode-cap .mode-cap-btn');
  btns.forEach(function(b){b.classList.toggle('active',b.getAttribute('data-mode')===mode)});
  // 滑块帽宽/位按实际按钮数动态计算：导入/多机器人/修补（2 或 3 个选项）都能正确铺满，
  // 否则多机器人选项加入后帽仍是 50% 宽、只认两个位置（胶囊不动/大小不对）。
  if(thumb&&btns.length>0){
    var idx=0;
    btns.forEach(function(b,i){if(b.getAttribute('data-mode')===mode)idx=i});
    var pct=100/btns.length;
    thumb.style.width='calc('+pct+'% - 4px)';
    thumb.style.left='calc('+(pct*idx)+'% + 4px)';
  }
  var impParams=document.getElementById('imp-params');
  var repParams=document.getElementById('imp-repair-params');
  if(impParams)impParams.classList.toggle('gone',mode!=='import'&&mode!=='multi');
  if(repParams)repParams.classList.toggle('gone',mode!=='repair');
  var btn=document.getElementById('imp-go');
  if(btn){
    if(mode==='repair'){
      btn.textContent='开始修补';
      btn.onclick=function(){doRepair()};
    }else{
      btn.textContent='开始导入';
      btn.onclick=function(){doImp()};
    }
  }
}
function showRepairAreaPicker(){
  showPicker({title:'选择修补区域类型',items:[
    {value:'rect',label:'矩形范围 — 输入起点终点'},
    {value:'circle',label:'圆形范围 — 区块对齐'}
  ],selected:document.getElementById('rep-area-picker').getAttribute('data-value'),
  onConfirm:function(v){
    var p=document.getElementById('rep-area-picker');
    p.setAttribute('data-value',v);
    p.querySelector('.picker-label').textContent=v==='circle'?'圆形范围（区块对齐）':'矩形范围';
    document.getElementById('rep-rect-params').classList.toggle('gone',v!=='rect');
    document.getElementById('rep-circle-params').classList.toggle('gone',v!=='circle');
  }})
}
function showRepRotPicker(){
  showPicker({title:'选择旋转',items:ROT_ITEMS,selected:document.getElementById('rep-rot-picker').getAttribute('data-value'),
  onConfirm:function(v){
    var p=document.getElementById('rep-rot-picker');p.setAttribute('data-value',v);
    p.querySelector('.picker-label').textContent=ROT_ITEMS.find(function(i){return i.value===v}).label
  }})
}
function showRepDimPicker(){
  showPicker({title:'选择维度',items:DIM_ITEMS,selected:document.getElementById('rep-dim-picker').getAttribute('data-value'),
  onConfirm:function(v){
    var p=document.getElementById('rep-dim-picker');p.setAttribute('data-value',v);
    p.querySelector('.picker-label').textContent=DIM_ITEMS.find(function(i){return i.value===v}).label
  }})
}
async function fillBotPos(){
  var r=await A('GET','/api/fly/position');
  if(!r.ok){T('获取位置失败: '+(r.error||'未知'),'e');return}
  var cx=document.getElementById('rep-cx');
  var cz=document.getElementById('rep-cz');
  if(cx)cx.value=Math.round(r.x);
  if(cz)cz.value=Math.round(r.z);
  T('已填入机器人当前位置 ('+Math.round(r.x)+', '+Math.round(r.z)+')','o')
}
async function doRepair(){
  if(checkUpdateLock())return;
  var btn=document.getElementById('imp-go');if(!btn)return;
  btn.disabled=true;btn.innerHTML='<span class="spin"></span> 启动中...';
  document.getElementById('imp-stop').classList.remove('gone');
  document.getElementById('imp-pg').classList.remove('gone');
  document.getElementById('imp-log-card').classList.remove('gone');
  document.getElementById('imp-bar').style.width='0%';
  var rx=parseInt(document.getElementById('rep-x').value)||0;
  var ry=parseInt(document.getElementById('rep-y').value)||0;
  var rz=parseInt(document.getElementById('rep-z').value)||0;
  if(!rx&&!ry&&!rz){T('请填写起始坐标','e');btn.innerHTML='开始修补';btn.disabled=false;return}
  var spd=Math.min(200,Math.max(1,parseInt(document.getElementById('rep-spd').value)||20));
  document.getElementById('rep-spd').value=spd;
  var areaType=document.getElementById('rep-area-picker').getAttribute('data-value');
  var body={
    path:impPath,
    x:rx,
    y:ry,
    z:rz,
    speed:spd,
    rotation:parseInt(document.getElementById('rep-rot-picker').getAttribute('data-value'))||0,
    dimension:document.getElementById('rep-dim-picker').getAttribute('data-value'),
    repair_circle:areaType==='circle',
    repair_nbt_only:document.getElementById('rep-nbt-only').checked,
    repair_cx:parseInt(document.getElementById('rep-cx').value)||0,
    repair_cz:parseInt(document.getElementById('rep-cz').value)||0,
    repair_radius:parseInt(document.getElementById('rep-radius').value)||16,
    repair_x1:parseInt(document.getElementById('rep-x1').value)||0,
    repair_y1:parseInt(document.getElementById('rep-y1').value)||0,
    repair_z1:parseInt(document.getElementById('rep-z1').value)||0,
    repair_x2:parseInt(document.getElementById('rep-x2').value)||0,
    repair_y2:parseInt(document.getElementById('rep-y2').value)||0,
    repair_z2:parseInt(document.getElementById('rep-z2').value)||0,
    import_commands:document.getElementById('rep-cmds').checked,
    cmd_disabled:document.getElementById('rep-cmd-disabled').checked
  };
  var r=await A('POST','/api/repair/start',body);
  if(r.ok){btn.innerHTML='开始修补';btn.disabled=false;T('修补任务已启动','o')}
  else{T('启动失败: '+r.error,'e');btn.innerHTML='开始修补';btn.disabled=false;document.getElementById('imp-stop').classList.add('gone')}
}

function toggleAdvImp(){
  var adv=document.getElementById('imp-adv');
  var btn=document.getElementById('imp-adv-btn');
  if(!adv||!btn)return;
  var open=adv.classList.contains('gone');
  adv.classList.toggle('gone');
  btn.textContent=open?'收起高级选项':'展开高级选项'
}
function toggleDimInput(){
  var el=document.getElementById('imp-dim-dm');
  var picker=document.getElementById('imp-dim-picker');
  if(el)el.classList.toggle('gone',!picker||picker.getAttribute('data-value')!='dm')
}
async function doPick(){
  var p=useBuiltinPicker?await fpicker('building'):await pick('building');if(!p)return;impPath=p;var isMCWorld=p.toLowerCase().endsWith('.mcworld');
  if(isMCWorld){
    var m=p.match(/\[(-?\d+)[,\s]+(-?\d+)[,\s]+(-?\d+)\]\s*~\s*\[(-?\d+)[,\s]+(-?\d+)[,\s]+(-?\d+)\]/);
    if(m){document.getElementById('imp-x1').value=m[1];document.getElementById('imp-y1').value=m[2];document.getElementById('imp-z1').value=m[3];
      document.getElementById('imp-x2').value=m[4];document.getElementById('imp-y2').value=m[5];document.getElementById('imp-z2').value=m[6]}
  }
  await doAnalyze(p,isMCWorld)
}

async function doAnalyze(p,isMCWorld){
  // 重置为导入模式
  if(typeof setImpMode==='function')setImpMode('import');
  // 清理旧 3D 预览（THREE.js 场景和 WebGL 上下文），避免内存累积和旧预览残留
  activeRenderers.forEach(function(kill){try{kill()}catch(e){}});activeRenderers=[];
  document.getElementById('imp-pick-txt').innerHTML='<span class="spin"></span> 分析中...';
  var body={path:p};
  if(isMCWorld){body.x1=I('imp-x1',0);body.y1=I('imp-y1',-64);body.z1=I('imp-z1',0);body.x2=I('imp-x2',256);body.y2=I('imp-y2',256);body.z2=I('imp-z2',256)}
  var r=await A('POST','/api/building/analyze',body);
  if(!r.ok){T('分析失败: '+r.error,'e');document.getElementById('imp-pick-txt').innerHTML='点击选择建筑文件';return}
  var s=r.stats;
  document.getElementById('imp-pick-txt').textContent='已选择: '+p.split('/').pop();
  document.getElementById('imp-info').classList.remove('gone');
  document.getElementById('imp-mode-cap').classList.remove('gone');
  var nbtSub = [];
  if(s.cmd_blocks) nbtSub.push('命令' + s.cmd_blocks);
  if(s.container_items) nbtSub.push('容器' + s.container_items);
  var nbtSubHtml = nbtSub.length ? '<div style="font-size:9px;color:var(--phx-text-secondary);font-weight:400">' + nbtSub.join(' ') + '</div>' : '';
  document.getElementById('imp-info').innerHTML=
    '<div class="stats">'+
    '<div class="s"><div class="n">'+s.solid_blocks+'</div><div class="l">实体方块</div></div>'+
    '<div class="s"><div class="n">'+s.nbt_blocks+'</div><div class="l">NBT' + nbtSubHtml + '</div></div>'+
    '<div class="s"><div class="n">'+s.water_blocks+'</div><div class="l">含水</div></div>'+
    '<div class="s"><div class="n">'+s.size_x+'×'+s.size_y+'×'+s.size_z+'</div><div class="l">尺寸</div></div></div>'+
    (s.block_counts_list?'<div style="font-size:11px;max-height:100px;overflow-y:auto;margin-top:6px">'+s.block_counts_list.slice(0,15).map(function(b){return'<div style="display:flex;justify-content:space-between;padding:3px 6px;border-bottom:1px solid var(--phx-border-light)"><span>'+b.name+'</span><span style="color:var(--phx-primary);font-weight:700">'+b.count+'</span></div>'}).join('')+'</div>':'');
  if(isMCWorld){
    document.getElementById('imp-mcworld-params').classList.remove('gone');
  }else{document.getElementById('imp-mcworld-params').classList.add('gone')}
  document.getElementById('imp-params').classList.remove('gone');
  document.getElementById('imp-go').classList.remove('gone');
  document.getElementById('imp-go').textContent='开始导入';
  document.getElementById('imp-go').disabled=false;
  document.getElementById('imp-adv-btn').style.display='';
  document.getElementById('imp-stop').classList.add('gone');
  document.getElementById('imp-pg').classList.add('gone');
  document.getElementById('imp-log-card').classList.add('gone');
  var impCmds=document.getElementById('imp-cmds');
  if(impCmds)impCmds.checked=cfgImportCommands;
  toggleSub('imp-cmds','imp-cmd-sub');
  if(s.cmd_blocks&&s.cmd_blocks>0){
    document.getElementById('imp-cmd-opts').classList.remove('gone')
  }else{document.getElementById('imp-cmd-opts').classList.add('gone')}
  // 3D 体素预览（原版方案：动态创建容器 + setTimeout）
  if(r.voxel_deferred){
    // 大文件：保留预览容器，但先不解析、不渲染，只显示警告，点击才按需生成
    var v=document.createElement('div');v.className='viewer';v.id='viewer';
    v.style.display='flex';v.style.alignItems='center';v.style.justifyContent='center';
    v.style.flexDirection='column';v.style.gap='8px';
    v.innerHTML='<div style="font-size:12px;color:var(--phx-text-secondary);text-align:center;padding:0 12px">方块较多（'+s.solid_blocks+'），3D 预览已暂停</div>'+
      '<button class="btn-s" onclick="doGenVoxel()" id="voxel-gen-btn" style="width:auto;padding:8px 16px;font-size:12px">仍要有预览</button>';
    document.getElementById('imp-info').appendChild(v);
  } else if(r.voxel && !r.voxel.too_large){
    var v=document.createElement('div');v.className='viewer';v.id='viewer';
    document.getElementById('imp-info').appendChild(v);
    setTimeout(function(){if(typeof THREE!=='undefined')show3D(v,r.voxel)},200)
  }
}

// 按需生成 3D 预览（大文件点击"仍要有预览"时触发）
async function doGenVoxel(){
  if(!impPath)return;
  var v=document.getElementById('viewer');
  if(!v)return;
  v.innerHTML='<div style="display:flex;align-items:center;gap:8px;font-size:12px;color:var(--phx-text-secondary)"><span class="spin"></span> 正在生成预览...</div>';
  var r=await A('POST','/api/building/voxel',{path:impPath});
  if(!r.ok||r.error){v.innerHTML='<div style="font-size:12px;color:var(--phx-error)">生成失败: '+(r.error||'未知')+'</div>';return}
  if(!r.voxel||r.voxel.too_large||!r.voxel.points||!r.voxel.points.length){v.innerHTML='<div style="font-size:12px;color:var(--phx-text-secondary)">该建筑体积过大，无法预览</div>';return}
  v.innerHTML='';
  if(typeof THREE!=='undefined')show3D(v,r.voxel)
}

async function doReanalyze(){
  if(!impPath)return;
  await doAnalyze(impPath,impPath.toLowerCase().endsWith('.mcworld'))
}

async function doImp(){
  if(checkUpdateLock())return;
  var btn=document.getElementById('imp-go');if(!btn)return;
  btn.disabled=true;btn.innerHTML='<span class="spin"></span> 启动中...';
  document.getElementById('imp-stop').classList.remove('gone');
  document.getElementById('imp-pg').classList.remove('gone');
  document.getElementById('imp-log-card').classList.remove('gone');
  document.getElementById('imp-bar').style.width='0%';

  var spd=Math.min(20000,Math.max(1,parseInt(document.getElementById('imp-spd').value)||9500));
  document.getElementById('imp-spd').value=spd;
  var cmdOpts={};
  if(document.getElementById('imp-cmds').checked){
    cmdOpts.cmd_disabled=document.getElementById('imp-cmd-disabled').checked
  }
  var isMulti=impMode==='multi';
  var body={
    type:isMulti?'import_multi':'import',params:{
      path:impPath,
      x:parseInt(document.getElementById('imp-x').value)||0,
      y:parseInt(document.getElementById('imp-y').value)||0,
      z:parseInt(document.getElementById('imp-z').value)||0,
      rotation:parseInt(document.getElementById('imp-rot-picker').getAttribute('data-value'))||0,
      speed:spd,
      import_commands:document.getElementById('imp-cmds').checked,
      cmd_disabled:document.getElementById('imp-cmd-disabled').checked,
      exclude_fluids:document.getElementById('imp-exclude-fluids').checked,
      exclude_water:document.getElementById('imp-exclude-water').checked,
      exclude_waterlogged:document.getElementById('imp-exclude-waterlogged').checked,
      exclude_lava:document.getElementById('imp-exclude-lava').checked,
      deny_enable:document.getElementById('imp-deny')?.checked||false,
      deny_y_offset:parseInt(document.getElementById('imp-deny-offset')?.value)||-1,
      border_enable:document.getElementById('imp-border')?.checked||false,
      border_y:parseInt(document.getElementById('imp-border-y')?.value)||0,
      border_block:'border_block',
      dimension:document.getElementById('imp-dim-picker').getAttribute('data-value'),
      dm_id:document.getElementById('imp-dim-id')?.value||'3',
      region_mode:parseInt(document.getElementById('imp-region-picker').getAttribute('data-value'))||1,
      pre_clear_mode:parseInt(document.getElementById('imp-preclear-picker').getAttribute('data-value'))||0
    }
  };
  if(impPath.toLowerCase().endsWith('.mcworld')){
    body.params.x1=I('imp-x1',0);body.params.y1=I('imp-y1',-64);body.params.z1=I('imp-z1',0);
    body.params.x2=I('imp-x2',256);body.params.y2=I('imp-y2',256);body.params.z2=I('imp-z2',256)
  }
  var r=await A('POST','/api/task/start',body);
  if(r.ok){btn.innerHTML='开始导入';btn.disabled=false;T('任务已启动','o')}
  else{T('启动失败: '+r.error,'e');btn.innerHTML='开始导入';btn.disabled=false;document.getElementById('imp-stop').classList.add('gone')}
}

function doImpStop(){A('POST','/api/task/stop');T('已停止')}

// ── 搜索 ──
async function doSearch(){
  var inp=document.getElementById('sc-query');if(!inp)return;
  var kw=inp.value.trim();if(!kw)return;
  var btn=document.getElementById('sc-btn');if(btn){btn.disabled=true;btn.innerHTML='<span class="spin"></span>'}
  var res=document.getElementById('sc-results');
  if(res)res.innerHTML='<div style="text-align:center;padding:16px"><span class="spin" style="color:var(--phx-primary)"></span></div>';
  try{
    if(scMode==='server'){
      var r=await APrism('GET','/api/server/find?keyword='+encodeURIComponent(kw));
      if(btn){btn.disabled=false;btn.textContent='搜索'}
      if(!r.ok){if(res)res.innerHTML='<div style="text-align:center;padding:16px;color:var(--phx-error)">'+escHtml(r.error)+'</div>';return}
      if(!r.servers||!r.servers.length){if(res)res.innerHTML='<div style="text-align:center;padding:16px;color:var(--phx-text-secondary);font-size:12px">未找到匹配的租赁服</div>';return}
      var h='<div style="font-size:11px;color:var(--phx-text-secondary);font-weight:700;margin-bottom:6px">找到 '+r.servers.length+' 个租赁服</div>';
      for(var i=0;i<Math.min(r.servers.length,20);i++){
        var sv=r.servers[i];
        h+='<div class="qcard" style="cursor:pointer;margin-bottom:6px" onclick="showServerDetail(\''+escHtml(sv.entity_id||'')+'\',\''+escHtml(sv.server_name||sv.name||'未知')+'\')">'+
          (sv.image_url?'<img src="'+sv.image_url+'" style="width:36px;height:36px;border-radius:10px;flex-shrink:0" onerror="this.style.display=\'none\'">':'<div style="width:36px;height:36px;border-radius:10px;background:var(--phx-border-light);flex-shrink:0;display:flex;align-items:center;justify-content:center;font-size:14px">&#9906;</div>')+
          '<div class="qc-body"><div class="qc-title">'+escHtml(sv.server_name||sv.name||'未知')+'</div>'+
          '<div class="qc-sub">在线 '+(sv.player_count!=null?sv.player_count:'?')+'/'+(sv.capacity!=null?sv.capacity:'?')+'</div></div>'+
          '<span class="qc-arrow">&#8250;</span></div>'
      }
      if(r.servers.length>20)h+='<div class="dim" style="text-align:center;font-size:10px">仅显示前 20 条结果</div>';
      if(res)res.innerHTML=h
    }else{
      var r=await APrism('GET','/api/social/search?keyword='+encodeURIComponent(kw));
      if(btn){btn.disabled=false;btn.textContent='搜索'}
      if(!r.ok){if(res)res.innerHTML='<div style="text-align:center;padding:16px;color:var(--phx-error)">'+escHtml(r.error)+'</div>';return}
      if(!r.users||!r.users.length){if(res)res.innerHTML='<div style="text-align:center;padding:16px;color:var(--phx-text-secondary);font-size:12px">未找到匹配的玩家</div>';return}
      var h='<div style="font-size:11px;color:var(--phx-text-secondary);font-weight:700;margin-bottom:6px">找到 '+r.users.length+' 个玩家</div>';
      for(var i=0;i<Math.min(r.users.length,20);i++){
        var u=r.users[i];
        h+='<div class="qcard" style="cursor:pointer;margin-bottom:6px" onclick="showPlayerDetail(\''+escHtml(u.uid||u.user_id||'')+'\',\''+escHtml(u.nickname||u.name||'未知')+'\')">'+
          (u.headImage?'<img src="'+u.headImage+'" style="width:36px;height:36px;border-radius:10px;flex-shrink:0" onerror="this.style.display=\'none\'">':'<div style="width:36px;height:36px;border-radius:10px;background:var(--phx-border-light);flex-shrink:0;display:flex;align-items:center;justify-content:center;font-size:14px">&#9787;</div>')+
          '<div class="qc-body"><div class="qc-title">'+escHtml(u.nickname||u.name||'未知')+'</div>'+
          '<div class="qc-sub">Lv.'+(u.pe_growth&&u.pe_growth.lv!=null?u.pe_growth.lv:'?')+'</div></div>'+
          '<span class="qc-arrow">&#8250;</span></div>'
      }
      if(res)res.innerHTML=h
    }
  }catch(e){
    if(btn){btn.disabled=false;btn.textContent='搜索'}
    if(res)res.innerHTML='<div style="text-align:center;padding:16px;color:var(--phx-error)">请求失败: '+e.message+'</div>'
  }
}

async function showServerDetail(sid,sname){
  if(!sid){T('缺少服务器ID','e');return}
  var r=await APrism('GET','/api/server/detail?server_id='+encodeURIComponent(sid));
  var sv=r.ok&&r.server?r.server:null;
  var plR=await APrism('GET','/api/server/players?server_id='+encodeURIComponent(sid)+'&length=10');
  var m=document.createElement('div');m.className='modal-overlay';
  var h='<div class="modal-box" style="max-width:380px;max-height:85vh;display:flex;flex-direction:column;padding:18px 20px">'+
    '<div style="display:flex;align-items:center;gap:8px;margin-bottom:10px;flex-shrink:0">'+
    (sv&&sv.image_url?'<img src="'+sv.image_url+'" style="width:36px;height:36px;border-radius:10px;flex-shrink:0" onerror="this.style.display=\'none\'">':'')+
    '<div style="flex:1;min-width:0"><div style="font-size:15px;font-weight:800">'+escHtml(sname)+'</div>'+
    (sv?'<div class="dim" style="font-size:10px">ID: '+escHtml(sid)+'</div>':'')+
    '</div></div>';
  if(sv){
    h+='<div class="stats" style="grid-template-columns:1fr 1fr 1fr">'+
      '<div class="s"><div class="n">'+(sv.player_count!=null?sv.player_count:'?')+'/'+(sv.capacity!=null?sv.capacity:'?')+'</div><div class="l">在线</div></div>'+
      '<div class="s"><div class="n">'+(sv.mc_version||'?')+'</div><div class="l">版本</div></div>'+
      '<div class="s"><div class="n">'+(sv.like_num||0)+'</div><div class="l">点赞</div></div></div>'
  }
  if(plR.ok&&plR.players&&plR.players.length){
    h+='<div style="margin-top:8px;padding-top:8px;border-top:1px solid var(--phx-border-light)">'+
      '<div class="dim" style="font-size:10px;font-weight:700;margin-bottom:6px">在线玩家 ('+plR.players.length+')</div>';
    for(var i=0;i<Math.min(plR.players.length,10);i++){
      var p=plR.players[i];
      h+='<div style="display:flex;align-items:center;gap:8px;padding:5px 0;border-bottom:1px solid var(--phx-border-light);font-size:12px">'+
        (p.head_image?'<img src="'+p.head_image+'" style="width:24px;height:24px;border-radius:6px;flex-shrink:0" onerror="this.style.display=\'none\'">':'')+
        '<span style="flex:1;font-weight:600">'+escHtml(p.name||'未知')+'</span>'+
        '<span class="dim">Lv.'+(p.level||'?')+'</span></div>'
    }
    h+='</div>'
  }
  h+='<button class="btn-s" style="width:100%;margin-top:10px;flex-shrink:0" onclick="closeModalOverlay(this)">关闭</button></div>';
  m.innerHTML=h;document.body.appendChild(m);
  m.addEventListener('click',function(e){if(e.target===m)closeOverlay(m)})
}

async function showPlayerDetail(uid,uname){
  if(!uid){T('缺少玩家UID','e');return}
  var r=await APrism('GET','/api/server/owner?uid='+encodeURIComponent(uid));
  var u=r.ok&&r.user?r.user:null;
  var m=document.createElement('div');m.className='modal-overlay';
  var h='<div class="modal-box" style="max-width:360px;padding:18px 20px">'+
    '<div style="display:flex;align-items:center;gap:10px;margin-bottom:12px">'+
    (u&&u.headImage?'<img src="'+u.headImage+'" style="width:44px;height:44px;border-radius:12px;flex-shrink:0" onerror="this.style.display=\'none\'">':'<div style="width:44px;height:44px;border-radius:12px;background:var(--phx-border-light);flex-shrink:0"></div>')+
    '<div style="flex:1;min-width:0"><div style="font-size:15px;font-weight:800">'+escHtml(uname)+'</div>'+
    '<div class="dim" style="font-size:10px">UID: '+escHtml(String(uid).substring(0,15))+'</div></div></div>';
  if(u){
    h+='<div class="stats" style="grid-template-columns:1fr 1fr">'+
      '<div class="s"><div class="n">'+(u.level||u.pe_growth&&u.pe_growth.lv||'?')+'</div><div class="l">等级</div></div>'+
      '<div class="s"><div class="n">'+(u.friend_count||'?')+'</div><div class="l">好友</div></div></div>'
  }else{
    h+='<div style="text-align:center;padding:12px;color:var(--phx-text-secondary);font-size:12px">无法获取详细信息</div>'
  }
  h+='<button class="btn-s" style="width:100%;margin-top:10px" onclick="closeModalOverlay(this)">关闭</button></div>';
  m.innerHTML=h;document.body.appendChild(m);
  m.addEventListener('click',function(e){if(e.target===m)closeOverlay(m)})
}

// ── 导出 ──
var expRunning=false;

function doExp(){
  openSubPage('建筑导出', RexpHTML());
  setTimeout(onExpFmtChange, 100)
}
function onExpFmtChange(){
  var prev=document.getElementById('exp-name-preview');
  if(prev)prev.textContent=expBuildPath()
}
function expBuildPath(){
  var fmt=document.getElementById('exp-fmt-picker').getAttribute('data-value');
  var name=(document.getElementById('exp-name').value||'').trim();
  if(!name)return '';
  var ext=fmt==='schematic'?'.schematic':fmt==='mcworld'?'.mcworld':'.mcstructure';
  var x1=I('exp-x1',0), y1=I('exp-y1',0), z1=I('exp-z1',0);
  var x2=I('exp-x2',64), y2=I('exp-y2',32), z2=I('exp-z2',64);
  var coordSuffix = '@[' + x1 + ' ' + y1 + ' ' + z1 + ']~[' + x2 + ' ' + y2 + ' ' + z2 + ']';
  return '/sdcard/Download/' + name + coordSuffix + ext
}
async function doExpGo(){
  if(checkUpdateLock())return;
  var b=document.getElementById('exp-go');L(b,true,'导出中...');
  var path=expBuildPath();
  if(!path){L(b,false,'开始导出');T('请填写文件名','e');return}
  var r=await A('POST','/api/task/start',{type:'export',params:{format:document.getElementById('exp-fmt-picker').getAttribute('data-value'),output:path,x1:I('exp-x1',0),y1:I('exp-y1',0),z1:I('exp-z1',0),x2:I('exp-x2',64),y2:I('exp-y2',32),z2:I('exp-z2',64)}});
  L(b,false,'开始导出');if(!r.ok){T('失败: '+r.error,'e');return}
  expRunning=true;
  document.getElementById('exp-go').classList.add('gone');
  document.getElementById('exp-stop').classList.remove('gone');
  document.getElementById('exp-pg').classList.remove('gone');
  T('导出已启动','o')
}
async function doExpStop(){
  var b=document.getElementById('exp-stop');
  L(b,true,'停止中...');
  var r=await A('POST','/api/task/stop');
  L(b,false,'停止导出');
  expRunning=false;
  document.getElementById('exp-stop').classList.add('gone');
  document.getElementById('exp-go').classList.remove('gone');
  T(r.ok?'已停止':'停止失败: '+(r.error||'未知'), r.ok?'o':'e')
}

function resetTaskButtons(){
  ['imp-stop','exp-stop','md-stop','skin-stop'].forEach(function(id){
    var el=document.getElementById(id);if(el)el.classList.add('gone')
  });
  ['imp-go','exp-go','md-go','skin-go'].forEach(function(id){
    var el=document.getElementById(id);if(el){el.classList.remove('gone');el.disabled=false;el.textContent='重新导入'}
  });
  ['imp-pg','clear-pg','exp-pg','md-pg'].forEach(function(id){var el=document.getElementById(id);if(el)el.classList.add('gone')});
  expRunning=false
}
