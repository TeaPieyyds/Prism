// mcfunction 执行器 — 语法高亮 + 四步向导
// 预编译正则，单次扫描，移动端优化

var mcfnLoaded = false;
var mcfnStyles = {};
var mcfnColors = {};
var mcfnTokens = [];

function mcfnLoadSyntax() {
  if (mcfnLoaded) return;
  mcfnLoaded = true;
  try {
    var xhr = new XMLHttpRequest();
    xhr.open('GET', '/mcf-syntax.mtsx', false);
    xhr.send();
    if (xhr.status === 200) mcfnParseMTSX(xhr.responseText);
  } catch(e) { console.error('语法加载失败:', e); }
}

function mcfnParseMTSX(text) {
  var sm = text.match(/styles\s*:\s*\[([\s\S]*?)\]\s*(?:\w|$)/);
  if (sm) {
    var re = /"([^"]+)"\s*[,>]\s*#?([0-9A-Fa-f]{6})/g, m;
    while ((m = re.exec(sm[1])) !== null) {
      if (m[1].length === 1 && m[1].match(/[0-9a-f]/)) mcfnColors[m[1]] = '#' + m[2];
      else mcfnStyles[m[1]] = '#' + m[2];
    }
  }
  var dm = text.match(/defines\s*:\s*\[([\s\S]*?)\]\s*(?:\w|\})/);
  if (dm) {
    var defs = {}, defRe = /"(\w+)"\s*:\s*\/((?:[^\/\\]|\\.)*)\//g, mm;
    while ((mm = defRe.exec(dm[1])) !== null) defs[mm[1]] = mm[2];
    function resolve(s) { return s.replace(/\/\+include\("([^"]+)"\)\+\//g, function(m, n) { return defs[n] || ''; }); }
    var sc = mcfnStyles;
    // 从带有样式名的 match 对象中提取正则
    var objRe = /"(\w+)"\s*:\s*\{[^}]*?(?:0\s*:\s*"(\w+)"|style\s*:\s*"(\w+)")[^}]*?\}/g, om;
    while ((om = objRe.exec(dm[1])) !== null) {
      var name = om[1], st = om[2] || om[3];
      // 跳过 Error 样式（它会匹配一切，导致全部爆红）
      if (st === 'Error' || st === 'error') continue;
      var mr = dm[1].match(new RegExp('"' + name + '"\\s*:\\s*\\{[^}]*?match\\s*:\\s*\\/((?:[^\/\\\\]|\\\\.)*)\\/'));
      if (mr && st && sc[st]) { try { mcfnTokens.push({ regex: new RegExp(resolve(mr[1]), 'g'), color: sc[st] }); } catch(e) {} }
    }
  }
  var selC = mcfnStyles['Sele'] || '#E8BF6A', numC = mcfnStyles['Number'] || '#CC7832', strC = mcfnStyles['String'] || '#6A8759';
  try { mcfnTokens.push({ regex: /@[aperserchelinv]+(?:\[[^\]]*\])?/g, color: selC }); } catch(e) {}
  try { mcfnTokens.push({ regex: /[~^][0-9.-]*/g, color: numC }); } catch(e) {}
  try { mcfnTokens.push({ regex: /\b-?[0-9]+(?:\.[0-9]+)?\b/g, color: numC }); } catch(e) {}
  try { mcfnTokens.push({ regex: /"(?:[^"\\]|\\.)*"/g, color: strC }); } catch(e) {}
}

