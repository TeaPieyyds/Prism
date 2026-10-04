// ═══════════════════════════════════════
// 相机工具 — 四步向导
// ═══════════════════════════════════════

// SVG 图标（全部带显式 width/height）
var CI = {
  camera: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z"/><circle cx="12" cy="13" r="4"/></svg>',
  user: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>',
  record: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="4" fill="currentColor"/></svg>',
  stop: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="3"/></svg>',
  path: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>',
  code: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/></svg>',
  play: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polygon points="5 3 19 12 5 21 5 3"/></svg>',
  download: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>',
  copy: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>',
  edit: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/></svg>',
  refresh: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>',
  check: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  delete: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
  dot: '<svg width="16" height="16" viewBox="0 0 16 16"><circle cx="8" cy="8" r="6" fill="currentColor"/></svg>',
  pause: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><rect x="6" y="4" width="4" height="16"/><rect x="14" y="4" width="4" height="16"/></svg>',
  target: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/></svg>',
  plus: '<svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
};

// ========== 状态 ==========

var camState = {
  step: 1,
  player: '',
  recording: false,
  started: false,
  idleCount: 0,
  shots: [],
  statusTimer: null,
};

// ========== 入口 ==========

function RcameraHTML() {
  setTimeout(function() {
    renderCamStep(camState.step);
  }, 50);
  return '<div id="cam-wrap" style="width:100%"><div id="cam-body"></div></div>';
}

function renderCamStep(step) {
  camState.step = step;
  var body = document.getElementById('cam-body');
  if (!body) return;
  switch (step) {
    case 1: renderCamStep1(); break;
    case 2: renderCamStep2(); break;
    case 3: renderCamStep3(); break;
    case 4: renderCamStep4(); break;
  }
}

// ========== Step 1: 选择玩家 ==========

function renderCamStep1() {
  var body = document.getElementById('cam-body');
  if (!body) return;

  // 获取在线玩家列表
  A('GET', '/api/camera/status').then(function(r) {
    if (!r || !r.ok) return;
    camState.shots = r.shots || [];
  });

  var html = '<div class="wz-section-title">' +
    CI.user + '步骤 1/4 · 选择玩家</div>' +
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin:6px 0 12px">选择要录制镜头的玩家</div>' +
    '<div id="cam-player-list" style="display:flex;flex-direction:column;gap:6px"></div>' +
    '<div id="cam-player-info" class="gone" style="margin-top:12px;padding:12px;border-radius:12px;background:var(--phx-surface);border:1px solid var(--phx-border)">' +
      '<div style="font-size:14px;font-weight:700;margin-bottom:6px">' +
        '<span id="cam-pi-name"></span>' +
      '</div>' +
      '<div style="font-size:12px;color:var(--phx-text-secondary);display:grid;grid-template-columns:1fr 1fr;gap:4px">' +
        '<div>坐标: <span id="cam-pi-pos" style="color:var(--phx-text)">-</span></div>' +
        '<div>仰角: <span id="cam-pi-rot" style="color:var(--phx-text)">-</span></div>' +
        '<div>维度: <span id="cam-pi-dim" style="color:var(--phx-text)">-</span></div>' +
        '<div>OP: <span id="cam-pi-op" style="color:var(--phx-text)">-</span></div>' +
      '</div>' +
    '</div>' +
    '<div class="row" style="margin-top:20px">' +
    '<button class="btn" id="cam-step1-next" onclick="camGoStep2()" disabled>' + ICONS.arrowRight + ' 下一步</button></div>';

  body.innerHTML = html;

  // 加载玩家列表
  refreshPlayerList();
}

function refreshPlayerList() {
  A('GET', '/api/players/list').then(function(r) {
    if (!r || !r.ok) {
      setTimeout(refreshPlayerList, 2000);
      return;
    }
    var players = r.players || [];
    // 按名称排序，保证顺序稳定
    players.sort(function(a, b) { return a.name.localeCompare(b.name); });
    renderPlayerList(players);
    setTimeout(refreshPlayerList, 2000);
  });
}

