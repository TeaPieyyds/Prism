// ════════════════════════════════════════
// Prism工具箱 — 首次启动向导逻辑
// ════════════════════════════════════════

var wzActive = false;
var wzStep = 1;
var wzVerified = false;
var wzToken = '';

// ── SVG 图标（替代 emoji） ──
var WZ_ICONS = {
  check: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  arrowLeft: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg>',
  arrowRight: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="5" y1="12" x2="19" y2="12"/><polyline points="12 5 19 12 12 19"/></svg>',
  eye: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/></svg>',
  eyeOff: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/></svg>',
  question: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
  question_small: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
  settings: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>',
  import: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="15" x2="12" y2="3"/></svg>',
  mapArt: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2"/><path d="M3 9h18"/><path d="M9 3v18"/></svg>',
  info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>',
  music: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 18V5l12-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="18" cy="16" r="3"/></svg>',
  connect: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>',
  checkCircle: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>',
  xCircle: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
  switch: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/></svg>',
  prismLogo: '<img src="data:image/png;base64,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" style="width:56px;height:56px;display:block">',
  doneCheck: '<svg width="72" height="72" viewBox="0 0 80 80"><circle cx="40" cy="40" r="36" fill="none" stroke="#d48a0e" stroke-width="4"/><polyline points="24 40 34 50 56 30" fill="none" stroke="#d48a0e" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/></svg>'
};

// ── 初始化 ──
// 返回 true=启动了向导(主界面不应加载), false=直接进主界面
function initWizard() {
  var done = false;
  try { done = localStorage.getItem('prism_wizard_done') === 'true'; } catch (e) {}
  if (!done) {
    startWizard();
    return true;
  }
  return false;
}

// 由 index.html 在 wizard.js 加载完成后调用
function wzBoot() {
  fixWzInsets();
  var done = false;
  try { done = localStorage.getItem('prism_wizard_done') === 'true'; } catch (e) {}
  if (!done) {
    startWizard();
    return true;
  }
  return false;
}

// 全屏模式下把内容稍微向下推一点，避开状态栏图标
function fixWzInsets() {
  // 由 CSS env(safe-area-inset-top) 统一处理
}

function startWizard() {
  wzActive = true;
  // 全屏模式下修正状态栏内边距
  fixWzInsets();
  // 向导强制浅色，状态栏背景色匹配页面
  if (typeof android !== 'undefined' && android.setStatusBarColor) {
    android.setStatusBarColor('#f8f8f0');
  }
  if (typeof android !== 'undefined' && android.setLightStatusBar) {
    android.setLightStatusBar(false); // 浅色背景用深色图标
  }
  var saved = 1;
  try { var s = localStorage.getItem('prism_wizard_step'); if (s) saved = parseInt(s) || 1; } catch (e) {}
  wzStep = Math.max(1, Math.min(4, saved));
  var ov = document.getElementById('wizard-overlay');
  if (ov) ov.style.display = 'flex';
  goToStep(wzStep);
}

// ── 页面切换 ──
function goToStep(n) {
  if (n < 1 || n > 4) return;
  var old = wzStep;
  wzStep = n;
  try { localStorage.setItem('prism_wizard_step', n); } catch (e) {}

  var pages = document.querySelectorAll('.wz-page');
  var dots = document.querySelectorAll('.wz-dot');

  pages.forEach(function (el, i) {
    var pn = i + 1;
    el.classList.remove('active', 'exit-left', 'pg-center');
    if (pn === 1 || pn === 4) el.classList.add('pg-center');
    if (pn === n) {
      el.classList.add('active');
    } else if (pn === old && n > old) {
      el.classList.add('exit-left');
    }
  });
  dots.forEach(function (el, i) {
    el.classList.toggle('active', i + 1 === n);
  });

  // 渲染当前页内容
  renderWizardPage(n);

  // 视差：背景光斑随页面移动
  updateParallax(n);
}

function updateParallax(n) {
  var blobs = document.querySelectorAll('.wz-blob');
  if (!blobs.length) return;
  var offsets = [
    { gold: '0,0', green: '0,0', brown: '0,0' },
    { gold: '-30px,-20px', green: '20px,10px', brown: '-10px,-15px' },
    { gold: '20px,10px', green: '-30px,20px', brown: '10px,-10px' },
    { gold: '-10px,20px', green: '10px,-20px', brown: '-20px,10px' }
  ];
  var o = offsets[Math.min(n - 1, offsets.length - 1)];
  blobs.forEach(function (b) {
    if (b.classList.contains('wz-blob-gold')) {
      b.style.transform = 'translate(' + (o.gold || '0,0') + ')';
    } else if (b.classList.contains('wz-blob-green')) {
      b.style.transform = 'translate(' + (o.green || '0,0') + ')';
    } else if (b.classList.contains('wz-blob-brown')) {
      b.style.transform = 'translate(' + (o.brown || '0,0') + ')';
    }
  });
}

