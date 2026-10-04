// ═══════════════════════════════════════
// 物品制作器 — 生成 mcstructure 后导入
// 在当前位置生成木桶，内含自定义物品
// 支持换行(\\n)、颜色代码(§)、附魔等 NBT
// 普通客户端铁砧无法换行，此方法不受限制
// ═══════════════════════════════════════

var IM_ICONS = {
  craft: '<svg width="26" height="26" viewBox="0 0 22 22" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>',
  pin: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/></svg>',
  warn: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
  check: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>',
  plus: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>',
  trash: '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>',
};

// 状态
var IM_STATE = {
  items: [],
  editIdx: -1,
};

// 附魔名称映射
var ENCH_NAMES = {
  0: '保护', 1: '防火', 2: '摔落保护', 3: '爆炸保护',
  4: '弹射物保护', 5: '荆棘', 6: '水下呼吸', 7: '深海探索者',
  8: '水下速掘', 9: '锋利', 10: '亡灵杀手', 11: '节肢杀手',
  12: '击退', 13: '火焰附加', 14: '抢夺', 15: '效率',
  16: '精准采集', 17: '耐久', 18: '时运', 19: '力量',
  20: '冲击', 21: '火矢', 22: '无限', 23: '海之眷顾',
  24: '饵钓', 25: '冰霜行者', 26: '经验修补', 27: '绑定诅咒',
  28: '消失诅咒', 29: '穿刺', 30: '激流', 31: '忠诚',
  32: '引雷', 33: '多重射击', 34: '穿透', 35: '快速装填',
  36: '灵魂疾行', 37: '迅捷潜行', 38: '风爆', 39: '致密',
  40: '破甲',
};

function RitemmakerHTML() {
  setTimeout(function() {
    imFetchPos();
    imRenderItems();
  }, 50);

  return '<div class="card">' +
    '<div style="display:flex;align-items:center;gap:8px;font-size:16px;font-weight:800;margin-bottom:8px">' +
      IM_ICONS.craft + ' 物品制作器</div>' +
    '<div class="dim" style="font-size:11px;line-height:1.6;margin-bottom:12px">' +
    '在当前位置生成一个木桶，内含自定义物品。<br>' +
    '支持 <b>换行(\\n)</b>、颜色代码(§)、附魔等 NBT。<br>' +
    '<span style="color:var(--phx-primary);font-weight:700">普通客户端铁砧无法换行，此方法不受限制。</span></div>' +

    '<div id="im-pos-bar" style="padding:8px 12px;border-radius:10px;background:var(--phx-bg);border:2px solid var(--phx-border-light);margin-bottom:12px;font-size:11px;display:flex;align-items:center;gap:6px">' +
      '<span class="spin" id="im-pos-spin"></span>' +
      '<span id="im-pos-text" style="color:var(--phx-text-secondary)">获取位置中...</span>' +
    '</div>' +

    // ── 物品编辑区 ──
    '<div id="im-editor" style="padding:12px;border-radius:12px;background:var(--phx-bg);border:2px solid var(--phx-border-light);margin-bottom:12px">' +
      '<div style="font-size:13px;font-weight:700;margin-bottom:8px">' + IM_ICONS.plus + ' 添加物品</div>' +
      '<label>物品名称</label>' +
      '<textarea id="im-name" rows="2" placeholder="显示名称，支持 § 颜色，回车换行" style="width:100%;margin:0;padding:10px;font-family:inherit;font-size:13px;border-radius:10px;border:2px solid var(--phx-border-light);background:var(--phx-bg);resize:vertical;outline:none;color:var(--phx-text)"></textarea>' +
      '<label>物品 ID</label>' +
      '<input id="im-id" type="text" value="minecraft:diamond_sword" placeholder="minecraft:diamond_sword" style="margin:0">' +
      '<label>特殊值（Damage）</label>' +
      '<input id="im-data" type="number" value="0" placeholder="0" style="margin:0;width:100px">' +
      // 附魔列表
      '<div style="margin-top:10px;padding-top:10px;border-top:2px solid var(--phx-border-light)">' +
        '<div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:6px">' +
          '<span style="font-size:11px;font-weight:700;color:var(--phx-text-secondary)">附魔</span>' +
          '<button class="btn-s" onclick="imAddEnch()" style="width:auto;margin:0;padding:4px 10px;font-size:10px">' + IM_ICONS.plus + ' 添加附魔</button>' +
        '</div>' +
        '<div id="im-ench-list"></div>' +
      '</div>' +
      '<div class="row" style="margin-top:10px">' +
        '<button class="btn-s" onclick="imClearForm()" style="flex:1;margin:0">清空</button>' +
        '<button class="btn" id="im-add-btn" onclick="imAddItem()" style="flex:2;margin:0">添加到列表</button>' +
      '</div>' +
    '</div>' +

    // ── 物品列表 ──
    '<div id="im-item-list" style="margin-bottom:12px">' +
      '<div style="font-size:12px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:6px">物品列表 (0/27)</div>' +
    '</div>' +

    '<div class="row">' +
      '<button class="btn" id="im-go" onclick="doItemMake()" style="flex:1">生成并导入</button>' +
    '</div>' +

    '<div id="im-status" class="gone" style="margin-top:10px">' +
      '<div class="pg"><div id="im-bar" style="width:0%"></div></div>' +
      '<div class="dim" id="im-stat" style="margin-top:4px;text-align:center"></div>' +
    '</div>' +

    '<div id="im-result" class="gone" style="margin-top:10px;padding:12px;border-radius:10px;background:var(--phx-success-bg);color:var(--phx-success);font-size:12px;font-weight:700;text-align:center;line-height:1.6"></div>' +
    '<div id="im-error" class="gone" style="margin-top:10px;padding:12px;border-radius:10px;background:rgba(224,90,90,.12);color:var(--phx-error);font-size:12px;font-weight:700;text-align:center;line-height:1.6"></div>' +
  '</div>';
}