function renderPlayerList(players) {
  var list = document.getElementById('cam-player-list');
  if (!list) return;

  if (!players || players.length === 0) {
    list.innerHTML = '<div style="padding:24px;text-align:center;color:var(--phx-text-secondary);font-size:13px">' +
      CI.user + ' 没有在线玩家</div>';
    return;
  }

  var html = '';
  for (var i = 0; i < players.length; i++) {
    var p = players[i];
    var sel = p.name === camState.player ? ' style="outline:2px solid var(--phx-primary);outline-offset:2px"' : '';
    var posStr = p.pos_x ? Math.round(p.pos_x*10)/10 + ', ' + Math.round(p.pos_y*10)/10 + ', ' + Math.round(p.pos_z*10)/10 : '坐标获取中';
    html += '<div class="pl-item" data-name="' + escHtml(p.name) + '"' + sel + ' onclick="camSelectPlayer(\'' + escHtml(p.name) + '\')">' +
      '<div style="display:flex;align-items:center;gap:10px;padding:12px 14px;border-radius:12px;background:var(--phx-surface);border:1px solid var(--phx-border);cursor:pointer">' +
      CI.user +
      '<div><div style="font-weight:600;font-size:14px">' + escHtml(p.name) + '</div>' +
      '<div style="font-size:11px;color:var(--phx-text-secondary)">' + posStr + '</div></div></div></div>';
  }
  list.innerHTML = html;

  if (camState.player) {
    updatePlayerInfo(camState.player);
  }
}

function camSelectPlayer(name) {
  camState.player = name;
  // 高亮选中
  document.querySelectorAll('#cam-player-list .pl-item').forEach(function(el) {
    el.style.outline = 'none';
    if (el.getAttribute('data-name') === name) {
      el.style.outline = '2px solid var(--phx-primary)';
      el.style.outlineOffset = '2px';
    }
  });
  document.getElementById('cam-step1-next').disabled = false;
  updatePlayerInfo(name);
}

function updatePlayerInfo(name) {
  var info = document.getElementById('cam-player-info');
  if (!info) return;
  info.classList.remove('gone');
  document.getElementById('cam-pi-name').textContent = name;

  // 轮询显示玩家实时信息
  function pollPos() {
    if (!camState.player || camState.step !== 1) return;
    A('GET', '/api/players/list').then(function(r) {
      if (r && r.ok && r.players) {
        for (var i = 0; i < r.players.length; i++) {
          if (r.players[i].name === camState.player) {
            var p = r.players[i];
            document.getElementById('cam-pi-pos').textContent = Math.round(p.pos_x*10)/10 + ', ' + Math.round(p.pos_y*10)/10 + ', ' + Math.round(p.pos_z*10)/10;
            document.getElementById('cam-pi-rot').textContent = 'Pitch ' + p.pitch + ' Yaw ' + p.yaw;
            document.getElementById('cam-pi-dim').textContent = p.dimension === 0 ? '主世界' : p.dimension === 1 ? '下界' : p.dimension === 2 ? '末地' : 'dm' + p.dimension;
            document.getElementById('cam-pi-op').textContent = p.is_op ? '是' : '否';
            break;
          }
        }
      }
    });
    setTimeout(pollPos, 1000);
  }
  pollPos();
}

function camGoStep2() {
  if (!camState.player) { T('请先选择玩家', 'w'); return; }
  renderCamStep(2);
}

// ========== Step 2: 录制镜头 ==========