// ── 渲染页内容 ──
function renderWizardPage(n) {
  var el = document.getElementById('wz-page-' + n);
  if (!el) return;
  try {
    switch (n) {
      case 1: renderWzWelcome(el); break;
      case 2: renderWzAuth(el); break;
      case 3: renderWzParams(el); break;
      case 4: renderWzDone(el); break;
    }
  } catch (e) {
    el.innerHTML = '<div style="padding:40px;text-align:center;color:#e05a5a">页面加载失败<br><span style="font-size:11px">' + escHtml(e.message) + '</span></div>';
    if (typeof T === 'function') T('向导错误: ' + e.message, 'e');
  }
}

// 第1页：欢迎
function renderWzWelcome(el) {
  el.innerHTML =
    '<div class="wz-hero">' +
    '  <div class="wz-hero-icon">' + WZ_ICONS.prismLogo + '</div>' +
    '  <h1>Prism 工具箱</h1>' +
    '  <div class="wz-sub">网易我的世界租赁服<br>自动化管理工具</div>' +
    '</div>' +
    '<div class="wz-features">' +
    '  <div class="wz-feat-row">' + WZ_ICONS.import + ' 建筑导入/导出</div>' +
    '  <div class="wz-feat-row">' + WZ_ICONS.mapArt + ' 像素画绘制</div>' +
    '  <div class="wz-feat-row">' + WZ_ICONS.info + ' 信息查看</div>' +
    '  <div class="wz-feat-row">' + WZ_ICONS.music + ' 音乐播放</div>' +
    '</div>' +
    '<button class="wz-btn wz-btn-primary" onclick="goToStep(2)">' +
      WZ_ICONS.arrowRight + ' 开始设置</button>';
}

// 第2页：认证配置
function renderWzAuth(el) {
  el.innerHTML =
    '<div class="wz-topbar">' +
    '  <button onclick="goToStep(1)">' + WZ_ICONS.arrowLeft + ' 返回</button>' +
    '</div>' +
    '<div style="width:100%;max-width:320px;margin:0 auto">' +
    '  <div class="wz-section-title">' + WZ_ICONS.connect + ' 连接认证服务</div>' +
    '  <div style="font-size:12px;font-weight:700;color:#9f927d;margin-bottom:4px">管理 Token' +
    '    <span style="margin-left:4px;cursor:pointer;vertical-align:middle" onclick="showTokenHelp()">' + WZ_ICONS.question_small + '</span>' +
    '  </div>' +
    '  <div style="position:relative;margin-bottom:12px">' +
    '    <input id="wz-token" type="password" placeholder="adb//xxxx...（留空=匿名使用）" value="' + escHtml(wzToken) + '"' +
    '      style="width:100%;height:44px;padding:0 40px 0 14px;border:2px solid #e8e2d6;border-radius:12px;background:#fffcf4;font-size:14px;color:#794f27;outline:none;box-sizing:border-box">' +
    '    <button style="position:absolute;right:10px;top:50%;transform:translateY(-50%);background:none;border:none;cursor:pointer;padding:4px;color:#9f927d" onclick="toggleTokenVis()">' + WZ_ICONS.eye + '</button>' +
    '  </div>' +
    '  <div id="wz-auth-status"></div>' +
    '  <button class="wz-btn wz-btn-primary" id="wz-auth-btn" onclick="verifyToken()">验证 Token</button>' +
'  <div style="margin-top:12px;padding-top:12px;border-top:1px solid #e8e2d6;text-align:center">' +
'    <button class="wz-btn" style="background:none;border:2px solid #c4b89e;color:#9f927d;width:100%" onclick="skipAuth()">跳过，匿名使用</button>' +
'    <div class="dim" style="font-size:10px;color:#c4b89e;margin-top:6px">匿名模式下部分功能受限，可在设置中随时绑定 Token</div>' +
'  </div>' +
    '  <div id="wz-auth-after" style="display:none;margin-top:12px;padding-top:12px;border-top:1px solid #e8e2d6">' +
    '    <div style="display:flex;align-items:center;gap:8px;font-size:13px;font-weight:600;color:#6fba2c;margin-bottom:12px">' + WZ_ICONS.checkCircle + ' <span id="wz-auth-user-label"></span></div>' +
    '    <button class="wz-acc-btn" onclick="fetchAccData()">' + WZ_ICONS.switch + ' 获取账号数据</button>' +
    '  </div>' +
    '  <button class="wz-btn wz-btn-primary" id="wz-auth-next" style="margin-top:12px;display:none" onclick="goToNextAfterAuth()">' + WZ_ICONS.arrowRight + ' 下一步</button>' +
    '</div>';
}

