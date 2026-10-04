// 权限检查：进入预览功能前必须拥有 MANAGE_ALL_FILES
function previewCheckPermission() {
  return new Promise(function(resolve) {
    // 桌面模式不需要权限检查
    if (typeof android === 'undefined') {
      resolve(true);
      return;
    }
    try {
      if (typeof android.checkStoragePermission === 'function') {
        if (android.checkStoragePermission()) {
          resolve(true);
          return;
        }
      }
    } catch(e) {
      console.error('previewCheckPermission error:', e);
    }
    var overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.innerHTML =
      '<div class="modal-box" style="text-align:center;max-width:340px">' +
      '<div style="margin:0 auto 16px">' +
      '<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--phx-warning)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">' +
      '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg></div>' +
      '<div style="font-size:15px;font-weight:800;margin-bottom:8px">需要管理所有文件权限</div>' +
      '<div style="font-size:12px;color:var(--phx-text-secondary);line-height:1.6;margin-bottom:16px">浏览和选择存储中的建筑文件需要此权限</div>' +
      '<div class="row"><button class="btn-s" onclick="var r=window.__previewPermissionResolve__;if(r){r(false)};closeModalOverlay(this)">取消</button>' +
      '<button class="btn" onclick="closeModalOverlay(this);requestPreviewPermission()">前往设置</button></div></div>';
    document.body.appendChild(overlay);
    window.__previewPermissionResolve__ = resolve;
  });
}

function requestPreviewPermission() {
  if (typeof android !== 'undefined' && android.requestStoragePermission) {
    android.requestStoragePermission();
  }
  var orig = window.__onStoragePermissionResult__;
  window.__onStoragePermissionResult__ = function(granted) {
    if (orig) orig(granted);
    if (window.__previewPermissionResolve__) {
      window.__previewPermissionResolve__(granted);
      window.__previewPermissionResolve__ = null;
    }
  };
}

var previewState = {
  step: 1,
  folder: '',
  recursive: false,
  files: [],
  filters: {},
  angles: [],
  output: {
    format: 'png',
    width: 1024,
    height: 1024,
    mode: 'folder',
    dir: '/sdcard/Download/Prism预览/',
    gif: false,
    saveConfig: false
  },
  generateTaskId: null
};

function RpreviewHTML() {
  setTimeout(function() {
    // 如果正在生成中，恢复进度显示
    if (previewState._generating && previewGenState) {
      previewResumeGenerate();
      return;
    }
    // 恢复之前的步骤（previewState 是全局的，按返回再进入时仍保留）
    var step = previewState.step || 1;
    renderPreviewStep(step);
  }, 50);
  return '<div id="preview-wrap" style="width:100%"><div id="preview-body"></div></div>';
}

function previewResumeGenerate() {
  var body = document.getElementById('preview-body');
  if (!body) return;
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div class="wz-section-title">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>' +
    '正在生成预览图</div>' +

    '<div class="card" style="margin-top:8px">' +
    '<div style="display:flex;justify-content:space-between;margin-bottom:6px">' +
    '<span style="font-size:12px;font-weight:700" id="pv-progress-text">0%</span>' +
    '<span class="dim" style="font-size:10px" id="pv-progress-count">0 / 0</span></div>' +
    '<div class="pg"><div id="pv-progress-bar" style="width:0%"></div></div></div>' +

    '<div class="card" style="margin-top:8px">' +
    '<div style="font-size:12px;font-weight:700;margin-bottom:6px">当前建筑</div>' +
    '<div style="font-size:13px" id="pv-current-building">等待中...</div>' +
    '<div class="dim" style="font-size:10px;margin-top:2px" id="pv-current-angle"></div></div>' +

    '<div class="card" style="margin-top:8px;padding:10px;display:none" id="pv-gif-status">' +
    '<div style="font-size:10px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:4px">GIF 后台合成</div>' +
    '<div id="pv-gif-status-text" style="font-size:10px;color:var(--phx-text-secondary);line-height:1.6"></div></div>' +

    '<div class="card" style="margin-top:8px;max-height:200px;overflow-y:auto;background:var(--phx-bg)">' +
    '<div style="font-size:11px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:4px">处理日志</div>' +
    '<div id="pv-log" style="font-size:10px;font-family:monospace;line-height:1.6"></div></div>' +

    '<div class="row" style="margin-top:16px">' +
    '<button class="btn-d" id="pv-cancel-btn" onclick="previewCancelGenerate()">取消</button></div></div>';

  updateProgressGen();
  if (previewGenState) {
    updateProgressBuilding(previewGenState.files[previewGenState.currentFileIdx]?.name?.replace(/\.[^.]+$/, '') || '');
  }
}

// 预览入口：先检查权限再打开子页面
async function previewOpen() {
  try {
    var ok = await previewCheckPermission();
    if (!ok) return;
    delete subPageCache['生成预览图'];
    openSubPage('生成预览图', RpreviewHTML());
  } catch(e) {
    console.error('previewOpen error:', e);
    T('打开预览失败: ' + e.message, 'e');
  }
}

function renderPreviewStep(step) {
  previewState.step = step;
  var body = document.getElementById('preview-body');
  if (!body) return;
  switch (step) {
    case 1: renderStep1(); break;
    case 2: renderStep2(); break;
    case 3: renderStep3(); break;
    case 4: renderStep4(); break;
  }
}

function renderStep1() {
  var body = document.getElementById('preview-body');
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div class="wz-section-title">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/></svg>' +
    '选择文件夹</div>' +
    '<div class="dim" style="margin-bottom:12px">选择包含建筑文件的文件夹，系统将自动扫描</div>' +
    '<div class="pick" onclick="previewPickFolder()" style="margin:0">' +
    '<div class="pi"><svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/></svg></div>' +
    '<div id="pv-folder-txt">点击选择文件夹</div></div>' +
    '<div id="pv-folder-info" class="gone" style="margin-top:8px">' +
    '<div class="stats" style="grid-template-columns:1fr"><div class="s"><div class="n" id="pv-folder-path"></div><div class="l">已选路径</div></div></div></div>' +
    '<div class="wz-dot-bar" style="padding:8px 0 12px"><span class="wz-dot active"></span><span class="wz-dot"></span><span class="wz-dot"></span><span class="wz-dot"></span></div>' +
    '<div class="row"><button class="btn-s" onclick="closeSubPage()">取消</button><button class="btn" id="pv-step1-next" onclick="previewStep1Next()" disabled>下一步 →</button></div></div>';
}

function previewPickFolder() {
  if (typeof showFilePicker === 'function') {
    showFilePicker('/storage/emulated/0', 'building', function(path) {
      previewState.folder = path;
      updateFolderDisplay();
    }, 'folder');
  } else if (typeof android !== 'undefined' && android.pickFile) {
    var fw = function(p) {
      if (p) {
        previewState.folder = p.substring(0, p.lastIndexOf('/'));
        updateFolderDisplay();
      }
    };
    window.__onFilePicked__ = fw;
    android.pickFile('building');
  }
}

function updateFolderDisplay() {
  var txt = document.getElementById('pv-folder-txt');
  var info = document.getElementById('pv-folder-info');
  var path = document.getElementById('pv-folder-path');
  var next = document.getElementById('pv-step1-next');
  if (txt) txt.textContent = '已选择: ' + previewState.folder.split('/').pop();
  if (info) info.classList.remove('gone');
  if (path) path.textContent = previewState.folder;
  if (next) next.disabled = false;
}

function previewStep1Next() {
  if (!previewState.folder) return;
  renderPreviewStep(2);
}

function renderStep2() {
  var body = document.getElementById('preview-body');
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div class="wz-topbar">' +
    '<button onclick="renderPreviewStep(1)"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg> 返回</button>' +
    '</div>' +
    '<div class="wz-section-title">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="8" y1="6" x2="21" y2="6"/><line x1="8" y1="12" x2="21" y2="12"/><line x1="8" y1="18" x2="21" y2="18"/><line x1="3" y1="6" x2="3.01" y2="6"/><line x1="3" y1="12" x2="3.01" y2="12"/><line x1="3" y1="18" x2="3.01" y2="18"/></svg>' +
    '扫描选项</div>' +
    '<div class="card" style="padding:12px 14px;margin-bottom:10px;font-size:12px;font-weight:700;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' +
    '<span style="color:var(--phx-primary)">' + previewState.folder.split('/').pop() + '</span>' +
    '<div class="dim" style="font-size:10px;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + previewState.folder + '</div></div>' +
    '<div id="pv-filter-list"></div>' +
    '<div id="pv-scan-result" class="card" style="text-align:center;margin-top:12px">' +
    '<div class="n" style="font-size:18px;font-weight:800;color:var(--phx-primary)" id="pv-file-count">0</div>' +
    '<div class="l" style="font-size:11px;color:var(--phx-text-secondary)">个建筑文件</div></div>' +
    '<div id="pv-file-list" style="margin-top:8px;max-height:300px;overflow-y:auto"></div>' +
    '<div class="wz-dot-bar" style="padding:8px 0 12px"><span class="wz-dot"></span><span class="wz-dot active"></span><span class="wz-dot"></span><span class="wz-dot"></span></div>' +
    '<div class="row"><button class="btn-s" onclick="renderPreviewStep(1)">上一步</button><button class="btn" onclick="renderPreviewStep(3)">下一步 →</button></div></div>';

  renderFilterList();
}