function renderCamStep2() {
  var body = document.getElementById('cam-body');
  if (!body) return;

  body.innerHTML =
    '<div class="wz-section-title">' + CI.record + '步骤 2/4 · 录制镜头</div>' +
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin:6px 0 12px">录制对象: <strong style="color:var(--phx-text)">' + escHtml(camState.player) + '</strong></div>' +

    '<div id="cam-rec-status" style="padding:16px;border-radius:12px;background:var(--phx-surface);border:1px solid var(--phx-border);text-align:center;font-size:14px;font-weight:600">' +
      CI.dot + ' 准备就绪，点击开始录制</div>' +

    '<div id="cam-rec-pos" class="gone" style="margin-top:10px;padding:12px;border-radius:12px;background:var(--phx-surface-muted);font-size:12px;color:var(--phx-text-secondary)">' +
      '<div style="display:grid;grid-template-columns:1fr 1fr;gap:4px">' +
        '<div>坐标: <span id="cam-rec-pos-v" style="color:var(--phx-text);font-family:monospace">-</span></div>' +
        '<div>仰角: <span id="cam-rec-rot-v" style="color:var(--phx-text);font-family:monospace">-</span></div>' +
        '<div>采样: <span id="cam-rec-samples" style="color:var(--phx-text);font-family:monospace">0</span></div>' +
        '<div>时长: <span id="cam-rec-time" style="color:var(--phx-text);font-family:monospace">0.0s</span></div>' +
      '</div>' +
      '<div id="cam-rec-progress" style="margin-top:8px;height:4px;background:var(--phx-border);border-radius:2px;overflow:hidden">' +
        '<div id="cam-rec-bar" style="height:100%;width:0%;background:var(--phx-primary);border-radius:2px;transition:width .3s"></div>' +
      '</div>' +
    '</div>' +

    '<div class="row" style="margin-top:16px;justify-content:center;gap:12px">' +
      '<button class="btn" id="cam-rec-start" onclick="camStartRec()">' + CI.record + ' 开始录制</button>' +
      '<button class="btn-d gone" id="cam-rec-stop" onclick="camStopRec()">' + CI.stop + ' 停止</button>' +
    '</div>' +

    '<div class="row" style="margin-top:20px;justify-content:space-between">' +
      '<button class="btn-s" onclick="renderCamStep(1)">' + ICONS.arrowLeft + ' 上一步</button>' +
      '<button class="btn" id="cam-step2-next" onclick="camGoStep3()" disabled>' + ICONS.arrowRight + ' 下一步</button></div>';
}

function camStartRec() {
  A('POST', '/api/camera/start', { player: camState.player }).then(function(r) {
    if (!r || !r.ok) {
      T('开始录制失败: ' + (r && r.error || '未知错误'), 'e');
      return;
    }
    camState.recording = true;
    camState.started = false;
    document.getElementById('cam-rec-start').classList.add('gone');
    document.getElementById('cam-rec-stop').classList.remove('gone');
    document.getElementById('cam-rec-pos').classList.remove('gone');
    document.getElementById('cam-step2-next').disabled = true;
    updateCamRecStatus('等待移动...', 'var(--phx-warning)');
    startCamPolling();
  });
}

function camStopRec() {
  A('POST', '/api/camera/stop', {}).then(function(r) {
    if (!r || !r.ok) {
      T('停止录制失败: ' + (r && r.error || '未知错误'), 'e');
      return;
    }
    camState.recording = false;
    camState.started = false;
    stopCamPolling();
    document.getElementById('cam-rec-start').classList.remove('gone');
    document.getElementById('cam-rec-stop').classList.add('gone');
    document.getElementById('cam-step2-next').disabled = false;
    updateCamRecStatus('录制完成', 'var(--phx-success)');
    T('镜头录制完成!', 'o');
  });
}

var camPollTimer = null;

function startCamPolling() {
  function poll() {
    if (!camState.recording) return;
    A('GET', '/api/camera/status').then(function(r) {
      if (!r) return;
      if (r.running) {
        if (r.started) {
          updateCamRecStatus(r.idle_count > 3 ? '已暂停，点击停止按钮结束录制' : '录制中...', 'var(--phx-success)');
        } else {
          updateCamRecStatus('等待移动...', 'var(--phx-warning)');
        }
        // 更新实时数据
        updateCamRecLiveData(r);
      }
    });
    camPollTimer = setTimeout(poll, 200);
  }
  poll();
}

function stopCamPolling() {
  if (camPollTimer) {
    clearTimeout(camPollTimer);
    camPollTimer = null;
  }
}

function updateCamRecStatus(text, color) {
  var el = document.getElementById('cam-rec-status');
  if (!el) return;
  el.innerHTML = '<span style="color:' + color + '">' + CI.dot + '</span> ' + text;
}