function toggleTokenVis() {
  var inp = document.getElementById('wz-token');
  var btn = document.getElementById('wz-token-tgl');
  if (!inp || !btn) return;
  var isPw = inp.type === 'password';
  inp.type = isPw ? 'text' : 'password';
  btn.innerHTML = isPw ? WZ_ICONS.eyeOff : WZ_ICONS.eye;
}

async function verifyToken() {
  var inp = document.getElementById('wz-token');
  var btn = document.getElementById('wz-auth-btn');
  var st = document.getElementById('wz-auth-status');
  if (!inp || !btn || !st) return;
  var token = inp.value.trim();
  if (!token) { st.innerHTML = '<div class="wz-status err">' + WZ_ICONS.xCircle + ' 请输入 Token</div>'; return; }
  btn.classList.add('loading');
  btn.innerHTML = '验证中...';
  st.innerHTML = '';
  // 先保存 Token，后端 prism-proxy 需要用它进行认证
  try {
    // 读取当前配置，保留 auth_url 避免被覆写
    var curCfg = null;
    try { var curR = await A('GET', '/api/config'); if (curR.ok && curR.config) curCfg = curR.config; } catch(e) {}
    var save = await A('POST', '/api/config', {
      token: token,
      auth_url: (curCfg && curCfg.auth_url) || 'https://prism.adblanlu.qzz.io',
      server_code: curCfg?.server_code || '',
      server_pass: curCfg?.server_pass || '',
      import_speed: curCfg?.import_speed || 9500,
      import_commands: curCfg?.import_commands !== false,
      cmd_disabled: !!curCfg?.cmd_disabled,
      exclude_fluids: !!curCfg?.exclude_fluids,
      exclude_water: curCfg?.exclude_water !== false,
      exclude_waterlogged: curCfg?.exclude_waterlogged !== false,
      exclude_lava: curCfg?.exclude_lava !== false,
      use_gravity_blocks: !!curCfg?.use_gravity_blocks,
      gravity_platform: !!curCfg?.gravity_platform,
      use_new_protocol: !!curCfg?.use_new_protocol,
      region_mode: curCfg?.region_mode || '1'
    });
    if (!save.ok) { throw new Error(save.error || '保存失败'); }
    // 缓存 token 到 localStorage，确保设置页面能读取到
    try { localStorage.setItem('prism_token', token); } catch(e) {}
    // 通过 prism-proxy 调 auth/me 验证
    var r = await APrism('GET', '/api/auth/me');
    if (r.ok) {
      wzToken = token;
      wzVerified = true;
      st.innerHTML = '<div class="wz-status ok">' + WZ_ICONS.checkCircle + ' 验证通过</div>';
      var after = document.getElementById('wz-auth-after');
      var userEl = document.getElementById('wz-auth-user-label');
      if (after) after.style.display = 'block';
      if (userEl) userEl.textContent = '已认证: ' + escHtml(r.display_name || r.nickname || r.username || '用户');
      var nextBtn = document.getElementById('wz-auth-next');
      if (nextBtn) nextBtn.style.display = 'flex';
      btn.innerHTML = WZ_ICONS.check + ' 已验证';
      btn.style.border = '2px solid #6fba2c';
      btn.style.background = 'none';
      btn.style.color = '#6fba2c';
      btn.classList.remove('loading');
    } else {
      wzVerified = false;
      st.innerHTML = '<div class="wz-status err">' + WZ_ICONS.xCircle + ' ' + escHtml(r.error || 'Token 无效') + '</div>';
      btn.classList.remove('loading');
      btn.innerHTML = '验证 Token';
    }
  } catch (e) {
    wzVerified = false;
    st.innerHTML = '<div class="wz-status err">' + WZ_ICONS.xCircle + ' ' + escHtml(e.message || '网络错误') + '</div>';
    btn.classList.remove('loading');
    btn.innerHTML = '验证 Token';
  }
}