// 筛选条件定义
var previewFilters = [
  {id: 'scanMode',      label: '扫描方式',     summary: '递归扫描',   type: 'single'},
  {id: 'nameInclude',   label: '文件名包含',   summary: '未设置',   type: 'text'},
  {id: 'nameExclude',   label: '文件名不包含', summary: '未设置',   type: 'text'},
  {id: 'langFilter',    label: '语言过滤',     summary: '不限制',   type: 'single'},
  {id: 'sizeRange',     label: '文件大小',     summary: '不限制',   type: 'range'},
  {id: 'formatFilter',  label: '文件格式',     summary: '全部',     type: 'multi'}
];

function renderFilterList() {
  var list = document.getElementById('pv-filter-list');
  if (!list) return;
  var html = '';
  for (var i = 0; i < previewFilters.length; i++) {
    var f = previewFilters[i];
    // 从 previewState 获取实际值
    var summary = getFilterSummary(f.id);
    html += '<div class="card" style="cursor:pointer;padding:14px 16px;display:flex;align-items:center;justify-content:space-between;margin-bottom:8px" onclick="previewOpenFilter(\'' + f.id + '\')">' +
      '<div><div style="font-size:13px;font-weight:700">' + f.label + '</div>' +
      '<div class="dim" style="font-size:10px;margin-top:2px" id="pv-filter-summary-' + f.id + '">' + summary + '</div></div>' +
      '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--phx-text-disabled)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="9 18 15 12 9 6"/></svg></div>';
  }
  list.innerHTML = html;
  // 自动扫描
  previewDoScan();
}

function getFilterSummary(id) {
  switch (id) {
    case 'scanMode': return previewState.recursive ? '递归扫描' : '只扫当前';
    case 'nameInclude': {
      var kw = previewState.filters.name_include || [];
      return kw.length ? kw.join(', ') : '未设置';
    }
    case 'nameExclude': {
      var kw = previewState.filters.name_exclude || [];
      return kw.length ? kw.join(', ') : '未设置';
    }
    case 'langFilter': {
      var labels = {none: '不限制', chinese_only: '必须含中文', no_chinese: '必须不含中文'};
      return labels[previewState.filters.lang_filter] || '不限制';
    }
    case 'sizeRange': {
      var min = previewState.filters.size_min || 0;
      var max = previewState.filters.size_max || 0;
      if (!min && !max) return '不限制';
      var parts = [];
      if (min) parts.push((min / 1024) + (min >= 1048576 ? 'MB' : 'KB'));
      if (min && max) parts.push('～');
      if (max) parts.push((max / 1024) + (max >= 1048576 ? 'MB' : 'KB'));
      return parts.join(' ') || '不限制';
    }
    case 'formatFilter': {
      var fmts = previewState.filters.formats || [];
      if (!fmts.length) return '全部';
      var labels = {'.mcstructure':'MCStructure','.schematic':'Schematic','.schem':'Schem','.mcworld':'MCWorld'};
      return fmts.map(function(f) { return labels[f] || f; }).join(', ');
    }
    default: return '未设置';
  }
}

function previewDoScan() {
  var body = {
    path: previewState.folder,
    recursive: previewState.recursive,
    filters: previewState.filters
  };
  A('POST', '/api/preview/scan', body).then(function(r) {
    if (r.ok) {
      previewState.files = r.files || [];
      var count = document.getElementById('pv-file-count');
      if (count) count.textContent = previewState.files.length;
      renderFileList();
    }
  });
}

function renderFileList() {
  var list = document.getElementById('pv-file-list');
  if (!list) return;
  var files = previewState.files || [];
  if (!files.length) {
    list.innerHTML = '<div class="dim" style="text-align:center;padding:12px">无匹配文件</div>';
    return;
  }
  var html = '<div style="font-size:11px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:6px">已选文件（点击可移除）</div>';
  for (var i = 0; i < files.length; i++) {
    var f = files[i];
    var name = f.name || f.path.split('/').pop() || '';
    var extLabel = {'.mcstructure':'MC','.schematic':'SC','.schem':'SC','.mcworld':'WLD'}[f.format] || (f.format || '').replace('.', '').substring(0,3).toUpperCase();
    html += '<div class="fp-item" style="cursor:pointer;overflow:hidden" data-idx="' + i + '">' +
      '<div class="fp-icon fp-icon-' + (f.format || 'file').replace('.', '') + '" style="font-size:9px;width:24px;height:24px;border-radius:4px;flex-shrink:0">' + extLabel + '</div>' +
      '<div class="fp-info" style="min-width:0;flex:1"><div class="fp-name" style="font-size:12px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">' + name + '</div>' +
      '<div class="fp-meta">' + (f.size ? formatSize(f.size) : '') + '</div></div>' +
      '<button class="q-item-del" style="flex-shrink:0" onclick="previewRemoveFile(' + i + ')"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button></div>';
  }
  list.innerHTML = html;
}

function previewRemoveFile(idx) {
  previewState.files.splice(idx, 1);
  var count = document.getElementById('pv-file-count');
  if (count) count.textContent = previewState.files.length;
  renderFileList();
}

function previewOpenFilter(id) {
  switch (id) {
    case 'scanMode': previewFilterSheetScanMode(); break;
    case 'nameInclude': previewFilterSheetText('nameInclude', '文件名包含', '输入关键词，多个用逗号分隔', previewState.filters.name_include || []); break;
    case 'nameExclude': previewFilterSheetText('nameExclude', '文件名不包含', '输入关键词，多个用逗号分隔', previewState.filters.name_exclude || []); break;
    case 'langFilter': previewFilterSheetLang(); break;
    case 'sizeRange': previewFilterSheetSize(); break;
    case 'formatFilter': previewFilterSheetFormats(); break;
  }
}

function previewFilterSheetScanMode() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">扫描方式</div>' +
    '<div class="bs-body">' +
    '<div class="bs-option" data-value="current"><div class="bs-dot"></div><span class="bs-label">只扫当前文件夹</span><span class="dim" style="font-size:10px">不进入子文件夹</span></div>' +
    '<div class="bs-option" data-value="recursive"><div class="bs-dot"></div><span class="bs-label">递归扫描全部子文件夹</span><span class="dim" style="font-size:10px">扫描所有子文件夹</span></div>' +
    '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="closeBsOverlay(this)">确定</button></div></div>';
  document.body.appendChild(overlay);
  var sel = previewState.recursive ? 'recursive' : 'current';
  overlay.querySelectorAll('.bs-option').forEach(function(o) {
    if (o.dataset.value === sel) o.classList.add('active');
    o.addEventListener('click', function() {
      overlay.querySelectorAll('.bs-option').forEach(function(x) { x.classList.remove('active'); });
      this.classList.add('active');
      previewState.recursive = this.dataset.value === 'recursive';
      previewFilterUpdateSummary('scanMode', this.dataset.value === 'recursive' ? '递归扫描' : '只扫当前');
      previewDoScan();
    });
  });
}

function previewFilterSheetText(id, title, placeholder, current) {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">' + title + '</div>' +
    '<div class="bs-body" style="padding:8px 18px">' +
    '<div class="dim" style="font-size:11px;margin-bottom:8px">' + placeholder + '</div>' +
    '<input id="pv-filter-input-' + id + '" type="text" placeholder="关键词1, 关键词2, ..." style="margin:0" value="' + ((current && current.length) ? current.join(', ') : '') + '">' +
    '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewFilterTextConfirm(\'' + id + '\')">确定</button></div></div>';
  document.body.appendChild(overlay);
}

function previewFilterTextConfirm(id) {
  var input = document.getElementById('pv-filter-input-' + id);
  if (!input) return;
  var val = input.value.trim();
  var keywords = val ? val.split(',').map(function(s) { return s.trim(); }).filter(function(s) { return s; }) : [];
  if (id === 'nameInclude') { previewState.filters.name_include = keywords; }
  else if (id === 'nameExclude') { previewState.filters.name_exclude = keywords; }
  previewFilterUpdateSummary(id, keywords.length ? keywords.join(', ') : '未设置');
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
  previewDoScan();
}

function previewFilterSheetLang() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var current = previewState.filters.lang_filter || 'none';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">语言过滤</div>' +
    '<div class="bs-body">' +
    '<div class="bs-option" data-value="none"><div class="bs-dot"></div><span class="bs-label">不限制</span></div>' +
    '<div class="bs-option" data-value="chinese_only"><div class="bs-dot"></div><span class="bs-label">必须含中文</span></div>' +
    '<div class="bs-option" data-value="no_chinese"><div class="bs-dot"></div><span class="bs-label">必须不含中文</span></div>' +
    '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="closeBsOverlay(this)">确定</button></div></div>';
  document.body.appendChild(overlay);
  overlay.querySelectorAll('.bs-option').forEach(function(o) {
    if (o.dataset.value === current) o.classList.add('active');
    o.addEventListener('click', function() {
      overlay.querySelectorAll('.bs-option').forEach(function(x) { x.classList.remove('active'); });
      this.classList.add('active');
      previewState.filters.lang_filter = this.dataset.value;
      var labels = {none: '不限制', chinese_only: '必须含中文', no_chinese: '必须不含中文'};
      previewFilterUpdateSummary('langFilter', labels[this.dataset.value] || '不限制');
      previewDoScan();
    });
  });
}

