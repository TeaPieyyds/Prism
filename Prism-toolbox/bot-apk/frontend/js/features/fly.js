// ═══════════════════════════════════════
// 飞行控制 — 机器人移动控制面板
// ═══════════════════════════════════════

var flyState = {
  flying: false,
  sprinting: false,
  x: 0, y: 0, z: 0,
  pitch: 0, yaw: 0,
  dimension: 0,
  polling: null,
};

// 按方向名跟踪当前按下的方向键，支持多指同时按下
var flyActiveDirs = {};
var flyDragging = false; // 用于画布拖拽锁定

var FLY_ICONS = {
  up: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="18 15 12 9 6 15"/></svg>',
  down: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="6 9 12 15 18 9"/></svg>',
  left: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>',
  right: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg>',
  jump: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/></svg>',
  takeoff: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 14l-5-5-5 5"/><path d="M17 20l-5-5-5 5"/><line x1="12" y1="2" x2="12" y2="9"/></svg>',
  land: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 10l-5 5-5-5"/><path d="M17 4l-5 5-5-5"/><line x1="12" y1="22" x2="12" y2="15"/></svg>',
  sprint: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="13" cy="4" r="2"/><path d="M5 20l5-8 3 3 4-9 4 2"/><path d="M17 3l4 3"/></svg>',
  sprint_active: '<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 4 14 11 14 10 22 20 10 13 10"/></svg>',
};

// ========== 入口 ==========

function RflyHTML() {
  if (flyState.polling) { clearInterval(flyState.polling); flyState.polling = null; }
  flyActiveDirs = {};
  setTimeout(function() {
    flyPollStatus();
    flyInitCanvases();
    flyState.polling = setInterval(flyPollStatus, 500);
  }, 100);

  // 注册清理函数：面板关闭时自动恢复飞行 + 清理轮询
  activeRenderers.push(function() {
    if (flyState.polling) {
      clearInterval(flyState.polling);
      flyState.polling = null;
    }
    // 离开面板时自动恢复飞行
    if (flyState.flying === false) {
      flyCall('/start');
      flyState.flying = true;
    }
  });

  return flyRenderPanel();
}