async function fetchAccData() {
  // 复用设置页的 showAccountModal()
  if (typeof showAccountModal === 'function') {
    // 保持向导层级不变，将账号弹窗显示在向导之上
    showAccountModal();
    // 提高弹窗 z-index 使其在向导之上，并隐藏底部导航栏
    setTimeout(function() {
      document.querySelectorAll('.modal-overlay').forEach(function(el) {
        el.style.zIndex = '99999';
      });
      var _nav = document.getElementById('bottom-nav');
      if (_nav) _nav.style.display = 'none';
    }, 50);
    // 监测弹窗关闭后恢复导航栏
    wzObserveModalClose(function() {
      var _nav2 = document.getElementById('bottom-nav');
      if (_nav2) _nav2.style.display = '';
    });
  } else {
    // 兜底：直接弹 Toast
    if (typeof T === 'function') T('加载账号数据...', 'o');
    try {
      var r = await A('GET', '/api/accounts');
      if (r.ok && r.accounts) {
        showWzAccountList(r.accounts);
      } else {
        if (typeof T === 'function') T('加载失败: ' + (r.error || ''), 'e');
      }
    } catch (e) {
      if (typeof T === 'function') T('网络错误', 'e');
    }
  }
}

function showWzAccountList(accounts) {
  // 简单的账号列表面板（兜底用，主要复用 showAccountModal）
  var ov = document.createElement('div');
  ov.className = 'wz-modal';
  var h = '<div class="wz-modal-box"><div class="wz-modal-title">切换账号</div><div class="wz-modal-body">';
  var list = accounts || [];
  for (var i = 0; i < list.length; i++) {
    var a = list[i];
    h += '<div style="display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid #e8e2d6">' +
      '<div style="flex:1"><div style="font-size:13px;font-weight:700">' + escHtml(a.display_name || '') + '</div>' +
      '<div style="font-size:10px;color:#9f927d">UID ' + (a.uid || '') + '</div></div>' +
      (a.is_active ? '<span style="font-size:10px;font-weight:700;color:#d48a0e;background:#fef7e6;padding:3px 8px;border-radius:6px">使用中</span>' : '') +
      '</div>';
  }
  h += '</div><button class="wz-btn wz-btn-primary" style="margin-top:12px" onclick="this.closest(\'.wz-modal\').remove()">关闭</button></div>';
  ov.innerHTML = h;
  document.body.appendChild(ov);
  ov.addEventListener('click', function (e) { if (e.target === ov) ov.remove(); });
}

// MutationObserver 监听弹窗关闭，替代 setInterval 轮询
function wzObserveModalClose(onClose) {
  var target = document.body;
  var obs = new MutationObserver(function() {
    if (!document.querySelector('.modal-overlay')) {
      onClose();
      obs.disconnect();
    }
  });
  obs.observe(target, { childList: true, subtree: true });
}

function showTokenHelp() {
  var ov = document.createElement('div');
  ov.className = 'wz-modal';
  ov.innerHTML =
    '<div class="wz-modal-box">' +
    '  <div class="wz-modal-title">如何获取 Token</div>' +
    '  <div class="wz-modal-body">' +
    '    <ol><li>打开 prism 管理后台</li><li>登录你的账号</li><li>在设置页面复制 Token</li></ol>' +
    '    <div style="margin-top:8px;font-size:11px;color:#c4b89e">后台地址: prism.adblanlu.qzz.io</div>' +
    '  </div>' +
    '  <button class="wz-btn wz-btn-primary" style="margin-top:14px" onclick="this.closest(\'.wz-modal\').remove()">知道了</button>' +
    '</div>';
  document.body.appendChild(ov);
  ov.addEventListener('click', function (e) { if (e.target === ov) ov.remove(); });
}

function goToNextAfterAuth() {
  if (!wzVerified) {
    T && T('请先验证 Token', 'o');
    return;
  }
  goToStep(3);
}

// 第3页：导入默认参数
var wzCfgLoaded = false;
var wzCfg = {};

async function renderWzParams(el) {
  // 先以默认值渲染，避免白页
  if (!wzCfgLoaded) {
    buildWzParamsHtml(el, wzCfg);
    try {
      var r = await A('GET', '/api/config');
      if (r.ok && r.config) {
        wzCfg = r.config;
      }
    } catch (e) {}
    wzCfgLoaded = true;
  }
  buildWzParamsHtml(el, wzCfg);
}

