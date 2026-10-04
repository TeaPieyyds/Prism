// ═══════════════════════════════════════
// 其他功能 — 子页面卡片网格
// ═══════════════════════════════════════

function RothersHTML() {
var cards = [
    {i: '<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="2" width="18" height="18" rx="2"/><line x1="2" y1="7" x2="20" y2="7"/><line x1="2" y1="12" x2="20" y2="12"/><line x1="2" y1="17" x2="20" y2="17"/><line x1="7" y1="2" x2="7" y2="20"/><line x1="12" y1="2" x2="12" y2="20"/><line x1="17" y1="2" x2="17" y2="20"/></svg>', t: '滚动字幕', d: 'ASCII 大字滚动', act: function(){openSubPage('滚动字幕', RmarqueeHTML())}},
    {i: '<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>', t: 'mcfunction 执行器', d: '命令文件执行 / 语法高亮', act: function(){openSubPage('mcfunction 执行器', RmcfunctionHTML())}},
    {i: '<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/><circle cx="12" cy="13" r="4"/></svg>', t: '相机工具', d: 'camera 指令生成 / 镜头录制', act: function(){openSubPage('相机工具', RcameraHTML())}},
    {i: '<svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 14l-5-5-5 5"/><path d="M17 20l-5-5-5 5"/><line x1="12" y1="2" x2="12" y2="9"/></svg>', t: '飞行控制', d: '机器人移动 / 视角 / 传送', act: function(){delete subPageCache['飞行控制'];openSubPage('飞行控制', RflyHTML())}},
    {i: '<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>', t: '物品制作器', d: '木桶自定义物品 / 换行 / 颜色', act: function(){delete subPageCache['物品制作器'];openSubPage('物品制作器', RitemmakerHTML())}},
      ];

  var h = '<div class="grid-2" style="margin-top:8px">';
  for (var i = 0; i < cards.length; i++) {
    var f = cards[i];
    h += '<div class="grid-card" onclick="(' + f.act.toString() + ')()">' +
      '<div class="gc-icon">' + f.i + '</div>' +
      '<div class="gc-title">' + f.t + '</div>' +
      '<div class="gc-sub">' + f.d + '</div></div>';
  }
  h += '</div>';
  return h;
}

// ═══════════════════════════════════════
// 滚动字幕 — 控制面板
// ═══════════════════════════════════════

var mcColorMap = {};
var rainbowColorIds = ['c', '6', 'e', 'a', 'b', 'd', '5'];

var mcColors = [
  {id: '0', label: '黑', color: '#000000'},
  {id: '1', label: '深蓝', color: '#0000AA'},
  {id: '2', label: '深绿', color: '#00AA00'},
  {id: '3', label: '深青', color: '#00AAAA'},
  {id: '4', label: '深红', color: '#AA0000'},
  {id: '5', label: '深紫', color: '#AA00AA'},
  {id: '6', label: '金', color: '#FFAA00'},
  {id: '7', label: '灰', color: '#AAAAAA'},
  {id: '8', label: '深灰', color: '#555555'},
  {id: '9', label: '蓝', color: '#5555FF'},
  {id: 'a', label: '绿', color: '#55FF55'},
  {id: 'b', label: '青', color: '#55FFFF'},
  {id: 'c', label: '红', color: '#FF5555'},
  {id: 'd', label: '粉', color: '#FF55FF'},
  {id: 'e', label: '黄', color: '#FFFF55'},
  {id: 'f', label: '白', color: '#FFFFFF'},
];

(function() {
  for (var i = 0; i < mcColors.length; i++) {
    mcColorMap[mcColors[i].id] = mcColors[i].color;
  }
})();

// ── 预览动画 ──

var mqPreviewTimer = null;
var mqAnimTimer = null;
var mqAnimRows = [];
var mqAnimFrame = 0;

