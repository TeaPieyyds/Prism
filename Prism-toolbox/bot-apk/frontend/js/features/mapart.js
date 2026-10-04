var maPath='',maClarity=1,maCrop={cx:0,cy:0,cw:0,ch:0};

function RmaHTML(){
  return '<div class="card"><div class="pick" onclick="doMaPick()"><i class="pi">' + ICONS.upload + '</i><div id="ma-pick-txt">点击选择图片</div></div>'+
    '<div id="ma-info" class="gone"></div>'+
    '<div id="ma-params" class="gone">'+
    '<label>放置坐标</label><div class="row"><input id="ma-x" type="number" placeholder="X"><input id="ma-y" type="number" placeholder="Y"><input id="ma-z" type="number" placeholder="Z"></div>'+
    '<button class="btn-s" id="ma-align-btn" onclick="alignMaCoords()" style="width:100%;margin:2px 0 6px;font-size:11px;display:none">对齐 128 网格</button>'+
    '<label>自定义尺寸</label><div class="row"><input id="ma-w" type="number" placeholder="宽（自动）"><input id="ma-h" type="number" placeholder="高（自动）"></div>'+
    '<label>摆放方向</label><select id="ma-ori" onchange="toggleMaRelief()"><option value="horizontal">水平（地面）</option><option value="vertical">垂直（墙壁）</option></select>'+
    '<div id="ma-relief-opts" class="qopt">'+
    '<label class="lbl-cb"><input type="checkbox" id="ma-relief" onchange="toggleMaRelief()"><span class="tgl"></span> 浮雕模式</label>'+
    '<div class="dim" style="font-size:10px">根据颜色亮度调整方块Y轴高度，产生立体感</div></div>'+
    '<div id="ma-gravity-opts" class="qopt">'+
    '<label class="lbl-cb"><input type="checkbox" id="ma-gravity" onchange="toggleGravitySub()"><span class="tgl"></span> 重力方块模式</label>'+
    '<div id="ma-gravity-sub" class="gone qopt-sub">'+
    '<label class="lbl-cb"><input type="checkbox" id="ma-platform"><span class="tgl"></span> 放置玻璃平台</label>'+
    '<div class="dim" style="font-size:10px">水平导入时在下方铺一层玻璃</div></div></div>'+
    '<label>速度（方块/秒）</label><input id="ma-spd" type="number" value="'+cfgSpeed+'" style="margin-bottom:6px">'+
    // ── 色彩空间 ──
    '<label>色彩空间</label>'+
    '<div class="row" style="gap:6px;margin:2px 0 8px">'+
    '<button class="btn-s" id="ma-cs-btn" onclick="showColorSpacePicker()" style="flex:1;text-align:left;font-size:12px;padding:10px 12px">LAB（推荐）</button></div>'+
    // ── 抖动算法 ──
    '<label>抖动算法</label>'+
    '<div class="row" style="gap:6px;margin:2px 0 0">'+
    '<button class="btn-s" id="ma-dither-btn" onclick="showDitherPicker()" style="flex:1;text-align:left;font-size:12px;padding:10px 12px">Floyd-Steinberg（推荐）</button></div>'+
    '<div class="row" style="margin-top:8px">'+
    '<button class="btn gone" id="md-go" onclick="doMa()">开始生成</button>'+
    '<button class="btn-d gone" id="md-stop" onclick="doMaStop()">停止</button></div>'+
    '<div class="pg gone" id="md-pg"><div id="md-bar"></div></div>'+
    '<div class="dim" id="ma-stat"></div></div>'+
    '<div class="card gone" id="ma-log-card"><div class="log" id="ma-log"></div></div>'
}

async function doMaPick(){
  var p=useBuiltinPicker?await fpicker('image'):await pick('image');if(!p)return;
  showCropUI(p,function(croppedPath,cw,ch,cx,cy,clarity){
    maPath=croppedPath;maCrop={cx:cx,cy:cy,cw:cw,ch:ch};maClarity=clarity;
    showMaPreview(croppedPath,cw,ch,cx,cy,clarity)
  })
}