function buildWzParamsHtml(el, cfg) {
  var spd = cfg.import_speed || 9500;
  var cmds = cfg.import_commands !== false;
  var cmdDis = !!cfg.cmd_disabled;
  var exFluids = !!cfg.exclude_fluids;
  var exWater = cfg.exclude_water !== false;
  var exWLog = cfg.exclude_waterlogged !== false;
  var exLava = cfg.exclude_lava !== false;
  var gravity = !!cfg.use_gravity_blocks;
  var platform = !!cfg.gravity_platform;
  var relief = loadWzDefault('relief', false);
  var newProto = cfg.use_new_protocol ?? loadWzDefault('new_protocol', false);
  var regionMode = cfg.region_mode || loadWzDefault('region_mode', '1');
  var builtinPicker = loadWzDefault('builtin_picker', true);

  el.innerHTML =
    '<div class="wz-topbar">' +
    '  <button onclick="goToStep(2)">' + WZ_ICONS.arrowLeft + ' 返回</button>' +
    '  <button onclick="goToStep(4)">跳过</button>' +
    '</div>' +
    '<div class="wz-section-title">' + WZ_ICONS.settings + ' 导入默认参数</div>' +
    '<div class="wz-params-scroll">' +
    // 导入速度
    '<div class="wz-param-group">' +
    '  <div class="wz-param-group-title">导入速度</div>' +
    '  <div class="wz-param-row">' +
    '    <label>速度（方块/秒）</label>' +
    '    <input type="number" id="wz-cfg-speed" value="' + spd + '" min="500" max="50000" onchange="saveWzCfg()">' +
    '  </div>' +
    '</div>' +
    // 导入选项
    '<div class="wz-param-group">' +
    '  <div class="wz-param-group-title">导入选项</div>' +
    '  <div class="wz-param-row">' +
    '    <label>导入粒度</label>' +
    '    <div class="picker-btn" id="wz-cfg-region" data-value="' + regionMode + '" onclick="showWzRegionPicker()" style="flex:1;padding:6px 10px;border:2px solid #e8e2d6;border-radius:8px;background:#f8f8f0;font-size:12px;color:#794f27;cursor:pointer;display:flex;align-items:center;min-width:0">' +
    '      <span class="picker-label" style="flex:1;font-size:12px;color:#794f27;font-family:inherit;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">' + (regionMode === '2' ? '2x2 小区 32x32' : regionMode === '3' ? '4x4 小区 64x64' : '单区块 16x16') + '</span><span class="picker-arrow" style="color:#c4b89e;font-size:10px;margin-left:6px">▾</span></div>' +
    '  </div>' +
    wzToggle('wz-cfg-cmds', cmds, '导入指令', 'saveWzCfg()') +
    '<div id="wz-cfg-cmd-sub" class="wz-toggle-sub" style="display:' + (cmds ? 'block' : 'none') + '">' +
    wzToggle('wz-cfg-cmd-dis', cmdDis, '关闭状态导入', 'saveWzCfg()') +
    '</div>' +
    wzToggle('wz-cfg-ex-fluids', exFluids, '排除流体方块', 'saveWzCfg();toggleWzFluidSub()') +
    '<div id="wz-cfg-fluid-sub" class="wz-toggle-sub" style="display:' + (exFluids ? 'block' : 'none') + '">' +
    wzToggle('wz-cfg-ex-water', exWater, '排除水', 'saveWzCfg()') +
    wzToggle('wz-cfg-ex-wlog', exWLog, '排除含水方块', 'saveWzCfg()') +
    wzToggle('wz-cfg-ex-lava', exLava, '排除岩浆', 'saveWzCfg()') +
    '</div>' +
    wzToggle('wz-cfg-new-proto', newProto, '新版协议 (1.21.120)', 'saveWzCfg()') +
    '</div>' +
    // 地图画
    '<div class="wz-param-group">' +
    '  <div class="wz-param-group-title">地图画选项</div>' +
    wzToggle('wz-cfg-relief', relief, '三级明暗（立体起伏）', 'saveWzCfg();toggleWzReliefSub()') +
    '<div id="wz-cfg-relief-notice" class="dim" style="font-size:10px;margin:2px 0 8px;color:#c4b89e;display:' + (relief ? 'block' : 'none') + '">开启后重力/玻璃不可用</div>' +
    '<div id="wz-cfg-gravity-wrap" style="display:' + (relief ? 'none' : 'block') + '">' +
    wzToggle('wz-cfg-gravity', gravity, '使用重力方块', 'saveWzCfg();toggleWzGravitySub()') +
    '  <div id="wz-cfg-platform-wrap" class="wz-toggle-sub" style="display:' + (gravity && !relief ? 'block' : 'none') + '">' +
    wzToggle('wz-cfg-platform', platform, '放置玻璃平台', 'saveWzCfg()') +
    '  </div>' +
    '</div>' +
    '</div>' +
    // 界面设置
    '<div class="wz-param-group">' +
    '  <div class="wz-param-group-title">界面设置</div>' +
    wzToggle('wz-cfg-picker', builtinPicker, '使用内置文件选择器', 'saveWzCfg()') +
    '</div>' +
    '</div>' +
    '<div style="padding:12px 0 0;flex-shrink:0">' +
    '<button class="wz-btn wz-btn-primary" onclick="goToStep(4)">' + WZ_ICONS.arrowRight + ' 下一步</button>' +
    '</div>';
}

