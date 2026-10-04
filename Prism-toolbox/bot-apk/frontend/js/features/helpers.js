function show3D(c, voxel) {
  // save voxel data for sub-page cache recreation
  var _t=document.querySelector('.sp-title');
  if(_t&&window._voxelData)window._voxelData[_t.textContent]=voxel;
  if (!voxel || !voxel.points || !voxel.points.length || typeof THREE === 'undefined') return;
  var s = voxel.step, sx = voxel.size_x, sy = voxel.size_y, sz2 = voxel.size_z;
  var nbtInfo = voxel.nbt_info || [];
  var W = 300, H = 240;
  c.style.cssText = 'width:' + W + 'px;height:' + H + 'px;min-height:' + H + 'px;overflow:hidden;position:relative;border:2px solid var(--phx-border-light);border-radius:var(--phx-radius)';
  var sc = new THREE.Scene(); sc.background = new THREE.Color(0xf8f8f0);
  var cam = new THREE.PerspectiveCamera(45, W / H, 1, 5000);
  var cx = sx / 2, cy = sy / 2, cz = sz2 / 2, m = Math.max(sx, sy, sz2);
  if (sy <= 3) cy = 0;
  if (sx <= 3) cam.position.set(cx + m, cy + sz2 * .3, cz + sz2 * .5);
  else if (sz2 <= 3) cam.position.set(cx + sx * .5, cy + sx * .3, cz + m);
  else cam.position.set(cx + m * .8, cy + m * .6, cz + m * .8);
  cam.lookAt(cx, cy, cz);
  var r = new THREE.WebGLRenderer({ antialias: true });
  r.setSize(W, H);
  r.setPixelRatio(Math.min(devicePixelRatio, 2));
  c.innerHTML = '';
  c.appendChild(r.domElement);

  var nbtMap = {};
  var nbtPositions = [];
  for (var ni = 0; ni < nbtInfo.length; ni++) {
    var n = nbtInfo[ni];
    var key = n.x + ',' + n.y + ',' + n.z;
    nbtMap[key] = n;
    nbtPositions.push({ x: n.x, y: n.y, z: n.z, info: n });
  }

  var ps = new Set(voxel.points.map(function(p) { return p.x + ',' + p.y + ',' + p.z; }));
  var hx = sx - s, hy = sy - s, hz = sz2 - s;
  var pts = voxel.points.filter(function(p) {
    if (p.x <= 0 || p.x >= hx || p.y <= 0 || p.y >= hy || p.z <= 0 || p.z >= hz) return true;
    return !ps.has((p.x - s) + ',' + p.y + ',' + p.z) || !ps.has((p.x + s) + ',' + p.y + ',' + p.z) || !ps.has(p.x + ',' + (p.y - s) + ',' + p.z) || !ps.has(p.x + ',' + (p.y + s) + ',' + p.z) || !ps.has(p.x + ',' + p.y + ',' + (p.z - s)) || !ps.has(p.x + ',' + p.y + ',' + (p.z + s));
  });
  if (!pts.length) return;

  var groups = {};
  for (var i = 0; i < pts.length; i++) { var col = pts[i].c || '#e94560'; if (!groups[col]) groups[col] = []; groups[col].push(pts[i]); }
  var gk = Object.keys(groups);
  var geom = new THREE.BoxGeometry(s * .85, s * .85, s * .85);
  sc.add(new THREE.AmbientLight(0x888888));
  var dl = new THREE.DirectionalLight(0xffffff, .8);
  dl.position.set(cx + m, cy + m * 2, cz + m);
  sc.add(dl);

  var allMeshes = [];
  for (var gi = 0; gi < gk.length; gi++) {
    var color = gk[gi], arr = groups[color], n = arr.length;
    var mat = new THREE.MeshPhongMaterial({ color: color, specular: 0x111111, shininess: 20 });
    var mesh = new THREE.InstancedMesh(geom, mat, n);
    var dummy = new THREE.Object3D();
    for (var j = 0; j < n; j++) { dummy.position.set(arr[j].x, arr[j].y, arr[j].z); dummy.updateMatrix(); mesh.setMatrixAt(j, dummy.matrix); }
    mesh.instanceMatrix.needsUpdate = true;
    sc.add(mesh);
    allMeshes.push(mesh);
  }

  var edge = new THREE.EdgesGeometry(new THREE.BoxGeometry(sx, sy, sz2));
  var boxLine = new THREE.LineSegments(edge, new THREE.LineBasicMaterial({ color: 0xddd6c8 }));
  boxLine.position.set(sx / 2, sy / 2, sz2 / 2);
  sc.add(boxLine);
  var axes = new THREE.AxesHelper(m * .5);
  axes.position.set(0, 0, 0);
  sc.add(axes);

  // NBT markers
  var markerGeom = new THREE.BoxGeometry(s * 0.25, s * 0.25, s * 0.25);
  var cmdMarkerMat = new THREE.MeshPhongMaterial({ color: 0xff8844, emissive: 0x331100 });
  var contMarkerMat = new THREE.MeshPhongMaterial({ color: 0x44cc88, emissive: 0x003311 });
  var otherMarkerMat = new THREE.MeshPhongMaterial({ color: 0x8888cc, emissive: 0x111133 });
  var markerGroup = new THREE.Group();
  sc.add(markerGroup);
  var markerList = [];
  for (var mi = 0; mi < nbtPositions.length; mi++) {
    var mp = nbtPositions[mi];
    var markerMat = mp.info.type === 'command' ? cmdMarkerMat : mp.info.type === 'container' ? contMarkerMat : otherMarkerMat;
    var marker = new THREE.Mesh(markerGeom, markerMat);
    marker.position.set(mp.x, mp.y + s * 0.85, mp.z);
    markerGroup.add(marker);
    markerList.push({ marker: marker, pos: mp });
  }

  var infoBar = document.createElement('div');
  infoBar.style.cssText = 'position:absolute;bottom:2px;left:2px;right:2px;background:rgba(20,14,10,.85);color:#ccc;border-radius:6px;padding:4px 8px;font-size:9px;line-height:1.35;z-index:5;pointer-events:none;font-family:sans-serif;max-height:72px;overflow:hidden;opacity:0;transition:opacity .2s';
  c.appendChild(infoBar);

  function updateInfoBar() {
    var camPos = cam.position;
    var best = null, bestDist = Infinity;
    for (var mi = 0; mi < markerList.length; mi++) {
      var bp = markerList[mi].pos;
      var dx = camPos.x - bp.x, dy = camPos.y - bp.y, dz = camPos.z - bp.z;
      var d2 = dx * dx + dy * dy + dz * dz;
      if (d2 < bestDist) { bestDist = d2; best = bp; }
    }
    if (best && bestDist < (m * 4) * (m * 4)) {
      var info = best.info;
      var html = '';
      if (info.type === 'command') {
        html = '<span style="color:#ff9944;font-weight:700">' + escHtml(shortCmdV(info.command)) + '</span>';
        if (info.command && info.command.length > 30) {
          html += ' <span style="color:#888;font-size:8px">' + escHtml(info.command.substring(0, 50)) + (info.command.length > 50 ? '...' : '') + '</span>';
        } else if (info.command) {
          html += ' <span style="color:#bbb">' + escHtml(info.command) + '</span>';
        }
      } else if (info.type === 'container') {
        var items = info.items || [];
        html = '<span style="color:#80d0a0;font-weight:700">' + items.length + ' items</span>';
        var names = [];
        for (var ii = 0; ii < Math.min(items.length, 6); ii++) {
          names.push('<span style="color:#aaa">' + escHtml(items[ii].name) + '</span>');
        }
        if (items.length > 6) names.push('<span style="color:#555">+' + (items.length - 6) + '</span>');
        if (names.length) html += ' ' + names.join(' ');
      } else if (info.type === 'sign') {
        html = '<span style="color:#66d9ff;font-weight:700">sign</span>';
        if (info.extra && info.extra.text) html += ' <span style="color:#ccc">' + escHtml(String(info.extra.text).substring(0, 40)) + '</span>';
      }
      infoBar.innerHTML = html;
      infoBar.style.opacity = '1';
    } else {
      infoBar.style.opacity = '0';
    }
  }

  function shortCmdV(cmd) {
    if (!cmd) return '#';
    var t = cmd.trim();
    if (!t) return '#';
    if (t[0] === '/') { var sp = t.indexOf(' ', 1); return sp > 0 ? t.substring(1, sp) : t.substring(1); }
    var sp = t.indexOf(' ');
    return sp > 0 ? t.substring(0, sp) : t;
  }

  var angX = 0.5, angY = 0.7, dist = m * 1.8, dr = false, lx = 0, ly = 0, full = false, pd = 0;
  var el = r.domElement;
  el.style.touchAction = 'none';

  el.addEventListener('pointerdown', function(e) { dr = true; lx = e.clientX; ly = e.clientY; el.setPointerCapture(e.pointerId); });
  el.addEventListener('pointermove', function(e) {
    if (!dr) return;
    angY += (e.clientX - lx) * 0.01;
    angX -= (e.clientY - ly) * 0.01;
    angX = Math.max(-1.4, Math.min(1.4, angX));
    lx = e.clientX; ly = e.clientY;
  });
  el.addEventListener('pointerup', function() { dr = false; });
  el.addEventListener('wheel', function(e) { e.preventDefault(); dist *= 1 + e.deltaY * 0.002; dist = Math.max(m * .5, Math.min(m * 8, dist)); }, { passive: false });
  el.addEventListener('touchstart', function(e) { if (e.touches.length === 2) { pd = Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY); dr = false; } }, { passive: true });
  el.addEventListener('touchmove', function(e) { if (e.touches.length === 2 && pd > 0) { var d2 = Math.hypot(e.touches[0].clientX - e.touches[1].clientX, e.touches[0].clientY - e.touches[1].clientY); dist *= pd / d2; dist = Math.max(m * .5, Math.min(m * 8, dist)); pd = d2; } }, { passive: true });

  var frameCount = 0;
  var animId;
  (function A() {
    animId = requestAnimationFrame(A);
    cam.position.set(cx + dist * Math.sin(angY) * Math.cos(angX), cy + dist * Math.sin(angX), cz + dist * Math.cos(angY) * Math.cos(angX));
    cam.lookAt(cx, cy, cz);
    frameCount++;
    if (frameCount % 8 === 0) updateInfoBar();
    r.render(sc, cam);
  })();

  var cleanup = function() { cancelAnimationFrame(animId); r.dispose(); if (obs) obs.disconnect(); if (infoBar && infoBar.parentNode) infoBar.remove(); };
  var fsBtn = document.createElement('button');
  fsBtn.innerHTML = ICONS.expand;
  fsBtn.style.cssText = 'position:absolute;top:4px;right:4px;background:rgba(0,0,0,.5);color:#fff;border:none;width:28px;height:28px;border-radius:14px;cursor:pointer;font-size:14px;z-index:10;line-height:1';
  fsBtn.onclick = function() {
    full = !full;
    if (full) {
      c.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:9999;overflow:hidden';
      r.setSize(c.clientWidth, c.clientHeight);
      cam.aspect = c.clientWidth / c.clientHeight; cam.updateProjectionMatrix();
    } else {
      c.style.cssText = 'width:' + W + 'px;height:' + H + 'px;min-height:' + H + 'px;overflow:hidden;position:relative;border:2px solid var(--phx-border-light);border-radius:var(--phx-radius)';
      r.setSize(W, H); cam.aspect = W / H; cam.updateProjectionMatrix();
    }
  };
  c.appendChild(fsBtn);
  activeRenderers.push(cleanup);
  var obs = new MutationObserver(function() { if (!document.body.contains(c)) { cancelAnimationFrame(animId); r.dispose(); obs.disconnect(); if (infoBar && infoBar.parentNode) infoBar.remove(); } });
  if (c.parentNode) obs.observe(c.parentNode, { childList: true });
}