async function showMaPreview(path,tw,th,cx,cy,clarity){
  document.getElementById('ma-pick-txt').innerHTML='<span class="spin"></span> 生成预览...';
  var r=await A('POST','/api/mapart/preview',{
    path:path,targetW:tw,targetH:th,
    orientation:document.getElementById('ma-ori').value,
    use_relief:document.getElementById('ma-relief')?.checked||false,
    color_space:maCs,
    dither:maDither
  });
  if(!r.ok){T('预览失败: '+r.error,'e');return}
  document.getElementById('ma-pick-txt').textContent='已选择: '+path.split('/').pop();
  document.getElementById('ma-info').classList.remove('gone');
  document.getElementById('ma-info').innerHTML=
    '<div class="stats"><div class="s"><div class="n">'+r.blocks+'</div><div class="l">方块数</div></div>'+
    '<div class="s"><div class="n">'+(tw*clarity)+'×'+(th*clarity)+'</div><div class="l">尺寸</div></div></div>';
  // 设置默认自定义尺寸
  var wEl=document.getElementById('ma-w'),hEl=document.getElementById('ma-h');
  if(wEl)wEl.value=tw*clarity;
  if(hEl)hEl.value=th*clarity;
  document.getElementById('ma-params').classList.remove('gone');
  document.getElementById('md-go').classList.remove('gone');
  document.getElementById('ma-align-btn').style.display='';
  document.getElementById('md-go').textContent='开始生成';
  document.getElementById('md-go').disabled=false;
  // 3D 预览（原版方案）
  if(r.voxel && !r.voxel.too_large){
    var v=document.createElement('div');v.className='viewer';v.id='viewer';
    document.getElementById('ma-info').appendChild(v);
    setTimeout(function(){if(typeof THREE!=='undefined')show3D(v,r.voxel)},100)
  }
  // ── 绑定动态重新解析 ──
  var refreshTimer;
  function scheduleRefresh(){
    clearTimeout(refreshTimer);
    refreshTimer=setTimeout(refreshMaPreview,300)
  }
  if(wEl)wEl.addEventListener('input',scheduleRefresh);
  if(hEl)hEl.addEventListener('input',scheduleRefresh);
  document.getElementById('ma-ori').addEventListener('change',function(){
    toggleMaRelief();
    scheduleRefresh()
  });
  document.getElementById('ma-relief').addEventListener('change',function(){
    toggleMaRelief();
    scheduleRefresh()
  })
}

async function refreshMaPreview(){
  var w=parseInt(document.getElementById('ma-w').value)||maCrop.cw;
  var h=parseInt(document.getElementById('ma-h').value)||maCrop.ch;
  var path=maPath;
  if(!path)return;
  document.getElementById('ma-pick-txt').innerHTML='<span class="spin"></span> 重新解析...';
  var r=await A('POST','/api/mapart/preview',{
    path:path,targetW:w,targetH:h,
    orientation:document.getElementById('ma-ori').value,
    use_relief:document.getElementById('ma-relief')?.checked||false,
    color_space:maCs,
    dither:maDither
  });
  if(!r.ok){T('解析失败: '+r.error,'e');document.getElementById('ma-pick-txt').textContent='已选择: '+path.split('/').pop();return}
  document.getElementById('ma-pick-txt').textContent='已选择: '+path.split('/').pop();
  document.getElementById('ma-info').innerHTML=
    '<div class="stats"><div class="s"><div class="n">'+r.blocks+'</div><div class="l">方块数</div></div>'+
    '<div class="s"><div class="n">'+w+'×'+h+'</div><div class="l">尺寸</div></div></div>';
  // 更新 3D 预览
  var existing=document.getElementById('viewer');
  if(r.voxel&&!r.voxel.too_large){
    var v=existing||document.createElement('div');
    if(!existing){v.className='viewer';v.id='viewer';document.getElementById('ma-info').appendChild(v)}
    setTimeout(function(){if(typeof THREE!=='undefined')show3D(v,r.voxel)},100)
  }else if(existing){existing.remove()}
}