function loadWzDefault(key, def) {
  try { var v = localStorage.getItem('wz_def_' + key); if (v !== null) return JSON.parse(v); } catch (e) {}
  return def;
}

function saveWzDefault(key, val) {
  try { localStorage.setItem('wz_def_' + key, JSON.stringify(val)); } catch (e) {}
}

function toggleWzReliefSub() {
  var cb = document.getElementById('wz-cfg-relief');
  if (!cb) return;
  var on = cb.checked;
  var note = document.getElementById('wz-cfg-relief-notice');
  var wrap = document.getElementById('wz-cfg-gravity-wrap');
  if (note) note.style.display = on ? 'block' : 'none';
  if (wrap) wrap.style.display = on ? 'none' : 'block';
}
function toggleWzGravitySub() {
  var cb = document.getElementById('wz-cfg-gravity');
  var wrap = document.getElementById('wz-cfg-platform-wrap');
  var relief = document.getElementById('wz-cfg-relief');
  if (wrap && cb && relief && !relief.checked) {
    wrap.style.display = cb.checked ? 'block' : 'none';
  }
}

function wzToggle(id, checked, label, onChange) {
  return '<label class="wz-toggle">' +
    '<input type="checkbox" id="' + id + '" ' + (checked ? 'checked' : '') +
    (onChange ? ' onchange="' + onChange + '"' : '') + '>' +
    '<span class="wz-tgl-track"></span>' +
    '<span class="wz-tgl-label">' + label + '</span></label>';
}

function toggleWzFluidSub() {
  var el = document.getElementById('wz-cfg-fluid-sub');
  var cb = document.getElementById('wz-cfg-ex-fluids');
  if (el && cb) el.style.display = cb.checked ? 'block' : 'none';
}

// 指令子选项联动
document.addEventListener('change', function (e) {
  if (e.target.id === 'wz-cfg-cmds') {
    var sub = document.getElementById('wz-cfg-cmd-sub');
    if (sub) sub.style.display = e.target.checked ? 'block' : 'none';
  }
});

function getWzCfgVal(id, def) {
  var el = document.getElementById(id);
  if (!el) return def;
  if (el.type === 'checkbox') return el.checked;
  if (el.type === 'number') return parseInt(el.value) || def;
  return el.value || def;
}

async function saveWzCfg() {
  // localStorage 同步保存（先执行，不依赖网络）
  var _newProto = getWzCfgVal('wz-cfg-new-proto', false);
  var _regionMode = getWzCfgVal('wz-cfg-region', '1');
  var _builtinPicker = getWzCfgVal('wz-cfg-picker', true);
  var _relief = getWzCfgVal('wz-cfg-relief', false);
  saveWzDefault('new_protocol', _newProto);
  saveWzDefault('region_mode', _regionMode);
  saveWzDefault('builtin_picker', _builtinPicker);
  saveWzDefault('relief', _relief);
  // 从后端读取当前配置，保留 token/server_code/server_pass 避免被全量替换清空
  try { var _wzCurCfg = await A('GET', '/api/config'); } catch(e) {}
  var _wzCfg = _wzCurCfg && _wzCurCfg.config;
  var _wzAuthUrl = (_wzCfg && _wzCfg.auth_url) || 'https://prism.adblanlu.qzz.io';
  var _wzToken = _wzCfg?.token || '';
  var _wzServerCode = _wzCfg?.server_code || '';
  var _wzServerPass = _wzCfg?.server_pass || '';
  // 如果向导第2页的 token 输入框有值，优先用页面值
  var _wzTokenInput = document.getElementById('wz-token');
  if (_wzTokenInput && _wzTokenInput.value.trim()) _wzToken = _wzTokenInput.value.trim();
  var cfg = {
    token: _wzToken,
    auth_url: _wzAuthUrl,
    server_code: _wzServerCode,
    server_pass: _wzServerPass,
    import_speed: getWzCfgVal('wz-cfg-speed', 9500),
    import_commands: getWzCfgVal('wz-cfg-cmds', true),
    cmd_disabled: getWzCfgVal('wz-cfg-cmd-dis', false),
    exclude_fluids: getWzCfgVal('wz-cfg-ex-fluids', false),
    exclude_water: getWzCfgVal('wz-cfg-ex-water', true),
    exclude_waterlogged: getWzCfgVal('wz-cfg-ex-wlog', true),
    exclude_lava: getWzCfgVal('wz-cfg-ex-lava', true),
    use_gravity_blocks: getWzCfgVal('wz-cfg-gravity', false),
    gravity_platform: getWzCfgVal('wz-cfg-platform', false),
    use_new_protocol: _newProto,
    region_mode: _regionMode
  };
  try {
    var r = await A('POST', '/api/config', cfg);
    if (r.ok) wzCfg = cfg;
  } catch (e) { console.error('save cfg error:', e); }
}

