var cfgToken='';
var updateLocked=false;
var updateURL='';
var useBuiltinPicker = (function(){try{return localStorage.getItem('use_builtin_picker')!=='0'}catch(e){return true}})();
var _cfgRefreshTimer = null;
var _toolboxPollTimer = null;
var _cachedToolboxInfo = null;

async function Rcfg(){
  // 立即渲染骨架，不等待API
  document.getElementById('tab-settings').innerHTML='<div class="content-inner" style="padding:4px 0">'+
    ((typeof android!=='undefined')?'':'<button class="btn-d" style="width:100%;margin-bottom:12px;background:#d32f2f" onclick="restartServer()">重启服务</button>')+
    '<div class="card" id="cfg-conn-card"><div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px">'+
    '<span style="font-size:14px;font-weight:800">服务器连接</span><span class="dim" id="cfg-conn-st">检测中...</span></div>'+
    '<input id="bot-server" placeholder="服务器号" oninput="toggleServerPass()">'+
    '<div class="dim" style="font-size:10px;margin:2px 0 6px;color:var(--phx-text-secondary)">#房间号=联机房间 &nbsp; @房间号=本地联机</div>'+
    '<label class="lbl-cb" style="margin:4px 0;font-size:12px"><input type="checkbox" id="cfg-new-protocol"><span class="tgl"></span> 新版协议 (1.21.120)</label>'+
    '<input id="bot-pass" type="password" class="gone" placeholder="服务器密码">'+
    '<div class="row"><button class="btn" onclick="doConn()">连接服务器</button><button class="btn-s" onclick="doDisc()">断开</button></div></div>'+

    // ═══ 认证信息（支持匿名） ═══
    '<div class="card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">认证信息</div>'+
    '<div style="display:flex;align-items:center;gap:4px"><input id="cfg-token" type="password" placeholder="Token（留空=匿名使用）" oninput="onTokenInput()" style="flex:1"><button id="cfg-token-add" class="btn-s" style="width:32px;height:32px;padding:0;font-size:18px;line-height:1;display:none" onclick="addSubToken()">+</button></div>'+
    '<div id="cfg-sub-token-list"></div>'+
    '<div class="dim" style="font-size:10px;margin:4px 0 8px;color:var(--phx-text-secondary)">不填 Token 也可以匿名使用工具箱，但部分功能受限</div>'+
    '<label style="font-size:12px;font-weight:700;margin:6px 0 4px">验证服务器</label>'+
    '<div class="picker-btn" id="cfg-server-picker" data-url="https://prism.adblanlu.qzz.io" onclick="showServerPicker()"><span class="picker-label">prism (默认)</span><span class="picker-arrow">▾</span></div>'+
    '<div id="cfg-verif-btn-wrap"><button class="btn" style="width:100%;margin-top:8px" onclick="saveCfg();setTimeout(function(){openVerifSite()},200)">打开验证服务器</button></div>'+
    '<div id="cfg-acc-card"><div class="dim" style="font-size:11px;text-align:center;padding:8px">加载中...</div></div>'+
    '</div>'+

    // ═══ 多机器人 ═══
    '<div class="card" id="cfg-multi-card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">多机器人作业</div>'+
    '<div class="row" style="margin-top:6px"><button class="btn" onclick="doFleetConn()">连接子机器人</button><button class="btn-s" onclick="doFleetDisc()">断开</button></div>'+
    '<div id="cfg-fleet-status" style="font-size:11px;margin-top:6px;padding:4px;border-radius:4px;background:var(--phx-bg-secondary)"></div>'+
    '</div>'+

    // ═══ 工具箱信息 ═══
    '<div class="card" id="cfg-toolbox-card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">工具箱状态</div>'+
    '<div id="cfg-toolbox-body"><div class="dim" style="font-size:11px;text-align:center;padding:8px">填写 Token 后查看工具箱状态</div></div></div>'+

    '<div class="card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">默认参数</div>'+
    '<label>导入速度（方块/秒）</label><input id="cfg-speed" type="number" value="9500"></div>'+
    '<div class="card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">导入默认选项</div>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-import-cmds" checked><span class="tgl"></span> 默认导入指令</label>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-cmd-disabled"><span class="tgl"></span> 默认关闭状态导入</label>'+
    '<div style="font-size:10px;color:var(--phx-text-secondary);margin-bottom:6px">循环命令方块以红石控制模式导入</div>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-exclude-fluids"><span class="tgl"></span> 默认排除流体方块</label>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-exclude-water" checked><span class="tgl"></span> 默认排除水</label>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-exclude-waterlogged" checked><span class="tgl"></span> 默认排除含水方块</label>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-exclude-lava" checked><span class="tgl"></span> 默认排除岩浆</label>'+
    '</div>'+
    '<div class="card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">地图画默认选项</div>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-gravity"><span class="tgl"></span> 默认使用重力方块</label>'+
    '<label class="lbl-cb" style="margin:0 0 6px"><input type="checkbox" id="cfg-platform"><span class="tgl"></span> 默认放置玻璃平台</label>'+
    '<div style="font-size:10px;color:var(--phx-text-secondary);margin-bottom:6px">水平导入时在下方铺玻璃</div>'+
    '</div>'+
    '<div class="card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">界面设置</div>'+
    '<label>主题配色</label><div class="picker-btn" id="cfg-theme-picker" onclick="showThemePicker()"><span class="picker-label">' + ({'light':'浅色 — 暖黄','sepia':'复古棕','dark':'深色 — 蓝灰','forest':'墨绿 — 自然','amethyst':'暗紫 — 优雅','ocean':'海洋蓝','sakura':'樱花粉','midnight':'午夜深蓝','custom':'自定义（图片取色）'}[appTheme] || '浅色 — 暖黄') + '</span><span class="picker-arrow">▾</span></div>'+
    '<button class="picker-btn" style="margin-top:8px" onclick="openPersonalizeSheet()"><span class="picker-label">更多个性化设置</span><span class="picker-arrow">▸</span></button>'+
    '<div class="dim" style="margin-top:4px">界面缩放、自定义背景图片、图片取色生成主题</div></div>'+
    '<div class="card"><div style="font-size:14px;font-weight:800;margin-bottom:10px">文件选择器</div>'+
    '<label class="lbl-cb"><input type="checkbox" id="cfg-picker" checked onchange="setPickerMode(this.checked)"><span class="tgl"></span> 使用内置文件选择器</label>'+
    '<div class="dim" style="font-size:10px;margin-top:4px">开启后可浏览设备文件目录树；关闭则使用系统文件选择对话框</div></div>'+
    '<button class="btn" onclick="saveCfg()">保存配置</button>'+
    '<div style="text-align:center;margin-top:10px;display:flex;gap:4px">'+
    '<button class="btn-s" onclick="reopenAnnouncements()" style="flex:1;margin:0">重新打开公告</button>'+
    '<button class="btn-s" onclick="checkUpdateManual()" style="flex:1;margin:0">检测更新</button>'+
    '<button class="btn-s" onclick="clearCache()" style="flex:1;margin:0">清理缓存</button>'+
    '<button class="btn-s" onclick="openQA()" style="flex:1;margin:0">工单反馈</button>'+
    '</div></div>';

  // 异步填充配置
  refreshCfg();

  // 每10秒刷新账号信息
  if(_cfgRefreshTimer) clearInterval(_cfgRefreshTimer);
  _cfgRefreshTimer = setInterval(refreshCfgAccount, 10000);

  // 每30秒轮询工具箱信息
  if(_toolboxPollTimer) clearInterval(_toolboxPollTimer);
  _toolboxPollTimer = setInterval(pollToolboxInfo, 30000);
}

