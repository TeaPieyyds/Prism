function RmapDisplayHTML(){
  return '<div class="card"><div style="font-size:16px;font-weight:800;margin-bottom:10px">展示框显示器</div>'+
    '<label>扫描区域</label><div class="row"><input id="md-x1" type="number" placeholder="X1"><input id="md-y1" type="number" placeholder="Y1"><input id="md-z1" type="number" placeholder="Z1"></div>'+
    '<div class="row"><input id="md-x2" type="number" placeholder="X2"><input id="md-y2" type="number" placeholder="Y2"><input id="md-z2" type="number" placeholder="Z2"></div>'+
    '<button class="btn" onclick="doMapScan()" style="margin:6px 0">扫描展示框区域</button>'+
    '<div id="md-grid-info" class="gone" style="margin-bottom:8px"></div>'+
    '<div id="md-file-section" class="gone">'+
    '<label>选择图片或视频</label><div class="pick" onclick="doMapPick()"><i class="pi">' + ICONS.upload + '</i><div id="md-pick-txt">点击选择文件</div></div>'+
    '<div class="row">'+
    '<button class="btn" id="md-display-btn" onclick="doMapDisplay()">显示图片</button>'+
    '<button class="btn" id="md-play-btn" onclick="doMapPlay()">播放视频</button></div></div>'+
    '<div class="row"><button class="btn-d gone" id="md-stop" onclick="doMapStop()">停止</button></div></div>'+
    '<div class="card gone" id="md-log-card"><div class="log" id="md-log"></div></div>'
}

var mapGridInfo=null;

async function doMapScan(){
  var fd=new FormData();
  fd.append('x1',document.getElementById('md-x1').value||0);
  fd.append('y1',document.getElementById('md-y1').value||0);
  fd.append('z1',document.getElementById('md-z1').value||0);
  fd.append('x2',document.getElementById('md-x2').value||0);
  fd.append('y2',document.getElementById('md-y2').value||0);
  fd.append('z2',document.getElementById('md-z2').value||0);
  var r=await fetch('/api/map/scan',{method:'POST',body:fd}).then(function(x){return x.json()});
  if(!r.ok){T('扫描失败: '+r.error,'e');return}
  mapGridInfo=r;
  document.getElementById('md-grid-info').classList.remove('gone');
  document.getElementById('md-grid-info').innerHTML='<div class="stats"><div class="s"><div class="n">'+r.rows+'×'+r.cols+'</div><div class="l">网格</div></div>'+
    '<div class="s"><div class="n">'+(r.planeType||'未知')+'</div><div class="l">平面</div></div></div>';
  document.getElementById('md-file-section').classList.remove('gone');
  T('扫描完成: '+r.rows+'行×'+r.cols+'列','o')
}

async function doMapPick(){
  var p=await pick('image');if(!p)return;
  document.getElementById('md-pick-txt').textContent=p.split('/').pop();
  document.getElementById('md-pick-txt').dataset.path=p
}

async function doMapDisplay(){
  var path=document.getElementById('md-pick-txt').dataset.path;
  if(!path){T('请先选择文件','e');return}
  var fd=new FormData();fd.append('path',path);
  var r=await fetch('/api/map/display',{method:'POST',body:fd}).then(function(x){return x.json()});
  if(r.ok){T('正在显示...','o');document.getElementById('md-log-card').classList.remove('gone')}
  else T('显示失败: '+r.error,'e')
}

async function doMapPlay(){
  var path=document.getElementById('md-pick-txt').dataset.path;
  if(!path){T('请先选择文件','e');return}
  var fd=new FormData();fd.append('path',path);fd.append('fps','10');
  var r=await fetch('/api/map/play',{method:'POST',body:fd}).then(function(x){return x.json()});
  if(r.ok){T('开始播放','o');document.getElementById('md-stop').classList.remove('gone');document.getElementById('md-log-card').classList.remove('gone')}
  else T('播放失败: '+r.error,'e')
}

function doMapStop(){fetch('/api/map/stop',{method:'POST'});T('已停止');document.getElementById('md-stop').classList.add('gone')}