function flyRenderPanel() {
  var isFlying = flyState.flying;
  return '<div class="card" style="padding:12px 14px">' +
    // 状态栏
    '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px;padding:8px 10px;border-radius:12px;background:var(--phx-bg);border:2px solid var(--phx-border-light)">' +
      '<div style="display:flex;align-items:center;gap:6px">' +
        '<span id="fly-status-dot" style="display:inline-block;width:10px;height:10px;border-radius:50%;background:var(--phx-text-disabled);flex-shrink:0"></span>' +
        '<span id="fly-status-text" style="font-size:12px;font-weight:700;color:var(--phx-text-secondary)">未连接</span>' +
      '</div>' +
      '<span id="fly-pos" style="font-size:11px;font-family:monospace;color:var(--phx-text-secondary)">- / - / -</span>' +
    '</div>' +

    // 视角信息 + 维度
    '<div style="display:flex;gap:6px;margin-bottom:10px">' +
      '<div style="flex:1;display:flex;align-items:center;gap:4px;padding:6px 8px;border-radius:8px;background:var(--phx-bg);border:1px solid var(--phx-border-light);font-size:11px;font-family:monospace;color:var(--phx-text-secondary)">' +
        '<span style="color:var(--phx-primary);font-weight:700">仰</span> <span id="fly-pitch">0</span>' +
      '</div>' +
      '<div style="flex:1;display:flex;align-items:center;gap:4px;padding:6px 8px;border-radius:8px;background:var(--phx-bg);border:1px solid var(--phx-border-light);font-size:11px;font-family:monospace;color:var(--phx-text-secondary)">' +
        '<span style="color:var(--phx-primary);font-weight:700">转</span> <span id="fly-yaw">0</span>' +
      '</div>' +
      '<div style="flex:1;display:flex;align-items:center;gap:4px;padding:6px 8px;border-radius:8px;background:var(--phx-bg);border:1px solid var(--phx-border-light);font-size:11px;font-family:monospace;color:var(--phx-text-secondary)">' +
        '<span style="color:var(--phx-primary);font-weight:700">维</span> <span id="fly-dim">主世界</span>' +
      '</div>' +
    '</div>' +

    // 方向键（十字布局）
    '<div style="margin-bottom:10px">' +
      '<div style="display:flex;justify-content:center;gap:6px;margin-bottom:6px">' +
        flyDirBtn('forward', '前', 'forward') +
      '</div>' +
      '<div style="display:flex;justify-content:center;gap:6px">' +
        flyDirBtn('left', '左', 'left') +
        flyCenterBtn(isFlying ? '下' : '潜行', isFlying) +
        flyDirBtn('right', '右', 'right') +
      '</div>' +
      '<div style="display:flex;justify-content:center;gap:6px;margin-top:6px">' +
        flyDirBtn('backward', '后', 'backward') +
      '</div>' +
    '</div>' +

    // 奔跑按钮（状态指示）
    '<div style="display:flex;justify-content:center;gap:6px;margin-bottom:10px">' +
        '<button id="fly-sprint-btn" class="btn" onpointerdown="flySprintStart(event)" onpointerup="flySprintStop(event)" onpointercancel="flySprintStop(event)"' +
        ' style="flex:1;margin:0;font-size:12px;padding:8px;display:flex;align-items:center;justify-content:center;gap:4px;transition:all .15s">' +
        '<span id="fly-sprint-icon">' + FLY_ICONS.sprint + '</span> <span id="fly-sprint-label">奔跑</span></button>' +
    '</div>' +

    // 操作按钮
    '<div class="row" style="margin-bottom:10px;gap:6px">' +
      '<button class="' + (isFlying ? 'btn-d' : 'btn') + '" id="fly-toggle-btn" onclick="flyToggle()" style="flex:1;margin:0;font-size:13px;padding:10px">' + (isFlying ? FLY_ICONS.land + ' 降落' : FLY_ICONS.takeoff + ' 起飞') + '</button>' +
      '<button class="btn-s" id="fly-jump-btn" onpointerdown="flyJumpStart(event)" onpointerup="flyJumpStop(event)" onpointercancel="flyJumpStop(event)" style="width:auto;margin:0;font-size:13px;padding:10px 14px;flex-shrink:0;touch-action:manipulation">' + FLY_ICONS.jump + (isFlying ? ' 上' : ' 跳') + '</button>' +
    '</div>' +

    // 视角控制：竖直半圆(俯仰) + 圆形转盘(偏航)
    '<div style="display:flex;gap:10px;justify-content:center;align-items:flex-start;padding:10px;border-radius:12px;background:var(--phx-bg);border:2px solid var(--phx-border-light)">' +
      '<div style="text-align:center">' +
        '<input type="range" id="fly-pitch-slider" min="-90" max="90" value="0" style="writing-mode:vertical-lr;width:36px;height:130px;margin:0 auto;display:block;cursor:pointer;accent-color:var(--phx-primary);-webkit-appearance:slider-vertical">' +
        '<div style="font-size:10px;color:var(--phx-text-secondary);margin-top:4px">俯仰 <span id="fly-pitch-val" style="color:var(--phx-primary);font-weight:700">0°</span></div>' +
      '</div>' +
      '<div style="text-align:center">' +
        '<canvas id="fly-yaw-canvas" width="150" height="150" style="touch-action:none;display:block;margin:0 auto"></canvas>' +
        '<div style="font-size:10px;color:var(--phx-text-secondary);margin-top:4px">转向 <span id="fly-yaw-val" style="color:var(--phx-primary);font-weight:700">0°</span></div>' +
      '</div>' +
    '</div>' +
  '</div>';
}

