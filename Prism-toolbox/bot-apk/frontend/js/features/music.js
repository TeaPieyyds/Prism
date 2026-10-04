var muPath='',muNotes=[],muPlaying=false,muPaused=false,muTotal=0,muCurrent=0;
var muQueue=[],muQueueIndex=-1,muQueueActive=false,muLoopMode='none';
var muView='midi';
var pianoInstrument='note.harp',pianoLocalSound=true,pianoSlideLock=false,pianoBroadcast=true;
var muPreviewTimer=null,muPreviewRunning=false;

// 循环模式统一数据源
var LOOP_MODES=[
  {key:'none',label:'不循环'},
  {key:'single',label:'单曲循环'},
  {key:'list',label:'列表循环'},
  {key:'shuffle',label:'随机播放'}
];

var MC_SOUNDS=[
  {v:'note.harp',l:'竖琴'},{v:'note.pling',l:'钟琴'},
  {v:'note.bell',l:'铃铛'},{v:'note.chime',l:'风铃'},
  {v:'note.iron_xylophone',l:'铁木琴'},{v:'note.xylophone',l:'木琴'},
  {v:'note.flute',l:'长笛'},{v:'note.guitar',l:'吉他'},
  {v:'note.bass',l:'贝斯'},{v:'note.banjo',l:'班卓琴'},
  {v:'note.bit',l:'电子音'},{v:'note.cow_bell',l:'牛铃'},
  {v:'note.didgeridoo',l:'迪吉里杜'},{v:'note.bassattack',l:'击弦贝斯'},
  {v:'note.bd',l:'底鼓'},{v:'note.snare',l:'军鼓'},
  {v:'note.hat',l:'踩镲'}
];

var audioCtx=null,audioBufs={},audioReady=false,audioLoading=false;