function previewFilterSheetSize() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var min = previewState.filters.size_min || '';
  var max = previewState.filters.size_max || '';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">文件大小范围</div>' +
    '<div class="bs-body" style="padding:8px 18px">' +
    '<label>最小值</label><div class="row"><input id="pv-size-min" type="number" placeholder="0" value="' + min + '" style="margin:0"><select id="pv-size-min-unit" style="width:80px;flex:none"><option value="KB">KB</option><option value="MB">MB</option></select></div>' +
    '<label>最大值</label><div class="row"><input id="pv-size-max" type="number" placeholder="不限" value="' + max + '" style="margin:0"><select id="pv-size-max-unit" style="width:80px;flex:none"><option value="KB">KB</option><option value="MB">MB</option></select></div>' +
    '<label class="lbl-cb" style="margin-top:8px"><input type="checkbox" id="pv-size-unlimited" onchange="var els=document.querySelectorAll(\'#pv-size-min,#pv-size-max,#pv-size-min-unit,#pv-size-max-unit\');for(var i=0;i<els.length;i++){els[i].disabled=this.checked}"' + (!min && !max ? ' checked' : '') + '><span class="tgl"></span> 不限制大小</label>' +
    '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewFilterSizeConfirm()">确定</button></div></div>';
  document.body.appendChild(overlay);
}

function previewFilterSizeConfirm() {
  var unlimited = document.getElementById('pv-size-unlimited');
  if (unlimited && unlimited.checked) {
    previewState.filters.size_min = 0;
    previewState.filters.size_max = 0;
    previewFilterUpdateSummary('sizeRange', '不限制');
  } else {
    var rawMin = parseInt(document.getElementById('pv-size-min').value) || 0;
    var minUnit = document.getElementById('pv-size-min-unit').value;
    var rawMax = parseInt(document.getElementById('pv-size-max').value) || 0;
    var maxUnit = document.getElementById('pv-size-max-unit').value;
    var min = rawMin, max = rawMax;
    if (minUnit === 'MB') min *= 1024;
    if (maxUnit === 'MB') max *= 1024;
    previewState.filters.size_min = min * 1024;
    previewState.filters.size_max = max * 1024;
    var summary = '';
    if (rawMin) summary += rawMin + (minUnit === 'MB' ? 'MB' : 'KB');
    if (rawMin && rawMax) summary += ' ～ ';
    if (rawMax) summary += rawMax + (maxUnit === 'MB' ? 'MB' : 'KB');
    previewFilterUpdateSummary('sizeRange', summary || '不限制');
  }
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
  previewDoScan();
}

function previewFilterSheetFormats() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var current = previewState.filters.formats || [];
  var allFormats = [{v:'.mcstructure',l:'MCStructure 结构文件'},{v:'.schematic',l:'Schematic 示意图'},{v:'.schem',l:'旧版 Schematic'},{v:'.mcworld',l:'MCWorld 世界文件'}];
  var itemsHtml = '';
  for (var i = 0; i < allFormats.length; i++) {
    var f = allFormats[i];
    var checked = (!current.length || current.indexOf(f.v) >= 0) ? 'checked' : '';
    itemsHtml += '<label class="lbl-cb" style="padding:10px 18px;margin:0"><input type="checkbox" class="pv-format-cb" value="' + f.v + '" ' + checked + '><span class="tgl"></span> ' + f.l + '</label>';
  }
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">文件格式</div>' +
    '<div class="bs-body">' + itemsHtml + '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewFilterFormatsConfirm()">确定</button></div></div>';
  document.body.appendChild(overlay);
}

function previewFilterFormatsConfirm() {
  var cbs = document.querySelectorAll('.pv-format-cb:checked');
  var formats = [];
  cbs.forEach(function(cb) { formats.push(cb.value); });
  previewState.filters.formats = formats;
  var labels = {'.mcstructure':'MCStructure','.schematic':'Schematic','.schem':'Schem','.mcworld':'MCWorld'};
  var summary = formats.length ? formats.map(function(f) { return labels[f] || f; }).join(', ') : '全部';
  previewFilterUpdateSummary('formatFilter', summary);
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
  previewDoScan();
}

function previewFilterUpdateSummary(id, text) {
  var el = document.getElementById('pv-filter-summary-' + id);
  if (el) el.textContent = text;
}

function renderStep3() {
  // 内存清理：确保之前的三维场景已释放
  previewCleanup3D();

  var body = document.getElementById('preview-body');
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div class="wz-topbar">' +
    '<button onclick="renderPreviewStep(2)"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg> 返回</button>' +
    '</div>' +
    '<div class="wz-section-title">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2L2 7l10 5 10-5-10-5z"/><path d="M2 17l10 5 10-5"/><path d="M2 12l10 5 10-5"/></svg>' +
    '镜头配置</div>' +
    '<div id="pv-angle-list" class="card" style="min-height:60px"></div>' +
    '<button class="btn-s" onclick="previewShowAnglePicker()" style="width:100%;margin-top:8px">' +
    '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align:middle;margin-right:4px"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg> 添加镜头</button>' +
    '<div style="margin-top:12px;padding:12px;background:var(--phx-bg);border-radius:var(--phx-radius-sm)">' +
    '<label class="lbl-cb"><input type="checkbox" id="pv-gen-gif" onchange="previewState.genGif=this.checked"' + (previewState.genGif ? ' checked' : '') + '><span class="tgl"></span> 同时生成 GIF 环绕动画</label></div>' +
    '<div class="wz-dot-bar" style="padding:8px 0 12px"><span class="wz-dot"></span><span class="wz-dot"></span><span class="wz-dot active"></span><span class="wz-dot"></span></div>' +
    '<div class="row"><button class="btn-s" onclick="renderPreviewStep(2)">上一步</button><button class="btn" onclick="renderPreviewStep(4)">下一步 →</button></div></div>';

  renderAngleList();
}

function renderAngleList() {
  var list = document.getElementById('pv-angle-list');
  if (!list) return;
  if (!previewState.angles.length) {
    list.innerHTML = '<div style="text-align:center;padding:20px;color:var(--phx-text-secondary);font-size:12px">尚未添加镜头，点击下方按钮添加</div>';
    return;
  }
  var html = '<div style="font-size:11px;color:var(--phx-text-secondary);font-weight:700;margin-bottom:8px">已选镜头（' + previewState.angles.length + ' 个）</div>';
  for (var i = 0; i < previewState.angles.length; i++) {
    var a = previewState.angles[i];
    html += '<div style="display:flex;align-items:center;gap:8px;padding:8px 0;border-bottom:1px solid var(--phx-border-light)" data-idx="' + i + '">' +
      '<span style="cursor:grab;color:var(--phx-text-disabled)">⠿</span>' +
      '<span style="flex:1;font-size:13px;font-weight:600">' + a.label + '</span>' +
      '<button class="q-item-del" onclick="previewRemoveAngle(' + i + ')"><svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg></button></div>';
  }
  list.innerHTML = html;
}

function previewShowAnglePicker() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">选择镜头类型</div>' +
    '<div class="bs-body">' +
    '<div class="bs-option" onclick="closeBsOverlay(this);previewShowAngleCategory(\'direction\',\'正方向\',[\'正面\',\'背面\',\'左面\',\'右面\'],[\'front\',\'back\',\'left\',\'right\'])"><div class="bs-dot"></div><span class="bs-label">正方向</span><span class="dim">前 / 后 / 左 / 右</span></div>' +
    '<div class="bs-option" onclick="closeBsOverlay(this);previewShowAngleCategory(\'corner\',\'对角\',[\'东北角\',\'东南角\',\'西北角\',\'西南角\'],[\'northeast\',\'southeast\',\'northwest\',\'southwest\'])"><div class="bs-dot"></div><span class="bs-label">对角</span><span class="dim">4 个等轴测角</span></div>' +
    '<div class="bs-option" onclick="closeBsOverlay(this);previewShowAngleCategory(\'octo\',\'八角\',[\'0°\',\'45°\',\'90°\',\'135°\',\'180°\',\'225°\',\'270°\',\'315°\'],[\'0\',\'45\',\'90\',\'135\',\'180\',\'225\',\'270\',\'315\'])"><div class="bs-dot"></div><span class="bs-label">八角</span><span class="dim">45° 增量 8 个方向</span></div>' +
    '<div class="bs-option" onclick="closeBsOverlay(this);previewShowAngleCategory(\'birdseye\',\'鸟瞰\',[\'正上方俯视\'],[\'top\'])"><div class="bs-dot"></div><span class="bs-label">鸟瞰</span><span class="dim">正上方俯视</span></div>' +
    '<div class="bs-option" onclick="closeBsOverlay(this);previewShowAngleCustom()"><div class="bs-dot"></div><span class="bs-label">自定义</span><span class="dim">自由拖拽调节角度</span></div></div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button></div></div>';
  document.body.appendChild(overlay);
}

function previewShowAngleCategory(type, title, labels, values) {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var itemsHtml = '';
  for (var i = 0; i < labels.length; i++) {
    itemsHtml += '<label class="lbl-cb" style="padding:12px 18px;margin:0;border-bottom:1px solid var(--phx-border-light)">' +
      '<input type="checkbox" class="pv-angle-cb" value="' + values[i] + '" data-label="' + labels[i] + '"><span class="tgl"></span> ' + labels[i] + '</label>';
  }
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title" style="display:flex;align-items:center;gap:8px">' +
    '<button onclick="closeBsOverlay(this);previewShowAnglePicker()" style="background:none;border:none;padding:0;cursor:pointer;color:var(--phx-text-secondary)">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg></button>' +
    title + '</div>' +
    '<div class="bs-body">' + itemsHtml + '</div>' +
    '<div class="bs-footer">' +
    '<button class="bs-btn bs-btn-cancel" onclick="previewShowAnglePicker()">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewAngleCategoryConfirm()">确定添加</button></div></div>';
  document.body.appendChild(overlay);
}