function highlightMcfunction(text) {
  if (!text) return '';
  mcfnLoadSyntax();
  var lines = text.split('\n'), html = '';
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i], t = line.trim();
    if (t === '') { html += '<br>'; continue; }
    if (t.charAt(0) === '#') {
      if (/^#\s*[组群分区类别][：: 　]/.test(t)) html += '<span class="mcf-group-mark">' + escHtml(line) + '</span><br>';
      else if (/^#\s*[定义测试规定][：: 　]/.test(t)) html += '<span class="mcf-test-mark">' + escHtml(line) + '</span><br>';
      else if (/^#\s*[执行运行][：: 　]/.test(t)) html += '<span class="mcf-exec-mark">' + escHtml(line) + '</span><br>';
      else html += '<span class="mcf-comment">' + escHtml(line) + '</span><br>';
    } else {
      html += '<span class="mcf-cmd-line">' + hlLine(line) + '</span><br>';
    }
  }
  return html;
}

function hlLine(line) {
  mcfnLoadSyntax();
  var e = escHtml(line);
  // 单次扫描：所有 token 正则合成为一个，避免嵌套匹配
  if (mcfnTokens.length > 0) {
    var sources = [];
    for (var ti = 0; ti < mcfnTokens.length; ti++) {
      sources.push('(?:' + mcfnTokens[ti].regex.source + ')');
    }
    var combined = new RegExp(sources.join('|'), 'g');
    e = e.replace(combined, function(m) {
      for (var si = 0; si < mcfnTokens.length; si++) {
        var re = new RegExp('^(?:' + mcfnTokens[si].regex.source + ')$');
        if (re.test(m)) {
          return '<span style="color:' + mcfnTokens[si].color + '">' + m + '</span>';
        }
      }
      return m;
    });
  }
  // § 颜色码
  e = e.replace(/§([0-9a-gklmnopqrstu])/g, function(m, c) {
    var co = mcfnColors[c];
    if (c === 'l') return '<span class="mcf-bold">';
    if (c === 'o') return '<span class="mcf-italic">';
    if (c === 'r') return '</span>';
    if (co) return '<span style="color:' + co + '">';
    return m;
  });
  return e;
}

function checkMcfunctionSyntax(text) {
  var lines = text.split('\n'), w = [];
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i].trim();
    if (!line || line.charAt(0) === '#') continue;
    var q = false;
    for (var j = 0; j < line.length; j++) { if (line[j] === '"' && (j === 0 || line[j-1] !== '\\')) q = !q; }
    if (q) w.push({ line: i+1, text: line.substring(0,60), message: '引号未闭合' });
  }
  return w;
}

var mcfnState = {step:1,filePath:'',fileName:'',content:'',parsed:null,edited:false,globalConfig:{channel:'ai',speed:200,prefix:'',loop:1},groupConfigs:{},testExecs:[],warnings:[],executing:false};

function RmcfunctionHTML() {
  setTimeout(function(){renderMcfunctionStep(mcfnState.step||1)},50);
  return '<div id="mcfn-wrap" style="width:100%"><div id="mcfn-body"></div></div>';
}

function renderMcfunctionStep(s) {
  mcfnState.step = s;
  var b = document.getElementById('mcfn-body');
  if (!b) return;
  switch(s) { case 1: s1(); break; case 2: s2(); break; case 3: s3(); break; case 4: s4(); break; }
}

function s1() {
  document.getElementById('mcfn-body').innerHTML =
    '<div class="wz-section-title"><svg width="18" height="18" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>步骤 1/4 · 选择文件</div>' +
    '<div class="pick" onclick="mcfnPickFile()" style="margin-top:10px"><i class="pi"><svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M13 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><polyline points="13 2 13 9 20 9"/></svg></i><div id="mcfn-pick-txt">点击选择 .mcfunction 文件</div><div style="font-size:10px;color:var(--phx-text-secondary)">.mcfunction .txt .mccmd .command</div></div>' +
    '<div style="text-align:center;margin-top:8px"><button class="btn-s" onclick="mcfnGenerateExample()" style="width:auto;margin:0;padding:8px 16px;font-size:11px"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle;margin-right:4px"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>生成示例文件</button></div>' +
    '<div id="mcfn-file-info" class="gone" style="margin-top:10px"></div>' +
    '<div class="row" style="margin-top:20px"><button class="btn" id="mcfn-step1-next" onclick="s2()" disabled>下一步</button></div>';
}

function mcfnPickFile() {
  showFilePicker('/storage/emulated/0','text',function(path){
    mcfnState.filePath=path; mcfnState.fileName=path.split('/').pop();
    document.getElementById('mcfn-pick-txt').innerHTML='<span class="spin"></span> 加载中...';
    fetch('/api/files/read?path='+encodeURIComponent(path)).then(function(r){if(!r.ok)throw Error('HTTP '+r.status);return r.text()})
    .then(function(c){mcfnState.content=c;return A('POST','/api/mcfunction/parse',{content:c})})
    .then(function(r){if(!r.ok){T('解析失败: '+(r.error||''),'e');return}
      mcfnState.parsed=r.parsed; mcfnState.warnings=checkMcfunctionSyntax(mcfnState.content);
      var l=mcfnState.content.split('\n').length, g=r.parsed.groups||[], t=r.parsed.tests||[];
      document.getElementById('mcfn-pick-txt').textContent=mcfnState.fileName;
      document.getElementById('mcfn-file-info').classList.remove('gone');
      document.getElementById('mcfn-file-info').innerHTML='<div class="stats"><div class="s"><div class="n">'+l+'</div><div class="l">行</div></div><div class="s"><div class="n">'+g.length+'</div><div class="l">命令组</div></div><div class="s"><div class="n">'+t.length+'</div><div class="l">测试定义</div></div></div>';
      document.getElementById('mcfn-step1-next').disabled=false;
    }).catch(function(e){T('读取失败: '+e.message,'e');document.getElementById('mcfn-pick-txt').textContent='点击选择文件'});
  },'text');
}