var MU_TARGET_ITEMS=[{value:'@a',label:'@a (所有玩家)'},{value:'@p',label:'@p (最近玩家)'},{value:'@s',label:'@s (机器人自身)'}];
var MU_SPEED_ITEMS=[{value:'0.5',label:'0.5x'},{value:'0.75',label:'0.75x'},{value:'1.0',label:'1.0x'},{value:'1.5',label:'1.5x'},{value:'2.0',label:'2.0x'}];
var MU_INST_ITEMS=[{value:'',label:'保持原色'}].concat(MC_SOUNDS.map(function(s){return{value:s.v,label:s.l}}));
var PIANO_INST_ITEMS=MC_SOUNDS.map(function(s){return{value:s.v,label:s.l}});
function showMuTargetPicker(){
  showPicker({title:'选择目标玩家',items:MU_TARGET_ITEMS,selected:document.getElementById('mu-target').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('mu-target');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=MU_TARGET_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showMuSpeedPicker(){
  showPicker({title:'选择播放速度',items:MU_SPEED_ITEMS,selected:document.getElementById('mu-speed').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('mu-speed');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=MU_SPEED_ITEMS.find(function(i){return i.value===v}).label
    }})
}
function showMuInstPicker(){
  showPicker({title:'选择强制音色',items:MU_INST_ITEMS,selected:document.getElementById('mu-instrument').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('mu-instrument');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=MU_INST_ITEMS.find(function(i){return i.value===v}).label;
      onMuInstChange()
    }})
}
function showPianoInstPicker(){
  showPicker({title:'选择音色',items:PIANO_INST_ITEMS,selected:document.getElementById('piano-inst').getAttribute('data-value'),
    onConfirm:function(v){
      var p=document.getElementById('piano-inst');p.setAttribute('data-value',v);
      p.querySelector('.picker-label').textContent=PIANO_INST_ITEMS.find(function(i){return i.value===v}).label;
      pianoInstrument=v
    }})
}

function RmusicHTML(){
  initAudio();
  // 页面渲染后异步加载队列状态
  setTimeout(fetchQueueState, 100);
  return '<div class="card"><div style="font-size:16px;font-weight:800;margin-bottom:10px">' + ICONS.musicNote + ' MIDI 播放</div>'+
    '<div class="pick" onclick="pickMidi()"><i class="pi">' + ICONS.musicNote + '</i><div id="mu-pick-txt">点击选择 .mid 文件</div><div style="font-size:10px;color:var(--phx-text-secondary)">.mid .midi</div></div>'+
    '<div id="mu-info" class="gone"></div>'+
    '<div id="mu-params" class="gone">'+
    '<label>目标玩家</label><div class="picker-btn" id="mu-target" data-value="@a" onclick="showMuTargetPicker()"><span class="picker-label">@a (所有玩家)</span><span class="picker-arrow">▾</span></div>'+
    '<label>播放速度</label><div class="picker-btn" id="mu-speed" data-value="1.0" onclick="showMuSpeedPicker()"><span class="picker-label">1.0x</span><span class="picker-arrow">▾</span></div>'+
    '<label>强制音色</label><div class="picker-btn" id="mu-instrument" data-value="" onclick="showMuInstPicker()"><span class="picker-label">保持原色</span><span class="picker-arrow">▾</span></div>'+
    '<div class="row" style="margin-top:6px">'+
    '<button class="btn gone" id="mu-go" onclick="doMusicPlay()" style="flex:2">开始播放</button>'+
    '<button class="btn-s gone" id="mu-preview" onclick="previewMidi()" style="flex:1">预览</button>'+
    '<button class="btn-s gone" id="mu-stop" onclick="doMusicStop()" style="flex:1">停止</button></div>'+
    '<div class="pg gone" id="mu-pg"><div id="mu-bar"></div></div>'+
    '<div class="dim" id="mu-stat" style="margin-top:4px"></div>'+
    // Queue 加入队列按钮
    '<button class="btn-s gone" id="mu-queue-add" onclick="muNotes.length&&muPath&&addToQueue()" style="width:100%;margin:6px 0;font-size:12px">' + ICONS.plus + ' 当前歌曲加入队列</button>'+
    // Queue section（嵌套在 MIDI 卡片内）
    '<div class="gone" id="mu-queue-card" style="margin-top:4px;padding-top:8px;border-top:1px solid var(--phx-border-light)"><div style="font-size:14px;font-weight:800;margin-bottom:8px">播放队列</div>'+
    '<div id="mu-queue-list"></div>'+
    '<div id="mu-queue-controls" class="gone" style="margin-top:8px">'+
    '<div class="row"><button class="btn-s" onclick="doMusicPrev()" style="flex:1;margin:0">' + ICONS.prev + ' 上一曲</button>'+
    '<button class="btn-s" id="mu-play-q" onclick="doMusicStop()" style="flex:1;margin:0">' + ICONS.stop + ' 停止</button>'+
    '<button class="btn-s" onclick="doMusicNext()" style="flex:1;margin:0">' + ICONS.next + ' 下一曲</button></div>'+
    '<div style="display:flex;gap:4px;margin-top:6px">'+
    '<button class="btn-s" id="mu-loop-btn" onclick="cycleLoopMode()" style="flex:1;margin:0;font-size:11px">' + ICONS.loop + ' 列表循环</button>'+
    '<button class="btn-s" onclick="clearQueue()" style="flex:1;margin:0;font-size:11px">' + ICONS.trash + ' 清空队列</button></div></div>'+
    '</div>'+  // end mu-queue-card
    '</div>'+  // end mu-params
    '</div>'+  // end MIDI card
    '<div class="qcard" onclick="openSubPage(\'钢琴弹奏\',RpianoHTML());setTimeout(attachPianoEvents,300)">'+
    '<div class="qc-icon" style="font-size:22px">#</div><div class="qc-body"><div class="qc-title">钢琴弹奏</div>'+
    '<div class="qc-sub">虚拟钢琴键盘，实时弹奏到服务器</div></div><span class="qc-arrow">' + ICONS.arrowRight + '</span></div>'+
    '<div class="card gone" id="mu-log-card"><div class="log" id="mu-log"></div></div>'
}

function RpianoHTML(){
  var instOpts=MC_SOUNDS.map(function(s){return'<option value="'+s.v+'">'+s.l+'</option>'}).join('');
  return '<div class="card">'+
    '<label>音色</label><div class="picker-btn" id="piano-inst" data-value="note.harp" onclick="showPianoInstPicker()"><span class="picker-label">竖琴</span><span class="picker-arrow">▾</span></div>'+
    '<div style="display:flex;align-items:center;justify-content:space-between;margin:10px 0">'+
    '<span>本地预览</span><label class="tgl-cb"><input type="checkbox" id="piano-local-snd" checked onchange="pianoLocalSound=this.checked"><span class="tgl"></span></label></div>'+
    '<div style="display:flex;align-items:center;justify-content:space-between;margin:10px 0">'+
    '<span>滑动锁</span><label class="tgl-cb"><input type="checkbox" id="piano-slide-lock" onchange="pianoSlideLock=this.checked"><span class="tgl"></span></label></div>'+
    '<div style="display:flex;align-items:center;justify-content:space-between;margin:10px 0">'+
    '<span>行动栏通告</span><label class="tgl-cb"><input type="checkbox" id="piano-broadcast" checked onchange="pianoBroadcast=this.checked"><span class="tgl"></span></label></div>'+
    '<div id="piano-last-note" class="dim" style="margin:6px 0;font-size:11px"></div>'+
    '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:4px">'+
    '<span class="dim" style="font-size:10px">缩放: <span id="piano-zoom-val">100%</span></span>'+
    '<span class="dim" style="font-size:10px">滑动键盘·点击弹奏</span></div>'+
    renderPianoKeys()+
    '<div style="margin-top:8px"><div class="dim" style="font-size:10px;margin-bottom:2px">位置</div>'+
    '<input type="range" id="piano-scroll-slider" min="0" max="1000" value="0" style="width:100%;margin:0;accent-color:var(--phx-primary)" oninput="onPianoScrollSlide(this.value)"></div>'+
    '<div style="margin-top:4px"><div class="dim" style="font-size:10px;margin-bottom:2px">缩放</div>'+
    '<input type="range" id="piano-zoom-slider" min="50" max="250" value="100" style="width:100%;margin:0;accent-color:var(--phx-primary)" oninput="onPianoZoomSlide(this.value)"></div>'+
    '</div>'
}

function midiToPitch(note){return Math.round(Math.pow(2,(note-45)/12)*100)/100}

var PIANO_KEYS=[];
(function(){
  var names=['C','C#','D','D#','E','F','F#','G','G#','A','A#','B'];
  var isB=[false,true,false,true,false,false,true,false,true,false,true,false];
  for(var o=2;o<=6;o++){
    for(var i=0;i<12;i++){
      var midi=(o+1)*12+i;
      PIANO_KEYS.push({l:names[i]+o,m:midi,b:isB[i]})
    }
  }
})();

function initAudio(){
  if(!audioCtx){
    try{audioCtx=new(window.AudioContext||window.webkitAudioContext)()}catch(e){return}
  }
  if(audioCtx.state==='suspended'){audioCtx.resume().catch(function(){})}
  if(!audioReady&&!audioLoading){
    audioLoading=true;
    var total=0;
    MC_SOUNDS.forEach(function(s){
      fetch('/sounds/'+s.v+'.wav').then(function(r){
        if(!r.ok)throw new Error('HTTP '+r.status);
        return r.arrayBuffer()
      }).then(function(buf){
        return audioCtx.decodeAudioData(buf)
      }).then(function(ab){
        audioBufs[s.v]=ab; total++;
        if(total>=MC_SOUNDS.length){audioReady=true}
      }).catch(function(){
        total++; if(total>=MC_SOUNDS.length){audioReady=true}
      })
    });
  }
}

function pickMidi(){
  if(typeof android!=='undefined'&&android.pickFile){
    fw=function(p){if(!p)return;uploadMidiFile(p)};android.pickFile('midi')
  }else{
    var i=document.createElement('input');i.type='file';i.accept='.mid,.midi';
    i.onchange=function(){var f=i.files[0];if(!f)return;uploadMidiFile(f)};i.click()
  }
}

function uploadMidiFile(file){
  var name=file.name||file.split('/').pop();
  document.getElementById('mu-pick-txt').innerHTML='<span class="spin"></span> 解析中...';
  var fd=new FormData();
  if(typeof file==='string'){
    fetch('/uploads/'+encodeURIComponent(name)).then(function(r){return r.blob()}).then(function(blob){
      var fd2=new FormData();fd2.append('file',blob,name);
      return fetch('/api/music/upload',{method:'POST',body:fd2})
    }).then(function(r){return r.json()}).then(onMidiParsed).catch(function(e){T('加载失败: '+e.message,'e')})
    return
  }
  fd.append('file',file);
  fetch('/api/music/upload',{method:'POST',body:fd}).then(function(r){return r.json()}).then(onMidiParsed).catch(function(e){T('上传失败: '+e.message,'e')})
}

function onMidiParsed(r){
  if(!r.ok){T('解析失败: '+r.error,'e');resetMusicUI();return}
  muNotes=r.notes;muPath=r.cache_key||'';
  muPlaying=false;muPaused=false;muCurrent=0;muTotal=r.total_notes;
  var pt=document.getElementById('mu-pick-txt');
  if(pt)pt.textContent='已选择';
  document.getElementById('mu-info').classList.remove('gone');
  var instHtml='';
  if(r.instruments&&r.instruments.length){
    instHtml='<div class="dim" style="font-size:9px;margin-top:4px">音色: '+r.instruments.map(function(x){return x.name.replace('note.','')+' x'+x.count}).join(', ')+'</div>'
  }
  document.getElementById('mu-info').innerHTML=
    '<div class="stats"><div class="s"><div class="n">'+r.total_notes+'</div><div class="l">音符</div></div>'+
    '<div class="s"><div class="n">'+r.duration+'</div><div class="l">时长</div></div></div>'+instHtml;
  document.getElementById('mu-params').classList.remove('gone');
  var btn=document.getElementById('mu-go');btn.classList.remove('gone');btn.textContent='开始播放';btn.disabled=false;
  var sb=document.getElementById('mu-stop');if(sb)sb.classList.add('gone');
  var pg=document.getElementById('mu-pg');if(pg)pg.classList.add('gone');
  var bar=document.getElementById('mu-bar');if(bar)bar.style.width='0%';
  document.getElementById('mu-preview').classList.remove('gone');
  var qa=document.getElementById('mu-queue-add');if(qa)qa.classList.remove('gone');
  T('解析完成: '+r.total_notes+' 音符','o')
}

function resetMusicUI(){
  muPlaying=false;muPaused=false;muCurrent=0;
  var go=document.getElementById('mu-go');if(go){go.textContent='开始播放';go.classList.add('gone');go.disabled=false}
  var pg=document.getElementById('mu-pg');if(pg)pg.classList.add('gone');
  var bar=document.getElementById('mu-bar');if(bar)bar.style.width='0%';
  var stat=document.getElementById('mu-stat');if(stat)stat.innerHTML='';
  var sb=document.getElementById('mu-stop');if(sb)sb.classList.add('gone');
  var qa=document.getElementById('mu-queue-add');if(qa)qa.classList.add('gone');
}

function previewMidi(){
  if(muPreviewRunning){stopPreview();return}
  if(!muNotes.length){T('请先选择MIDI文件','e');return}
  initAudio();
  muPreviewRunning=true;
  // 注册清理函数，离开子页面时停止预览
  activeRenderers.push(function(){stopPreview()});
  var btn=document.getElementById('mu-preview');if(btn)btn.textContent='停止预览';
  var i=0,speed=parseFloat(document.getElementById('mu-speed').getAttribute('data-value'))||1;
  var instOv=document.getElementById('mu-instrument').getAttribute('data-value');
  function next(){
    if(!muPreviewRunning||i>=muNotes.length){muPreviewRunning=false;if(btn)btn.textContent='预览';return}
    var n=muNotes[i];playLocalSound(instOv||n.instrument,n.pitch,n.volume);i++;
    muPreviewTimer=setTimeout(next,Math.max(30,n.delay_ms/speed))
  }
  next()
}
function stopPreview(){muPreviewRunning=false;clearTimeout(muPreviewTimer);var btn=document.getElementById('mu-preview');if(btn)btn.textContent='预览'}
function onMuInstChange(){var v=document.getElementById('mu-instrument').getAttribute('data-value');if(v&&muNotes.length>0){for(var i=0;i<muNotes.length;i++)muNotes[i].instrument=v}}
function showMidiView(){
  muView='midi';
  var piano=document.getElementById('mu-piano-section');
  var midi=document.getElementById('mu-midi-section');
  if(piano)piano.style.display='none';
  if(midi)midi.style.display='block';
}

// 进入播放状态 UI：显示播放按钮、进度条、停止按钮
function setPlayingUI(){
  muPlaying=true;
  var go=document.getElementById('mu-go');if(go){go.classList.remove('gone');go.textContent='暂停'}
  var pg=document.getElementById('mu-pg');if(pg)pg.classList.remove('gone');
  var sb=document.getElementById('mu-stop');if(sb)sb.classList.remove('gone');
}

async function doMusicPlay(){
  stopPreview();
  if(muPlaying && !muPaused){
    var r=await A('POST','/api/music/pause');if(!r.ok){T('暂停失败: '+r.error,'e');return}
    muPaused=true;document.getElementById('mu-go').textContent='继续播放';return
  }
  if(muPaused){
    var r=await A('POST','/api/music/resume');if(!r.ok){T('继续失败: '+r.error,'e');return}
    muPaused=false;document.getElementById('mu-go').textContent='暂停';return
  }
  var r=await A('POST','/api/music/play',{cache_key:muPath,target:document.getElementById('mu-target').getAttribute('data-value'),speed:parseFloat(document.getElementById('mu-speed').getAttribute('data-value'))||1});
  if(r.ok){setPlayingUI()}
  else{T('播放失败: '+r.error,'e')}
}

async function doMusicStop(){var r=await A('POST','/api/music/stop');T('已停止');resetMusicUI()}

function playLocalSound(instrument,pitch,vol){
  initAudio();
  if(!audioCtx||!audioReady||!audioBufs[instrument])return;
  if(audioCtx.state==='suspended'){audioCtx.resume().catch(function(){})}
  try{
    var src=audioCtx.createBufferSource();
    src.buffer=audioBufs[instrument];
    src.playbackRate.value=pitch||1;
    var gain=audioCtx.createGain();
    gain.gain.value=Math.min(1,Math.max(0.05,vol||1));
    src.connect(gain);gain.connect(audioCtx.destination);
    src.start(0)
  }catch(e){}
}

// 从服务器加载队列状态
function fetchQueueState(){
  A('GET','/api/music/status').then(function(r){
    if(!r.ok||!r.queue)return;
    muQueue=r.queue||[];
    muQueueIndex=r.queue_index!=null?r.queue_index:-1;
    muQueueActive=r.queue_active||false;
    muLoopMode=r.queue_loop_mode||'none';
    updateLoopBtn();
    renderQueueUI();
    var qc=document.getElementById('mu-queue-controls');
    if(qc)qc.classList.toggle('gone',!muQueue.length);
    var qcard=document.getElementById('mu-queue-card');
    if(qcard)qcard.classList.toggle('gone',!muQueue.length);
    // 如果队列正在播放，恢复播放状态 UI
    if(r.playing&&r.queue_active){
      setPlayingUI();
      var stat=document.getElementById('mu-stat');
      if(stat&&muQueue[muQueueIndex]){
        stat.innerHTML='<div style="font-weight:600;font-size:13px">'+escHtml(muQueue[muQueueIndex].title||'')+'</div>';
      }
    }
  })
}

// Queue functions
function addToQueue(){
  if(!muPath||!muNotes.length){T('请先选择MIDI文件','e');return}
  A('POST','/api/music/queue/add',{cache_key:muPath}).then(function(r){
    if(r.ok){
      T('已添加到队列','o');
      // 立即更新本地队列，不等 SSE
      fetchQueueState();
    } else T('添加失败: '+r.error,'e')
  })
}
function renderQueueUI(){
  var list=document.getElementById('mu-queue-list');
  if(!list)return;
  if(!muQueue.length){list.innerHTML='<div class="q-empty">队列为空，选择 MIDI 文件后点击"加入队列"</div>';return}
  var h='';
  for(var i=0;i<muQueue.length;i++){
    var it=muQueue[i],isCur=i===muQueueIndex,isPlaying=isCur&&muQueueActive;
    h+='<div class="q-item'+(isCur?' q-cur':'')+'" onclick="playQueueItem('+i+')">'+
      '<div class="q-item-left">'+
        '<div class="q-item-num">'+(isPlaying?'<span class="q-playing-icon">'+ICONS.musicNote+'</span>':(i+1))+'</div>'+
        '<div class="q-item-info">'+
          '<div class="q-item-title">'+escHtml(it.title)+'</div>'+
          '<div class="q-item-meta">'+escHtml(it.duration)+' · '+it.notes+' 音符</div>'+
        '</div>'+
      '</div>'+
      '<button class="q-item-del" onclick="event.stopPropagation();removeQueueItem('+i+')" title="移除">'+ICONS.x+'</button>'+
    '</div>'
  }
  list.innerHTML=h
}
function playQueueItem(idx){
  if(idx<0||idx>=muQueue.length)return;
  A('POST','/api/music/play',{queue_index:idx}).then(function(r){if(!r.ok)T('播放失败: '+r.error,'e');else fetchQueueState()})
}
function removeQueueItem(idx){
  A('POST','/api/music/queue/remove',{index:idx}).then(function(r){if(!r.ok)T('删除失败: '+r.error,'e');else fetchQueueState()})
}
function updateLoopBtn(){
  var btn=document.getElementById('mu-loop-btn');
  if(!btn)return;
  var m=LOOP_MODES.find(function(x){return x.key===(muLoopMode||'none')});
  btn.innerHTML=ICONS.loop+' '+(m?m.label:'不循环')
}
function cycleLoopMode(){
  var idx=-1;
  for(var i=0;i<LOOP_MODES.length;i++){if(LOOP_MODES[i].key===(muLoopMode||'none')){idx=i;break}}
  var next=LOOP_MODES[(idx+1)%LOOP_MODES.length].key;
  A('POST','/api/music/loop',{mode:next}).then(function(r){if(!r.ok)T('设置失败: '+r.error,'e');else fetchQueueState()})
}
function clearQueue(){A('POST','/api/music/queue/clear').then(function(){muQueue=[];fetchQueueState()})}
async function doMusicNext(){await A('POST','/api/music/next');fetchQueueState()}
async function doMusicPrev(){await A('POST','/api/music/prev');fetchQueueState()}

// ── Piano ──
var pianoZoom=1.0,pianoZoomMin=0.5,pianoZoomMax=2.5;
var pianoBaseWk=28; // base white key width in px
var pianoBroadcastTimer=null;

function renderPianoKeys(){
  var wk=PIANO_KEYS.filter(function(k){return!k.b});
  var bk=PIANO_KEYS.filter(function(k){return k.b});
  var wkWidth=Math.round(pianoBaseWk*pianoZoom);
  var html='<div class="piano-viewport" id="piano-vp">'+
    '<div class="piano-inner" id="piano-inner">';
  for(var i=0;i<wk.length;i++){
    html+='<div class="pkey-white" data-pidx="'+i+'" data-midi="'+wk[i].m+'" style="width:'+wkWidth+'px;min-width:'+wkWidth+'px;max-width:'+wkWidth+'px">'+
      '<span class="pkey-label">'+wk[i].l+'</span></div>'
  }
  for(var j=0;j<bk.length;j++){
    var wkIdx=PIANO_KEYS.indexOf(bk[j]);
    var wBefore=0;for(var n=0;n<wkIdx;n++){if(!PIANO_KEYS[n].b)wBefore++}
    var bw=Math.round(wkWidth*0.6);
    var left=wBefore*wkWidth+wkWidth-bw/2;
    html+='<div class="pkey-black" data-midi="'+bk[j].m+'" style="left:'+left+'px;width:'+bw+'px"></div>'
  }
  html+='</div></div>';
  return html
}

function attachPianoEvents(){
  var vp=document.getElementById('piano-vp');if(!vp)return;
  var activeTouches={};
  var touchState={mode:null,startX:0,startY:0,startOff:0,moved:false};

  function findKey(px,py){
    var rect=vp.getBoundingClientRect();
    var relX=px-rect.left+vp.scrollLeft;
    var relY=py-rect.top;
    var blacks=vp.querySelectorAll('.pkey-black');
    for(var i=blacks.length-1;i>=0;i--){
      var b=blacks[i].getBoundingClientRect();
      var bx=b.left-rect.left+vp.scrollLeft,by=b.top-rect.top;
      if(relX>=bx&&relX<=bx+b.width&&relY>=by&&relY<=by+b.height)return blacks[i]
    }
    var whites=vp.querySelectorAll('.pkey-white');
    for(var i=0;i<whites.length;i++){
      var w=whites[i].getBoundingClientRect();
      var wx=w.left-rect.left+vp.scrollLeft,wy=w.top-rect.top;
      if(relX>=wx&&relX<=wx+w.width&&relY>=wy&&relY<=wy+w.height)return whites[i]
    }
    return null
  }

  function triggerKey(el){
    if(!el)return;
    var midi=parseInt(el.dataset.midi);if(!midi)return;
    el.classList.add('pressed');
    var pitch=midiToPitch(midi);
    stopPreview();
    if(pianoLocalSound)playLocalSound(pianoInstrument,pitch,1.0);
    A('POST','/api/music/note',{instrument:pianoInstrument,pitch:pitch,volume:1.0});
    A('POST','/api/music/actionbar',{msg:'§e§l▍人工演奏中 §r§f···'});
    clearTimeout(pianoBroadcastTimer);
    pianoBroadcastTimer=setTimeout(function(){A('POST','/api/music/actionbar',{msg:''})},3000);
    var lastEl=document.getElementById('piano-last-note');
    if(lastEl)lastEl.textContent='最后弹奏: '+midi+' ('+pitch.toFixed(2)+')'
  }
  function releaseKeyEl(el){if(el)el.classList.remove('pressed')}

  vp.addEventListener('touchstart',function(e){
    e.preventDefault();
    if(e.touches.length>=2){
      touchState.mode='pinch';
      touchState.startD=Math.hypot(e.touches[0].clientX-e.touches[1].clientX,e.touches[0].clientY-e.touches[1].clientY);
    }else{
      var t=e.touches[0];
      var key=findKey(t.clientX,t.clientY);
      touchState.mode='tap';touchState.startX=t.clientX;touchState.startY=t.clientY;
      touchState.startOff=vp.scrollLeft;touchState.moved=false;
      if(key){triggerKey(key);activeTouches[t.identifier]={el:key}}
    }
  },{passive:false});

  vp.addEventListener('touchmove',function(e){
    e.preventDefault();
    if(touchState.mode!=='tap'||e.touches.length!==1)return;
    var t=e.touches[0];
    var dx=t.clientX-touchState.startX;
    if(Math.abs(dx)>5||Math.abs(t.clientY-touchState.startY)>5)touchState.moved=true;
    if(!touchState.moved)return;
    if(pianoSlideLock){
      var key=findKey(t.clientX,t.clientY);
      var prev=activeTouches[t.identifier];
      if(key&&(!prev||key!==prev.el)){if(prev)releaseKeyEl(prev.el);triggerKey(key);activeTouches[t.identifier]={el:key}}
    }else{
      vp.scrollLeft=touchState.startOff-dx;
      var key2=findKey(t.clientX,t.clientY);
      var prev2=activeTouches[t.identifier];
      if(key2&&(!prev2||key2!==prev2.el)){if(prev2)releaseKeyEl(prev2.el);triggerKey(key2);activeTouches[t.identifier]={el:key2}}
    }
  },{passive:false});

  vp.addEventListener('touchend',function(e){
    e.preventDefault();
    for(var i=0;i<e.changedTouches.length;i++){
      var id=e.changedTouches[i].identifier;
      var rec=activeTouches[id];if(rec){releaseKeyEl(rec.el);delete activeTouches[id]}
    }
    if(e.touches.length===0){touchState.mode=null;vp.querySelectorAll('.pressed').forEach(function(k){k.classList.remove('pressed')});activeTouches={}}
  },{passive:false});
  vp.addEventListener('touchcancel',function(){touchState.mode=null;vp.querySelectorAll('.pressed').forEach(function(k){k.classList.remove('pressed')});activeTouches={}});
  vp.addEventListener('scroll',function(){var sl=document.getElementById('piano-scroll-slider');if(sl)sl.value=vp.scrollLeft});
  vp.style.touchAction='none';
  vp.addEventListener('contextmenu',function(e){e.preventDefault()});
}

function onPianoScrollSlide(v){
  var vp=document.getElementById('piano-vp');if(!vp)return;
  vp.scrollLeft=parseInt(v)
}
function onPianoZoomSlide(v){
  pianoZoom=parseInt(v)/100;
  var vp=document.getElementById('piano-vp');if(!vp)return;
  var wk=PIANO_KEYS.filter(function(k){return!k.b});
  var ww=Math.round(pianoBaseWk*pianoZoom);
  vp.querySelectorAll('.pkey-white').forEach(function(el,i){el.style.width=ww+'px';el.style.minWidth=ww+'px';el.style.maxWidth=ww+'px'});
  var bk=PIANO_KEYS.filter(function(k){return k.b});
  vp.querySelectorAll('.pkey-black').forEach(function(el,j){
    var wkIdx=PIANO_KEYS.indexOf(bk[j]);var wBefore=0;
    for(var n=0;n<wkIdx;n++){if(!PIANO_KEYS[n].b)wBefore++}
    var bw=Math.round(ww*0.6);
    el.style.left=(wBefore*ww+ww-bw/2)+'px';el.style.width=bw+'px'
  });
  var z=document.getElementById('piano-zoom-val');
  if(z)z.textContent=Math.round(pianoZoom*100)+'%'
}