function previewAngleCategoryConfirm() {
  var cbs = document.querySelectorAll('.pv-angle-cb:checked');
  cbs.forEach(function(cb) {
    previewState.angles.push({
      type: 'preset',
      value: cb.value,
      label: cb.dataset.label
    });
  });
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
  renderAngleList();
}

function previewShowAngleCustom() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title" style="display:flex;align-items:center;gap:8px">' +
    '<button onclick="closeBsOverlay(this);previewCleanupAnglePreview();previewShowAnglePicker()" style="background:none;border:none;padding:0;cursor:pointer;color:var(--phx-text-secondary)">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg></button>' +
    '自定义角度</div>' +
    '<div class="bs-body" style="padding:8px 18px">' +
    '<div id="pv-angle-preview-3d" style="width:100%;height:220px;border-radius:12px;border:2px solid var(--phx-border-light);margin-bottom:12px;overflow:hidden"></div>' +
    '<label>名称</label><input id="pv-custom-angle-name" type="text" placeholder="自定义角度" value="自定义" style="margin:0">' +
    '<label>水平角</label><div class="row"><input id="pv-custom-yaw" type="number" value="45" style="margin:0" onchange="if(window.__previewAngleUpdate)window.__previewAngleUpdate()"><button class="btn-s" style="width:auto;padding:11px 16px;flex:none" onclick="var i=document.getElementById(\'pv-custom-yaw\');i.value=parseInt(i.value||0)-5;if(window.__previewAngleUpdate)window.__previewAngleUpdate()">-5°</button><button class="btn-s" style="width:auto;padding:11px 16px;flex:none" onclick="var i=document.getElementById(\'pv-custom-yaw\');i.value=parseInt(i.value||0)+5;if(window.__previewAngleUpdate)window.__previewAngleUpdate()">+5°</button></div>' +
    '<label>仰角</label><div class="row"><input id="pv-custom-pitch" type="number" value="30" style="margin:0" onchange="if(window.__previewAngleUpdate)window.__previewAngleUpdate()"><button class="btn-s" style="width:auto;padding:11px 16px;flex:none" onclick="var i=document.getElementById(\'pv-custom-pitch\');i.value=parseInt(i.value||0)-5;if(window.__previewAngleUpdate)window.__previewAngleUpdate()">-5°</button><button class="btn-s" style="width:auto;padding:11px 16px;flex:none" onclick="var i=document.getElementById(\'pv-custom-pitch\');i.value=parseInt(i.value||0)+5;if(window.__previewAngleUpdate)window.__previewAngleUpdate()">+5°</button></div>' +
    '<label>距离</label><div class="row"><input id="pv-custom-dist" type="number" value="200" style="margin:0" onchange="if(window.__previewAngleUpdate)window.__previewAngleUpdate()"><button class="btn-s" style="width:auto;padding:11px 16px;flex:none" onclick="var i=document.getElementById(\'pv-custom-dist\');i.value=Math.max(50,parseInt(i.value||200)-20);if(window.__previewAngleUpdate)window.__previewAngleUpdate()">-</button><button class="btn-s" style="width:auto;padding:11px 16px;flex:none" onclick="var i=document.getElementById(\'pv-custom-dist\');i.value=Math.min(500,parseInt(i.value||200)+20);if(window.__previewAngleUpdate)window.__previewAngleUpdate()">+</button></div>' +
    '</div>' +
    '<div class="bs-footer">' +
    '<button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this);previewCleanupAnglePreview();previewShowAnglePicker()">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewAngleCustomConfirm()">添加此角度</button></div></div>';
  document.body.appendChild(overlay);

  // 初始化 3D 预览
  setTimeout(function() { previewInitAnglePreview('pv-angle-preview-3d'); }, 100);
}

function previewAngleCustomConfirm() {
  var name = document.getElementById('pv-custom-angle-name').value || '自定义';
  var yaw = parseInt(document.getElementById('pv-custom-yaw').value) || 45;
  var pitch = parseInt(document.getElementById('pv-custom-pitch').value) || 30;
  var dist = parseInt(document.getElementById('pv-custom-dist').value) || 200;
  previewState.angles.push({
    type: 'custom',
    yaw: yaw,
    pitch: pitch,
    dist: dist,
    label: name + ' ' + yaw + '°'
  });
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
  renderAngleList();
}

function previewRemoveAngle(idx) {
  previewState.angles.splice(idx, 1);
  renderAngleList();
}

function previewCleanup3D() {
  // 清理之前的三维渲染资源
  activeRenderers.forEach(function(kill) { try { kill(); } catch(e) {} });
  activeRenderers = [];
}

// 3D 角度预览场景
var previewAngleScene = null;
var previewAngleRenderer = null;
var previewAngleAnimId = null;

function previewInitAnglePreview(containerId) {
  var container = document.getElementById(containerId);
  if (!container || typeof THREE === 'undefined') return;

  // 清理之前的场景
  previewCleanupAnglePreview();

  var W = container.clientWidth || 280;
  var H = 200;

  var scene = new THREE.Scene();
  scene.background = new THREE.Color(0xf0eee8);

  // 相机
  var camera = new THREE.PerspectiveCamera(40, W / H, 1, 100);
  camera.position.set(8, 6, 8);
  camera.lookAt(0, 0, 0);

  // 渲染器
  var renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
  renderer.setSize(W, H);
  renderer.setPixelRatio(1);
  container.innerHTML = '';
  container.appendChild(renderer.domElement);

  // 灯光
  scene.add(new THREE.AmbientLight(0x888888));
  var dl = new THREE.DirectionalLight(0xffffff, 0.8);
  dl.position.set(10, 15, 10);
  scene.add(dl);
  var dl2 = new THREE.DirectionalLight(0xffffff, 0.3);
  dl2.position.set(-5, 5, -5);
  scene.add(dl2);

  // 迷你建筑轮廓（简化的房子 + 方块群）
  var buildingGroup = new THREE.Group();

  // 主体
  var bodyMat = new THREE.MeshPhongMaterial({ color: 0xd48a0e, specular: 0x222222, shininess: 10 });
  var body = new THREE.Mesh(new THREE.BoxGeometry(3, 2, 2), bodyMat);
  body.position.set(0, 1, 0);
  buildingGroup.add(body);

  // 屋顶
  var roofMat = new THREE.MeshPhongMaterial({ color: 0xb8780b, specular: 0x222222, shininess: 5 });
  var roof = new THREE.Mesh(new THREE.ConeGeometry(2.5, 1.5, 4), roofMat);
  roof.position.set(0, 2.75, 0);
  roof.rotation.y = Math.PI / 4;
  buildingGroup.add(roof);

  // 烟囱
  var chimneyMat = new THREE.MeshPhongMaterial({ color: 0x8b6914 });
  var chimney = new THREE.Mesh(new THREE.BoxGeometry(0.6, 1.2, 0.6), chimneyMat);
  chimney.position.set(1.2, 2.6, 0.5);
  buildingGroup.add(chimney);

  // 门
  var doorMat = new THREE.MeshPhongMaterial({ color: 0x6b4c2a });
  var door = new THREE.Mesh(new THREE.BoxGeometry(0.8, 1.2, 0.1), doorMat);
  door.position.set(0, 0.6, 1.05);
  buildingGroup.add(door);

  // 窗户（发光）
  var windowMat = new THREE.MeshPhongMaterial({ color: 0x80b0f0, emissive: 0x204060, emissiveIntensity: 0.3 });
  var window1 = new THREE.Mesh(new THREE.BoxGeometry(0.6, 0.6, 0.1), windowMat);
  window1.position.set(-0.8, 1.2, 1.05);
  buildingGroup.add(window1);
  var window2 = new THREE.Mesh(new THREE.BoxGeometry(0.6, 0.6, 0.1), windowMat);
  window2.position.set(0.8, 1.2, 1.05);
  buildingGroup.add(window2);

  // 小方块（代表周围建筑）
  var blockMat = new THREE.MeshPhongMaterial({ color: 0xc0a878, specular: 0x222222, shininess: 5 });
  for (var i = 0; i < 3; i++) {
    var b = new THREE.Mesh(new THREE.BoxGeometry(0.8, 0.5 + i * 0.3, 0.8), blockMat);
    b.position.set(-2.5 + i * 1.2, 0.25 + i * 0.15, -1.5);
    buildingGroup.add(b);
  }

  // 地面网格
  var gridHelper = new THREE.GridHelper(8, 8, 0xddd6c8, 0xe8e2d6);
  gridHelper.position.y = -0.01;
  scene.add(gridHelper);

  scene.add(buildingGroup);

  // 更新相机位置
  var currentYaw = 45;
  var currentPitch = 30;
  var currentDist = 200;

  function updateCamera() {
    var radYaw = currentYaw * Math.PI / 180;
    var radPitch = currentPitch * Math.PI / 180;
    var d = currentDist / 30;
    camera.position.set(
      d * Math.sin(radYaw) * Math.cos(radPitch),
      d * Math.sin(radPitch) + 2,
      d * Math.cos(radYaw) * Math.cos(radPitch)
    );
    camera.lookAt(0, 1, 0);
  }

  updateCamera();

  // 同步输入框
  function syncInputs() {
    var yawEl = document.getElementById('pv-custom-yaw');
    var pitchEl = document.getElementById('pv-custom-pitch');
    var distEl = document.getElementById('pv-custom-dist');
    if (yawEl) yawEl.value = currentYaw;
    if (pitchEl) pitchEl.value = currentPitch;
    if (distEl) distEl.value = currentDist;
  }
  syncInputs();

  // 拖拽控制
  var isDragging = false;
  var lastX = 0, lastY = 0;
  renderer.domElement.style.touchAction = 'none';
  renderer.domElement.addEventListener('pointerdown', function(e) {
    isDragging = true;
    lastX = e.clientX;
    lastY = e.clientY;
    renderer.domElement.setPointerCapture(e.pointerId);
  });
  renderer.domElement.addEventListener('pointermove', function(e) {
    if (!isDragging) return;
    currentYaw += (e.clientX - lastX) * 0.5;
    currentPitch = Math.max(5, Math.min(85, currentPitch - (e.clientY - lastY) * 0.5));
    lastX = e.clientX;
    lastY = e.clientY;
    updateCamera();
    syncInputs();
  });
  renderer.domElement.addEventListener('pointerup', function() { isDragging = false; });

  // 动画循环
  function animate() {
    previewAngleAnimId = requestAnimationFrame(animate);
    renderer.render(scene, camera);
  }
  animate();

  // 保存引用
  previewAngleScene = { scene: scene, camera: camera, renderer: renderer, group: buildingGroup, updateCamera: updateCamera, syncInputs: syncInputs };
  previewAngleRenderer = renderer;

  // 暴露更新函数给按钮
  window.__previewAngleUpdate = function() {
    var yawEl = document.getElementById('pv-custom-yaw');
    var pitchEl = document.getElementById('pv-custom-pitch');
    var distEl = document.getElementById('pv-custom-dist');
    if (yawEl) currentYaw = parseInt(yawEl.value) || 45;
    if (pitchEl) currentPitch = parseInt(pitchEl.value) || 30;
    if (distEl) currentDist = parseInt(distEl.value) || 200;
    updateCamera();
  };
}