async function refreshCfg(){
  var c=await A('GET','/api/config');
  if(c.ok&&c.config){
    cfgToken=c.config.token||'';
    if(c.config.auth_url){curServerUrl=c.config.auth_url;applyServerSelection()}
    document.getElementById('bot-server').value=c.config.server_code||'';
    document.getElementById('cfg-new-protocol').checked=c.config.use_new_protocol||false;
    document.getElementById('cfg-token').value=c.config.token||'';
    onTokenInput();
    document.getElementById('cfg-speed').value=c.config.import_speed||9500;
    document.getElementById('cfg-import-cmds').checked=c.config.import_commands!==false;
    document.getElementById('cfg-cmd-disabled').checked=c.config.cmd_disabled||false;
    document.getElementById('cfg-exclude-fluids').checked=c.config.exclude_fluids||false;
    document.getElementById('cfg-exclude-water').checked=c.config.exclude_water!==false;
    document.getElementById('cfg-exclude-waterlogged').checked=c.config.exclude_waterlogged!==false;
    document.getElementById('cfg-exclude-lava').checked=c.config.exclude_lava!==false;
    document.getElementById('cfg-gravity').checked=c.config.use_gravity_blocks||false;
    document.getElementById('cfg-platform').checked=c.config.gravity_platform||false;
    if(c.config.bot_tokens){
      _subTokens=(c.config.bot_tokens||[]).slice();
      renderSubTokenList();
    }
    toggleServerPass();
  }
  refreshCfgStatus();
  refreshCfgAccount();
  pollToolboxInfo();
}