function RmarqueeHTML() {
  setTimeout(function() {
    mqRefreshPreview();
    ['mq-text','mq-tchar','mq-fchar','mq-width'].forEach(function(id) {
      var el = document.getElementById(id);
      if (el) el.addEventListener('input', mqDebouncePreview);
    });
  }, 100);
  return '<div class="card">' +
    '<label>文字内容</label>' +
    '<input id="mq-text" type="text" value="HELLO" placeholder="输入要显示的文字" maxlength="30">' +

    '<div class="row">' +
      '<div><label>文字像素</label><input id="mq-tchar" type="text" value="■" maxlength="1" style="text-align:center;font-size:18px"></div>' +
      '<div style="flex:1.5"><label>背景填充</label><input id="mq-fchar" type="text" value=" " placeholder="任意长度" style="text-align:center;font-size:14px"></div>' +
      '<div style="flex:0.7"><label>宽度(列)</label><input id="mq-width" type="number" value="40" min="10" max="80"></div>' +
    '</div>' +

    '<label>颜色</label>' +
    '<div id="mq-colors" style="display:flex;gap:6px;flex-wrap:wrap">' +
      mcColors.map(function(c) {
        var sel = c.id === 'a' ? ' data-selected="true" style="outline:3px solid var(--phx-primary);outline-offset:2px"' : '';
        return '<span class="mq-cb" data-color="' + c.id + '"' + sel + ' onclick="pickMarqueeColor(this);mqDebouncePreview()" style="display:inline-block;width:32px;height:32px;border-radius:8px;background:' + c.color + ';cursor:pointer;border:2px solid rgba(0,0,0,.15)"></span>';
      }).join('') +
    '</div>' +

    '<label>滚动速度 <span id="mq-speed-lbl" style="color:var(--phx-primary)">200ms</span></label>' +
    '<input id="mq-speed" type="range" min="30" max="500" value="200" step="10" oninput="document.getElementById(\'mq-speed-lbl\').textContent=this.value+\'ms\';mqDebouncePreview()" style="width:100%">' +

    '<div class="row" style="margin-top:10px;align-items:center">' +
      '<label class="lbl-cb" style="margin:0;flex:1"><input type="checkbox" id="mq-rainbow" onchange="toggleMarqueeRainbow();mqDebouncePreview()"><span class="tgl"></span>彩虹渐变</label>' +
      '<label class="lbl-cb" style="margin:0;flex:1"><input type="checkbox" id="mq-right" onchange="mqDebouncePreview()"><span class="tgl"></span>向右滚动</label>' +
    '</div>' +

    '<div id="mq-preview" style="margin-top:10px;padding:12px;border-radius:10px;background:#0D1B1A;font-family:monospace;font-size:7px;line-height:1.5;overflow:hidden;min-height:76px;white-space:pre"></div>' +

    '<div id="mq-status" style="display:none;margin-top:10px;padding:10px 14px;border-radius:12px;background:var(--phx-success-bg);color:var(--phx-success);font-size:12px;font-weight:700;text-align:center"><span class="spin"></span> 滚动中...</div>' +

    '<div class="row" style="margin-top:12px">' +
      '<button class="btn" id="mq-go" onclick="doMarqueeStart()">启动滚动</button>' +
      '<button class="btn-d gone" id="mq-stop" onclick="doMarqueeStop()">停止</button>' +
    '</div>' +
  '</div>';
}

function mqDebouncePreview() {
  if (mqPreviewTimer) clearTimeout(mqPreviewTimer);
  mqPreviewTimer = setTimeout(mqRefreshPreview, 200);
}

function mqRefreshPreview() {
  var text = document.getElementById('mq-text');
  var tc = document.getElementById('mq-tchar');
  var fc = document.getElementById('mq-fchar');
  if (!text || !tc || !fc) return;
  var t = text.value.trim();
  if (!t) {
    if (mqAnimTimer) { clearInterval(mqAnimTimer); mqAnimTimer = null; }
    document.getElementById('mq-preview').innerHTML = '<span style="color:#64748b">输入文字预览</span>';
    return;
  }
  fetch('/api/marquee/preview?text=' + encodeURIComponent(t) + '&text_char=' + encodeURIComponent(tc.value || '■') + '&fill_char=' + encodeURIComponent(fc.value || ' '))
    .then(function(r) { return r.json(); })
    .then(function(data) {
      if (data.ok && data.rows) {
        mqAnimRows = data.rows;
        mqAnimFrame = 0;
        if (mqAnimTimer) { clearInterval(mqAnimTimer); }
        mqRenderPreview();
        var speed = parseInt(document.getElementById('mq-speed').value) || 200;
        mqAnimTimer = setInterval(mqRenderPreview, speed);
      }
    });
}