function previewCleanupAnglePreview() {
  if (previewAngleAnimId) {
    cancelAnimationFrame(previewAngleAnimId);
    previewAngleAnimId = null;
  }
  if (previewAngleRenderer) {
    previewAngleRenderer.dispose();
    previewAngleRenderer = null;
  }
  previewAngleScene = null;
}

function renderStep4() {
  previewCleanupAnglePreview();

  // 从 previewState 读取复选框状态
  var genGif = previewState.genGif || false;
  var gifFrames = genGif ? 36 : 0;
  var totalImages = (previewState.angles.length + gifFrames || 1) * previewState.files.length;

  var body = document.getElementById('preview-body');
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div class="wz-topbar">' +
    '<button onclick="renderPreviewStep(3)"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"/><polyline points="12 19 5 12 12 5"/></svg> 返回</button>' +
    '</div>' +
    '<div class="wz-section-title">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>' +
    '输出设置</div>' +

    '<div class="card" style="cursor:pointer;padding:14px 16px;display:flex;align-items:center;justify-content:space-between;margin-bottom:8px" onclick="previewShowFormatPicker()">' +
    '<div><div style="font-size:13px;font-weight:700">图片格式</div><div class="dim" style="font-size:10px;margin-top:2px" id="pv-format-summary">PNG</div></div>' +
    '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--phx-text-disabled)" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg></div>' +

    '<div class="card" style="cursor:pointer;padding:14px 16px;display:flex;align-items:center;justify-content:space-between;margin-bottom:8px" onclick="previewShowResolutionPicker()">' +
    '<div><div style="font-size:13px;font-weight:700">分辨率</div><div class="dim" style="font-size:10px;margin-top:2px" id="pv-resolution-summary">1024×1024</div></div>' +
    '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--phx-text-disabled)" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg></div>' +

    '<div class="card" style="cursor:pointer;padding:14px 16px;display:flex;align-items:center;justify-content:space-between;margin-bottom:8px" onclick="previewShowOutputModePicker()">' +
    '<div><div style="font-size:13px;font-weight:700">输出方式</div><div class="dim" style="font-size:10px;margin-top:2px" id="pv-mode-summary">按建筑分文件夹</div></div>' +
    '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--phx-text-disabled)" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg></div>' +

    '<div class="card" style="cursor:pointer;padding:14px 16px;display:flex;align-items:center;justify-content:space-between;margin-bottom:8px" onclick="previewShowDirPicker()">' +
    '<div><div style="font-size:13px;font-weight:700">输出目录</div><div class="dim" style="font-size:10px;margin-top:2px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;max-width:200px" id="pv-dir-summary">' + previewState.output.dir + '</div></div>' +
    '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--phx-text-disabled)" stroke-width="2"><polyline points="9 18 15 12 9 6"/></svg></div>' +

    '<div class="card" style="text-align:center;margin-top:12px;background:var(--phx-primary-bg);border-color:var(--phx-primary)">' +
    '<div style="font-size:14px;font-weight:800;color:var(--phx-primary)">共 ' + previewState.files.length + ' 个建筑 × ' + (previewState.angles.length + gifFrames || 1) + ' 个镜头 = ' + totalImages + ' 张图片</div></div>' +

    '<div class="wz-dot-bar" style="padding:8px 0 12px"><span class="wz-dot"></span><span class="wz-dot"></span><span class="wz-dot"></span><span class="wz-dot active"></span></div>' +
    '<div class="row"><button class="btn-s" onclick="renderPreviewStep(3)">上一步</button><button class="btn" onclick="previewStartGenerate()" style="background:var(--phx-success)">✨ 开始生成</button></div></div>';
}

function previewShowFormatPicker() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var current = previewState.output.format || 'png';
  var formats = [{v:'png',l:'PNG（无损）',d:'高保真，文件较大'},{v:'jpg',l:'JPG（有损压缩）',d:'文件小，适合分享'},{v:'webp',l:'WebP（现代格式）',d:'小体积，质量好'}];
  var itemsHtml = '';
  for (var i = 0; i < formats.length; i++) {
    itemsHtml += '<div class="bs-option' + (formats[i].v === current ? ' active' : '') + '" data-value="' + formats[i].v + '"><div class="bs-dot"></div><span class="bs-label">' + formats[i].l + '</span><span class="dim" style="font-size:10px">' + formats[i].d + '</span></div>';
  }
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">图片格式</div>' +
    '<div class="bs-body">' + itemsHtml + '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewFormatConfirm()">确定</button></div></div>';
  document.body.appendChild(overlay);
  overlay.querySelectorAll('.bs-option').forEach(function(o) {
    o.addEventListener('click', function() {
      overlay.querySelectorAll('.bs-option').forEach(function(x) { x.classList.remove('active'); });
      this.classList.add('active');
    });
  });
}

function previewFormatConfirm() {
  var active = document.querySelector('.bs-sheet .bs-option.active');
  if (active) {
    previewState.output.format = active.dataset.value;
    var labels = {png:'PNG',jpg:'JPG',webp:'WebP'};
    document.getElementById('pv-format-summary').textContent = labels[previewState.output.format] || previewState.output.format;
  }
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
}

function previewShowResolutionPicker() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var current = previewState.output.width + '×' + previewState.output.height;
  var resolutions = [{v:'512',l:'512×512'},{v:'1024',l:'1024×1024'},{v:'2048',l:'2048×2048'}];
  var itemsHtml = '';
  for (var i = 0; i < resolutions.length; i++) {
    itemsHtml += '<div class="bs-option' + (resolutions[i].v === String(previewState.output.width) ? ' active' : '') + '" data-value="' + resolutions[i].v + '"><div class="bs-dot"></div><span class="bs-label">' + resolutions[i].l + '</span></div>';
  }
  itemsHtml += '<div style="padding:12px 18px;border-top:1px solid var(--phx-border-light)">' +
    '<label>自定义</label><div class="row"><input id="pv-res-w" type="number" value="' + previewState.output.width + '" style="margin:0"><span style="align-self:center;color:var(--phx-text-secondary)">×</span><input id="pv-res-h" type="number" value="' + previewState.output.height + '" style="margin:0"></div></div>';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">分辨率</div>' +
    '<div class="bs-body">' + itemsHtml + '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewResolutionConfirm()">确定</button></div></div>';
  document.body.appendChild(overlay);
  overlay.querySelectorAll('.bs-option').forEach(function(o) {
    o.addEventListener('click', function() {
      overlay.querySelectorAll('.bs-option').forEach(function(x) { x.classList.remove('active'); });
      this.classList.add('active');
    });
  });
}

function previewResolutionConfirm() {
  var active = document.querySelector('.bs-sheet .bs-option.active');
  if (active) {
    previewState.output.width = parseInt(active.dataset.value);
    previewState.output.height = parseInt(active.dataset.value);
  } else {
    previewState.output.width = parseInt(document.getElementById('pv-res-w').value) || 1024;
    previewState.output.height = parseInt(document.getElementById('pv-res-h').value) || 1024;
  }
  document.getElementById('pv-resolution-summary').textContent = previewState.output.width + '×' + previewState.output.height;
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
}