function flyCenterBtn(label, isFlying) {
  var icon = FLY_ICONS.down || '';
  return '<button class="fly-center-btn"' +
    ' onpointerdown="flyCenterPress(event)"' +
    ' onpointerup="flyCenterRelease(event)"' +
    ' onpointercancel="flyCenterRelease(event)"' +
    ' style="width:60px;height:52px;border:none;border-radius:12px;background:var(--phx-bg);border:2px solid var(--phx-border-light);color:var(--phx-text);cursor:pointer;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:1px;font-size:10px;font-weight:700;font-family:inherit;transition:all .1s;user-select:none;-webkit-user-select:none;touch-action:manipulation">' +
    icon + '<span id="fly-center-label">' + label + '</span></button>';
}

function flyDirBtn(dir, label, apiDir) {
  var icon = FLY_ICONS[dir] || '';
  return '<button class="fly-dir-btn" data-dir="' + apiDir + '"' +
    ' onpointerdown="flyPress(\'' + apiDir + '\',event)"' +
    ' onpointerup="flyRelease(\'' + apiDir + '\',event)"' +
    ' onpointercancel="flyRelease(\'' + apiDir + '\',event)"' +
    ' style="width:60px;height:52px;border:none;border-radius:12px;background:var(--phx-bg);border:2px solid var(--phx-border-light);color:var(--phx-text);cursor:pointer;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:1px;font-size:10px;font-weight:700;font-family:inherit;transition:all .1s;user-select:none;-webkit-user-select:none;touch-action:none">' +
    icon + '<span>' + label + '</span></button>';
}

// ========== 飞行控制 API ==========

var flyLookInFlight = false; // 是否有视角请求正在路上
var flyLookPending = false; // 视角变化后等待发送

async function flyCall(endpoint, body) {
  var r = await A('POST', '/api/fly' + endpoint, body || undefined);
  return r;
}

// 发送视角请求（单请求模式，最多只有一个请求在途）
function flyMarkLookPending() {
  if (flyLookInFlight) {
    flyLookPending = true;
    return;
  }
  flyDoSendLook();
}

// 强制立即发送视角请求（用于拖拽结束时的最终值）
function flyFlushLook() {
  if (flyLookInFlight) {
    flyLookPending = true;
  } else {
    flyDoSendLook();
  }
}

async function flyDoSendLook() {
  flyLookInFlight = true;
  flyLookPending = false;
  var r = await flyCall('/look', {pitch: flyState.pitch, yaw: flyState.yaw});
  flyLookInFlight = false;
  if (flyLookPending) {
    flyDoSendLook();
  }
}

async function flyPress(dir, e) {
  if (e && e.target && e.target.setPointerCapture) {
    e.target.setPointerCapture(e.pointerId);
  }
  flyActiveDirs[dir] = true;
  var btn = e && e.target || document.querySelector('.fly-dir-btn[data-dir="' + dir + '"]');
  if (btn) {
    btn.style.background = 'var(--phx-primary-bg)';
    btn.style.borderColor = 'var(--phx-primary)';
    btn.style.color = 'var(--phx-primary)';
    btn.style.transform = 'scale(.95)';
  }
  var r = await flyCall('/press', {direction: dir});
  if (!r.ok) T('移动失败: ' + (r.error || '未知'), 'e');
}

async function flyRelease(dir, e) {
  delete flyActiveDirs[dir];
  var btn = e && e.target || document.querySelector('.fly-dir-btn[data-dir="' + dir + '"]');
  if (btn) {
    btn.style.background = '';
    btn.style.borderColor = '';
    btn.style.color = '';
    btn.style.transform = '';
  }
  if (Object.keys(flyActiveDirs).length === 0) {
    var r = await flyCall('/release');
    if (!r.ok) T('停止移动失败: ' + (r.error || '未知'), 'e');
  }
}