function mqRenderPreview() {
  if (!mqAnimRows.length) return;
  var el = document.getElementById('mq-preview');
  if (!el) return;
  var canvasWidth = mqAnimRows[0].length;
  if (canvasWidth <= 0) return;
  var isRainbow = document.getElementById('mq-rainbow').checked;
  var color = getMarqueeColor();
  var width = parseInt(document.getElementById('mq-width').value) || 40;
  var right = document.getElementById('mq-right').checked;

  var fc = document.getElementById('mq-fchar').value || ' ';
  // 重复填充字符到 width 列
  var pad = '';
  while (pad.length < width) pad += fc;
  pad = pad.substr(0, width);

  var maxFrame = canvasWidth + width;
  var pos = mqAnimFrame % maxFrame;
  if (right) pos = (maxFrame - mqAnimFrame % maxFrame) % maxFrame;
  mqAnimFrame++;

  var html = '';
  for (var r = 0; r < mqAnimRows.length; r++) {
    var line = pad + mqAnimRows[r] + pad;
    var windowed = line.substr(pos, width);
    while (windowed.length < width) windowed += fc;
    windowed = windowed.substr(0, width);

    if (isRainbow) {
      for (var ci = 0; ci < windowed.length; ci++) {
        var ch = windowed[ci];
        var ri = (ci + mqAnimFrame) % rainbowColorIds.length;
        var rc = mcColorMap[rainbowColorIds[ri]] || '#55FF55';
        if (ch === ' ') {
          html += '<span style="color:' + rc + '">&nbsp;</span>';
        } else {
          html += '<span style="color:' + rc + '">' + escHtml(ch) + '</span>';
        }
      }
    } else {
      var c = mcColorMap[color] || '#55FF55';
      html += '<span style="color:' + c + '">';
      for (var ci2 = 0; ci2 < windowed.length; ci2++) {
        var ch2 = windowed[ci2];
        html += ch2 === ' ' ? '&nbsp;' : escHtml(ch2);
      }
      html += '</span>';
    }
    if (r < mqAnimRows.length - 1) html += '<br>';
  }
  el.innerHTML = html;
}

function pickMarqueeColor(el) {
  document.querySelectorAll('.mq-cb').forEach(function(x) {
    x.style.outline = 'none';
    x.removeAttribute('data-selected');
  });
  el.style.outline = '3px solid var(--phx-primary)';
  el.style.outlineOffset = '2px';
  el.setAttribute('data-selected', 'true');
}

function toggleMarqueeRainbow() {
  var rb = document.getElementById('mq-rainbow').checked;
  document.querySelectorAll('.mq-cb').forEach(function(x) {
    x.style.opacity = rb ? '0.4' : '1';
    x.style.pointerEvents = rb ? 'none' : 'auto';
  });
}

function getMarqueeColor() {
  var sel = document.querySelector('.mq-cb[data-selected="true"]');
  return sel ? sel.getAttribute('data-color') : 'a';
}

async function doMarqueeStart() {
  var text = document.getElementById('mq-text').value.trim();
  if (!text) { T('请输入文字内容', 'e'); return; }

  var st = await A('GET', '/api/bot/status');
  if (!st.ok || !st.connected) {
    T('未连接到服务器，请先连接机器人', 'e');
    return;
  }

  var config = {
    text: text,
    text_char: document.getElementById('mq-tchar').value || '■',
    fill_char: document.getElementById('mq-fchar').value || ' ',
    width: parseInt(document.getElementById('mq-width').value) || 40,
    speed: parseInt(document.getElementById('mq-speed').value) || 200,
    color: document.getElementById('mq-rainbow').checked ? 'f' : getMarqueeColor(),
    direction: document.getElementById('mq-right').checked ? 'right' : 'left',
    rainbow: document.getElementById('mq-rainbow').checked,
  };

  var r = await A('POST', '/api/marquee/start', config);
  if (r.ok) {
    T('滚动字幕已启动', 'o');
    document.getElementById('mq-go').classList.add('gone');
    document.getElementById('mq-stop').classList.remove('gone');
    document.getElementById('mq-status').style.display = 'block';
  } else {
    T('启动失败: ' + (r.error || '未知错误'), 'e');
  }
}