function updateCamRecLiveData(r) {
  var posEl = document.getElementById('cam-rec-pos-v');
  var rotEl = document.getElementById('cam-rec-rot-v');
  var samplesEl = document.getElementById('cam-rec-samples');
  var timeEl = document.getElementById('cam-rec-time');
  var barEl = document.getElementById('cam-rec-bar');
  if (!posEl) return;

  if (r.samples > 0) {
    posEl.textContent = '...';
    rotEl.textContent = '...';
    samplesEl.textContent = r.samples;
    // 时长从后端获取
  }
}

function camGoStep3() {
  renderCamStep(3);
}

// ========== Step 3: 路径预览 ==========

function renderCamStep3() {
  var body = document.getElementById('cam-body');
  if (!body) return;

  // 获取所有镜头
  A('GET', '/api/camera/shots').then(function(r) {
    if (!r || !r.ok) return;
    camState.shots = r.shots || [];
    renderCamStep3Body();
  });

  body.innerHTML =
    '<div class="wz-section-title">' + CI.path + '步骤 3/4 · 路径预览</div>' +
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin:6px 0 12px">' +
      '每个镜头可以调整围绕点、坐标、时长和缓动类型</div>' +
    '<div id="cam-shot-list"></div>' +
    '<div class="row" style="margin-top:20px;justify-content:space-between">' +
      '<button class="btn-s" onclick="renderCamStep(2)">' + ICONS.arrowLeft + ' 上一步</button>' +
      '<button class="btn" onclick="camTestAllShots()" style="flex:1;margin:0 8px">' + CI.play + ' 完整测试</button>' +
      '<button class="btn" id="cam-step3-next" onclick="camGoStep4()">' + ICONS.arrowRight + ' 下一步</button></div>';
}