// 第4页：完成
function renderWzDone(el) {
  // 异步获取工具箱信息
  if (wzVerified && wzToken) {
    setTimeout(function() {
      A('GET', '/api/toolbox/my-info').then(function(r) {
        if (r.ok && r.data) {
          var info = r.data;
          var status = info.enabled && !info.expired ? '\u2705 \u5df2\u5f00\u901a' : (info.expired ? '\u274c \u5df2\u8fc7\u671f' : '\u26aa \u672a\u5f00\u901a');
          var expiry = info.expires_at ? new Date(info.expires_at).toLocaleDateString('zh-CN') : '\u6c38\u4e45';
          var tbEl = document.getElementById('wz-toolbox-status');
          if (tbEl) {
            tbEl.innerHTML = '<div style="background:#fef7e6;border-radius:12px;padding:12px;margin:12px 0;text-align:left">' +
              '<div style="display:flex;justify-content:space-between;padding:4px 0"><span style="color:#9f927d;font-size:12px">\u72b6\u6001</span><span style="font-size:13px;font-weight:700">' + status + '</span></div>' +
              '<div style="display:flex;justify-content:space-between;padding:4px 0"><span style="color:#9f927d;font-size:12px">\u5230\u671f</span><span style="font-size:12px">' + expiry + '</span></div>' +
              (info.days_remaining ? '<div style="display:flex;justify-content:space-between;padding:4px 0"><span style="color:#9f927d;font-size:12px">\u5269\u4f59</span><span style="font-size:12px">' + info.days_remaining + ' \u5929</span></div>' : '') +
              (info.effective_prompt ? '<div style="padding:4px 0;border-top:1px solid #e8e2d6;margin-top:4px;font-size:11px;color:#9f927d;word-break:break-all">\u63d0\u793a\u8bcd: ' + escHtml(info.effective_prompt) + '</div>' : '') +
              '</div>';
          }
        }
      }).catch(function(){});
    }, 500);
  }

  var showTrial = wzVerified && wzToken;
  el.innerHTML =
    '<div style="text-align:center;width:100%;max-width:320px;margin:0 auto">' +
    '  <div class="wz-done-check anim">' + WZ_ICONS.doneCheck + '</div>' +
    '  <div class="wz-done-title">准备就绪！</div>' +
    '  <div class="wz-done-sub">配置已完成，<br>现在开始使用 Prism 工具箱</div>' +
    '  <div id="wz-toolbox-status"></div>' +
    (showTrial ?
      '<button class="wz-btn" style="background:none;border:2px solid #d48a0e;color:#d48a0e;width:100%;margin-bottom:8px" onclick="startTrialFromWizard()">' + WZ_ICONS.checkCircle + ' 激活 90 天免费试用</button>' : '') +
    '  <button class="wz-btn wz-btn-primary" onclick="finishWizard()">' + WZ_ICONS.arrowRight + ' 开始使用</button>' +
    '</div>';
}

// ── 完成向导 ──
// 跳过认证，匿名使用
function skipAuth() {
  wzVerified = false;
  wzToken = "";
  goToStep(3);
}

// 向导中激活试用
function startTrialFromWizard() {
  A("POST", "/api/toolbox/start-trial").then(function(r) {
    if (r.ok) {
      if (typeof T === "function") T("\ud83c\udf89 \u8bd5\u7528\u6fc0\u6d3b\u6210\u529f\uff01\u6709\u6548\u671f 90 \u5929", "o");
      var el = document.getElementById("wz-page-4");
      if (el) renderWzDone(el);
    } else {
      if (typeof T === "function") T("\u6fc0\u6d3b\u5931\u8d25: " + (r.error || "\u672a\u77e5"), "e");
    }
  }).catch(function(e) {
    if (typeof T === "function") T("\u6fc0\u6d3b\u5931\u8d25: " + e.message, "e");
  });
}