function previewShowOutputModePicker() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  var current = previewState.output.mode || 'folder';
  var modes = [{v:'folder',l:'按建筑分文件夹',d:'每个建筑一个文件夹，里面放对应图片'},{v:'flat',l:'直出到目录（平铺）',d:'所有图片直接放在输出目录'},{v:'zip',l:'打包成 ZIP',d:'所有图片打包成一个 ZIP 文件'}];
  var itemsHtml = '';
  for (var i = 0; i < modes.length; i++) {
    itemsHtml += '<div class="bs-option' + (modes[i].v === current ? ' active' : '') + '" data-value="' + modes[i].v + '"><div class="bs-dot"></div><span class="bs-label">' + modes[i].l + '</span><span class="dim" style="font-size:10px">' + modes[i].d + '</span></div>';
  }
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">输出方式</div>' +
    '<div class="bs-body">' + itemsHtml + '</div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewOutputModeConfirm()">确定</button></div></div>';
  document.body.appendChild(overlay);
  overlay.querySelectorAll('.bs-option').forEach(function(o) {
    o.addEventListener('click', function() {
      overlay.querySelectorAll('.bs-option').forEach(function(x) { x.classList.remove('active'); });
      this.classList.add('active');
    });
  });
}

function previewOutputModeConfirm() {
  var active = document.querySelector('.bs-sheet .bs-option.active');
  if (active) {
    previewState.output.mode = active.dataset.value;
    var labels = {folder:'按建筑分文件夹',flat:'直出到目录（平铺）',zip:'打包成 ZIP'};
    document.getElementById('pv-mode-summary').textContent = labels[previewState.output.mode] || previewState.output.mode;
  }
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
}

function previewShowDirPicker() {
  var overlay = document.createElement('div'); overlay.className = 'bs-overlay';
  overlay.innerHTML =
    '<div class="bs-sheet">' +
    '<div class="bs-title">输出目录</div>' +
    '<div class="bs-body" style="padding:8px 18px">' +
    '<input id="pv-dir-input" type="text" value="' + previewState.output.dir + '" style="margin:0">' +
    '<div class="dim" style="font-size:10px;margin-top:6px">默认为 /sdcard/Download/Prism预览/</div></div>' +
    '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" onclick="closeBsOverlay(this)">取消</button>' +
    '<button class="bs-btn bs-btn-confirm" onclick="previewDirConfirm()">确定</button></div></div>';
  document.body.appendChild(overlay);
}

function previewDirConfirm() {
  var input = document.getElementById('pv-dir-input');
  if (input) {
    previewState.output.dir = input.value.trim() || '/sdcard/Download/Prism预览/';
    document.getElementById('pv-dir-summary').textContent = previewState.output.dir;
  }
  document.querySelectorAll('.bs-overlay').forEach(function(o) { closeOverlay(o); });
}

function previewStartGenerate() {
  if (!previewState.angles.length && !previewState.genGif) {
    T('请至少添加一个镜头或开启环绕动画', 'w');
    return;
  }

  // 收集输出设置
  previewState.output.format = previewState.output.format || 'png';
  previewState.output.mode = previewState.output.mode || 'folder';
  if (document.getElementById('pv-gen-gif')) previewState.genGif = document.getElementById('pv-gen-gif').checked;

  var genGif = previewState.genGif || false;

  var body = document.getElementById('preview-body');
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div class="wz-section-title">' +
    '<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>' +
    '正在生成预览图</div>' +

    '<div class="card" style="margin-top:8px">' +
    '<div style="display:flex;justify-content:space-between;margin-bottom:6px">' +
    '<span style="font-size:12px;font-weight:700" id="pv-progress-text">0%</span>' +
    '<span class="dim" style="font-size:10px" id="pv-progress-count">0 / 0</span></div>' +
    '<div class="pg"><div id="pv-progress-bar" style="width:0%"></div></div></div>' +

    '<div class="card" style="margin-top:8px">' +
    '<div style="font-size:12px;font-weight:700;margin-bottom:6px">当前建筑</div>' +
    '<div style="font-size:13px" id="pv-current-building">等待中...</div>' +
    '<div class="dim" style="font-size:10px;margin-top:2px" id="pv-current-angle"></div></div>' +

    '<div class="card" style="margin-top:8px;padding:10px;display:none" id="pv-gif-status">' +
    '<div style="font-size:10px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:4px">GIF 后台合成</div>' +
    '<div id="pv-gif-status-text" style="font-size:10px;color:var(--phx-text-secondary);line-height:1.6"></div></div>' +

    '<div class="card" style="margin-top:8px;max-height:200px;overflow-y:auto;background:var(--phx-bg)">' +
    '<div style="font-size:11px;font-weight:700;color:var(--phx-text-secondary);margin-bottom:4px">处理日志</div>' +
    '<div id="pv-log" style="font-size:10px;font-family:monospace;line-height:1.6"></div></div>' +

    '<div class="row" style="margin-top:16px">' +
    '<button class="btn-d" id="pv-cancel-btn" onclick="previewCancelGenerate()">取消</button></div></div>';

  // 启动生成
  previewState._cancelGen = false;
  previewState._generating = true;
  previewGenerateAll(genGif);
}

var previewGenState = null;
var gifBackendQueue = [];   // 等待后端合成的 GIF 帧数据
var gifBackendActive = 0;   // 正在后端合成的 GIF 数

function previewGenerateAll(genGif) {
  var files = previewState.files;
  var angles = previewState.angles;
  var gifFrames = genGif ? 36 : 0;
  var totalSteps = files.length * (angles.length + gifFrames);
  if (totalSteps === 0) {
    previewLog('没有需要生成的内容');
    return;
  }

  previewGenState = {
    files: files,
    angles: angles,
    genGif: genGif,
    gifFrames: gifFrames,
    total: totalSteps,
    done: 0,
    currentFileIdx: 0,
    canceled: false
  };

  updateProgressGen();
  previewGenNextFile();
}

function previewLog(msg) {
  var el = document.getElementById('pv-log');
  if (!el) return;
  el.innerHTML = '<div>' + new Date().toLocaleTimeString() + ' ' + msg + '</div>' + el.innerHTML;
  while (el.children.length > 50) {
    el.removeChild(el.lastChild);
  }
}

function previewGenNextFile() {
  if (previewGenState.canceled) return;
  var gs = previewGenState;
  if (gs.currentFileIdx >= gs.files.length) {
    tryComplete();
    return;
  }

  var file = gs.files[gs.currentFileIdx];
  var buildingName = file.name.replace(/\.[^.]+$/, '');
  updateProgressBuilding(buildingName);
  previewLog('加载体素: ' + buildingName);

  // 先加载 voxel 数据
  A('POST', '/api/building/voxel', {path: file.path}).then(function(r) {
    if (gs.canceled) return;
    if (!r.ok || !r.voxel) {
      previewLog('体素加载失败: ' + buildingName + ' - ' + (r.error || ''));
      gs.done += (gs.angles.length || 1) + (gs.gifFrames || 0);
      updateProgressGen();
      gs.currentFileIdx++;
      previewGenNextFile();
      return;
    }
    // 缓存 voxel 数据，逐个角度渲染截图
    previewGenState._currentVoxel = r.voxel;
    previewGenAngles(file, buildingName, 0, function() {
      if (gs.genGif) {
        previewGenGif(file, buildingName, function() {
          finishBuilding(buildingName);
        });
      } else {
        finishBuilding(buildingName);
      }
    });
  });
}

function finishBuilding(buildingName) {
  var gs = previewGenState;
  // ZIP 模式：每个建筑单独打包，然后删除源文件
  if (previewState.output.mode === 'zip') {
    var buildingDir = previewState.output.dir + '/' + buildingName;
    previewLog('打包: ' + buildingName + '.zip');
    A('POST', '/api/preview/zip-building', {
      dir: buildingDir,
      name: buildingName
    }).then(function(r) {
      if (r.ok) {
        previewLog('压缩包完成: ' + (r.zip_path || ''));
      } else {
        previewLog('打包失败: ' + (r.error || ''));
      }
      previewGenState._currentVoxel = null;
      gs.currentFileIdx++;
      previewGenNextFile();
    });
  } else {
    previewGenState._currentVoxel = null;
    gs.currentFileIdx++;
    previewGenNextFile();
  }
}

function previewGenAngles(file, buildingName, angleIdx, onDone) {
  if (previewGenState.canceled) { onDone(); return; }
  var gs = previewGenState;
  if (angleIdx >= gs.angles.length) { onDone(); return; }

  var angle = gs.angles[angleIdx];
  updateProgressAngle(angle.label);
  previewLog('截图: ' + buildingName + ' - ' + angle.label);

  var voxel = gs._currentVoxel;
  if (!voxel || !voxel.points || !voxel.points.length) {
    previewLog('跳过（无体素数据）: ' + angle.label);
    gs.done++;
    updateProgressGen();
    previewGenAngles(file, buildingName, angleIdx + 1, onDone);
    return;
  }

  // 用 Three.js 渲染并截图
  previewRenderAndSave(file, buildingName, angle, function() {
    gs.done++;
    updateProgressGen();
    previewGenAngles(file, buildingName, angleIdx + 1, onDone);
  });
}