async function flyToggle() {
  if (flyState.flying) {
    var r = await flyCall('/stop');
    if (r.ok) { flyState.flying = false; T('已降落', 'o'); flyUpdateUI(); }
    else { T('降落失败: ' + (r.error || '未知'), 'e'); }
  } else {
    var r = await flyCall('/start');
    if (r.ok) { flyState.flying = true; T('已起飞', 'o'); flyUpdateUI(); }
    else { T('起飞失败: ' + (r.error || '未知'), 'e'); }
  }
}

async function flyJump() {
  var r = await flyCall('/jump');
  if (r.ok) { T(flyState.flying ? '上升' : '跳跃', 'o'); }
  else { T('操作失败: ' + (r.error || '未知'), 'e'); }
}

async function flyJumpStart(e) {
  if (e && e.target && e.target.setPointerCapture) e.target.setPointerCapture(e.pointerId);
  var r = await flyCall('/jump-start');
  if (!r.ok) T((flyState.flying ? '上升' : '跳跃') + '失败: ' + (r.error || '未知'), 'e');
}

async function flyJumpStop(e) {
  var r = await flyCall('/jump-stop');
  if (!r.ok) T('停止' + (flyState.flying ? '上升' : '跳跃') + '失败: ' + (r.error || '未知'), 'e');
}

async function flySneakStart(e) {
  if (e && e.target && e.target.setPointerCapture) e.target.setPointerCapture(e.pointerId);
  var r = await flyCall('/sneak-start');
  if (!r.ok) T('潜行失败: ' + (r.error || '未知'), 'e');
}

async function flySneakStop(e) {
  var r = await flyCall('/sneak-stop');
  if (!r.ok) T('停止潜行失败: ' + (r.error || '未知'), 'e');
}

async function flySprintStart(e) {
  if (e && e.target && e.target.setPointerCapture) e.target.setPointerCapture(e.pointerId);
  flyState.sprinting = true;
  flyUpdateUI();
  var r = await flyCall('/sprint-start');
  if (!r.ok) { T('奔跑失败: ' + (r.error || '未知'), 'e'); flyState.sprinting = false; flyUpdateUI(); }
}

async function flySprintStop(e) {
  flyState.sprinting = false;
  flyUpdateUI();
  var r = await flyCall('/sprint-stop');
  if (!r.ok) { T('停止奔跑失败: ' + (r.error || '未知'), 'e'); }
}

async function flyDown(e) {
  if (e) e.preventDefault();
  var r = await flyCall('/down');
  if (!r.ok) { T('操作失败: ' + (r.error || '未知'), 'e'); return; }
  T(flyState.flying ? '下降' : '潜行', 'o');
}

// 中心按钮长按支持
async function flyCenterPress(e) {
  var btn = e && e.currentTarget;
  if (btn) {
    if (btn.setPointerCapture) btn.setPointerCapture(e.pointerId);
    btn.style.background = 'var(--phx-primary-bg)';
    btn.style.borderColor = 'var(--phx-primary)';
    btn.style.color = 'var(--phx-primary)';
    btn.style.transform = 'scale(.95)';
  }
  if (flyState.flying) {
    var r = await flyCall('/press', {direction: 'down'});
    if (!r.ok) T('下降失败: ' + (r.error || '未知'), 'e');
  } else {
    var r = await flyCall('/sneak-start');
    if (!r.ok) T('潜行失败: ' + (r.error || '未知'), 'e');
  }
}

async function flyCenterRelease(e) {
  var btn = e && e.currentTarget;
  if (btn) {
    btn.style.background = '';
    btn.style.borderColor = '';
    btn.style.color = '';
    btn.style.transform = '';
  }
  if (flyState.flying) {
    var r = await flyCall('/release');
    if (!r.ok) T('停止下降失败: ' + (r.error || '未知'), 'e');
  } else {
    var r = await flyCall('/sneak-stop');
    if (!r.ok) T('停止潜行失败: ' + (r.error || '未知'), 'e');
  }
}

