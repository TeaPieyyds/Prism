/* ═══ 文件选择器 — 支持搜索 + 排序 ═══ */

function showFilePicker(startPath, filter, onSelect, mode) {
  var folderMode = mode === 'folder';
  var overlay = document.createElement('div');
  overlay.className = 'fp-overlay';
  var currentPath = startPath || '/storage/emulated/0';
  var selectedPath = '';
  var selectedName = '';
  var dataList = [];
  var dataParent = '';
  var searchTerm = '';
  var sortMode = 'date_desc';
  var virtualMode = false;   // 是否在「在线玩家皮肤」虚拟文件夹内
  var virtualPlayers = [];   // 在线玩家列表 [{name, width, height}]
  var onlinePrefix = 'online://';

  var sortLabels = {
    name_asc: '名称 A→Z',
    name_desc: '名称 Z→A',
    date_desc: '最新优先',
    date_asc: '最旧优先',
    size_desc: '最大优先',
    size_asc: '最小优先'
  };
  var sortShort = {
    name_asc: '名称↑',
    name_desc: '名称↓',
    date_desc: '最新',
    date_asc: '最早',
    size_desc: '最大',
    size_asc: '最小'
  };

  overlay.innerHTML =
    '<div class="fp-modal">' +
      '<div class="fp-header"><button class="fp-back" id="fp-nav-back">&#8592;</button><span class="fp-path" id="fp-path">' + currentPath + '</span></div>' +
      '<div class="fp-toolbar">' +
        '<div class="fp-search">' +
          '<span class="fp-search-icon">' + ICONS.search + '</span>' +
          '<input class="fp-search-input" id="fp-search-input" type="text" placeholder="搜索文件..." autocomplete="off">' +
          '<button class="fp-search-clear" id="fp-search-clear" style="display:none">' + ICONS.x + '</button>' +
        '</div>' +
        '<button class="fp-sort-btn" id="fp-sort-btn">' +
          '<span class="fp-sort-icon">' + ICONS.sort + '</span>' +
          '<span class="fp-sort-label" id="fp-sort-label">' + sortShort[sortMode] + '</span>' +
        '</button>' +
      '</div>' +
      '<div class="fp-body" id="fp-body"><div class="fp-loading">加载中...</div></div>' +
      '<div class="fp-footer">' +
        '<button class="fp-btn fp-btn-cancel" id="fp-cancel">取消</button>' +
        '<button class="fp-btn fp-btn-ok" id="fp-ok"' + (folderMode ? '' : ' disabled') + '>' + (folderMode ? '确定' : '选择') + '</button>' +
      '</div>' +
    '</div>';
  document.body.appendChild(overlay);

  var pathEl = document.getElementById('fp-path');
  var bodyEl = document.getElementById('fp-body');
  var okBtn = document.getElementById('fp-ok');
  var searchInput = document.getElementById('fp-search-input');
  var searchClear = document.getElementById('fp-search-clear');
  var sortBtn = document.getElementById('fp-sort-btn');
  var sortLabel = document.getElementById('fp-sort-label');

  function navigateTo(path) {
    currentPath = path;
    selectedPath = '';
    selectedName = '';
    searchTerm = '';
    searchInput.value = '';
    searchClear.style.display = 'none';
    virtualMode = false;
    virtualPlayers = [];
    if (!folderMode) okBtn.disabled = true;
    pathEl.textContent = currentPath;
    bodyEl.innerHTML = '<div class="fp-loading">加载中...</div>';
    fetch('/api/files/scan?path=' + encodeURIComponent(currentPath) + '&filter=' + encodeURIComponent(filter))
      .then(function(r) { return r.json(); })
      .then(function(data) {
        if (data.ok) {
          dataList = data.entries || [];
          dataParent = data.parent || '';
          renderData(data);
        } else {
          bodyEl.innerHTML = '<div class="fp-empty">无法读取目录: ' + (data.error || '') + '</div>';
        }
      })
      .catch(function(err) {
        bodyEl.innerHTML = '<div class="fp-empty">网络错误</div>';
      });
  }

  function applySearch(entries) {
    if (!searchTerm) return entries;
    return entries.filter(function(item) {
      return item.name.toLowerCase().indexOf(searchTerm) !== -1;
    });
  }

  function sortEntries(entries) {
    var dirs = [];
    var files = [];
    for (var i = 0; i < entries.length; i++) {
      if (entries[i].type === 'dir') {
        dirs.push(entries[i]);
      } else {
        files.push(entries[i]);
      }
    }
    // 目录始终按名称升序
    dirs.sort(function(a, b) {
      return a.name.toLowerCase() < b.name.toLowerCase() ? -1 : 1;
    });
    // 文件按当前排序方式
    var cmp;
    switch (sortMode) {
      case 'name_asc':
        cmp = function(a, b) { return a.name.toLowerCase() < b.name.toLowerCase() ? -1 : 1; };
        break;
      case 'name_desc':
        cmp = function(a, b) { return a.name.toLowerCase() > b.name.toLowerCase() ? -1 : 1; };
        break;
      case 'date_asc':
        cmp = function(a, b) { return a.mtime - b.mtime; };
        break;
      case 'size_desc':
        cmp = function(a, b) { return (b.size || 0) - (a.size || 0); };
        break;
      case 'size_asc':
        cmp = function(a, b) { return (a.size || 0) - (b.size || 0); };
        break;
      default: // date_desc
        cmp = function(a, b) { return b.mtime - a.mtime; };
    }
    files.sort(cmp);
    return dirs.concat(files);
  }

  function renderData(data) {
    // 虚拟文件夹模式：直接渲染在线玩家列表
    if (virtualMode) {
      renderVirtualPlayers();
      return;
    }

    // 1. 过滤：建筑过滤 + 皮肤 PNG 过滤
    var filtered = dataList;
    if (filter === 'building') {
      filtered = dataList.filter(function(item) { return item.type !== 'dir' || item.has_building; });
    } else if (filter === 'skin') {
      filtered = dataList.filter(function(item) { return item.type === 'dir' || (item.ext && item.ext.toLowerCase() === '.png'); });
    }
    // 2. 搜索过滤
    filtered = applySearch(filtered);
    // 3. 排序
    filtered = sortEntries(filtered);

    // 搜索时不显示上级目录
    var parentPath = data.parent || dataParent;
    var html = parentPath && !searchTerm
      ? '<div class="fp-item" data-path="' + parentPath + '" data-type="parent"><div class="fp-icon fp-icon-dir">' + ICONS.parentDir + '</div><div class="fp-info"><div class="fp-name">.. (上级目录)</div></div></div>'
      : '';
    // 皮肤选择器：顶部永远显示「在线玩家皮肤」虚拟文件夹
    if (filter === 'skin') {
      html += '<div class="fp-item fp-virtual" data-virtual="1"><div class="fp-icon fp-icon-dir">' + ICONS.globe + '</div>' +
        '<div class="fp-info"><div class="fp-name">在线玩家皮肤</div></div></div>';
    }

    if (!filtered.length && !html) {
      bodyEl.innerHTML = '<div class="fp-empty">' + (searchTerm ? '无匹配文件' : '空目录') + '</div>';
      return;
    }

    for (var i = 0; i < filtered.length; i++) {
      var item = filtered[i];
      var iconClass = item.type === 'dir' ? 'fp-icon-dir' : 'fp-icon-file';
      var iconSvg = item.type === 'dir' ? ICONS.folder : ICONS.file;
      var extClass = item.ext ? 'fp-icon-' + item.ext.replace('.', '') : '';
      var sizeStr = item.type === 'file' ? formatSize(item.size) : '';
      // 皮肤选择器：PNG 文件显示头部缩略图
      var iconHtml;
      if (filter === 'skin' && item.type === 'file' && item.ext && item.ext.toLowerCase() === '.png') {
        iconHtml = '<div class="fp-icon fp-head"><img src="/api/skin/head?path=' + encodeURIComponent(item.path) + '" alt="" onerror="this.style.display=\'none\'"></div>';
      } else {
        iconHtml = '<div class="fp-icon ' + iconClass + ' ' + extClass + '">' + iconSvg + '</div>';
      }
      html += '<div class="fp-item" data-path="' + item.path + '" data-type="' + item.type + '">' +
        iconHtml +
        '<div class="fp-info"><div class="fp-name">' + item.name + '</div></div>' +
        (sizeStr ? '<span class="fp-size">' + sizeStr + '</span>' : '') +
        '</div>';
    }
    bodyEl.innerHTML = html;

    bodyEl.querySelectorAll('.fp-item').forEach(function(el) {
      el.addEventListener('click', function() {
        var path = el.dataset.path;
        var type = el.dataset.type;
        if (el.dataset.virtual) {
          enterVirtualFolder();
        } else if (type === 'dir' || type === 'parent') {
          navigateTo(path);
        } else if (!folderMode) {
          bodyEl.querySelectorAll('.fp-item').forEach(function(x) { x.style.background = ''; });
          el.style.background = 'var(--phx-primary-bg)';
          selectedPath = path;
          selectedName = el.querySelector('.fp-name').textContent;
          okBtn.disabled = false;
        }
      });
    });
  }

  // 进入「在线玩家皮肤」虚拟文件夹
  function enterVirtualFolder() {
    virtualMode = true;
    selectedPath = '';
    selectedName = '';
    searchTerm = '';
    searchInput.value = '';
    searchClear.style.display = 'none';
    if (!folderMode) okBtn.disabled = true;
    pathEl.textContent = '在线玩家皮肤';
    bodyEl.innerHTML = '<div class="fp-loading">加载中...</div>';
    fetch('/api/skin/online-list')
      .then(function(r) { return r.json(); })
      .then(function(data) {
        virtualPlayers = data.ok ? (data.players || []) : [];
        if (!virtualPlayers.length) {
          bodyEl.innerHTML = '<div class="fp-empty">暂无在线玩家皮肤</div>';
          return;
        }
        renderVirtualPlayers();
      })
      .catch(function() {
        bodyEl.innerHTML = '<div class="fp-empty">获取在线玩家失败</div>';
      });
  }

  // 渲染在线玩家皮肤列表（玩家名.png）
  function renderVirtualPlayers() {
    var list = searchTerm
      ? virtualPlayers.filter(function(p) { return p.name.toLowerCase().indexOf(searchTerm) !== -1; })
      : virtualPlayers;
    if (!list.length) {
      bodyEl.innerHTML = searchTerm ? '<div class="fp-empty">无匹配玩家</div>' : '<div class="fp-empty">暂无在线玩家皮肤</div>';
      return;
    }
    var html = searchTerm ? '' :
      '<div class="fp-item fp-parent" data-virtual-back="1"><div class="fp-icon fp-icon-dir">' + ICONS.parentDir + '</div><div class="fp-info"><div class="fp-name">.. (返回上级目录)</div></div></div>';
    for (var i = 0; i < list.length; i++) {
      var pl = list[i];
      html += '<div class="fp-item" data-virtual-player="' + pl.name + '">' +
        '<div class="fp-icon fp-head"><img src="/api/skin/online-head?player=' + encodeURIComponent(pl.name) + '" alt="" onerror="this.style.display=\'none\'"></div>' +
        '<div class="fp-info"><div class="fp-name">' + pl.name + '.png</div></div>' +
        '<span class="fp-size">' + (pl.width || '') + 'x' + (pl.height || '') + '</span>' +
        '</div>';
    }
    bodyEl.innerHTML = html;

    bodyEl.querySelectorAll('.fp-item').forEach(function(el) {
      el.addEventListener('click', function() {
        if (el.dataset.virtualBack) {
          // 返回上级目录（回到文件列表）
          navigateTo(currentPath);
          return;
        }
        var pname = el.dataset.virtualPlayer;
        if (!pname) return;
        bodyEl.querySelectorAll('.fp-item').forEach(function(x) { x.style.background = ''; });
        el.style.background = 'var(--phx-primary-bg)';
        selectedPath = onlinePrefix + pname;
        selectedName = pname + '.png';
        okBtn.disabled = false;
      });
    });
  }

  // ── 搜索 300ms 防抖 ──
  var debounceTimer = null;
  searchInput.addEventListener('input', function() {
    searchTerm = this.value.trim().toLowerCase();
    searchClear.style.display = searchTerm ? '' : 'none';
    clearTimeout(debounceTimer);
    if (virtualMode) {
      debounceTimer = setTimeout(renderVirtualPlayers, 300);
    } else {
      debounceTimer = setTimeout(function() {
        renderData({path: currentPath, entries: dataList});
      }, 300);
    }
  });

  // ── 搜索清除按钮 ──
  searchClear.addEventListener('click', function() {
    searchInput.value = '';
    searchTerm = '';
    searchClear.style.display = 'none';
    if (virtualMode) {
      renderVirtualPlayers();
    } else {
      renderData({path: currentPath, entries: dataList});
    }
    searchInput.focus();
  });

  // ── 排序弹窗 ──
  sortBtn.addEventListener('click', function() {
    var so = document.createElement('div');
    so.className = 'bs-overlay';
    var listHtml = '';
    var keys = ['name_asc', 'name_desc', 'date_desc', 'date_asc', 'size_desc', 'size_asc'];
    for (var i = 0; i < keys.length; i++) {
      var k = keys[i];
      var active = k === sortMode ? ' active' : '';
      var check = k === sortMode ? ICONS.check : '';
      listHtml += '<div class="bs-option' + active + '" data-sort="' + k + '">' +
        '<span class="bs-dot"></span>' +
        '<span class="bs-label">' + sortLabels[k] + '</span>' +
        (check ? '<span style="color:var(--phx-primary);flex-shrink:0">' + check + '</span>' : '') +
        '</div>';
    }
    so.innerHTML = '<div class="bs-sheet">' +
      '<div class="bs-title">排序方式</div>' +
      '<div class="bs-body">' + listHtml + '</div>' +
      '<div class="bs-footer"><button class="bs-btn bs-btn-cancel" id="fp-sort-cancel">取消</button></div>' +
      '</div>';
    document.body.appendChild(so);

    so.querySelectorAll('.bs-option').forEach(function(el) {
      el.addEventListener('click', function() {
        var mode = el.dataset.sort;
        sortMode = mode;
        sortLabel.textContent = sortShort[mode];
        renderData({path: currentPath, entries: dataList});
        closeOverlay(so);
      });
    });
    document.getElementById('fp-sort-cancel').addEventListener('click', function() { closeOverlay(so); });
    so.addEventListener('click', function(e) { if (e.target === so) closeOverlay(so); });
  });

  // ── 导航 ──
  document.getElementById('fp-nav-back').addEventListener('click', function() {
    if (dataParent) {
      navigateTo(dataParent);
    }
  });

  document.getElementById('fp-cancel').addEventListener('click', function() { overlay.remove(); });
  okBtn.addEventListener('click', function() {
    if (folderMode) {
      if (onSelect) onSelect(currentPath);
    } else {
      if (!selectedPath) return;
      if (onSelect) onSelect(selectedPath);
    }
    overlay.remove();
  });

  navigateTo(currentPath);
}