function mcfnGenerateExample() {
  var e='# =============================================\n# mcfunction 执行器 使用教程\n# =============================================\n#\n# 写法：#组:组名 配置项=值\n# 配置项：次数=N  通道=0/1/2  速率=N  前缀=...\n#\n#组:基础测试 通道=0 速率=200\nsay 这是第一组命令\nsetblock ~ ~ ~ stone\nfill ~ ~2 ~ ~ ~5 ~ oak_planks\n\n#测试:高频放置 坐标:≤~ ~2 ~≥ 次数:≤50≥\nsetblock ≤坐标≥ redstone_block\nsay 放了≤次数≥个红石块\n#执行:高频放置 坐标=~ ~3 ~ 次数=10\n\n#组:检测 通道=2 速率=100\ntestforblock ~ ~ ~ stone\n\n#组:批量发东西 前缀=execute as @a run 次数=3 通道=0\ngive @s diamond 1\nsay 发完了\n';
  var b=new Blob([e],{type:'text/plain'}),u=URL.createObjectURL(b),a=document.createElement('a');a.href=u;a.download='示例_注释说明.mcfunction';a.click();URL.revokeObjectURL(u);T('示例文件已生成','o');
}

var mcfnHTimer=null;

function s2() {
  mcfnLoadSyntax();
  var b=document.getElementById('mcfn-body'),p=mcfnState.parsed,g=p?p.groups||[]:[],t=p?p.tests||[]:[],w=mcfnState.warnings||[];
  var tb='<div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:10px">';
  for(var i=0;i<g.length;i++) tb+='<span class="mcfn-tab" data-idx="'+i+'" onclick="mcfnScrollToGroup('+i+')">'+escHtml(g[i].name)+':'+g[i].commands.length+'</span>';
  tb+='</div>';
  var wh=w.length?'<div style="padding:8px 12px;border-radius:10px;background:rgba(245,158,11,.12);color:var(--phx-warning);font-size:11px;margin-bottom:10px"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle;margin-right:4px"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>'+w.length+' 条语法警告</div>':'';
  b.innerHTML=
    '<div class="wz-section-title"><span style="display:flex;align-items:center;gap:8px"><svg width="18" height="18" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>步骤 2/4 · 命令预览<button class="btn-s" onclick="mcfnPickFile()" style="font-size:10px;padding:4px 10px;margin:0;width:auto">重新选择</button></span></div>'+
    wh+tb+
    '<div style="font-size:10px;color:var(--phx-text-secondary);margin-bottom:6px">'+g.length+' 组 / '+t.length+' 测试定义</div>'+
    '<div id="mcfn-editor" contenteditable style="width:100%;height:200px;padding:10px;font-family:monospace;font-size:11px;line-height:1.5;border-radius:10px;border:2px solid var(--phx-border-light);background:#0D1B1A;overflow:auto;white-space:pre-wrap;word-break:break-all;outline:none" oninput="mcfnState.edited=true;document.getElementById(\'mcfn-save-btn\').disabled=false;mcfnDebounceHighlight()">'+highlightMcfunction(mcfnState.content||'')+'</div>'+
    '<div class="row" style="margin-top:8px"><button class="btn-s" id="mcfn-save-btn" onclick="mcfnSaveFile()" disabled style="flex:1">保存修改</button><button class="btn-s" onclick="mcfnRevert()" style="flex:1">撤销修改</button></div>'+
    '<div class="row" style="margin-top:16px"><button class="btn-s" onclick="s1()" style="flex:1">上一步</button><button class="btn" onclick="s3()" style="flex:1">下一步</button></div>';
}