// ========== Canvas 视角控制 ==========

var YAW_DIAL = {w: 150, h: 150, cx: 75, cy: 75, r: 60};

function flyInitCanvases() {
  flyInitPitchSlider();
  flyInitYawCanvas();
}

function flyThemeColor(name, fallback) {
  try { return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback; }
  catch(e) { return fallback; }
}


// ── 俯仰竖直滑动条 ──

function flyInitPitchSlider() {
  var slider = document.getElementById('fly-pitch-slider');
  if (!slider) return;
  slider.value = flyState.pitch;
  slider.addEventListener('input', function() {
    var pitch = parseInt(slider.value) || 0;
    pitch = Math.max(-90, Math.min(90, pitch));
    flyState.pitch = pitch;
    document.getElementById('fly-pitch').textContent = pitch;
    document.getElementById('fly-pitch-val').textContent = pitch + '°';
    flyMarkLookPending();
  });
}

// ── 偏航圆形转盘 ──

function flyInitYawCanvas() {
  var c = document.getElementById('fly-yaw-canvas');
  if (!c) return;
  flyDrawYawDial(c, flyState.yaw);

  var lastSentYaw = flyState.yaw; // 上次发送的 yaw 值，用于 10 度阈值判断

  function handler(e) {
    var rect = c.getBoundingClientRect();
    var x = (e.clientX || e.changedTouches[0].clientX) - rect.left;
    var y = (e.clientY || e.changedTouches[0].clientY) - rect.top;
    var scale = YAW_DIAL.w / rect.width;
    var lx = x * scale, ly = y * scale;
    var dx = lx - YAW_DIAL.cx, dy = ly - YAW_DIAL.cy;
    var dist = Math.sqrt(dx*dx + dy*dy);
    if (dist < 10) return; // 太靠近中心忽略
    // MC yaw: 0=南, 90=西, 180=北, 270=东
    // 屏幕角度: 右=0, 下=90, 左=180, 上=270
    var angle = Math.atan2(dy, dx) * 180 / Math.PI; // 右=0, 下=90, 左=±180, 上=-90
    var yaw = Math.round((angle + 270 + 360) % 360); // 映射: 右→0+270=270(东), 下→90+270=0(南), 左→180+270=90(西), 上→-90+270=180(北)
    flyState.yaw = yaw;
    flyDrawYawDial(c, yaw);
    document.getElementById('fly-yaw').textContent = yaw;
    document.getElementById('fly-yaw-val').textContent = yaw + '°';
    // 每转 10 度调一次 API，避免高频请求卡顿
    if (flyDragging) {
      var diff = yaw - lastSentYaw;
      if (diff > 180) diff -= 360;
      else if (diff < -180) diff += 360;
      if (Math.abs(diff) >= 10) {
        lastSentYaw = yaw;
        flyMarkLookPending();
      }
    } else {
      flyMarkLookPending();
    }
  }

  c.addEventListener('pointerdown', function(e) { flyDragging = true; c.setPointerCapture(e.pointerId); lastSentYaw = flyState.yaw; handler(e); });
  c.addEventListener('pointermove', function(e) { if (flyDragging) handler(e); });
  c.addEventListener('pointerup', function(e) { flyDragging = false; handler(e); flyFlushLook(); });
  c.addEventListener('pointercancel', function(e) { flyDragging = false; flyFlushLook(); });
}