function renderCamStep3Body() {
  var list = document.getElementById('cam-shot-list');
  if (!list) return;

  if (!camState.shots || camState.shots.length === 0) {
    list.innerHTML = '<div style="padding:24px;text-align:center;color:var(--phx-text-secondary)">' +
      CI.camera + ' 没有镜头，请先录制</div>';
    return;
  }

  var html = '';
  for (var i = 0; i < camState.shots.length; i++) {
    var s = camState.shots[i];
    var dur = (s.duration / 1000).toFixed(1);
    html += '<div class="card" style="margin-bottom:10px">' +
      '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:8px">' +
        '<div style="display:flex;align-items:center;gap:8px">' +
          '<span style="font-weight:700;font-size:14px">#' + s.id + '</span>' +
          '<span style="font-size:12px;color:var(--phx-text-secondary)">' + dur + 's</span>' +
          '<span style="font-size:11px;padding:2px 8px;border-radius:4px;background:var(--phx-primary-bg);color:var(--phx-primary)">' + s.easing + '</span>' +
        '</div>' +
        '<div style="display:flex;gap:6px">' +
          '<button class="btn-xs" onclick="camTestShot(' + s.id + ')">' + CI.play + '</button>' +
          '<button class="btn-xs" onclick="camDeleteShot(' + s.id + ')">' + CI.delete + '</button>' +
        '</div>' +
      '</div>' +

      // 围绕点编辑（无围绕点时置灰显示）
      '<label style="font-size:11px;color:var(--phx-text-secondary)">围绕点' + (s.pivot ? '' : ' <span style="color:var(--phx-warning);font-weight:normal">(无焦点)</span>') + '</label>' +
      '<div class="row" style="gap:4px;margin-bottom:6px">' +
        '<input class="cam-pivot' + (s.pivot ? '' : ' cam-pivot-disabled') + '" data-shot="' + s.id + '" data-axis="x" type="number" step="0.1" value="' + (s.pivot ? s.pivot.x : '') + '" style="flex:1;padding:6px 8px;font-size:11px' + (s.pivot ? '' : ';opacity:0.5') + '" placeholder="' + (s.pivot ? 'X' : '无围绕点') + '">' +
        '<input class="cam-pivot' + (s.pivot ? '' : ' cam-pivot-disabled') + '" data-shot="' + s.id + '" data-axis="y" type="number" step="0.1" value="' + (s.pivot ? s.pivot.y : '') + '" style="flex:1;padding:6px 8px;font-size:11px' + (s.pivot ? '' : ';opacity:0.5') + '" placeholder="' + (s.pivot ? 'Y' : '无围绕点') + '">' +
        '<input class="cam-pivot' + (s.pivot ? '' : ' cam-pivot-disabled') + '" data-shot="' + s.id + '" data-axis="z" type="number" step="0.1" value="' + (s.pivot ? s.pivot.z : '') + '" style="flex:1;padding:6px 8px;font-size:11px' + (s.pivot ? '' : ';opacity:0.5') + '" placeholder="' + (s.pivot ? 'Z' : '无围绕点') + '">' +
      '</div>' +

      // 起点编辑
      '<label style="font-size:11px;color:var(--phx-text-secondary)">起点</label>' +
      '<div class="row" style="gap:4px;margin-bottom:6px">' +
        '<input class="cam-start" data-shot="' + s.id + '" data-axis="x" type="number" step="0.1" value="' + s.start.x + '" style="flex:1;padding:6px 8px;font-size:11px" placeholder="X">' +
        '<input class="cam-start" data-shot="' + s.id + '" data-axis="y" type="number" step="0.1" value="' + s.start.y + '" style="flex:1;padding:6px 8px;font-size:11px" placeholder="Y">' +
        '<input class="cam-start" data-shot="' + s.id + '" data-axis="z" type="number" step="0.1" value="' + s.start.z + '" style="flex:1;padding:6px 8px;font-size:11px" placeholder="Z">' +
      '</div>' +

      // 终点编辑
      '<label style="font-size:11px;color:var(--phx-text-secondary)">终点</label>' +
      '<div class="row" style="gap:4px;margin-bottom:6px">' +
        '<input class="cam-end" data-shot="' + s.id + '" data-axis="x" type="number" step="0.1" value="' + s.end.x + '" style="flex:1;padding:6px 8px;font-size:11px" placeholder="X">' +
        '<input class="cam-end" data-shot="' + s.id + '" data-axis="y" type="number" step="0.1" value="' + s.end.y + '" style="flex:1;padding:6px 8px;font-size:11px" placeholder="Y">' +
        '<input class="cam-end" data-shot="' + s.id + '" data-axis="z" type="number" step="0.1" value="' + s.end.z + '" style="flex:1;padding:6px 8px;font-size:11px" placeholder="Z">' +
      '</div>' +

      // 时长和缓动
      '<div class="row" style="gap:8px;align-items:center">' +
        '<div style="flex:1"><label style="font-size:11px;color:var(--phx-text-secondary)">时长 (ms)</label>' +
        '<input class="cam-duration" data-shot="' + s.id + '" type="number" step="100" value="' + s.duration + '" style="width:100%;padding:6px 8px;font-size:11px"></div>' +
        '<div style="flex:1"><label style="font-size:11px;color:var(--phx-text-secondary)">缓动</label>' +
        '<select class="cam-easing" data-shot="' + s.id + '" style="width:100%;padding:6px 8px;font-size:11px">' +
          '<option value="linear"' + (s.easing === 'linear' ? ' selected' : '') + '>linear</option>' +
          '<option value="out_sine"' + (s.easing === 'out_sine' ? ' selected' : '') + '>out_sine</option>' +
          '<option value="in_sine"' + (s.easing === 'in_sine' ? ' selected' : '') + '>in_sine</option>' +
          '<option value="in_out_sine"' + (s.easing === 'in_out_sine' ? ' selected' : '') + '>in_out_sine</option>' +
          '<option value="out_cubic"' + (s.easing === 'out_cubic' ? ' selected' : '') + '>out_cubic</option>' +
          '<option value="in_out_cubic"' + (s.easing === 'in_out_cubic' ? ' selected' : '') + '>in_out_cubic</option>' +
        '</select></div>' +
      '</div>' +
      '<div style="margin-top:6px;display:flex;gap:6px">' +
        '<button class="btn-xs" onclick="camApplyShotEdits(' + s.id + ')" style="flex:1">' + CI.check + ' 应用修改</button>' +
        '<button class="btn-xs" onclick="camRecaptureShot(' + s.id + ')" style="flex:1">' + CI.refresh + ' 重新录制</button>' +
      '</div>' +
    '</div>';
  }

  list.innerHTML = html;

  // 绑定编辑事件（自动保存）
  document.querySelectorAll('.cam-pivot, .cam-start, .cam-end, .cam-duration, .cam-easing').forEach(function(el) {
    el.addEventListener('change', function() {
      // 自动保存标记
    });
  });
}