async function refreshCfgStatus(){
  var s=await A('GET','/api/bot/status');
  U(s.ok&&s.connected, s.ok&&s.is_op, s.ok?s.server:'');
  var el=document.getElementById('cfg-conn-st');
  if(!el)return;
  if(s.ok&&s.connected){
    el.style.color=s.is_op?'var(--phx-success)':'var(--phx-warning)';
    el.innerHTML=s.is_op?ICONS.dot+' 已连接(OP)':ICONS.dot+' 已连接';
  }else{
    el.style.color='var(--phx-text-disabled)';
    el.innerHTML=ICONS.dotEmpty+' 未连接'
  }
}

async function refreshCfgAccount(){
  if(!cfgToken&&document.getElementById('cfg-token'))cfgToken=document.getElementById('cfg-token').value;
  if(!cfgToken){
    var el=document.getElementById('cfg-acc-card');
    if(el)el.innerHTML='<div class="dim" style="font-size:11px;text-align:center;padding:8px;color:var(--phx-text-secondary)">匿名使用中 — 填写 Token 后可管理账号</div>';
    return
  }
  var ar=await APrism('GET','/api/accounts/active');
  var el=document.getElementById('cfg-acc-card');
  if(!el)return;
  if(ar.ok&&ar.display_name){
    el.innerHTML='<div style="display:flex;align-items:center;gap:10px;padding:10px 12px;background:var(--phx-primary-bg);border-radius:14px;border:2px solid var(--phx-primary);cursor:pointer" onclick="showAccountModal()">'+
      (ar.avatar_image_url?'<img src="'+ar.avatar_image_url+'" style="width:36px;height:36px;border-radius:10px;flex-shrink:0">':'<div style="width:36px;height:36px;border-radius:10px;background:var(--phx-border-light);flex-shrink:0"></div>')+
      '<div style="flex:1;min-width:0"><div style="font-weight:700;font-size:13px;color:var(--phx-primary)">'+escHtml(ar.display_name)+'</div>'+
      '<div style="font-size:9px;color:var(--phx-primary);opacity:.7">UID: '+ar.uid+' &nbsp;Lv.'+ar.growth_level+'</div></div>'+
      '<span style="color:var(--phx-primary);font-size:16px">&#8250;</span></div>'
  }else{
    el.innerHTML='<div style="cursor:pointer" onclick="showAccountModal()">'+
      '<div style="text-align:center;padding:10px 12px;background:var(--phx-bg);border-radius:12px;border:2px dashed var(--phx-border)">'+
      '<div style="font-size:13px;font-weight:700;color:var(--phx-primary)">当前没有设置活跃账号</div>'+
      '<div class="dim" style="font-size:10px;margin-top:2px">点击选择账号</div></div></div>'
  }
}

// ═══ 工具箱信息轮询 ═══

async function pollToolboxInfo(){
  var token = cfgToken || (document.getElementById('cfg-token') ? document.getElementById('cfg-token').value : '');
  if(!token) {
    var body = document.getElementById('cfg-toolbox-body');
    if(body) body.innerHTML = '<div class="dim" style="font-size:11px;text-align:center;padding:8px;color:var(--phx-text-secondary)">匿名使用中 — 填写 Token 后可查看工具箱状态</div>';
    _cachedToolboxInfo = null;
    return;
  }
  var r = await A('GET', '/api/toolbox/my-info');
  if(r.ok && r.data) {
    _cachedToolboxInfo = r.data;
    renderToolboxInfo(r.data);
  }
}