async function doMa(){
  if(checkUpdateLock())return;
  var btn=document.getElementById('md-go');
  btn.disabled=true;btn.innerHTML='<span class="spin"></span> 启动中...';
  document.getElementById('md-stop').classList.remove('gone');
  document.getElementById('md-pg').classList.remove('gone');
  document.getElementById('ma-log-card').classList.remove('gone');
  var body={
    type:'mapart',params:{
      path:maPath,
      target_w:parseInt(document.getElementById('ma-w').value)||maCrop.cw,
      target_h:parseInt(document.getElementById('ma-h').value)||maCrop.ch,
      crop_x:maCrop.cx,crop_y:maCrop.cy,clarity:maClarity,
      x:parseInt(document.getElementById('ma-x').value)||0,
      y:parseInt(document.getElementById('ma-y').value)||0,
      z:parseInt(document.getElementById('ma-z').value)||0,
      orientation:document.getElementById('ma-ori').value,
      use_relief:document.getElementById('ma-relief')?.checked||false,
      color_space:maCs,
      dither:maDither,
      use_gravity:document.getElementById('ma-gravity')?.checked||false,
      gravity_platform:document.getElementById('ma-platform')?.checked||false,
      speed:Math.min(Math.max(parseInt(document.getElementById('ma-spd').value)||9500,1),20000)
    }
  };
  var r=await A('POST','/api/task/start',body);
  if(r.ok){btn.innerHTML='开始生成';btn.disabled=false;T('任务已启动','o')}
  else{T('启动失败: '+r.error,'e');btn.innerHTML='开始生成';btn.disabled=false;document.getElementById('md-stop').classList.add('gone')}
}

function doMaStop(){A('POST','/api/task/stop');T('已停止')}

function alignMaCoords(){
  var xEl=document.getElementById('ma-x'),zEl=document.getElementById('ma-z');
  if(!xEl||!zEl)return;
  var curX=parseInt(xEl.value)||0,curZ=parseInt(zEl.value)||0;
  var bw=Math.round((maCrop.cw||128)*maClarity);
  var bh=Math.round((maCrop.ch||128)*maClarity);
  // 网格大小按清晰度：clarity=1→128, clarity=2→256, clarity=3→512
  // 公式：floor((v+64)/grid)*grid - 64（参考 地图精准计算工具.py）
  function snap(v){
    var g=128<<(maClarity-1),offset=64;
    return Math.round((v+offset)/g)*g-offset
  }
  xEl.value=snap(curX);
  zEl.value=snap(curZ);
  T('已对齐到 '+xEl.value+', '+zEl.value,'o')
}

function toggleGravitySub(){
  var sub=document.getElementById('ma-gravity-sub');
  var grav=document.getElementById('ma-gravity');
  var ori=document.getElementById('ma-ori');
  var show=grav&&grav.checked&&ori&&ori.value==='horizontal';
  if(sub)sub.classList.toggle('gone',!show)
}
function toggleMaRelief(){
  var relief=document.getElementById('ma-relief')?.checked;
  var opts=document.getElementById('ma-gravity-opts');
  var ori=document.getElementById('ma-ori');
  if(opts)opts.style.display=relief||ori&&ori.value!=='horizontal'?'none':''
}