async function doMarqueeStop() {
  var r = await A('POST', '/api/marquee/stop', {});
  if (r.ok) {
    T('已停止', 'o');
    document.getElementById('mq-go').classList.remove('gone');
    document.getElementById('mq-stop').classList.add('gone');
    document.getElementById('mq-status').style.display = 'none';
  } else {
    T('停止失败: ' + (r.error || '未知错误'), 'e');
  }
}

function restoreMarqueeState() {
  A('GET', '/api/marquee/status').then(function(r) {
    if (r.ok && r.running) {
      var go = document.getElementById('mq-go');
      var stop = document.getElementById('mq-stop');
      var st = document.getElementById('mq-status');
      if (go) go.classList.add('gone');
      if (stop) stop.classList.remove('gone');
      if (st) st.style.display = 'block';
    }
  });
}

// ═══════════════════════════════════════
// 调试面板 — 坐标 / 状态
// ═══════════════════════════════════════

var dbgPollTimer = null;

function RdebugHTML() {
  setTimeout(function() {
    dbgPollStatus();
    if (dbgPollTimer) clearInterval(dbgPollTimer);
    dbgPollTimer = setInterval(dbgPollStatus, 1000);
  }, 100);
  return '<div class="card">' +
    '<div style="font-size:14px;font-weight:700;margin-bottom:10px">🔧 调试面板</div>' +

    // 状态信息
    '<div id="dbg-status" style="padding:10px;border-radius:10px;background:var(--phx-bg);border:1px solid var(--phx-border-light);font-size:12px;font-family:monospace;line-height:1.8;margin-bottom:10px">' +
      '加载中...' +
    '</div>' +

    // 调试日志
    '<label style="margin-top:10px">调试日志</label>' +
    '<div id="dbg-log" style="margin-top:4px;padding:8px;border-radius:8px;background:#0D1B1A;color:#a8e6cf;font-size:10px;font-family:monospace;line-height:1.5;max-height:300px;overflow-y:auto;white-space:pre-wrap;word-break:break-all"></div>' +
    '<div class="row" style="margin-top:6px;gap:6px">' +
      '<button class="btn" onclick="dbgRefreshLog()" style="flex:1;margin:0;font-size:11px">刷新日志</button>' +
      '<button class="btn-d" onclick="document.getElementById(\'dbg-log\').textContent=\'\'" style="flex:1;margin:0;font-size:11px">清空</button>' +
    '</div>' +
  '</div>';
}

function dbgCleanup() {
  if (dbgPollTimer) { clearInterval(dbgPollTimer); dbgPollTimer = null; }
}

function dbgPollStatus() {
  A('GET', '/api/fly/status').then(function(r) {
    var el = document.getElementById('dbg-status');
    if (!el) return;
    if (r.ok) {
      el.innerHTML =
        '<div style="display:flex;justify-content:space-between">' +
          '<span>坐标: <b style="color:var(--phx-primary)">' + r.x.toFixed(1) + ' / ' + r.y.toFixed(1) + ' / ' + r.z.toFixed(1) + '</b></span>' +
          '<span>状态: ' + (r.isFlying ? '🟦 飞行' : '🟫 地面') + '</span>' +
        '</div>' +
        '<div style="display:flex;justify-content:space-between;margin-top:4px">' +
          '<span>视角: <b>' + Math.round(r.pitch) + '° / ' + Math.round(r.yaw) + '°</b></span>' +
          '<span>区块: ' + (r.world_chunks || 0) + '个</span>' +
        '</div>' +
        '<div style="display:flex;justify-content:space-between;margin-top:4px">' +
          '<span>维度: ' + (r.dimension === 0 ? '主世界' : r.dimension === 1 ? '下界' : '末地') + '</span>' +
          '<span>冲刺: ' + (r.sprinting ? '🟢' : '⚪') + '</span>' +
        '</div>';
    } else {
      el.innerHTML = '<span style="color:var(--phx-error)">未连接</span>';
    }
  });
}











async function dbgRefreshLog() {
  var el = document.getElementById('dbg-log');
  if (!el) return;
  el.textContent = '加载中...';
  try {
    var r = await fetch('/api/pathfinder/log');
    var text = await r.text();
    el.textContent = text || '(无日志)';
    el.scrollTop = el.scrollHeight;
  } catch(e) {
    el.textContent = '(获取日志失败)';
  }
}