function mcfnDebounceHighlight() {
  if(mcfnHTimer)clearTimeout(mcfnHTimer);
  mcfnHTimer=setTimeout(function(){var e=document.getElementById('mcfn-editor'),h=document.getElementById('mcfn-highlight');if(e&&h)h.innerHTML=highlightMcfunction(e.value)},200);
}
function mcfnScrollToGroup(i){document.querySelectorAll('.mcfn-tab').forEach(function(e){e.style.background='';e.style.color=''});var t=document.querySelector('.mcfn-tab[data-idx="'+i+'"]');if(t){t.style.background='var(--phx-primary-bg)';t.style.color='var(--phx-primary)'}}
function mcfnSaveFile(){var c=document.getElementById('mcfn-editor').value;showConfirm({title:'确认保存',message:'确定要保存修改并覆盖原文件吗？',onConfirm:function(){var b=new Blob([c],{type:'text/plain'}),u=URL.createObjectURL(b),a=document.createElement('a');a.href=u;a.download=mcfnState.fileName;a.click();URL.revokeObjectURL(u);mcfnState.content=c;mcfnState.edited=false;document.getElementById('mcfn-save-btn').disabled=true;T('已保存','o')}})}
function mcfnRevert(){document.getElementById('mcfn-editor').value=mcfnState.content;mcfnState.edited=false;document.getElementById('mcfn-save-btn').disabled=true;var h=document.getElementById('mcfn-highlight');if(h)h.innerHTML=highlightMcfunction(mcfnState.content)}