function flyDrawYawDial(c, yaw) {
  var ctx = c.getContext('2d');
  var cx = YAW_DIAL.cx, cy = YAW_DIAL.cy, r = YAW_DIAL.r;
  var bg = flyThemeColor('--phx-bg-card', '#fffcf4');
  var border = flyThemeColor('--phx-border', '#ddd6c8');
  var prim = flyThemeColor('--phx-primary', '#d48a0e');
  var textCol = flyThemeColor('--phx-text-secondary', '#9f927d');

  ctx.clearRect(0, 0, c.width, c.height);

  // 外圈
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, 2 * Math.PI);
  ctx.strokeStyle = border;
  ctx.lineWidth = 3;
  ctx.stroke();

  // 内圈
  ctx.beginPath();
  ctx.arc(cx, cy, r * 0.55, 0, 2 * Math.PI);
  ctx.strokeStyle = border;
  ctx.lineWidth = 1;
  ctx.stroke();

  // 方位标：N=北(180), S=南(0), E=东(270), W=西(90)
  var dirs = [
    {y: 180, label: 'N'}, {y: 0, label: 'S'},
    {y: 270, label: 'E'}, {y: 90, label: 'W'},
  ];
  ctx.font = 'bold 11px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  for (var i = 0; i < dirs.length; i++) {
    var a = (dirs[i].y + 90) * Math.PI / 180;
    var dx = cx + (r - 14) * Math.cos(a);
    var dy = cy + (r - 14) * Math.sin(a);
    ctx.fillStyle = dirs[i].y === (yaw + 180) % 360 ? prim : textCol;
    ctx.fillText(dirs[i].label, dx, dy);
  }

  // 指示线
  var a = (yaw + 90) * Math.PI / 180;
  var ix = cx + r * 0.7 * Math.cos(a);
  var iy = cy + r * 0.7 * Math.sin(a);

  ctx.beginPath();
  ctx.moveTo(cx, cy);
  ctx.lineTo(ix, iy);
  ctx.strokeStyle = prim;
  ctx.lineWidth = 2.5;
  ctx.stroke();

  // 指示点
  ctx.beginPath();
  ctx.arc(ix, iy, 6, 0, 2 * Math.PI);
  ctx.fillStyle = prim;
  ctx.fill();
  ctx.beginPath();
  ctx.arc(ix, iy, 2.5, 0, 2 * Math.PI);
  ctx.fillStyle = '#fff';
  ctx.fill();

  // 中心点
  ctx.beginPath();
  ctx.arc(cx, cy, 3, 0, 2 * Math.PI);
  ctx.fillStyle = textCol;
  ctx.fill();

  // 当前角度文字
  ctx.fillStyle = prim;
  ctx.font = 'bold 12px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(yaw + '°', cx, cy);
}

// ========== 状态轮询 ==========

function flyIsMoving() {
  return Object.keys(flyActiveDirs).length > 0;
}

function flyPollStatus() {
  A('GET', '/api/fly/position').then(function(r) {
    if (r.ok) {
      flyState.x = r.x || 0;
      flyState.y = r.y || 0;
      flyState.z = r.z || 0;
      flyState.pitch = r.pitch || 0;
      flyState.yaw = r.yaw || 0;
      flyState.flying = r.isFlying || false;
      flyState.sprinting = r.sprinting || false;
      flyState.dimension = r.dimension != null ? r.dimension : 0;
      flyUpdateDisplay();
    } else {
      A('GET', '/api/fly/status').then(function(r2) {
        if (r2.ok) {
          flyState.x = r2.x || 0;
          flyState.y = r2.y || 0;
          flyState.z = r2.z || 0;
          flyState.pitch = r2.pitch || 0;
          flyState.yaw = r2.yaw || 0;
          flyState.flying = r2.isFlying || false;
          flyState.sprinting = r2.sprinting || false;
          flyUpdateDisplay();
        } else { flyShowDisconnected(); }
      }).catch(function() { flyShowDisconnected(); });
    }
  }).catch(function() { flyShowDisconnected(); });
}

var DIM_NAMES = {0:'主世界',1:'下界',2:'末地'};