var maCs='lab',maDither='floyd_steinberg';
var CS_ITEMS=[
  {value:'lab',label:'LAB',desc:'最符合人眼感知，颜色过渡自然',badge:'推荐'},
  {value:'rgb',label:'RGB',desc:'速度最快，纯色区域匹配好'},
  {value:'hsv',label:'HSV',desc:'对色调更敏感，渐变色表现好'}
];
var DITHER_ITEMS=[
  {value:'none',label:'无',desc:'直接匹配，速度最快，色块感强'},
  {value:'floyd_steinberg',label:'Floyd-Steinberg',desc:'经典算法，效果最好，推荐大多数场景',badge:'推荐'},
  {value:'atkinson',label:'Atkinson',desc:'轻量版，扩散范围小，适合大图'},
  {value:'burkes',label:'Burkes',desc:'Floyd 变体，更平滑自然'},
  {value:'stucki',label:'Stucki',desc:'扩散范围广，超大图效果好'},
  {value:'jarvis',label:'Jarvis',desc:'12方向扩散，最细腻的误差扩散'},
  {value:'bayer_2x2',label:'Bayer 2×2',desc:'有序抖动，速度快，有固定纹理'},
  {value:'bayer_4x4',label:'Bayer 4×4',desc:'比2×2更细腻'},
  {value:'bayer_8x8',label:'Bayer 8×8',desc:'最细腻的有序抖动'},
  {value:'ordered_3x3',label:'Ordered 3×3',desc:'另一种有序抖动图案'}
];
function showColorSpacePicker(){showMaPicker('色彩空间',CS_ITEMS,maCs,function(v,item){maCs=v;document.getElementById('ma-cs-btn').textContent=item.label+(item.badge?'（'+item.badge+'）':'');if(maPath)refreshMaPreview()})}
function showDitherPicker(){showMaPicker('抖动算法',DITHER_ITEMS,maDither,function(v,item){maDither=v;document.getElementById('ma-dither-btn').textContent=item.label+(item.badge?'（'+item.badge+'）':'');if(maPath)refreshMaPreview()})}
function showMaPicker(title,items,selected,cb){
  var overlay=document.createElement('div');overlay.className='bs-overlay';
  var itemsHtml='';
  for(var i=0;i<items.length;i++){
    var item=items[i];
    var sel=item.value===selected;
    itemsHtml+='<div class="bs-option'+(sel?' active':'')+'" data-value="'+item.value+'">'+
      '<div class="bs-dot"></div><span class="bs-label">'+item.label+'</span>'+
      '<span class="dim" style="font-size:11px;text-align:right;max-width:140px">'+item.desc+'</span></div>'
  }
  overlay.innerHTML='<div class="bs-sheet">'+
    '<div class="bs-title">'+title+'</div>'+
    '<div class="bs-body">'+itemsHtml+'</div>'+
    '<div class="bs-footer">'+
    '<button class="bs-btn bs-btn-cancel">取消</button>'+
    '<button class="bs-btn bs-btn-confirm">确定</button></div></div>';
  document.body.appendChild(overlay);
  var selVal=selected;
  overlay.querySelectorAll('.bs-option').forEach(function(opt){
    opt.addEventListener('click',function(){
      overlay.querySelectorAll('.bs-option').forEach(function(o){o.classList.remove('active')});
      this.classList.add('active');selVal=this.getAttribute('data-value')
    })
  });
  overlay.querySelector('.bs-btn-cancel').addEventListener('click',function(){closeOverlay(overlay)});
  overlay.querySelector('.bs-btn-confirm').addEventListener('click',function(){
    closeOverlay(overlay);
    var found=null;
    for(var i=0;i<items.length;i++){if(items[i].value===selVal){found=items[i];break}}
    if(found&&cb)cb(selVal,found)
  });
  overlay.addEventListener('click',function(e){if(!e.target.closest('.bs-sheet'))closeOverlay(overlay)})
}

var RATIOS=[[1,3],[1,2],[2,3],[3,4],[1,1],[4,3],[3,2],[2,1],[3,1]];

