/* ═══ 个性化设置 — 界面缩放 / 自定义背景 / 图片取色 ═══ */
/* 所有设置存 localStorage，启动时自动应用 */

// ── 读取图片路径 → dataURL（优先用 Java bridge，兜底 /api/files/read）──
async function persReadImageDataURL(path){
  if(!path)return '';
  // Java bridge 直接读文件（支持 content:// 和真实路径）
  if(typeof android!=='undefined'&&android.readFileAsBase64){
    try{var r=android.readFileAsBase64(path);if(r)return r}catch(e){}
  }
  // 兜底：通过 Go 端文件读取 API
  try{
    var resp=await fetch('/api/files/read?path='+encodeURIComponent(path));
    if(!resp.ok)return '';
    var blob=await resp.blob();
    return await new Promise(function(resolve){
      var fr=new FileReader();
      fr.onloadend=function(){resolve(fr.result||'')};
      fr.onerror=function(){resolve('')};
      fr.readAsDataURL(blob);
    });
  }catch(e){return ''}
}

// ── 压缩图片为 dataURL（限制最长边，JPEG q0.72）──
function persCompressImage(dataURL,maxSide){
  return new Promise(function(resolve){
    var img=new Image();
    img.onload=function(){
      var w=img.width,h=img.height;
      if(w>maxSide||h>maxSide){
        var scale=Math.min(maxSide/w,maxSide/h);
        w=Math.round(w*scale);h=Math.round(h*scale);
      }
      var c=document.createElement('canvas');c.width=w;c.height=h;
      var ctx=c.getContext('2d');
      ctx.fillStyle='#ffffff';ctx.fillRect(0,0,w,h);
      ctx.drawImage(img,0,0,w,h);
      resolve(c.toDataURL('image/jpeg',0.72));
    };
    img.onerror=function(){resolve('')};
    img.src=dataURL;
  });
}

// ── 选择系统图片，返回压缩后的 dataURL ──
async function persPickImage(maxSide){
  var p=await Promise.race([
    pick('image'),
    new Promise(function(r){setTimeout(function(){r('')},30000)})
  ]);
  if(!p)return '';
  var raw=await persReadImageDataURL(p);
  if(!raw){T('无法读取所选图片','e');return ''}
  return persCompressImage(raw,maxSide||1080);
}

// ═══════════════════ 界面缩放 ═══════════════════
function getZoom(){
  var z=85;try{z=parseInt(localStorage.getItem('prism_zoom'))||100}catch(e){}
  return Math.min(140,Math.max(70,z));
}
function applyZoom(v){
  v=Math.min(140,Math.max(70,v));
  try{localStorage.setItem('prism_zoom',v)}catch(e){}
  // 用 CSS zoom 整体缩放（WebView 支持），100 时还原
  document.documentElement.style.zoom=(v/100).toFixed(3);
  var el=document.getElementById('zoom-label');if(el)el.textContent=v+'%';
  var sl=document.getElementById('zoom-slider');if(sl)sl.value=v;
}

// ═══════════════════ 自定义背景 ═══════════════════
function applyBackground(){
  var data=null,overlay=40;
  try{data=localStorage.getItem('prism_bg')||null;overlay=parseInt(localStorage.getItem('prism_bg_overlay'))||40}catch(e){}
  var layer=document.getElementById('pers-bg');
  if(!layer)return;
  if(data){
    document.documentElement.setAttribute('data-custom-bg','1');
    layer.style.backgroundImage='url('+data+')';
    layer.style.backgroundSize='cover';
    layer.style.backgroundPosition='center center';
    layer.style.backgroundRepeat='no-repeat';
    layer.style.opacity='1';
    var ov=layer.querySelector('.pers-bg-overlay');
    if(ov)ov.style.background='rgba(0,0,0,'+(overlay/100)+')';
  }else{
    document.documentElement.removeAttribute('data-custom-bg');
    layer.style.backgroundImage='none';
    layer.style.opacity='0';
  }
  var cb=document.getElementById('clear-bg-btn');if(cb)cb.style.display=data?'':'none';
  var ov=document.getElementById('bg-overlay');if(ov)ov.value=overlay;
  var lbl=document.getElementById('bg-overlay-label');if(lbl)lbl.textContent=overlay+'%';
}
async function pickBackground(){
  var d=await persPickImage(1080);
  if(!d)return;
  try{localStorage.setItem('prism_bg',d)}catch(e){T('背景已启用（图片较大可能存储失败）','w')}
  applyBackground();
  T('背景已启用','o');
}
function clearBackground(){
  try{localStorage.removeItem('prism_bg')}catch(e){}
  applyBackground();
  T('已清除背景','o');
}
function setBgOverlay(v){
  v=Math.max(0,Math.min(90,parseInt(v)||0));
  try{localStorage.setItem('prism_bg_overlay',v)}catch(e){}
  var lbl=document.getElementById('bg-overlay-label');if(lbl)lbl.textContent=v+'%';
  var d=null;try{d=localStorage.getItem('prism_bg')}catch(e){}
  if(d){
    var layer=document.getElementById('pers-bg');
    var ov=layer&&layer.querySelector('.pers-bg-overlay');
    if(ov)ov.style.background='rgba(0,0,0,'+(v/100)+')';
  }
}