function flyUpdateDisplay() {
  var dot = document.getElementById('fly-status-dot');
  var text = document.getElementById('fly-status-text');
  var pos = document.getElementById('fly-pos');
  var pitch = document.getElementById('fly-pitch');
  var yaw = document.getElementById('fly-yaw');
  var dim = document.getElementById('fly-dim');

  if (dot) { dot.style.background = 'var(--phx-success)'; dot.style.boxShadow = '0 0 6px rgba(111,186,44,.5)'; }
  if (text) { text.textContent = flyState.flying ? '飞行中' : '已就绪'; text.style.color = 'var(--phx-text)'; }
  if (pos) { pos.textContent = Math.round(flyState.x) + ' / ' + Math.round(flyState.y) + ' / ' + Math.round(flyState.z); pos.style.color = 'var(--phx-text)'; }
  if (pitch) pitch.textContent = Math.round(flyState.pitch);
  if (yaw) yaw.textContent = Math.round(flyState.yaw);
  if (dim) dim.textContent = DIM_NAMES[flyState.dimension] || 'DM' + flyState.dimension;

  // 同步画布
  var pc = document.getElementById('fly-pitch-slider');
  var yc = document.getElementById('fly-yaw-canvas');
  if (pc) ;
  if (yc) flyDrawYawDial(yc, flyState.yaw);
  var pv = document.getElementById('fly-pitch-val');
  var yv = document.getElementById('fly-yaw-val');
  if (pv) pv.textContent = Math.round(flyState.pitch) + '°';
  if (yv) yv.textContent = Math.round(flyState.yaw) + '°';

  flyUpdateUI();
}

function flyShowDisconnected() {
  var dot = document.getElementById('fly-status-dot');
  var text = document.getElementById('fly-status-text');
  var pos = document.getElementById('fly-pos');
  var dim = document.getElementById('fly-dim');
  if (dot) { dot.style.background = 'var(--phx-text-disabled)'; dot.style.boxShadow = 'none'; }
  if (text) { text.textContent = '未连接'; text.style.color = 'var(--phx-text-disabled)'; }
  if (pos) { pos.textContent = '- / - / -'; pos.style.color = 'var(--phx-text-disabled)'; }
  if (dim) dim.textContent = '-';
}

function flyUpdateUI() {
  var btn = document.getElementById('fly-toggle-btn');
  if (btn) {
    if (flyState.flying) {
      btn.className = 'btn-d';
      btn.innerHTML = FLY_ICONS.land + ' 降落';
    } else {
      btn.className = 'btn';
      btn.innerHTML = FLY_ICONS.takeoff + ' 起飞';
    }
  }
  // 更新中间按钮文字
  var cl = document.getElementById('fly-center-label');
  if (cl) cl.textContent = flyState.flying ? '下' : '潜行';
  // 更新跳跃按钮文字
  var jumpBtn = document.getElementById('fly-jump-btn');
  if (jumpBtn) jumpBtn.innerHTML = FLY_ICONS.jump + (flyState.flying ? ' 上' : ' 跳');
  // 更新奔跑按钮状态
  var sprintBtn = document.getElementById('fly-sprint-btn');
  var sprintIcon = document.getElementById('fly-sprint-icon');
  var sprintLabel = document.getElementById('fly-sprint-label');
  if (sprintBtn && sprintIcon && sprintLabel) {
    if (flyState.sprinting) {
      sprintBtn.style.background = 'var(--phx-primary-bg)';
      sprintBtn.style.borderColor = 'var(--phx-primary)';
      sprintBtn.style.color = 'var(--phx-primary)';
      sprintIcon.innerHTML = FLY_ICONS.sprint_active;
      sprintLabel.textContent = '奔跑中';
    } else {
      sprintBtn.style.background = '';
      sprintBtn.style.borderColor = '';
      sprintBtn.style.color = '';
      sprintIcon.innerHTML = FLY_ICONS.sprint;
      sprintLabel.textContent = '奔跑';
    }
  }
}

function restoreFlyState() {
  if (flyState.polling) { clearInterval(flyState.polling); flyState.polling = null; }
  flyPollStatus();
  flyState.polling = setInterval(flyPollStatus, 500);
}