function previewRenderAndSave(file, buildingName, angle, onDone) {
  var angleName = sanitizeGenName(angle.label);
  var filename = buildingName + '_' + angleName + '.' + (previewState.output.format || 'png');
  var buildingDir = previewState.output.dir;
  if (previewState.output.mode === 'folder' || previewState.output.mode === 'zip') {
    buildingDir = buildingDir + '/' + buildingName;
  }
  var imgPath = buildingDir + '/' + filename;

  var voxel = previewGenState._currentVoxel;
  var s = voxel.step || 1, sx = voxel.size_x || 10, sy = voxel.size_y || 10, sz = voxel.size_z || 10;
  var W = previewState.output.width || 1024;
  var H = previewState.output.height || 1024;
  var cx = sx / 2, cy = sy / 2, cz = sz / 2, m = Math.max(sx, sy, sz);

  // 复用共享场景和渲染器（每个建筑只构建一次体素场景）
  var scene = previewBuildScene(voxel, s, sx, sy, sz, cx, cy, cz, m);
  var renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
  renderer.setSize(W, H);
  renderer.setPixelRatio(1);

  var yaw = 45, pitch = 30, dist = m * 1.8;
  if (angle.type === 'custom') {
    yaw = angle.yaw || 45;
    pitch = angle.pitch || 30;
    dist = (angle.dist || 200) / 100 * m;
  } else {
    var angleMap = {
      front: [0, 0], back: [180, 0], left: [270, 0], right: [90, 0],
      northeast: [45, 30], southeast: [135, 30], northwest: [315, 30], southwest: [225, 30],
      top: [0, 89], '0': [0, 30], '45': [45, 30], '90': [90, 30], '135': [135, 30],
      '180': [180, 30], '225': [225, 30], '270': [270, 30], '315': [315, 30]
    };
    var ap = angleMap[angle.value] || [45, 30];
    yaw = ap[0]; pitch = ap[1];
  }

  var radYaw = yaw * Math.PI / 180;
  var radPitch = pitch * Math.PI / 180;

  var cam = new THREE.PerspectiveCamera(45, W / H, 1, 5000);
  cam.position.set(
    cx + dist * Math.sin(radYaw) * Math.cos(radPitch),
    cy + dist * Math.sin(radPitch),
    cz + dist * Math.cos(radYaw) * Math.cos(radPitch)
  );
  cam.lookAt(cx, cy, cz);

  renderer.render(scene, cam);

  var canvas = renderer.domElement;
  canvas.toBlob(function(blob) {
    var reader = new FileReader();
    reader.onloadend = function() {
      var base64data = reader.result;
      A('POST', '/api/preview/save-image', {
        path: imgPath,
        data: base64data
      }).then(function(r) {
        if (r.ok) {
          previewLog('\u5b8c\u6210: ' + filename);
        } else {
          previewLog('\u4fdd\u5b58\u5931\u8d25: ' + (r.error || '') + ' - ' + filename);
        }
        // 每次渲染后清理，下次重建
        previewDisposeScene(scene);
        renderer.dispose();
        if (onDone) onDone();
      });
    };
    reader.readAsDataURL(blob);
  }, 'image/' + (previewState.output.format === 'jpg' ? 'jpeg' : 'png'));
}



// 构建体素场景（共享，供角度截图和 GIF 复用）
function previewBuildScene(voxel, s, sx, sy, sz, cx, cy, cz, m) {
  var scene = new THREE.Scene();
  scene.background = new THREE.Color(0xf8f8f0);
  if (cx === undefined) cx = sx / 2;
  if (cy === undefined) cy = sy / 2;
  if (cz === undefined) cz = sz / 2;
  if (m === undefined) m = Math.max(sx, sy, sz);

  var filtered = [];
  for (var i = 0; i < voxel.points.length; i++) {
    var p = voxel.points[i];
    if (p.c === '#ff00ff' || p.c === '#e94560') continue;
    filtered.push(p);
  }
  var points = filtered.length ? filtered : voxel.points;

  var groups = {};
  for (var i = 0; i < points.length; i++) {
    var col = points[i].c || '#e94560';
    if (!groups[col]) groups[col] = [];
    groups[col].push(points[i]);
  }
  var gk = Object.keys(groups);
  var geom = new THREE.BoxGeometry(s * 0.85, s * 0.85, s * 0.85);
  for (var gi = 0; gi < gk.length; gi++) {
    var color = gk[gi], arr = groups[color];
    var mat = new THREE.MeshPhongMaterial({ color: color, specular: 0x111111, shininess: 20 });
    var mesh = new THREE.InstancedMesh(geom, mat, arr.length);
    var dummy = new THREE.Object3D();
    for (var j = 0; j < arr.length; j++) {
      dummy.position.set(arr[j].x, arr[j].y, arr[j].z);
      dummy.updateMatrix();
      mesh.setMatrixAt(j, dummy.matrix);
    }
    mesh.instanceMatrix.needsUpdate = true;
    scene.add(mesh);
  }

  scene.add(new THREE.AmbientLight(0x888888));
  var dl = new THREE.DirectionalLight(0xffffff, 0.8);
  dl.position.set(cx + m, cy + m * 2, cz + m);
  scene.add(dl);

  return scene;
}

function previewGen2D(file, buildingName, onDone) {
  if (previewGenState.canceled) { onDone(); return; }
  var gs = previewGenState;

  var body = {
    files: [file.path],
    angles: [],
    output: {
      format: previewState.output.format,
      width: previewState.output.width,
      height: previewState.output.height,
      mode: previewState.output.mode || 'folder',
      dir: previewState.output.dir
    },
    gen_topdown: gs.genTopDown,
    gen_elevation: gs.genElevation,
    gen_gif: false,
    save_config: false
  };

  A('POST', '/api/preview/generate', body).then(function(r) {
    if (r.ok) {
      // 轮询等待完成
      var pollTimer = setInterval(function() {
        A('GET', '/api/preview/status?task_id=' + encodeURIComponent(r.task_id)).then(function(sr) {
          if (sr.status === 'complete' || sr.status === 'cancelled') {
            clearInterval(pollTimer);
            gs.done += (gs.genTopDown ? 1 : 0) + (gs.genElevation ? 4 : 0);
            updateProgressGen();
            onDone();
          }
        });
      }, 500);
    } else {
      previewLog('2D 渲染启动失败: ' + buildingName);
      onDone();
    }
  });
}

function previewGenGif(file, buildingName, onDone) {
  if (previewGenState.canceled) { if (onDone) onDone(); return; }
  var gs = previewGenState;
  previewLog('GIF \u6e32\u67d3: ' + buildingName);

  var voxel = gs._currentVoxel;
  if (!voxel || !voxel.points || !voxel.points.length) {
    previewLog('\u8df3\u8fc7\uff08\u65e0\u4f53\u7d20\u6570\u636e\uff09');
    gs.done += gs.gifFrames;
    updateProgressGen();
    if (onDone) onDone();
    tryComplete();
    return;
  }

  var frameCount = 36;
  gs.done += frameCount;
  updateProgressGen();

  // \u540c\u6b65\u6e32\u67d3 36 \u5e27
  var s = voxel.step || 1, sx = voxel.size_x || 10, sy = voxel.size_y || 10, sz = voxel.size_z || 10;
  var W = previewState.output.width || 1024;
  var H = previewState.output.height || 1024;
  var cx = sx / 2, cy = sy / 2, cz = sz / 2, m = Math.max(sx, sy, sz);
  var dist = m * 1.8;
  var frames = [];
  var scene = previewBuildScene(voxel, s, sx, sy, sz, cx, cy, cz, m);
  var renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
  renderer.setSize(W, H);
  renderer.setPixelRatio(1);
  var cam = new THREE.PerspectiveCamera(45, W / H, 1, 5000);
  var canvas = renderer.domElement;

  for (var i = 0; i < frameCount; i++) {
    var yaw = i * 10, pitch = 30;
    var radYaw = yaw * Math.PI / 180;
    var radPitch = pitch * Math.PI / 180;
    cam.position.set(
      cx + dist * Math.sin(radYaw) * Math.cos(radPitch),
      cy + dist * Math.sin(radPitch),
      cz + dist * Math.cos(radYaw) * Math.cos(radPitch)
    );
    cam.lookAt(cx, cy, cz);
    renderer.render(scene, cam);
    frames.push(canvas.toDataURL('image/png'));
  }
  updateProgressAngle('\u73af\u7ed5\u5b8c\u6210');

  previewDisposeScene(scene);
  renderer.dispose();

  // \u53d1\u9001\u5230\u540e\u7aef\u5408\u6210 GIF\uff08\u4e0d\u7b49\u5f85\uff0c\u7ee7\u7eed\u4e0b\u4e00\u4e2a\u5efa\u7b51\uff09
  var gifFilename = buildingName + '_\u73af\u7ed5\u52a8\u753b.gif';
  var gifPath = previewState.output.dir;
  if (previewState.output.mode === 'folder' || previewState.output.mode === 'zip') {
    gifPath = gifPath + '/' + buildingName;
  }
  gifPath = gifPath + '/' + gifFilename;

  gifBackendQueue.push({
    path: gifPath,
    frames: frames,
    filename: gifFilename
  });
  previewLog('GIF \u5408\u6210: ' + gifFilename + ' (' + frames.length + ' \u5e27)');
  processGifBackendQueue();

  if (onDone) onDone();
}

// GIF \u540e\u7aef\u5408\u6210\u961f\u5217\uff1a\u6700\u591a\u540c\u65f6 2 \u4e2a
function processGifBackendQueue() {
  while (gifBackendActive < 2 && gifBackendQueue.length > 0) {
    var task = gifBackendQueue.shift();
    gifBackendActive++;
    updateGifStatus();
    A('POST', '/api/preview/generate-gif', {
      path: task.path,
      frames: task.frames,
      delay: 5
    }).then(function(r) {
      gifBackendActive--;
      if (r.ok) {
        previewLog('GIF \u5b8c\u6210: ' + task.filename);
      } else {
        previewLog('GIF \u5408\u6210\u5931\u8d25: ' + (r.error || ''));
      }
      updateGifStatus();
      tryComplete();
      processGifBackendQueue();
    });
  }
}