// ═══════════════════ 图片取色生成主题 ═══════════════════
// 从图片提取主色板：主色、渐变色、背景色
function persExtractPalette(dataURL){
  return new Promise(function(resolve){
    var img=new Image();
    img.onload=function(){
      // 采样到 96x96 以内
      var w=Math.min(img.width,96),h=Math.min(img.height,96);
      var c=document.createElement('canvas');c.width=w;c.height=h;
      var ctx=c.getContext('2d');
      ctx.drawImage(img,0,0,w,h);
      var data;
      try{data=ctx.getImageData(0,0,w,h).data}catch(e){resolve(null);return}
      // 量化颜色统计
      var freq={};
      for(var i=0;i<data.length;i+=4){
        var r=data[i],g=data[i+1],b=data[i+2],a=data[i+3];
        if(a<128)continue;
        // 忽略过亮/过暗
        var lum=0.299*r+0.587*g+0.114*b;
        if(lum<20||lum>245)continue;
        // 量化到 4 位
        var key=(r>>4)+','+(g>>4)+','+(b>>4);
        freq[key]=(freq[key]||0)+1;
      }
      var entries=Object.keys(freq).map(function(k){
        var p=k.split(',');return {r:+p[0]*16,g:+p[1]*16,b:+p[2]*16,count:freq[k]}
      });
      entries.sort(function(a,b){return b.count-a.count});
      // 饱和度打分，挑选鲜艳的做主色
      var best=null,bestScore=-1;
      entries.forEach(function(e){
        var max=Math.max(e.r,e.g,e.b),min=Math.min(e.r,e.g,e.b);
        var sat= max===0?0:(max-min)/max;
        var score=Math.log(1+e.count)*sat;
        if(score>bestScore){bestScore=score;best=e}
      });
      if(!best){resolve(null);return}
      var pri='rgb('+best.r+','+best.g+','+best.b+')';
      // 背景色：最暗的常见色
      var dark=null;
      var darkLum=Infinity;
      entries.slice(0,12).forEach(function(e){
        var lum=0.299*e.r+0.587*e.g+0.114*e.b;
        if(lum<darkLum){darkLum=lum;dark=e}
      });
      var bg=dark&&darkLum<90
        ?'rgb('+Math.round(dark.r*0.55)+','+Math.round(dark.g*0.55)+','+Math.round(dark.b*0.55)+')'
        :'rgb(15,23,32)';
      // 判断明暗，决定文字色
      var isDark=darkLum<120;
      resolve({primary:pri,background:bg,dark:isDark});
    };
    img.onerror=function(){resolve(null)};
    img.src=dataURL;
  });
}
async function pickThemeColors(){
  var p=await Promise.race([
    pick('image'),
    new Promise(function(r){setTimeout(function(){r('')},30000)})
  ]);
  if(!p)return '';
  var raw=await persReadImageDataURL(p);
  if(!raw){T('无法读取所选图片','e');return}
  var pal=await persExtractPalette(raw);
  if(!pal){T('取色失败，请换一张图片','e');return}
  // 应用自定义主题
  document.documentElement.setAttribute('data-theme','custom');
  document.documentElement.style.setProperty('--phx-primary',pal.primary);
  // 生成 hover/active/bg 变体
  document.documentElement.style.setProperty('--phx-primary-hover',pal.primary);
  document.documentElement.style.setProperty('--phx-primary-active',pal.primary);
  document.documentElement.style.setProperty('--phx-primary-bg','rgba(255,255,255,.12)');
  document.documentElement.style.setProperty('--phx-bg',pal.background);
  document.documentElement.style.setProperty('--phx-bg-card',pal.dark?'#1a1f26':'#ffffff');
  document.documentElement.style.setProperty('--phx-text',pal.dark?'#e2e8f0':'#1f2937');
  document.documentElement.style.setProperty('--phx-text-secondary',pal.dark?'#94a3b8':'#6b7280');
  document.documentElement.style.setProperty('--phx-border',pal.dark?'#334155':'#d1d5db');
  document.documentElement.style.setProperty('--phx-border-light',pal.dark?'#1e2938':'#e5e7eb');
  // 保存
  try{
    localStorage.setItem('prism_custom_theme',JSON.stringify(makeThemeValue(pal)));
  }catch(e){T('主题已应用（保存失败）','w')}
  appTheme='custom';
  try{localStorage.setItem('prism_theme','custom')}catch(e){}
  syncStatusBarTheme();
  refreshThemePickerLabel();
  T('已根据图片生成主题','o');
}
function makeThemeValue(pal){
  return {primary:pal.primary,background:pal.background,dark:pal.dark,ts:Date.now()
    ,color1:pal.primary,color2:pal.primary,color3:pal.background};
}
clearCustomTheme=window.clearCustomTheme||function clearCustomTheme(){
  try{localStorage.removeItem('prism_custom_theme')}catch(e){}
  // 跳过 localStorage 里的 custom，恢复默认主题
  var t='light';try{t=localStorage.getItem('prism_theme')||'light'}catch(e){}
  if(t==='custom')t='light';
  setTheme(t); // 会清掉 style 内联变量
  // 手动再清一次内联
  ['--phx-primary','--phx-primary-hover','--phx-primary-active','--phx-primary-bg','--phx-bg','--phx-bg-card','--phx-text','--phx-text-secondary','--phx-border','--phx-border-light'].forEach(function(k){
    document.documentElement.style.removeProperty(k)
  });
  T('已清除自定义主题','o');
};