// ── 附魔行 ──

function imEnchOptions(selectedId) {
  var html = '<option value="">选择附魔...</option>';
  var ids = Object.keys(ENCH_NAMES).map(Number).sort(function(a, b) { return a - b; });
  for (var i = 0; i < ids.length; i++) {
    var id = ids[i];
    var sel = id === (selectedId || 9) ? ' selected' : '';
    html += '<option value="' + id + '"' + sel + '>' + id + ' - ' + ENCH_NAMES[id] + '</option>';
  }
  return html;
}

function imShowEnchPicker(btn) {
  var items = [];
  var ids = Object.keys(ENCH_NAMES).map(Number).sort(function(a, b) { return a - b; });
  for (var i = 0; i < ids.length; i++) {
    items.push({value: String(ids[i]), label: ids[i] + ' - ' + ENCH_NAMES[ids[i]]});
  }
  var currentId = btn.getAttribute('data-id') || '9';
  showPicker({
    title: '选择附魔',
    items: items,
    selected: currentId,
    onConfirm: function(v) {
      btn.setAttribute('data-id', v);
      btn.querySelector('.im-ench-picker-label').textContent = ENCH_NAMES[parseInt(v)] || v;
    }
  });
}


var imEnchCount = 0;
function imAddEnch(id, lvl) {
  var list = document.getElementById('im-ench-list');
  if (!list) return;
  var idx = imEnchCount++;
  var html = '<div class="im-ench-row" data-idx="' + idx + '" style="display:flex;gap:6px;align-items:center;margin-bottom:6px">' +
    '<div class="picker-btn im-ench-picker" data-id="' + (id || 9) + '" onclick="imShowEnchPicker(this)" style="flex:1;min-width:0;margin:0;padding:8px 10px;font-size:11px">' +
      '<span class="im-ench-picker-label">' + (ENCH_NAMES[id] || '选择附魔') + '</span>' +
      '<span class="picker-arrow">▾</span></div>' +
    '<input class="im-ench-lvl" type="number" value="' + (lvl || 1) + '" placeholder="等级" style="margin:0;width:60px;padding:8px 6px;font-size:11px">' +
    '<button class="q-item-del" onclick="imRemoveEnch(this)" style="flex-shrink:0">' + IM_ICONS.trash + '</button>' +
  '</div>';
  list.insertAdjacentHTML('beforeend', html);
  // 附魔选择器用底部弹出，不需要事件监听
}

function imRemoveEnch(btn) {
  var row = btn.closest('.im-ench-row');
  if (row) row.remove();
}

function imGetEnchList() {
  var rows = document.querySelectorAll('.im-ench-row');
  var list = [];
  rows.forEach(function(row) {
    var id = parseInt(row.querySelector('.im-ench-picker').getAttribute('data-id'));
    var lvl = parseInt(row.querySelector('.im-ench-lvl').value);
    if (id > 0 && lvl > 0) {
      list.push({id: id, lvl: lvl});
    }
  });
  return list;
}