function camApplyShotEdits(shotId) {
  var updates = {};

  // 读取围绕点（仅当有围绕点时发送）
  var pivotX = document.querySelector('.cam-pivot[data-shot="' + shotId + '"][data-axis="x"]');
  var pivotY = document.querySelector('.cam-pivot[data-shot="' + shotId + '"][data-axis="y"]');
  var pivotZ = document.querySelector('.cam-pivot[data-shot="' + shotId + '"][data-axis="z"]');
  // 检查是否是禁用状态（原无围绕点）
  if (pivotX && pivotY && pivotZ && !pivotX.classList.contains('cam-pivot-disabled')) {
    var px = parseFloat(pivotX.value);
    var py = parseFloat(pivotY.value);
    var pz = parseFloat(pivotZ.value);
    // 只有用户实际输入了值才发送 pivot
    if (pivotX.value !== '' && pivotY.value !== '' && pivotZ.value !== '') {
      updates.pivot = {
        x: px || 0,
        y: py || 0,
        z: pz || 0,
      };
    }
  }

  // 读取起点
  var startX = document.querySelector('.cam-start[data-shot="' + shotId + '"][data-axis="x"]');
  var startY = document.querySelector('.cam-start[data-shot="' + shotId + '"][data-axis="y"]');
  var startZ = document.querySelector('.cam-start[data-shot="' + shotId + '"][data-axis="z"]');
  if (startX && startY && startZ) {
    updates.start = {
      x: parseFloat(startX.value) || 0,
      y: parseFloat(startY.value) || 0,
      z: parseFloat(startZ.value) || 0,
    };
  }

  // 读取终点
  var endX = document.querySelector('.cam-end[data-shot="' + shotId + '"][data-axis="x"]');
  var endY = document.querySelector('.cam-end[data-shot="' + shotId + '"][data-axis="y"]');
  var endZ = document.querySelector('.cam-end[data-shot="' + shotId + '"][data-axis="z"]');
  if (endX && endY && endZ) {
    updates.end = {
      x: parseFloat(endX.value) || 0,
      y: parseFloat(endY.value) || 0,
      z: parseFloat(endZ.value) || 0,
    };
  }

  // 读取时长和缓动
  var dur = document.querySelector('.cam-duration[data-shot="' + shotId + '"]');
  var easing = document.querySelector('.cam-easing[data-shot="' + shotId + '"]');
  if (dur) updates.duration = parseInt(dur.value) || 0;
  if (easing) updates.easing = easing.value;

  A('POST', '/api/camera/shot/update', { shot_id: shotId, updates: updates }).then(function(r) {
    if (r && r.ok) {
      T('镜头 #' + shotId + ' 已更新', 'o');
    } else {
      T('更新失败: ' + (r && r.error || '未知错误'), 'e');
    }
  });
}

function camTestShot(shotId) {
  A('POST', '/api/camera/test', { shot_id: shotId, player: camState.player }).then(function(r) {
    if (r && r.ok) {
      T('测试回放已发送!', 'o');
    } else {
      T('测试失败: ' + (r && r.error || '未知错误'), 'e');
    }
  });
}

function camDeleteShot(shotId) {
  showConfirm({title:'确认删除',message:'确定删除镜头 #' + shotId + ' 吗?',onConfirm:function(){
    A('POST', '/api/camera/shot/delete', { shot_id: shotId }).then(function(r) {
      if (r && r.ok) {
        T('镜头已删除', 'o');
        renderCamStep3();
      } else {
        T('删除失败: ' + (r && r.error || '未知错误'), 'e');
      }
    });
  }});
}

function camRecaptureShot(shotId) {
  // 回到 Step 2 重新录制
  renderCamStep(2);
}

function camGoStep4() {
  renderCamStep(4);
}

// ========== Step 4: 生成指令 ==========