// ═══════════════════ 底部弹窗 ═══════════════════
function openPersonalizeSheet(){
  var overlay=document.createElement('div');overlay.className='bs-overlay';
  overlay.innerHTML=
    '<div class="bs-sheet">'+
    '<div class="bs-title">个性化设置</div>'+
    '<div class="bs-body" style="padding:4px 18px 12px">'+

    // 界面缩放
    '<div style="font-size:13px;font-weight:800;color:var(--phx-text);margin:10px 0 6px">界面缩放</div>'+
    '<div style="display:flex;align-items:center;gap:10px;margin-bottom:4px">'+
      '<button class="pers-step" onclick="zoomStep(-5)">−</button>'+
      '<input type="range" id="zoom-slider" min="70" max="140" step="5" value="100" '+
        'style="flex:1;-webkit-appearance:none;appearance:none;height:6px;border-radius:3px;background:var(--phx-border);outline:none;padding:0" '+
        'oninput="applyZoom(this.value)">'+
      '<button class="pers-step" onclick="zoomStep(5)">+</button>'+
      '<span id="zoom-label" style="font-size:13px;font-weight:800;min-width:45px;text-align:center;color:var(--phx-text)">100%</span>'+
    '</div>'+
    '<div class="dim" style="margin-bottom:14px">调整整个界面的文字和按钮大小</div>'+

    // 自定义背景
    '<div style="font-size:13px;font-weight:800;color:var(--phx-text);margin:6px 0 6px">自定义背景</div>'+
    '<div style="display:flex;gap:8px;margin-bottom:6px">'+
      '<button class="btn-s" style="flex:1;margin:0" onclick="closeBsOverlay(this);setTimeout(pickBackground,150)">选择图片</button>'+
      '<button class="btn-s" style="flex:1;margin:0" id="clear-bg-btn" onclick="closeBsOverlay(this);setTimeout(clearBackground,150)">清除背景</button>'+
    '</div>'+
    '<div style="display:flex;align-items:center;gap:10px;margin-bottom:14px">'+
      '<span class="dim">遮罩</span>'+
      '<input type="range" id="bg-overlay" min="0" max="90" step="5" value="40" '+
        'style="flex:1;-webkit-appearance:none;appearance:none;height:6px;border-radius:3px;background:var(--phx-border);outline:none;padding:0" '+
        'oninput="setBgOverlay(this.value)">'+
      '<span id="bg-overlay-label" style="font-size:12px;min-width:32px;text-align:center;color:var(--phx-text-secondary)">40%</span>'+
    '</div>'+

    // 图片取色
    '<div style="font-size:13px;font-weight:800;color:var(--phx-text);margin:6px 0 6px">图片取色生成主题</div>'+
    '<div style="display:flex;gap:8px;margin-bottom:6px">'+
      '<button class="btn-s" style="flex:1;margin:0" onclick="closeBsOverlay(this);setTimeout(pickThemeColors,150)">选择图片生成主题</button>'+
      '<button class="btn-s" style="flex:1;margin:0" id="clear-theme-btn" onclick="closeBsOverlay(this);setTimeout(clearCustomTheme,150)">清除自定义主题</button>'+
    '</div>'+
    '<div class="dim">根据所选图片自动提取主色，生成整套配色方案</div>'+
    '</div>'+
    '<div class="bs-footer">'+
    '<button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">关闭</button></div>'+
    '</div>';
  document.body.appendChild(overlay);
  // 初始化控件状态
  applyZoom(getZoom());
  applyBackground();
  var hasTheme='custom';try{hasTheme=localStorage.getItem('prism_theme')||'light'}catch(e){}
  var clearTheme=document.getElementById('clear-theme-btn');
  if(clearTheme)clearTheme.style.display=(hasTheme==='custom')?'':'none';
  overlay.addEventListener('click',function(e){if(e.target===overlay)closeOverlay(overlay)});
}
function zoomStep(d){
  var z=getZoom()+d;
  applyZoom(z);
}