function renderToolboxInfo(data){
  // 保存当前输入值，避免轮询覆盖
  var _savedPrompt = document.getElementById('cfg-custom-prompt')?.value || "";
  var _savedName = document.getElementById('cfg-custom-name')?.value || "";
  var body = document.getElementById('cfg-toolbox-body');
  if(!body) return;

  var enabled = data.enabled;
  var expired = data.expired;
  var daysRemaining = data.days_remaining || 0;
  var expiresAt = data.expires_at || '';
  var trialUsed = data.trial_used || false;
  var promptPurchased = data.prompt_purchased || false;
  var namePurchased = data.name_purchased || false;
  var customPrompt = data.custom_prompt || '';
  var customName = data.custom_name || '';
  var effectivePrompt = data.effective_prompt || '';
  var effectiveName = data.effective_name || '';

  var statusColor, statusText;
  if(!enabled) {
    statusColor = 'var(--phx-text-disabled)';
    statusText = '未开通';
  } else if(expired) {
    statusColor = '#e05a5a';
    statusText = '已过期';
  } else {
    statusColor = 'var(--phx-success)';
    statusText = '使用中';
  }
  var expiryStr = expiresAt ? new Date(expiresAt).toLocaleDateString('zh-CN') : '永久';

  // 紧凑状态行
  var html = '<div style="display:flex;align-items:center;gap:6px;flex-wrap:wrap;font-size:12px;margin-bottom:6px">'+
    '<span style="color:var(--phx-text-secondary)">状态</span>'+
    '<span style="font-weight:700;color:'+statusColor+'">'+statusText+'</span>';
  if(enabled) {
    html += '<span style="color:var(--phx-text-secondary);margin-left:6px">到期</span>'+
      '<span>'+escHtml(expiryStr)+'</span>'+
      '<span style="color:var(--phx-text-secondary)">剩余</span>'+
      '<span style="font-weight:700;color:'+(daysRemaining<7?'#e05a5a':'var(--phx-text)')+'">'+daysRemaining+'天</span>';
  }
  html += '</div>';

  // 功能购买状态
  var promptStatus = promptPurchased
    ? '<span style="color:var(--phx-success)">已购提示词</span>'
    : '<span style="color:#d48a0e;cursor:pointer" onclick="purchasePrompt()">购买提示词 500🌰</span>';
  var nameStatus = namePurchased
    ? '<span style="color:var(--phx-success)">已购命名</span>'
    : '<span style="color:#d48a0e;cursor:pointer" onclick="purchaseName()">购买命名 300🌰</span>';

  html += '<div style="display:flex;gap:8px;font-size:11px;margin-bottom:4px">'+promptStatus+' · '+nameStatus+'</div>';

  // 编辑区域（已购买时显示）
  if(promptPurchased) {
    html += '<div style="display:flex;gap:4px;margin-bottom:4px;align-items:center">'+
      '<input id="cfg-custom-prompt" type="text" style="flex:1;padding:6px;border:1px solid var(--phx-border);border-radius:6px;background:var(--phx-bg);font-size:11px;color:var(--phx-text);box-sizing:border-box;min-width:0" placeholder="自定义提示词…" value="'+escHtml(customPrompt)+'">'+
      '<button class="btn-s" style="width:auto;padding:5px 10px;margin:0;font-size:10px;flex-shrink:0" onclick="saveCustomPrompt()">保存</button></div>';
  }
  if(namePurchased) {
    html += '<div style="display:flex;gap:4px;margin-bottom:4px;align-items:center">'+
      '<input id="cfg-custom-name" type="text" style="flex:1;padding:6px;border:1px solid var(--phx-border);border-radius:6px;background:var(--phx-bg);font-size:11px;color:var(--phx-text);box-sizing:border-box;min-width:0" placeholder="自定义名称…" value="'+escHtml(customName)+'">'+
      '<button class="btn-s" style="width:auto;padding:5px 10px;margin:0;font-size:10px;flex-shrink:0" onclick="saveCustomName()">保存</button></div>';
  }

  // 操作按钮
  var btns = [];
  if(!trialUsed && !enabled) btns.push('<button class="btn-s" style="font-size:10px;padding:4px 8px;background:#6fba2c;color:#fff" onclick="startTrial()">90天试用</button>');
  if(enabled) btns.push('<button class="btn-s" style="font-size:10px;padding:4px 8px" onclick="purchaseExtension()">续期</button>');
  btns.push('<button class="btn-s" style="font-size:10px;padding:4px 8px" onclick="showNutsBalance()">余额</button>');
  html += '<div style="display:flex;gap:4px;flex-wrap:wrap">'+btns.join('')+'</div>';

  body.innerHTML = html;
  // 恢复之前保存的输入值
  var promptEl = document.getElementById('cfg-custom-prompt');
  if(promptEl && _savedPrompt) promptEl.value = _savedPrompt;
  var nameEl = document.getElementById('cfg-custom-name');
  if(nameEl && _savedName) nameEl.value = _savedName;
}

// ═══ 工具箱操作函数 ═══

async function startTrial(){
  showConfirm({
    title:'激活免费试用',
    message:'确定要激活 90 天免费试用吗？\n每人仅限一次。',
    confirmText:'激活',
    onConfirm: async function(){
      var r = await A('POST', '/api/toolbox/start-trial');
      if(r.ok){
        T('🎉 试用激活成功！有效期 90 天','o');
        pollToolboxInfo();
      } else {
        T('激活失败: '+(r.error||'未知'),'e');
      }
    }
  });
}