function updateGifStatus() {
  var el = document.getElementById('pv-gif-status');
  var txt = document.getElementById('pv-gif-status-text');
  if (!el || !txt) return;
  var active = gifBackendActive || 0;
  var pending = gifBackendQueue.length || 0;
  if (active + pending === 0) {
    el.style.display = 'none';
    return;
  }
  el.style.display = '';
  var parts = [];
  if (active > 0) parts.push(active + ' \u5408\u6210\u4e2d');
  if (pending > 0) parts.push(pending + ' \u7b49\u5f85\u5408\u6210');
  txt.textContent = parts.join('\uff0c');
}

function tryComplete() {
  var gs = previewGenState;
  if (!gs) return;
  if (gs.currentFileIdx < gs.files.length) return;
  if (gifBackendQueue.length > 0 || gifBackendActive > 0) return;
  previewLog('\u5168\u90e8\u5b8c\u6210\uff01');
  previewShowGenComplete();
}
function sanitizeGenName(name) {
  return String(name).replace(/[\/\\:*?"<>|°\s]/g, '_').replace(/_+/g, '_');
}

function updateProgressGen() {
  var gs = previewGenState;
  if (!gs || gs.total === 0) return;
  var pct = Math.min(1, gs.done / gs.total);
  var bar = document.getElementById('pv-progress-bar');
  var text = document.getElementById('pv-progress-text');
  var count = document.getElementById('pv-progress-count');
  if (bar) bar.style.width = (pct * 100) + '%';
  if (text) text.textContent = Math.round(pct * 100) + '%';
  if (count) count.textContent = gs.done + ' / ' + gs.total;
}

function updateProgressBuilding(name) {
  var el = document.getElementById('pv-current-building');
  if (el) el.textContent = name;
}

function updateProgressAngle(label) {
  var el = document.getElementById('pv-current-angle');
  if (el) el.textContent = '当前: ' + label;
}



function previewCancelGenerate() {
  previewState._generating = false;
  if (previewState.generateTaskId) {
    A('POST', '/api/preview/cancel?task_id=' + encodeURIComponent(previewState.generateTaskId));
  }
  if (previewState._pollTimer) {
    clearInterval(previewState._pollTimer);
    previewState._pollTimer = null;
  }
  if (previewGenState) { previewGenState.canceled = true; }
  var btn = document.getElementById('pv-cancel-btn');
  if (btn) { btn.textContent = '已取消'; btn.disabled = true; }
  previewLog('已取消');
}

function previewDoZip(onDone) {
  A('POST', '/api/preview/zip', {dir: previewState.output.dir}).then(function(r) {
    if (r.ok) {
      previewLog('ZIP 打包完成: ' + (r.zip_path || ''));
    } else {
      previewLog('ZIP 打包失败: ' + (r.error || ''));
    }
    if (onDone) onDone();
  });
}

function previewShowGenComplete() {
  previewState._generating = false;
  var body = document.getElementById('preview-body');
  if (!body) return;
  body.innerHTML =
    '<div class="wz-page active" style="position:relative;opacity:1;pointer-events:auto;transform:none;padding:0;align-items:stretch;width:100%">' +
    '<div style="text-align:center;padding:40px 20px">' +
    '<svg width="64" height="64" viewBox="0 0 24 24" fill="none" stroke="var(--phx-success)" stroke-width="1.5" style="margin-bottom:16px"><polyline points="20 6 9 17 4 12"/></svg>' +
    '<div style="font-size:20px;font-weight:900;margin-bottom:8px">生成完成</div>' +
    '<div class="dim" style="margin-bottom:24px">共处理 ' + (previewGenState ? previewGenState.done : 0) + ' 张图片</div>' +
    '<div class="card" style="text-align:left;font-size:12px;line-height:1.8">' +
    '<div>输出目录: <span style="color:var(--phx-primary);font-weight:700">' + previewState.output.dir + '</span></div>' +
    '</div>' +
    '<button class="btn" style="margin-top:20px" onclick="closeSubPage()">完成</button></div></div>';
}

// 3D 截图发送逻辑

// 渲染指定角度的 3D 预览并截图
// 注意：不在此函数中 dispose 场景，由调用方在 toBlob 回调后处理
function renderPreviewAngle(container, voxel, angle, onDone) {
  if (!voxel || !voxel.points || !voxel.points.length || typeof THREE === 'undefined') {
    if (onDone) onDone();
    return;
  }
  var s = voxel.step || 1, sx = voxel.size_x || 10, sy = voxel.size_y || 10, sz = voxel.size_z || 10;
  var W = container.clientWidth || previewState.output.width || 1024;
  var H = container.clientHeight || previewState.output.height || 1024;

  var scene = new THREE.Scene();
  scene.background = new THREE.Color(0xf8f8f0);
  var cam = new THREE.PerspectiveCamera(45, W / H, 1, 5000);
  var cx = sx / 2, cy = sy / 2, cz = sz / 2, m = Math.max(sx, sy, sz);

  // 根据角度配置设置相机位置
  var yaw = 45, pitch = 30, dist = m * 1.8;
  if (angle.type === 'custom') {
    yaw = angle.yaw || 45;
    pitch = angle.pitch || 30;
    dist = (angle.dist || 200) / 100 * m;
  } else {
    // 预设角度映射
    var angleMap = {
      front: [0, 0], back: [180, 0], left: [270, 0], right: [90, 0],
      northeast: [45, 30], southeast: [135, 30], northwest: [315, 30], southwest: [225, 30],
      top: [0, 89], '0': [0, 30], '45': [45, 30], '90': [90, 30], '135': [135, 30],
      '180': [180, 30], '225': [225, 30], '270': [270, 30], '315': [315, 30]
    };
    var ap = angleMap[angle.value] || [45, 30];
    yaw = ap[0]; pitch = ap[1];
  }

  var radYaw = yaw * Math.PI / 180;
  var radPitch = pitch * Math.PI / 180;
  cam.position.set(
    cx + dist * Math.sin(radYaw) * Math.cos(radPitch),
    cy + dist * Math.sin(radPitch),
    cz + dist * Math.cos(radYaw) * Math.cos(radPitch)
  );
  cam.lookAt(cx, cy, cz);

  var renderer = new THREE.WebGLRenderer({ antialias: true, preserveDrawingBuffer: true });
  renderer.setSize(W, H);
  renderer.setPixelRatio(1);
  container.innerHTML = '';
  container.appendChild(renderer.domElement);

  scene.add(new THREE.AmbientLight(0x888888));
  var dl = new THREE.DirectionalLight(0xffffff, 0.8);
  dl.position.set(cx + m, cy + m * 2, cz + m);
  scene.add(dl);

  // 过滤不可见方块颜色
  var filtered = [];
  for (var i = 0; i < voxel.points.length; i++) {
    var p = voxel.points[i];
    // 跳过不可见方块的默认粉色/品红色
    if (p.c === '#ff00ff' || p.c === '#ff00ff' || p.c === '#e94560') continue;
    filtered.push(p);
  }
  var points = filtered.length ? filtered : voxel.points;

  // 按颜色分组，用 InstancedMesh 高效渲染
  var groups = {};
  for (var i = 0; i < points.length; i++) {
    var col = points[i].c || '#e94560';
    if (!groups[col]) groups[col] = [];
    groups[col].push(points[i]);
  }
  var gk = Object.keys(groups);
  var geom = new THREE.BoxGeometry(s * 0.85, s * 0.85, s * 0.85);
  for (var gi = 0; gi < gk.length; gi++) {
    var color = gk[gi], arr = groups[color];
    var mat = new THREE.MeshPhongMaterial({ color: color, specular: 0x111111, shininess: 20 });
    var mesh = new THREE.InstancedMesh(geom, mat, arr.length);
    var dummy = new THREE.Object3D();
    for (var j = 0; j < arr.length; j++) {
      dummy.position.set(arr[j].x, arr[j].y, arr[j].z);
      dummy.updateMatrix();
      mesh.setMatrixAt(j, dummy.matrix);
    }
    mesh.instanceMatrix.needsUpdate = true;
    scene.add(mesh);
  }

  // 渲染一次
  renderer.render(scene, cam);
  if (onDone) onDone(scene, renderer);
}

// 彻底清理 Three.js 场景
function previewDisposeScene(scene) {
  if (!scene) return;
  scene.traverse(function(obj) {
    if (obj.geometry) {
      obj.geometry.dispose();
    }
    if (obj.material) {
      if (Array.isArray(obj.material)) {
        obj.material.forEach(function(m) { disposeMaterial(m); });
      } else {
        disposeMaterial(obj.material);
      }
    }
  });
}

function disposeMaterial(mat) {
  if (!mat) return;
  if (mat.map) mat.map.dispose();
  if (mat.lightMap) mat.lightMap.dispose();
  if (mat.emissiveMap) mat.emissiveMap.dispose();
  if (mat.specularMap) mat.specularMap.dispose();
  mat.dispose();
}

// 在关闭子页面时自动清理
var origCloseSubPage = closeSubPage;
closeSubPage = function() {
  previewCleanupAnglePreview();
  activeRenderers.forEach(function(kill) { try { kill(); } catch(e) {} });
  activeRenderers = [];
  origCloseSubPage();
};