async function finishWizard() {
  // 保存所有向导配置（等待完成，确保 localStorage 和 后端都写入）
  await saveWzCfg();
  try {
    localStorage.setItem('prism_wizard_done', 'true');
    localStorage.removeItem('prism_wizard_step');
  } catch (e) {}
  wzActive = false;
  var ov = document.getElementById('wizard-overlay');
  if (ov) {
    ov.style.opacity = '0';
    setTimeout(function () {
      ov.style.display = 'none';
      ov.style.opacity = '1';
      // 进入首页后检查公告
      if (typeof fetchAnnouncements === 'function') fetchAnnouncements();
      // 恢复 SSE 事件处理
      if (typeof Rhome === 'function') Rhome();
      // 应用向导默认值到各功能页面
      setTimeout(applyWzDefaults, 600);
    }, 300);
  }
}

// 第3页渲染完成后附加联动
function attachWzEvents() {
  // 流体子选项联动在 renderWzParams 和 toggleWzFluidSub 中处理
}

// ── 暴露到全局 ──
window.initWizard = initWizard;
window.startWizard = startWizard;
window.goToStep = goToStep;
window.verifyToken = verifyToken;
window.toggleTokenVis = toggleTokenVis;
window.fetchAccData = fetchAccData;
window.showTokenHelp = showTokenHelp;
window.saveWzCfg = saveWzCfg;
window.toggleWzFluidSub = toggleWzFluidSub;
window.getWzCfgVal = getWzCfgVal;
window.wzBoot = wzBoot;
// 向导完成后，将 localStorage 中的默认值应用到各功能页面
async function applyWzDefaults() {
  try {
    // 从后端重新加载配置，更新全局变量（向导中可能修改了这些值）
    try { var _r = await A('GET', '/api/config'); if (_r.ok && _r.config) {
      cfgSpeed = _r.config.import_speed || 9500;
      cfgNewProtocol = _r.config.use_new_protocol || false;
      cfgImportCommands = _r.config.import_commands !== false;
      cfgCmdDisabled = _r.config.cmd_disabled || false;
      cfgExcludeFluids = _r.config.exclude_fluids || false;
      cfgExcludeWater = _r.config.exclude_water !== false;
      cfgExcludeWaterlogged = _r.config.exclude_waterlogged !== false;
      cfgExcludeLava = _r.config.exclude_lava !== false;
      cfgGravity = _r.config.use_gravity_blocks || false;
      cfgPlatform = _r.config.gravity_platform || false;
      cfgRegionMode = _r.config.region_mode || '1';
    }} catch(e) {}
    // 导入粒度
    var rm = localStorage.getItem('wz_def_region_mode');
    if (rm) {
      var el = document.getElementById('imp-region-picker');
      if (el){el.setAttribute('data-value',rm);var lbs={'1':'单区块 16×16（当前）','2':'2×2 小区 32×32','3':'4×4 小区 64×64'};el.querySelector('.picker-label').textContent=lbs[rm]||'单区块 16×16（当前）'}
      cfgRegionMode = rm;
    }
    // 新版协议
    var np = localStorage.getItem('wz_def_new_protocol');
    if (np === 'true') {
      var el2 = document.getElementById('cfg-new-protocol');
      if (el2) el2.checked = true;
      cfgNewProtocol = true;
    } else {
      cfgNewProtocol = false;
    }
    // 内置文件选择器
    var bp = localStorage.getItem('wz_def_builtin_picker');
    if (bp === 'false' && typeof setPickerMode === 'function') {
      setPickerMode(false);
    }
    // 三级明暗 → 地图画页
    var rl = localStorage.getItem('wz_def_relief');
    if (rl === 'true') {
      var el4 = document.getElementById('ma-relief');
      if (el4) { el4.checked = true; if (typeof toggleMaRelief === 'function') toggleMaRelief(); }
    }
    // 重力方块 + 玻璃平台 → 地图画页
    if (cfgGravity) {
      var el5 = document.getElementById('ma-gravity');
      if (el5) { el5.checked = true; if (typeof toggleGravitySub === 'function') toggleGravitySub(); }
    }
    if (cfgPlatform) {
      var el6 = document.getElementById('ma-platform');
      if (el6) el6.checked = true;
    }
  } catch(e) {}
}

window.applyWzDefaults = applyWzDefaults;
window.toggleWzReliefSub = toggleWzReliefSub;
window.toggleWzGravitySub = toggleWzGravitySub;
window.goToNextAfterAuth = goToNextAfterAuth;
window.finishWizard = finishWizard;
window.WZ_ICONS = WZ_ICONS;
window.wzToggle = wzToggle;

// initWizard() 在 index.html 加载完成后调用