async function purchasePrompt(){
  showConfirm({
    title:'购买自定义提示词',
    message:'确定要花费 500 🌰 购买自定义提示词功能吗？\n一次性购买，永久解锁。',
    confirmText:'购买',
    onConfirm: async function(){
      var r = await A('POST', '/api/toolbox/purchase/prompt');
      if(r.ok){
        T('✅ 购买成功！现在可以设置自定义提示词','o');
        pollToolboxInfo();
      } else {
        T('购买失败: '+(r.error||'未知'),'e');
      }
    }
  });
}

async function purchaseName(){
  showConfirm({
    title:'购买自定义命名',
    message:'确定要花费 300 🌰 购买自定义命名功能吗？\n一次性购买，永久解锁。',
    confirmText:'购买',
    onConfirm: async function(){
      var r = await A('POST', '/api/toolbox/purchase/name');
      if(r.ok){
        T('✅ 购买成功！现在可以设置自定义名称','o');
        pollToolboxInfo();
      } else {
        T('购买失败: '+(r.error||'未知'),'e');
      }
    }
  });
}

async function purchaseExtension(){
  showPrompt({
    title:'续期工具箱使用权',
    message:'输入续期天数（每 30 天 100 🌰）：',
    placeholder:'30',
    default:'30',
    confirmText:'续期',
    onConfirm: async function(days){
      days = parseInt(days) || 30;
      if(days < 1) { T('天数必须大于 0','e'); return; }
      var cost = Math.ceil(days / 30) * 100;
      showConfirm({
        title:'确认续期',
        message:'续期 '+days+' 天，需要 '+cost+' 🌰\n确定要继续吗？',
        confirmText:'续期',
        onConfirm: async function(){
          var r = await A('POST', '/api/toolbox/purchase/extension?days='+days);
          if(r.ok){
            T('✅ 续期成功！','o');
            pollToolboxInfo();
          } else {
            T('续期失败: '+(r.error||'未知'),'e');
          }
        }
      });
    }
  });
}

async function showNutsBalance(){
  var r = await APrism('GET', '/api/auth/nuts');
  if(r.ok && r.balance !== undefined) {
    showAlertModal('板栗余额','<div style="text-align:center;padding:20px">'+
      '<div style="font-size:48px;font-weight:900;color:#d48a0e">'+(r.balance || 0)+'</div>'+
      '<div style="font-size:14px;color:var(--phx-text-secondary);margin-top:4px">🌰 板栗</div></div>',true,null,null,'1001');
  } else {
    T('查询失败: '+(r.error||'未知'),'e');
  }
}

async function saveCustomPrompt(){
  var val = document.getElementById('cfg-custom-prompt')?.value;
  if(!val) { T('请输入提示词内容','e'); return; }
  var r = await A('POST', '/api/toolbox/my-info/update', {custom_prompt: val});
  if(r.ok){
    T('✅ 提示词已保存','o');
    pollToolboxInfo();
  } else {
    T('保存失败: '+(r.error||'未知'),'e');
  }
}

async function saveCustomName(){
  var val = document.getElementById('cfg-custom-name')?.value;
  if(!val) { T('请输入名称','e'); return; }
  var r = await A('POST', '/api/toolbox/my-info/update', {custom_name: val});
  if(r.ok){
    T('✅ 名称已保存','o');
    pollToolboxInfo();
  } else {
    T('保存失败: '+(r.error||'未知'),'e');
  }
}

// ═══ 原有函数 ═══

function reopenAnnouncements(){
  seenAnnouncements={};dismissedAnnouncements={};
  try{localStorage.removeItem('ann_dismiss_')}catch(e){}
  fetchAnnouncements();
  T('已重新加载公告','o')
}

function checkUpdateManual(){
  A('POST','/api/version/check',{}).then(function(r){
    if(r.ok){
      if(r.latest_version_code>r.current_version_code){
        handleVersionCheck(r)
      }else{
        T('已是最新版本','o');
        showAlertModal('更新日志',
          '<p style="color:var(--phx-success)">' + ICONS.check + ' 当前已是最新版本</p>'+
          renderMD(r.update_message||''),true,null,null,'1001')
      }
    }else{T('检测失败: '+(r.error||'未知'),'e')}
  }).catch(function(e){T('检测失败: '+e.message,'e')})
}

function openQA(){
  var t=cfgToken||(document.getElementById('cfg-token')?document.getElementById('cfg-token').value:'');
  if(!t){T('请先填写Token','e');return}
  var url='https://qa.adblanlu.qzz.io/?token='+encodeURIComponent(t);
  if(typeof android!=='undefined'&&android.openExternal){android.openExternal(url)}
  else{window.open(url,'_blank')}
}