// ── 物品管理 ──

function imAddItem() {
  var name = document.getElementById('im-name').value.trim();
  // 文本域直接支持回车换行，无需转换 \n
  var id = document.getElementById('im-id').value.trim();
  var data = parseInt(document.getElementById('im-data').value) || 0;
  if (!name || !id) {
    T('请输入物品名称和 ID', 'w');
    return;
  }
  var enchList = imGetEnchList();
  var tag = {};
  if (enchList.length > 0) {
    tag.ench = enchList;
  }
  IM_STATE.items.push({
    名称: name,
    ID: id,
    特殊值: data,
    标签属性: tag
  });
  imClearForm();
  imRenderItems();
}

function imClearForm() {
  document.getElementById('im-name').value = '';
  document.getElementById('im-id').value = 'minecraft:diamond_sword';
  document.getElementById('im-data').value = '0';
  document.getElementById('im-ench-list').innerHTML = '';
  imEnchCount = 0;
}

function imRemoveItem(idx) {
  IM_STATE.items.splice(idx, 1);
  imRenderItems();
}

function imEditItem(idx) {
  var item = IM_STATE.items[idx];
  document.getElementById('im-name').value = item.名称;
  document.getElementById('im-id').value = item.ID;
  document.getElementById('im-data').value = item.特殊值 || 0;
  document.getElementById('im-ench-list').innerHTML = '';
  imEnchCount = 0;
  if (item.标签属性 && item.标签属性.ench) {
    item.标签属性.ench.forEach(function(e) {
      imAddEnch(e.id, e.lvl);
    });
  }
  IM_STATE.editIdx = idx;
  document.getElementById('im-add-btn').textContent = '更新物品';
}

function imRenderItems() {
  var list = document.getElementById('im-item-list');
  if (!list) return;
  var items = IM_STATE.items;
  var header = list.querySelector('.im-items-header');
  var html = '<div style="font-size:12px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:6px">物品列表 (' + items.length + '/27)</div>';
  if (items.length === 0) {
    html += '<div style="text-align:center;padding:16px;color:var(--phx-text-secondary);font-size:11px">暂无物品，请在上方添加</div>';
  } else {
    for (var i = 0; i < items.length; i++) {
      var item = items[i];
      var shortName = item.名称.replace(/§./g, '').replace(/\n/g, ' ').substring(0, 20);
      var enchInfo = '';
      if (item.标签属性 && item.标签属性.ench && item.标签属性.ench.length > 0) {
        enchInfo = ' <span style="color:var(--phx-primary);font-size:9px">[' + item.标签属性.ench.length + '附魔]</span>';
      }
      html += '<div style="display:flex;align-items:center;gap:6px;padding:8px 10px;border-radius:8px;background:var(--phx-bg);border:2px solid var(--phx-border-light);margin-bottom:4px">' +
        '<div style="flex:1;min-width:0;font-size:12px;font-weight:700;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + escHtml(shortName) + enchInfo + '</div>' +
        '<div class="dim" style="font-size:9px;flex-shrink:0">' + item.ID.split(':').pop() + '</div>' +
        '<button class="q-item-del" onclick="imEditItem(' + i + ')" title="编辑" style="flex-shrink:0;color:var(--phx-primary)">✎</button>' +
        '<button class="q-item-del" onclick="imRemoveItem(' + i + ')" style="flex-shrink:0">' + IM_ICONS.trash + '</button>' +
      '</div>';
    }
  }
  list.innerHTML = html;
}

// ── 获取玩家位置 ──