// 刷新设置页主题选择器标签（含自定义）
function refreshThemePickerLabel(){
  var tp=document.getElementById('cfg-theme-picker');
  if(!tp)return;
  var lbs={'light':'浅色 — 暖黄','sepia':'复古棕','dark':'深色 — 蓝灰','forest':'墨绿 — 自然','amethyst':'暗紫 — 优雅','ocean':'海洋蓝','sakura':'樱花粉','midnight':'午夜深蓝','custom':'自定义（图片取色）'};
  tp.setAttribute('data-value',appTheme);
  var lb=tp.querySelector('.picker-label');
  if(lb)lb.textContent=lbs[appTheme]||'浅色 — 暖黄';
}

// ═══════════════════ 启动恢复 ═══════════════════
function initPersonalize(){
  var z=null;try{z=localStorage.getItem('prism_zoom')}catch(e){}
  if(z)document.documentElement.style.zoom=(Math.min(140,Math.max(70,parseInt(z)||100))/100).toFixed(3);
  // 创建背景层
  if(!document.getElementById('pers-bg')){
    var layer=document.createElement('div');
    layer.id='pers-bg';
    layer.style.cssText='position:fixed;inset:0;z-index:0;background-size:cover;background-position:center center;background-repeat:no-repeat;pointer-events:none;opacity:0;transition:opacity .3s';
    layer.innerHTML='<div class="pers-bg-overlay" style="position:absolute;inset:0;background:rgba(0,0,0,.4)"></div>';
    document.body.appendChild(layer);
  }
  // 恢复自定义背景
  var bg=null;try{bg=localStorage.getItem('prism_bg')}catch(e){}
  if(bg)applyBackground();
  // 恢复自定义主题
  var ct=null;try{ct=JSON.parse(localStorage.getItem('prism_custom_theme'))}catch(e){}
  if(ct&&appTheme==='custom'){
    document.documentElement.setAttribute('data-theme','custom');
    document.documentElement.style.setProperty('--phx-primary',ct.primary);
    document.documentElement.style.setProperty('--phx-primary-hover',ct.primary);
    document.documentElement.style.setProperty('--phx-primary-active',ct.primary);
    document.documentElement.style.setProperty('--phx-primary-bg','rgba(255,255,255,.12)');
    document.documentElement.style.setProperty('--phx-bg',ct.background);
    document.documentElement.style.setProperty('--phx-bg-card',ct.dark?'#1a1f26':'#ffffff');
    document.documentElement.style.setProperty('--phx-text',ct.dark?'#e2e8f0':'#1f2937');
    document.documentElement.style.setProperty('--phx-text-secondary',ct.dark?'#94a3b8':'#6b7280');
    document.documentElement.style.setProperty('--phx-border',ct.dark?'#334155':'#d1d5db');
    document.documentElement.style.setProperty('--phx-border-light',ct.dark?'#1e2938':'#e5e7eb');
  }
}