function showConnModal(){
  var m=document.createElement('div');m.id='conn-modal';m.className='modal-overlay';
  m.innerHTML='<div class="modal-box" style="text-align:center">'+
    '<span class="spin" style="width:32px;height:32px;display:block;margin:0 auto 14px;color:var(--phx-primary)"></span>'+
    '<div style="font-size:15px;font-weight:800">正在连接服务器</div>'+
    '<div style="font-size:12px;color:var(--phx-text-secondary);margin-top:6px" id="conn-stage">正在连接服务器...</div>'+
    '<div class="log" id="conn-log" style="margin-top:10px;max-height:140px;text-align:left"></div></div>';
  document.body.appendChild(m)
}

function hideConnModal(){
  var el=document.getElementById('conn-modal');
  if(el)el.remove()
}

function updateConnStage(msg){
  var el=document.getElementById('conn-stage');
  if(el)el.textContent=msg
}

async function restartServer(){var r=await A('POST','/api/system/restart');if(r.ok){T('服务重启中，3秒后刷新...','o');setTimeout(function(){location.reload()},3000)}else{T('重启失败: '+(r.error||'未知'),'e')}}

function toggleServerPass(){
  var s=document.getElementById('bot-server');if(!s)return;
  var p=document.getElementById('bot-pass');if(!p)return;
  // 显示密码框：纯数字服务器号 OR @房间号 OR #房间号
  p.classList.toggle('gone',!/^\d+$|^[@#]/.test(s.value))
}

// ═══ 验证服务器配置 ═══
// 搜索功能和进入服务器使用用户配置的验证服务器；心跳上报等仍走 prism 默认。
var CFG_SERVERS=[
  {url:'https://prism.adblanlu.qzz.io',label:'prism (默认)'},
  {url:'https://fox.faka66.fun',label:'fox.faka66.fun'}
];
var curServerUrl='https://prism.adblanlu.qzz.io';

function showServerPicker(){
  var items=CFG_SERVERS.map(function(s){return{value:s.url,label:s.label}});
  items.push({value:'__custom__',label:'自定义'});
  showPicker({title:'选择验证服务器',items:items,selected:curServerUrl,
    onConfirm:function(v){
      if(v==='__custom__'){
        var input=prompt('输入验证服务器地址',curServerUrl);
        if(input&&input.trim()){curServerUrl=input.trim();applyServerSelection()}
      }else{curServerUrl=v;applyServerSelection()}
    }});
}
function getServerLabel(url){
  for(var i=0;i<CFG_SERVERS.length;i++){if(CFG_SERVERS[i].url===url)return CFG_SERVERS[i].label}
  return url.replace(/^https?:\/\//,'')
}
function applyServerSelection(){
  var p=document.getElementById('cfg-server-picker');
  if(p){p.setAttribute('data-url',curServerUrl);p.querySelector('.picker-label').textContent=getServerLabel(curServerUrl)}
  // 只有选择 prism 才显示"打开验证服务器"按钮
  var wrap=document.getElementById('cfg-verif-btn-wrap');
  if(wrap)wrap.style.display=curServerUrl.indexOf('prism.adblanlu')>=0?'':'none';
}

async function doConn(){
  showConnModal();updateConnStage('正在连接服务器...');
  var pw=document.getElementById('bot-pass');var pass='';
  if(pw&&!pw.classList.contains('gone'))pass=pw.value;
  var r=await A('POST','/api/bot/connect',{token:document.getElementById('cfg-token').value,server:document.getElementById('bot-server').value,use_new_protocol:document.getElementById('cfg-new-protocol')?.checked||false,auth:curServerUrl,password:pass});
  if(!r.ok){hideConnModal();T('连接失败: '+r.error,'e');return}
  // 轮询连接状态更新阶段文本；弹窗被 bot_log/conn_ready 关闭时自动停止。
  // 不再设强制超时：无权限等待授权时保持弹窗显示，避免误报"连接超时"。
  var readyCheck=setInterval(async function(){
    if(!document.getElementById('conn-modal')){clearInterval(readyCheck);return}
    var s=await A('GET','/api/bot/status');
    if(s.ok&&s.connected){
      var el=document.getElementById('conn-stage');if(el)el.textContent='已连接，等待权限验证...';
      // 兜底：即使 conn_ready/bot_log 事件未到达前端，只要轮询到已连接且有权限就关闭弹窗。
      // 否则机器人其实已就绪，但弹窗一直开着，看起来像"卡死"。
      if(s.is_op){
        var m=document.getElementById('conn-modal');
        if(m){m.remove();T('已连接','o')}
        clearInterval(readyCheck);
        U(true,true);
      }
    }
  },1500)
}

async function doDisc(){await A('POST','/api/bot/disconnect');T('已断开');U(false);Rhome()}

// ── 动态令牌列表 ──
var _subTokens=[];

// 主令牌输入时控制 "+" 按钮显隐
function onTokenInput(){
  var el=document.getElementById('cfg-token-add');
  if(el)el.style.display=document.getElementById('cfg-token').value.length>10?'block':'none';
}

// 添加一个新的子令牌输入框
function addSubToken(){
  _subTokens.push('');
  renderSubTokenList();
}

// 删除第 idx 个子令牌
function removeSubToken(idx){
  _subTokens.splice(idx,1);
  renderSubTokenList();
}

// 子令牌输入时控制其 "+" 按钮，并更新 _subTokens
function onSubTokenInput(idx){
  var inp=document.getElementById('cfg-sub-token-'+idx);
  if(!inp)return;
  _subTokens[idx]=inp.value;
  var btn=document.getElementById('cfg-sub-add-'+idx);
  if(btn)btn.style.display=inp.value.length>10?'block':'none';
}

// 渲染所有子令牌输入框
function renderSubTokenList(){
  var el=document.getElementById('cfg-sub-token-list');
  if(!el)return;
  if(_subTokens.length===0){
    el.innerHTML='';
    return;
  }
  el.innerHTML=_subTokens.map(function(t,i){
    return '<div style="display:flex;align-items:center;gap:4px;margin-top:4px">'+
      '<input id="cfg-sub-token-'+i+'" type="password" placeholder="子机器人'+(i+1)+' Token" value="'+escHtml(t)+'" oninput="onSubTokenInput('+i+')" style="flex:1;font-size:12px">'+
      '<button id="cfg-sub-add-'+i+'" class="btn-s" style="width:32px;height:32px;padding:0;font-size:18px;line-height:1;'+((t.length>10)?'':'display:none')+'" onclick="addSubToken()">+</button>'+
      '<button class="btn-s" style="width:32px;height:32px;padding:0;font-size:14px;line-height:1;color:var(--phx-error)" onclick="removeSubToken('+i+')">&times;</button>'+
      '</div>';
  }).join('');
}

// 收集所有子令牌值
function getSubTokens(){
  var tokens=[];
  for(var i=0;i<_subTokens.length;i++){
    var inp=document.getElementById('cfg-sub-token-'+i);
    if(inp&&inp.value.trim())tokens.push(inp.value.trim());
  }
  return tokens;
}

async function doFleetConn(){
  // 先保存配置（确保子令牌已写入）
  await saveCfg();
  var r=await A('POST','/api/fleet/connect');
  if(!r.ok){T('连接失败: '+r.error,'e');return}
  T('正在连接子机器人...','o');
  // 轮询舰队状态
  var poll=setInterval(async function(){
    var s=await A('GET','/api/fleet/status');
    var el=document.getElementById('cfg-fleet-status');
    if(!el)return;
    if(s.ok&&s.status){
      var connected=s.status.filter(function(b){return b.connected}).length;
      var total=s.status.length;
      el.innerHTML='<div style="font-size:11px">已连接: '+connected+'/'+total+'</div>'+
        s.status.map(function(b,i){
          var icon=b.connected?'<span style="color:#4caf50">●</span>':'<span style="color:#888">○</span>';
          var name=b.name||'子机器人'+(i+1);
          var err=b.error?'<span style="color:#f44">'+b.error+'</span>':'';
          return '<div style="padding:2px 0">'+icon+' '+name+' '+(b.is_op?'(OP)':'')+err+'</div>';
        }).join('');
      if(connected===total)clearInterval(poll);
    }
  },1500);
}

async function doFleetDisc(){
  await A('POST','/api/fleet/disconnect');
  T('子机器人已断开','o');
  var el=document.getElementById('cfg-fleet-status');
  if(el)el.innerHTML='<div class="dim" style="font-size:11px;text-align:center;padding:4px">已断开</div>';
}

async function saveCfg(){
  var r=await A('POST','/api/config',{
    token:document.getElementById('cfg-token').value,
    auth_url:curServerUrl,
    server_code:document.getElementById('bot-server').value,
    use_new_protocol:document.getElementById('cfg-new-protocol')?.checked||false,
    server_pass:document.getElementById('bot-pass')?.value||'',
    import_speed:Math.min(Math.max(parseInt(document.getElementById('cfg-speed').value)||9500,1),20000),
    import_commands:document.getElementById('cfg-import-cmds')?.checked!==false,
    cmd_disabled:document.getElementById('cfg-cmd-disabled')?.checked||false,
    exclude_fluids:document.getElementById('cfg-exclude-fluids')?.checked||false,
    exclude_water:document.getElementById('cfg-exclude-water')?.checked!==false,
    exclude_waterlogged:document.getElementById('cfg-exclude-waterlogged')?.checked!==false,
    exclude_lava:document.getElementById('cfg-exclude-lava')?.checked!==false,
    use_gravity_blocks:document.getElementById('cfg-gravity')?.checked||false,
    gravity_platform:document.getElementById('cfg-platform')?.checked||false,
    bot_tokens:getSubTokens()
  });
  // 保存后同步到全局变量，让导入页立即感知
  cfgBotTokens=getSubTokens();
  T(r.ok?'已保存':'失败: '+r.error,r.ok?'o':'e')
}

function openVerifSite(){
  var ov=document.createElement('div');ov.className='modal-overlay';ov.style.zIndex='1001';
  ov.innerHTML='<div style="position:fixed;inset:0;display:flex;flex-direction:column;background:var(--phx-bg-card)">'+
    '<div style="display:flex;align-items:center;justify-content:space-between;padding:12px 16px;border-bottom:2px solid var(--phx-border-light);flex-shrink:0">'+
    '<span style="font-size:14px;font-weight:800">验证服务器</span>'+
    '<button class="btn-s" onclick="this.closest(\'.modal-overlay\').remove()" style="width:auto;padding:8px 16px;font-size:13px;margin:0">关闭返回</button>'+
    '</div>'+
    '<iframe src="https://prism.adblanlu.qzz.io" style="flex:1;width:100%;border:none" sandbox="allow-same-origin allow-scripts allow-forms allow-popups"></iframe>'+
    '<div style="flex-shrink:0;padding:8px 16px;text-align:center;border-top:2px solid var(--phx-border-light)">'+
    '<span class="dim" style="font-size:11px">如无法加载，请用浏览器打开 </span>'+
    '<a href="https://prism.adblanlu.qzz.io" target="_blank" style="font-size:11px;color:var(--phx-primary);font-weight:700">prism.adblanlu.qzz.io</a>'+
    '</div></div>';
  document.body.appendChild(ov)
}

function setPickerMode(enabled){
  useBuiltinPicker=enabled;
  try{localStorage.setItem('use_builtin_picker',enabled?'1':'0')}catch(e){}
}

var THEME_ITEMS=[{value:'light',label:'浅色 — 暖黄'},{value:'sepia',label:'复古棕'},{value:'dark',label:'深色 — 蓝灰'},{value:'forest',label:'墨绿 — 自然'},{value:'amethyst',label:'暗紫 — 优雅'},{value:'ocean',label:'海洋蓝'},{value:'sakura',label:'樱花粉'},{value:'midnight',label:'午夜深蓝'}];
function showThemePicker(){
  var items=THEME_ITEMS.slice();
  if(appTheme==='custom')items.push({value:'custom',label:'自定义（图片取色）'});
  showPicker({title:'选择主题配色',items:items,selected:appTheme,
    onConfirm:function(v){
      var p=document.getElementById('cfg-theme-picker');p.setAttribute('data-value',v);
      var it=items.find(function(i){return i.value===v});
      if(it)p.querySelector('.picker-label').textContent=it.label;
      setTheme(v)
    }})
}

async function clearCache(){
  showConfirm({
    title:'清理缓存',message:'确定要清理缓存文件吗？\n将删除已上传的建筑文件、音乐文件等临时数据。\n配置和账号信息不受影响。',
    confirmText:'清理',onConfirm:function(){doClearCache()}
  })
}
async function doClearCache(){
  try{
    var r=await A('POST','/api/cache/clear');
    if(r.ok){
      var msg='已清理 '+r.deleted+' 个文件';
      if(r.freed>0){
        var size=r.freed;
        var unit='B';
        if(size>1073741824){size=(size/1073741824).toFixed(2);unit='GB'}
        else if(size>1048576){size=(size/1048576).toFixed(2);unit='MB'}
        else if(size>1024){size=(size/1024).toFixed(2);unit='KB'}
        msg+=', 释放 '+size+unit;
      }
      if(r.errors&&r.errors.length) msg+='\n部分文件删除失败: '+r.errors.join(', ');
      T(msg,'o');
    }else{
      T('清理失败: '+(r.error||'未知'),'e');
    }
  }catch(e){
    T('清理失败: '+e.message,'e');
  }
}