async function imFetchPos() {
  var spin = document.getElementById('im-pos-spin');
  var text = document.getElementById('im-pos-text');
  try {
    var st = await A('GET', '/api/bot/status');
    if (st.ok && st.position) {
      var p = st.position;
      if (spin) spin.style.display = 'none';
      if (text) {
        text.innerHTML = IM_ICONS.pin + ' 当前位置: <b>' + Math.floor(p.x) + ' ' + Math.floor(p.y) + ' ' + Math.floor(p.z) + '</b> &nbsp;|&nbsp; 木桶将放在脚下';
        text.style.color = 'var(--phx-text)';
      }
      return;
    }
    var fly = await A('GET', '/api/fly/position');
    if (fly.ok) {
      if (spin) spin.style.display = 'none';
      if (text) {
        text.innerHTML = IM_ICONS.pin + ' 当前位置: <b>' + Math.floor(fly.x) + ' ' + Math.floor(fly.y) + ' ' + Math.floor(fly.z) + '</b> &nbsp;|&nbsp; 木桶将放在脚下';
        text.style.color = 'var(--phx-text)';
      }
      return;
    }
  } catch(e) {}
  if (spin) spin.style.display = 'none';
  if (text) {
    text.innerHTML = IM_ICONS.warn + ' 无法获取位置，请确认机器人已连接';
    text.style.color = 'var(--phx-warning)';
  }
}

async function imGetPos() {
  var st = await A('GET', '/api/bot/status');
  if (st.ok && st.position) {
    return { x: Math.floor(st.position.x), y: Math.floor(st.position.y), z: Math.floor(st.position.z) };
  }
  var fly = await A('GET', '/api/fly/position');
  if (fly.ok) {
    return { x: Math.floor(fly.x), y: Math.floor(fly.y), z: Math.floor(fly.z) };
  }
  return null;
}

// ── 进度更新 ──

function imProgress(pct, msg) {
  var bar = document.getElementById('im-bar');
  var stat = document.getElementById('im-stat');
  if (bar) bar.style.width = pct + '%';
  if (stat) stat.textContent = msg;
}

function imShowResult(success, msg) {
  document.getElementById('im-status').classList.add('gone');
  document.getElementById('im-go').disabled = false;
  document.getElementById('im-go').textContent = '生成并导入';
  if (success) {
    var el = document.getElementById('im-result');
    el.innerHTML = msg;
    el.classList.remove('gone');
    document.getElementById('im-error').classList.add('gone');
  } else {
    var el = document.getElementById('im-error');
    el.innerHTML = msg;
    el.classList.remove('gone');
    document.getElementById('im-result').classList.add('gone');
  }
}

// ── 主流程 ──

async function doItemMake() {
  var go = document.getElementById('im-go');
  if (go.disabled) return;

  var items = IM_STATE.items;
  if (items.length === 0) {
    T('请先添加物品', 'w');
    return;
  }

  // 检查连接
  var st = await A('GET', '/api/bot/status');
  if (!st.ok || !st.connected) {
    T('未连接到服务器', 'e');
    return;
  }

  var pos = await imGetPos();
  if (!pos) {
    T('无法获取玩家位置', 'e');
    return;
  }

  go.disabled = true;
  go.textContent = '生成中...';
  document.getElementById('im-status').classList.remove('gone');
  document.getElementById('im-result').classList.add('gone');
  document.getElementById('im-error').classList.add('gone');

  imProgress(10, '生成建筑文件...');

  var gen = await A('POST', '/api/itemmaker/generate', {
    items: items,
    x: pos.x, y: pos.y, z: pos.z
  });
  if (!gen.ok) {
    imShowResult(false, '生成失败: ' + (gen.error || '未知错误'));
    return;
  }

  imProgress(30, '准备导入参数...');

  var ana = await A('POST', '/api/building/analyze', {path: gen.path});
  if (!ana.ok) {
    imShowResult(false, '分析失败: ' + (ana.error || ''));
    return;
  }

  imProgress(50, '正在导入木桶到 ' + pos.x + ' ' + pos.y + ' ' + pos.z + '...');

  var task = await A('POST', '/api/task/start', {
    type: 'import',
    params: {
      path: gen.path,
      x: pos.x, y: pos.y, z: pos.z,
      rotation: 0,
      speed: 9500,
      import_commands: false,
      dimension: 'overworld',
      region_mode: 1,
      pre_clear_mode: 0
    }
  });

  if (!task.ok) {
    imShowResult(false, '导入失败: ' + (task.error || ''));
    return;
  }

  imProgress(100, '完成！');
  var posStr = '[' + pos.x + ' ' + pos.y + ' ' + pos.z + ']';
  var resultMsg = IM_ICONS.check + ' 制作完成！共 ' + items.length + ' 个物品<br>' +
    '位置: <b>' + posStr + '</b><br>' +
    '打破木桶即可获取物品，支持换行/颜色/附魔';
  imShowResult(true, resultMsg);
  T('物品制作完成，共 ' + items.length + ' 个', 'o');
}