function s3(){
  var e=document.getElementById('mcfn-editor');if(e&&mcfnState.edited)mcfnState.content=e.value;
  var b=document.getElementById('mcfn-body'),p=mcfnState.parsed,g=p?p.groups||[]:[],t=p?p.tests||[]:[],gc=mcfnState.globalConfig,co=['ai','console','player'],cl={ai:'魔法指令 (0)',console:'控制台 (2)',player:'玩家 (1)'};
  var h='<div class="wz-section-title"><svg width="18" height="18" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>步骤 3/4 · 执行配置</div>'+
    '<div class="card" style="padding:14px;margin-bottom:10px"><div style="font-size:12px;font-weight:700;margin-bottom:8px;color:var(--phx-text)">全局默认</div>'+
    '<div class="row"><div style="flex:1"><label style="margin:0 0 2px">通道</label><select id="mcfn-global-channel" onchange="mcfnUpdateGlobal()">'+co.map(function(c){return '<option value="'+c+'"'+(gc.channel===c?' selected':'')+'>'+cl[c]+'</option>'}).join('')+'</select></div>'+
    '<div style="flex:1"><label style="margin:0 0 2px">速率(ms)</label><input id="mcfn-global-speed" type="number" value="'+(gc.speed||200)+'" min="10" max="5000" onchange="mcfnUpdateGlobal()"></div></div>'+
    '<div style="margin-top:6px"><label style="margin:0 0 2px">前缀</label><input id="mcfn-global-prefix" type="text" value="'+escHtml(gc.prefix||'')+'" placeholder="如 execute as @a run" onchange="mcfnUpdateGlobal()"></div></div>';
  h+='<div style="font-size:12px;font-weight:700;margin-bottom:6px;color:var(--phx-text)">命令组</div>';
  for(var i=0;i<g.length;i++){
    var gr=g[i],gc2=mcfnState.groupConfigs[gr.name]||{};if(gc2.enabled===undefined)gc2.enabled=true;
    var gch=gc2.channel||gr.config.channel||'',gsp=gc2.speed||gr.config.speed||0,glp=gc2.loop_count||gr.config.loop_count||1;
    h+='<div class="card" style="padding:12px;margin-bottom:6px;border-color:var(--phx-border)"><label class="lbl-cb" style="margin:0 0 6px;font-size:13px"><input type="checkbox"'+(gc2.enabled?' checked':'')+' onchange="mcfnToggleGroup(\''+escHtml(gr.name)+'\',this.checked)"><span class="tgl"></span><strong>'+escHtml(gr.name)+'</strong> <span class="dim" style="font-size:10px">'+gr.commands.length+' 条命令</span></label>'+
      '<div class="row" style="gap:4px"><select id="mcfn-grp-ch-'+i+'" onchange="mcfnSetGroup(\''+escHtml(gr.name)+'\',\'channel\',this.value)" style="font-size:10px;padding:6px 8px;flex:1"><option value="">默认</option>'+co.map(function(c){return '<option value="'+c+'"'+(gch===c?' selected':'')+'>'+cl[c]+'</option>'}).join('')+'</select>'+
      '<input id="mcfn-grp-spd-'+i+'" type="number" value="'+gsp+'" placeholder="速率" min="0" step="10" style="font-size:10px;padding:6px 8px;flex:1" onchange="mcfnSetGroup(\''+escHtml(gr.name)+'\',\'speed\',this.value)">'+
      '<input id="mcfn-grp-lp-'+i+'" type="number" value="'+glp+'" placeholder="次数" min="1" style="font-size:10px;padding:6px 8px;flex:0.6" onchange="mcfnSetGroup(\''+escHtml(gr.name)+'\',\'loop_count\',this.value)"></div></div>';
  }
  if(t.length>0){
    h+='<div style="font-size:12px;font-weight:700;margin:8px 0 4px;color:var(--phx-text)">测试命令</div>';
    for(var ti=0;ti<t.length;ti++){
      var td=t[ti],te=mcfnState.testExecs[ti]||{enabled:false,params:{}},ph='',pk=Object.keys(td.params);
      if(pk.length>0){ph='<div class="row" style="margin-top:4px;gap:4px">';for(var pi=0;pi<pk.length;pi++){ph+='<div style="flex:1"><span style="font-size:9px;color:var(--phx-text-secondary)">'+pk[pi]+'</span><input id="mcfn-test-param-'+ti+'-'+pk[pi]+'" type="text" value="'+escHtml(te.params[pk[pi]]||td.params[pk[pi]]||'')+'" style="font-size:10px;padding:6px 8px" placeholder="'+escHtml(pk[pi])+'"></div>'}ph+='</div>'}
      h+='<div class="card" style="padding:10px;margin-bottom:4px;border-color:var(--phx-border-light)"><label class="lbl-cb" style="margin:0;font-size:12px"><input type="checkbox"'+(te.enabled?' checked':'')+' onchange="mcfnToggleTest('+ti+',this.checked)"><span class="tgl"></span>'+escHtml(td.name)+' <span class="dim" style="font-size:10px">'+td.commands.length+' 条</span></label>'+ph+'</div>';
    }
  }
  h+='<div class="row" style="margin-top:16px"><button class="btn-s" onclick="s2()" style="flex:1">上一步</button><button class="btn" onclick="mcfnStartExec()" style="flex:1">开始执行</button></div>';
  b.innerHTML=h;
}
function mcfnUpdateGlobal(){mcfnState.globalConfig.channel=document.getElementById('mcfn-global-channel').value;mcfnState.globalConfig.speed=parseInt(document.getElementById('mcfn-global-speed').value)||200;mcfnState.globalConfig.prefix=document.getElementById('mcfn-global-prefix').value}
function mcfnToggleGroup(n,e){if(!mcfnState.groupConfigs[n])mcfnState.groupConfigs[n]={};mcfnState.groupConfigs[n].enabled=e}
function mcfnSetGroup(n,k,v){if(!mcfnState.groupConfigs[n])mcfnState.groupConfigs[n]={};mcfnState.groupConfigs[n][k]=(k==='speed'||k==='loop_count')?parseInt(v)||0:v}
function mcfnToggleTest(i,e){if(!mcfnState.testExecs[i])mcfnState.testExecs[i]={enabled:false,params:{}};mcfnState.testExecs[i].enabled=e}

function mcfnDoExec(p,t){
  try{
    mcfnState.executing=true;s4();
    var gc={},pg=p.groups||[];for(var i=0;i<pg.length;i++){var g=pg[i],gc2=mcfnState.groupConfigs[g.name];if(gc2)gc[g.name]=gc2}
    var te2=[];for(var ti2=0;ti2<t.length;ti2++){var te3=mcfnState.testExecs[ti2];if(te3&&te3.enabled)te2.push({name:t[ti2].name,params:te3.params||{},enabled:true})}
    A('POST','/api/mcfunction/execute',{content:mcfnState.content,global:mcfnState.globalConfig,groups:gc,tests:te2}).then(function(r){if(!r.ok){T('启动失败: '+(r.error||''),'e');mcfnState.executing=false;return}console.log('执行已启动');mcfnPollStatus()});
  }catch(e){T("执行错误: "+e.message,"e");mcfnState.executing=false;console.error(e)}
}
function mcfnStartExec(){
  T("正在启动...","o");
  var p=mcfnState.parsed;if(!p){T("请先选择文件","e");return;}
  try{
    var t=p.tests||[];
    for(var ti=0;ti<t.length;ti++){var te=mcfnState.testExecs[ti];if(!te||!te.enabled)continue;var pk=Object.keys(t[ti].params);for(var pi=0;pi<pk.length;pi++){var el=document.getElementById('mcfn-test-param-'+ti+'-'+pk[pi]);if(el)te.params[pk[pi]]=el.value}}
    var w=checkMcfunctionSyntax(mcfnState.content);if(w.length>0){showConfirm({title:'语法警告',message:'检测到 '+w.length+' 条语法警告（引号未闭合）。\n\n仍然执行？',onConfirm:function(){mcfnDoExec(p,t)}});return}
    mcfnDoExec(p,t);
  }catch(e){T("执行错误: "+e.message,"e");mcfnState.executing=false;console.error(e)}
}