function renderCamStep4() {
  var body = document.getElementById('cam-body');
  if (!body) return;

  body.innerHTML =
    '<div class="wz-section-title">' + CI.code + '步骤 4/4 · 生成指令</div>' +
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin:6px 0 12px">' +
      '共 <strong id="cam-shot-count" style="color:var(--phx-text)">0</strong> 个镜头</div>' +

    '<div id="cam-output-wrap" style="position:relative">' +
      '<pre id="cam-output" style="padding:14px;border-radius:12px;background:var(--phx-code-bg, #1a1a2e);color:var(--phx-code, #e0e0e0);font-size:11px;font-family:monospace;line-height:1.6;max-height:320px;overflow:auto;white-space:pre-wrap;word-break:break-all;margin:0"></pre>' +
      '<div id="cam-output-empty" style="padding:40px;text-align:center;color:var(--phx-text-secondary);font-size:13px;display:none">' +
        CI.camera + ' 没有镜头可生成</div>' +
    '</div>' +

    '<div class="row" style="margin-top:12px;gap:8px;flex-wrap:wrap">' +
      '<button class="btn" id="cam-gen-copy" onclick="camCopyOutput()" style="flex:1;min-width:100px">' + CI.copy + ' 复制</button>' +
      '<button class="btn" id="cam-gen-export" onclick="camExportMcfunction()" style="flex:1;min-width:100px">' + CI.download + ' 导出 .mcfunction</button>' +
          '</div>' +

    '<div class="row" style="margin-top:20px;justify-content:space-between">' +
      '<button class="btn-s" onclick="renderCamStep(3)">' + ICONS.arrowLeft + ' 上一步</button>' +
      '<button class="btn-s" onclick="camReset()">' + CI.refresh + ' 重新开始</button></div>';

  // 加载生成结果
  A('POST', '/api/camera/generate', { player: camState.player, format: 'text' }).then(function(r) {
    if (r && r.ok && r.text) {
      document.getElementById('cam-output').textContent = r.text;
      document.getElementById('cam-shot-count').textContent = (r.text.match(/camera_shot/g) || []).length;
    } else {
      document.getElementById('cam-output-empty').style.display = 'block';
      document.getElementById('cam-output').style.display = 'none';
    }
  });
}

function camCopyOutput() {
  var text = document.getElementById('cam-output');
  if (!text || !text.textContent) { T('没有可复制的内容', 'w'); return; }
  // 只复制命令主体，去掉注释和空行
  var lines = text.textContent.split('\n');
  var cmds = [];
  for (var i = 0; i < lines.length; i++) {
    var line = lines[i].trim();
    if (line && line[0] !== '#') {
      cmds.push(line);
    }
  }
  navigator.clipboard.writeText(cmds.join('\n')).then(function() {
    T('已复制到剪贴板', 'o');
  }).catch(function() {
    T('复制失败', 'e');
  });
}

function camExportMcfunction() {
  A('POST', '/api/camera/generate', { player: camState.player, format: 'mcfunction' }).then(function(r) {
    if (r && r.ok && r.path) {
      T('已导出到: ' + r.path, 'o');
    } else {
      T('导出失败: ' + (r && r.error || '未知错误'), 'e');
    }
  });
}

function camTestAllShots() {
  if (!camState.shots || camState.shots.length === 0) {
    T('没有镜头可测试', 'w');
    return;
  }
  A('POST', '/api/camera/test/all', {}).then(function(r) {
    if (r && r.ok) {
      T('所有镜头测试完成!', 'o');
    } else {
      T('测试失败: ' + (r && r.error || '未知错误'), 'e');
    }
  });
}

function camReset() {
  A('POST', '/api/camera/clear', {}).then(function(r) {
    camState.step = 1;
    camState.player = '';
    camState.shots = [];
    stopCamPolling();
    renderCamStep(1);
  });
}

// ========== 工具函数 ==========

function getOnlinePlayers(cb) {
  A('GET', '/api/players/list').then(function(r) {
    if (r && r.ok && r.players) {
      var names = r.players.map(function(p) { return p.name; });
      cb(names);
      return;
    }
    cb([]);
  });
}

function getPlayerInfo(name) {
  // 从 /api/players/list 获取
  return null;
}