function showCropUI(path,cb){
  var ov=document.createElement('div');ov.className='crop-overlay';
  ov.innerHTML='<canvas id="crop-canvas" style="position:absolute;top:0;left:0;right:0;bottom:88px;width:100%"></canvas><div class="crop-bottombar"><button class="crop-cancel" id="crop-cancel" style="flex-shrink:0;width:40px;height:40px;display:flex;align-items:center;justify-content:center">'+ICONS.close+'</button><div class="crop-label" id="crop-label" style="flex:1;text-align:center;font-size:12px;font-weight:700;color:rgba(255,255,255,.7);padding:0 8px">加载中...</div><div style="display:flex;align-items:center;gap:8px;flex-shrink:0;font-size:12px;color:#fff"><span>清晰度</span><input type="range" id="crop-clarity" min="1" max="3" value="1" style="width:120px;height:6px;accent-color:var(--phx-primary,#d48a0e)"><span id="crop-clarity-label" style="min-width:18px;font-weight:700;font-size:14px">1</span><button class="crop-done" id="crop-done" style="padding:8px 16px;border-radius:20px;font-size:14px">'+ICONS.check+' 确认</button></div></div><div id="crop-err" class="gone" style="position:absolute;top:50%;left:50%;transform:translate(-50%,-50%);color:#ff6b6b;font-size:15px;font-weight:700;text-align:center;z-index:50;pointer-events:none;line-height:1.6"></div>';
  document.body.appendChild(ov);
  var cv=document.getElementById('crop-canvas'),ctx=cv.getContext('2d');
  var errEl=document.getElementById('crop-err');
  var labelEl=document.getElementById('crop-label');
  // ── 固定 canvas 缓冲区尺寸（只设一次!）──
  var vw=ov.clientWidth,vh=ov.clientHeight-88;
  // 如果宽高为0（极罕见情况），fallback
  if(vw<1)vw=window.innerWidth;
  if(vh<1)vh=window.innerHeight-88;
  cv.width=vw;cv.height=vh;

  var img=new Image();
  // ── 加载失败超时保护 ──
  var loadTimer=setTimeout(function(){
    if(!img.complete){
      img.src=''; // 中止加载
      if(img.onerror)img.onerror()
    }
  },30000);
  img.onerror=function(){
    clearTimeout(loadTimer);
    errEl.classList.remove('gone');
    errEl.innerHTML='⚠ 图片加载失败<br><span style="font-size:12px;font-weight:400;color:rgba(255,107,107,.7)">请检查网络后重试</span>';
    labelEl.textContent='加载失败';
  };
  img.onload=function(){
    clearTimeout(loadTimer);
    errEl.classList.add('gone');
    labelEl.textContent='裁剪图片';

    var iw=img.width,ih=img.height;
    ctx.imageSmoothingEnabled=false;

    // ── 选最佳初始比例（复用 snapToStd）──
    var clarity=1;
    var g=snapToStd(iw,ih);
    var cw=g.pw,ch=g.ph;
    var cx=Math.round((iw-cw)/2),cy=Math.round((ih-ch)/2);

    // ── 缩放与位移 ──
    var sc=Math.min(vw/iw,vh/ih,1)*0.85;
    var ox=(vw-iw*sc)/2,oy=(vh-ih*sc)/2; // canvas 原点=(0,0)，无需44偏移

    // ── 状态 ──
    var drag=null; // null|'move'|'tl'|'tr'|'bl'|'br'|'t'|'b'|'l'|'r'
    var dlx=0,dly=0;
    var pinchDist=0;
    var cornerR=28; // 圆点半径（加大）
    var edgeR=38;   // 边缘吸附阈值（屏幕像素）

    // ── snapToStd：选最接近比例，金框尽可能大但不超过选区 ──
    function snapToStd(rcw,rch){
      var ar=rcw/rch;
      var bi=0,bs=Infinity;
      for(var i=0;i<RATIOS.length;i++){
        var a=RATIOS[i][0],b=RATIOS[i][1],sa=a/b;
        var s=Math.abs(sa-ar);
        if(s<bs){bs=s;bi=i}
      }
      var a=RATIOS[bi][0],b=RATIOS[bi][1];
      // 原版算法：k = 比例单位数，使输出 ≤ 选区
      var k=Math.min(Math.floor(rcw/a),Math.floor(rch/b));
      if(k<1)k=1;
      return {pw:a*k,ph:b*k,ai:bi}
    }

    // ── 渲染（永不重置 canvas 尺寸！）──
    function rd(){
      ctx.clearRect(0,0,cv.width,cv.height);
      ctx.fillStyle='#2b2118';ctx.fillRect(0,0,cv.width,cv.height);

      // 图片
      ctx.drawImage(img,ox,oy,iw*sc,ih*sc);

      // 白框（用户拖拽的选区）
      var bx=ox+cx*sc,by=oy+cy*sc,bw=cw*sc,bh=ch*sc;

      // 暗化外部
      ctx.fillStyle='rgba(0,0,0,.35)';
      ctx.fillRect(0,0,cv.width,by);
      ctx.fillRect(0,by+bh,cv.width,cv.height-by-bh);
      ctx.fillRect(0,by,bx,bh);
      ctx.fillRect(bx+bw,by,cv.width-bx-bw,bh);

      // 白色边框
      ctx.strokeStyle='#fff';ctx.lineWidth=2.5;
      ctx.strokeRect(bx,by,bw,bh);

      // 金色虚线框（实际裁剪区域——居中于白框内）
      var gcx=ox+(cx+(cw-g.pw)/2)*sc,gcy=oy+(cy+(ch-g.ph)/2)*sc;
      var ghw=g.pw*sc,ghh=g.ph*sc;
      ctx.setLineDash([5,3]);ctx.strokeStyle='rgba(255,179,71,.85)';ctx.lineWidth=2;
      ctx.strokeRect(gcx,gcy,ghw,ghh);ctx.setLineDash([]);
      ctx.fillStyle='rgba(255,179,71,.12)';ctx.fillRect(gcx,gcy,ghw,ghh);

      // 四角手柄
      var cr=cornerR;
      [[bx,by],[bx+bw,by],[bx,by+bh],[bx+bw,by+bh]].forEach(function(p){
        ctx.fillStyle='#fff';ctx.beginPath();ctx.arc(p[0],p[1],cr,0,6.28);ctx.fill();
        ctx.fillStyle='#333';ctx.beginPath();ctx.arc(p[0],p[1],cr-3,0,6.28);ctx.fill()
      });

      // 更新信息（显示实际输出尺寸 128×N × clarity）
      var ri=g.ai,ra=RATIOS[ri][0],rb=RATIOS[ri][1];
      var outW=ra*128*clarity,outH=rb*128*clarity;
      labelEl.textContent=outW+'×'+outH+' 方块  ['+ra+':'+rb+'] ×'+clarity;
      maCrop={cx:Math.round(cx+(cw-g.pw)/2),cy:Math.round(cy+(ch-g.ph)/2),cw:g.pw,ch:g.ph}
    }

    // ── 角/边缘检测（优先角→边缘→移动拖拽）──
    function hitCorner(px,py){
      var bx=ox+cx*sc,by=oy+cy*sc,bw=cw*sc,bh=ch*sc;
      var cr=cornerR+24; // 角热点半径
      function d(x,y){return Math.hypot(px-x,py-y)<=cr}
      // 四角优先
      if(d(bx,by))return'tl';if(d(bx+bw,by))return'tr';
      if(d(bx,by+bh))return'bl';if(d(bx+bw,by+bh))return'br';
      // 四边（排除角区域后）
      var er=edgeR;
      var inL=Math.abs(px-bx)<=er&&py>by+cr&&py<by+bh-cr;
      var inR=Math.abs(px-(bx+bw))<=er&&py>by+cr&&py<by+bh-cr;
      var inT=Math.abs(py-by)<=er&&px>bx+cr&&px<bx+bw-cr;
      var inB=Math.abs(py-(by+bh))<=er&&px>bx+cr&&px<bx+bw-cr;
      if(inT)return't';if(inB)return'b';
      if(inL)return'l';if(inR)return'r';
      return null
    }

    // ── 指针事件 ──
    ov.addEventListener('pointerdown',function(e){
      if(errEl&&!errEl.classList.contains('gone'))return;
      // 底部栏控件（滑块/按钮/标签）不触发裁剪操作
      if(e.target.closest('.crop-bottombar'))return;
      var p={x:e.clientX,y:e.clientY};
      var cn=hitCorner(p.x,p.y);
      drag=cn||'move';dlx=p.x;dly=p.y;e.preventDefault()
    });
    ov.addEventListener('pointermove',function(e){
      if(!drag)return;
      var dx=e.clientX-dlx,dy=e.clientY-dly;
      dlx=e.clientX;dly=e.clientY;
      if(drag==='move'){
        ox+=dx;oy+=dy;
        var minOx=cv.width-iw*sc-20,maxOx=20;
        var minOy=cv.height-ih*sc-20,maxOy=20;
        ox=Math.max(minOx,Math.min(maxOx,ox));
        oy=Math.max(minOy,Math.min(maxOy,oy));
        rd();return
      }
      // 角拖拽（不 round，累计浮点误差在约束中消化）
      var ndx=dx/sc,ndy=dy/sc;
      if(drag.indexOf('t')>=0){cy+=ndy;ch-=ndy}
      if(drag.indexOf('b')>=0){ch+=ndy}
      if(drag.indexOf('l')>=0){cx+=ndx;cw-=ndx}
      if(drag.indexOf('r')>=0){cw+=ndx}
      // 约束（仅防溢出和白框≥128，不强制对齐金框）
      var minS=128;
      cw=Math.max(minS,Math.min(iw-cx,cw));
      ch=Math.max(minS,Math.min(ih-cy,ch));
      cx=Math.max(0,Math.min(iw-minS,cx));
      cy=Math.max(0,Math.min(ih-minS,cy));
      // 金框跟随但不反拽白框
      g=snapToStd(cw,ch);
      rd()
    });
    ov.addEventListener('pointerup',function(){
      if(!drag)return;
      drag=null
    });
    ov.addEventListener('pointerleave',function(){
      if(!drag)return;
      drag=null
    });

    // ── 滚轮缩放 ──
    ov.addEventListener('wheel',function(e){
      if(errEl&&!errEl.classList.contains('gone'))return;
      e.preventDefault();
      var oldS=sc;
      sc*=e.deltaY<0?1.1:0.9;
      sc=Math.max(0.1,Math.min(5,sc));
      var r=e.clientX-ox,r2=e.clientY-oy;
      ox=e.clientX-(r/oldS)*sc;
      oy=e.clientY-(r2/oldS)*sc;
      rd()
    },{passive:false});

    // ── 双指缩放（修复方向）──
    ov.addEventListener('touchstart',function(e){
      if(e.touches.length===2){
        var t=e.touches;
        pinchDist=Math.hypot(t[0].clientX-t[1].clientX,t[0].clientY-t[1].clientY);
        drag=null
      }
    },{passive:true});
    ov.addEventListener('touchmove',function(e){
      if(e.touches.length===2&&pinchDist>0){
        e.preventDefault();
        var t=e.touches,d2=Math.hypot(t[0].clientX-t[1].clientX,t[0].clientY-t[1].clientY);
        // 修复：d2/pinchDist —— 张开手指 = 放大
        sc*=d2/pinchDist;
        sc=Math.max(0.1,Math.min(5,sc));
        pinchDist=d2;
        rd()
      }
    },{passive:false});

    // ── 控件 ──
    var cancelBtn=document.getElementById('crop-cancel');
    var clarityInput=document.getElementById('crop-clarity');
    var clarityLabel=document.getElementById('crop-clarity-label');
    var doneBtn=document.getElementById('crop-done');
    cancelBtn.addEventListener('click',function(){ov.remove()});
    clarityInput.addEventListener('input',function(){
      clarity=parseInt(this.value);
      clarityLabel.textContent=clarity;
      rd()
    });
    doneBtn.addEventListener('click',function(){
      try{
        var ri=g.ai,a=RATIOS[ri][0],b=RATIOS[ri][1];
        // 压缩到 128×N 标准尺寸上传
        var dw=a*128*clarity,dh=b*128*clarity;
        var srcX=Math.round(cx+(cw-g.pw)/2);
        var srcY=Math.round(cy+(ch-g.ph)/2);
        var cc=document.createElement('canvas');cc.width=dw;cc.height=dh;
        var cctx=cc.getContext('2d');
        cctx.imageSmoothingEnabled=false;
        cctx.drawImage(img,srcX,srcY,g.pw,g.ph,0,0,dw,dh);
        cc.toBlob(function(blob){
          if(!blob){showAlertModal('错误','裁剪失败，请重试');return}
          var fd=new FormData();fd.append('file',blob,'crop_'+Date.now()+'.png');
          fetch('/api/file/upload',{method:'POST',body:fd}).then(function(r){return r.json()}).then(function(u){
            ov.remove();
            if(u.ok&&u.path){maPath=u.path;maClarity=clarity;maCrop={cx:srcX,cy:srcY,cw:dw,ch:dh};cb(u.path,dw,dh,srcX,srcY,clarity)}
            else showAlertModal('上传失败',u.error||'服务器错误')
          }).catch(function(e){showAlertModal('上传失败',e.message)})
        },'image/png')
      }catch(e){showAlertModal('裁剪出错',e.message)}
    });

    // 首次渲染
    g=snapToStd(cw,ch);
    rd()
  };
  img.src='/uploads/'+encodeURIComponent(path.split('/').pop())
}