function mcfnPollStatus(){
  var poll=setInterval(function(){A('GET','/api/mcfunction/status').then(function(r){if(!r.ok){clearInterval(poll);mcfnState.executing=false;return}mcfnUpdateProgress(r);if(!r.running){clearInterval(poll);mcfnState.executing=false}})},200);
}
function s4(){mcfnState.step=4;var b=document.getElementById("mcfn-body");if(!b)return;b.innerHTML='<div class="wz-section-title"><svg width="18" height="18" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>步骤 4/4 · 执行中...</div>'+'<div class="card" style="margin-top:8px"><div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="font-size:12px;font-weight:700">进度</span><span class="dim" style="font-size:10px">0 / 0</span></div><div class="pg"><div style="width:0%"></div></div></div>'+'<div class="card" style="margin-top:8px;padding:10px;max-height:240px;overflow-y:auto;background:var(--phx-bg)"><div style="font-size:10px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:4px">执行日志</div></div>'+'<div class="row" style="margin-top:12px"><button class="btn-d" onclick="mcfnStopExec()" style="flex:1"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle;margin-right:4px"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>停止</button></div>';}

function mcfnUpdateProgress(st){
  var b=document.getElementById('mcfn-body');if(!b)return;
  var pct=Math.round((st.progress||0)*100),log=st.log||[],start=Math.max(0,log.length-50),lh='';
  for(var i=start;i<log.length;i++){
    var e=log[i],ico=e.status==='ok'?'<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle"><polyline points="20 6 9 17 4 12"/></svg>':'<svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>';
    var clr=e.status==='ok'?'var(--phx-success)':'var(--phx-error)';
    lh+='<div style="font-size:10px;color:'+clr+';line-height:1.6">['+(e.channel||'ai')+'] '+escHtml(e.group||'')+' '+ico+' '+escHtml((e.command||'').substring(0,50))+(e.error?' <span style="color:var(--phx-error)">'+escHtml(e.error)+'</span>':'')+'</div>';
  }
  b.innerHTML='<div class="wz-section-title"><svg width="18" height="18" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>'+(st.running?'步骤 4/4 · 执行中...':'步骤 4/4 · 执行完成')+'</div>'+
    '<div class="card" style="margin-top:8px"><div style="display:flex;justify-content:space-between;margin-bottom:6px"><span style="font-size:12px;font-weight:700">'+(st.running?'进度':'已完成')+'</span><span class="dim" style="font-size:10px">'+(st.done||0)+' / '+(st.total||0)+'</span></div><div class="pg"><div style="width:'+pct+'%"></div></div>'+(st.current_group?'<div style="font-size:11px;color:var(--phx-text-secondary);margin-top:4px">当前: '+escHtml(st.current_group)+' > '+escHtml((st.current_command||'').substring(0,40))+'</div>':'')+'</div>'+
    '<div class="card" style="margin-top:8px;padding:10px;max-height:240px;overflow-y:auto;background:var(--phx-bg)"><div style="font-size:10px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:4px">执行日志</div>'+lh+'</div>'+
    '<div class="row" style="margin-top:12px">'+(st.running?'<button class="btn-d" onclick="mcfnStopExec()" style="flex:1"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="vertical-align:middle;margin-right:4px"><rect x="6" y="6" width="12" height="12" rx="1"/></svg>停止</button>':'<button class="btn" onclick="mcfnRestart()" style="flex:1">重新执行</button><button class="btn-s" onclick="s3()" style="flex:1">返回配置</button>')+'</div>';
}
function mcfnStopExec(){A('POST','/api/mcfunction/stop',{})}
function mcfnRestart(){mcfnStartExec()}
function restoreMcfunctionState(){if(mcfnState.step===4&&mcfnState.executing)mcfnPollStatus()}