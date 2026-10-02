import { useEffect, useRef } from 'react';

/* ── 水波涟漪 Canvas 覆层 ──
 * 全局覆盖，pointer-events: none 不阻挡交互。
 * 点击/触摸时根据元素重要性产生不同大小的涟漪。
 * 鼠标/手指移动时产生细小尾迹涟漪。 */

function getRippleSize(target: EventTarget | null): number {
  const el = (target as HTMLElement)?.closest?.(
    '.btn-primary,.btn-danger,.btn-success,.btn-warning,'
    + '.hero-panel,.game-card.active,.nav-item.active,.bottom-nav-item.active'
  );
  if (el) return 3.2;  // 大波纹：主按钮/当前激活卡片/导航

  const med = (target as HTMLElement)?.closest?.(
    'button,.btn,a,input,select,textarea,'
    + '[role="button"],[tabindex],.game-card,.account-card,.card,.toggle-switch'
  );
  if (med) return 2.0; // 中波纹：按钮/卡片/可交互元素

  return 1.0;           // 小波纹：普通文本/空白区域
}

export default function RippleOverlay() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const simRef = useRef<{
    W: number; H: number; SW: number; SH: number;
    h1: Float32Array; h2: Float32Array;
    ctx: CanvasRenderingContext2D;
    simCtx: CanvasRenderingContext2D;
    outImg: ImageData;
    running: boolean;
    dpr: number;
    lastMoveTime: number;
  } | null>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    // 离屏 canvas 用于像素操作
    const simCanvas = document.createElement('canvas');
    const simCtx = simCanvas.getContext('2d')!;

    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const simScale = 0.3; // 更低仿真分辨率，低端设备不卡顿

    let W = 0, H = 0, SW = 0, SH = 0;
    let h1!: Float32Array, h2!: Float32Array;
    let outImg!: ImageData;

    function resize() {
      const newW = window.innerWidth;
      const newH = window.innerHeight;
      const newSW = Math.ceil(newW * simScale);
      const newSH = Math.ceil(newH * simScale);
      if (newSW === SW && newSH === SH) return;

      const oldH1 = h1, oldH2 = h2;
      const oldSW = SW, oldSH = SH;

      W = newW; H = newH;
      SW = newSW; SH = newSH;

      canvas!.width = W * dpr;
      canvas!.height = H * dpr;
      canvas!.style.width = W + 'px';
      canvas!.style.height = H + 'px';

      simCanvas.width = SW;
      simCanvas.height = SH;

      h1 = new Float32Array(SW * SH);
      h2 = new Float32Array(SW * SH);
      outImg = simCtx.createImageData(SW, SH);

      if (oldH1 && oldH1.length) {
        for (let y = 0; y < SH; y++) {
          for (let x = 0; x < SW; x++) {
            const sx = (x / SW) * oldSW | 0;
            const sy = (y / SH) * oldSH | 0;
            const si = sy * oldSW + sx;
            h1[y * SW + x] = oldH1[si] || 0;
            h2[y * SW + x] = oldH2[si] || 0;
          }
        }
      }
    }
    resize();
    window.addEventListener('resize', resize);

    const DAMP = 0.987;   // 比默认(0.985)略慢，但不过度堆积导致卡顿
    const GAIN = 460;
    const CAP = 200;
    const ALPHA_SCALE = 0.30; // 更小 → 更浅更克制

    function poke(cx: number, cy: number, str: number, rad: number) {
      active = true;
      cx |= 0; cy |= 0;
      const r2 = rad * rad;
      const R0 = Math.ceil(rad);
      for (let y = -R0; y <= R0; y++) {
        for (let x = -R0; x <= R0; x++) {
          const px = cx + x, py = cy + y;
          if (px < 1 || py < 1 || px >= SW - 1 || py >= SH - 1) continue;
          const f = (x * x + y * y) / r2;
          if (f > 1) continue;
          h1[py * SW + px] += str * (0.5 + 0.5 * Math.cos(Math.PI * Math.sqrt(f)));
        }
      }
    }

    // 沿线段拉出非圆形波痕（拖尾用）
    function pokeLine(x1: number, y1: number, x2: number, y2: number, str: number, width: number) {
      active = true;
      const minX = Math.max(1, Math.floor(Math.min(x1, x2) - width));
      const maxX = Math.min(SW - 2, Math.ceil(Math.max(x1, x2) + width));
      const minY = Math.max(1, Math.floor(Math.min(y1, y2) - width));
      const maxY = Math.min(SH - 2, Math.ceil(Math.max(y1, y2) + width));
      const dx = x2 - x1, dy = y2 - y1;
      const len2 = dx * dx + dy * dy;
      for (let y = minY; y <= maxY; y++) {
        for (let x = minX; x <= maxX; x++) {
          let t = len2 === 0 ? 0 : ((x - x1) * dx + (y - y1) * dy) / len2;
          t = Math.max(0, Math.min(1, t));
          const px = x1 + t * dx;
          const py = y1 + t * dy;
          const d = Math.hypot(x - px, y - py);
          if (d > width) continue;
          const f = 0.5 + 0.5 * Math.cos(Math.PI * d / width);
          h1[y * SW + x] += str * f;
        }
      }
    }

    let swT = 0;
    // 有能量才计算；波纹完全消散后 active=false，动画循环跳过耗时的逐像素运算
    let active = false;
    function stepWater() {
      swT += 1;
      let maxH = 0;
      for (let y = 1; y < SH - 1; y++) {
        const i0 = y * SW + 1;
        for (let x = 1; x < SW - 1; x++) {
          const i = i0 + x - 1;
          h2[i] = ((h1[i - 1] + h1[i + 1] + h1[i - SW] + h1[i + SW]) * 0.5 - h2[i]) * DAMP
            + 0.00012 * Math.sin(swT * 0.7 + x * 0.05 + y * 0.021)
            + 0.00009 * Math.sin(swT * 0.43 - x * 0.023 + y * 0.041);
          const a = Math.abs(h2[i]);
          if (a > maxH) maxH = a;
        }
      }
      const t = h1; h1 = h2; h2 = t;
      if (maxH < 0.002) {  // 波纹已平息 → 进入空闲，停止耗计算
        h1.fill(0); h2.fill(0);
        active = false;
      }
    }

    function renderWater() {
      const dst = outImg.data;
      const h = h1;
      const hi = { r: 175, g: 245, b: 236 };   // 隆起高光 — 更浅 teal
      const lo = { r: 96, g: 188, b: 176 };    // 凹陷暗部 — 更浅 teal

      for (let y = 0; y < SH; y++) {
        const yu = y > 0 ? y - 1 : y;
        const yd = y < SH - 1 ? y + 1 : y;
        for (let x = 0; x < SW; x++) {
          const i = y * SW + x;
          const gy = h[yu * SW + x] - h[yd * SW + x];
          const di = i * 4;

          const a = Math.abs(gy) * GAIN;
          if (a > 5.0) {
            const capped = Math.min(a, CAP) * ALPHA_SCALE;
            if (gy > 0) {
              dst[di]     = hi.r;
              dst[di + 1] = hi.g;
              dst[di + 2] = hi.b;
              dst[di + 3] = capped;
            } else {
              dst[di]     = lo.r;
              dst[di + 1] = lo.g;
              dst[di + 2] = lo.b;
              dst[di + 3] = capped * 0.7;
            }
          } else {
            dst[di] = dst[di + 1] = dst[di + 2] = dst[di + 3] = 0;
          }
        }
      }
      simCtx.putImageData(outImg, 0, 0);
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
      ctx!.clearRect(0, 0, W, H);
      ctx!.drawImage(simCanvas, 0, 0, W, H);
    }

    // 尾迹：用 pokeLine 沿路径拉出非圆形波痕
    let lastPt: { x: number; y: number } | null = null;

    function handlePointerDown(e: PointerEvent) {
      try {
        // 跳过弹窗上的交互，避免干扰
        if ((e.target as HTMLElement)?.closest?.('.modal-backdrop,.modal-content')) return;
        const size = getRippleSize(e.target);
        const sx = (e.clientX / W) * SW;
        const sy = (e.clientY / H) * SH;
        poke(sx, sy, size * 1.6, size * 2.0 + 1.5);
        lastPt = { x: sx, y: sy };
      } catch (_) { /* 波纹不干扰 React 交互 */ }
    }

    function handlePointerMove(e: PointerEvent) {
      const sx = (e.clientX / W) * SW;
      const sy = (e.clientY / H) * SH;
      if (lastPt) {
        const d = Math.hypot(sx - lastPt.x, sy - lastPt.y);
        if (d >= 3.5) { // 距离阈值加大 → 拖尾更稀疏，防波浪堆积卡顿
          pokeLine(lastPt.x, lastPt.y, sx, sy, Math.min(0.32, 0.08 + d * 0.02), 2.0);
        }
      }
      lastPt = { x: sx, y: sy };
    }

    function handleTouchMove(e: TouchEvent) {
      const t = e.touches[0];
      const sx = (t.clientX / W) * SW;
      const sy = (t.clientY / H) * SH;
      if (lastPt) {
        const d = Math.hypot(sx - lastPt.x, sy - lastPt.y);
        if (d >= 3.5) { // 距离阈值加大 → 拖尾更稀疏
          pokeLine(lastPt.x, lastPt.y, sx, sy, Math.min(0.32, 0.08 + d * 0.02), 2.0);
        }
      }
      lastPt = { x: sx, y: sy };
    }

    function handlePointerUp() { lastPt = null; }

    // 注册全局事件
    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('pointerup', handlePointerUp);
    document.addEventListener('pointermove', handlePointerMove);
    document.addEventListener('touchmove', handleTouchMove, { passive: true });

    // 动画循环 — 空闲时(无波纹)降频到 10fps，不做任何逐像素运算
    let animId = 0, idleTimer = 0, skip = 0;
    function loop() {
      if (active) {
        skip++;
        if (skip % 2 === 0) stepWater();
        renderWater();
        animId = requestAnimationFrame(loop);
      } else {
        // 空闲时降频，释放 CPU 给首页大量卡片渲染
        idleTimer = window.setTimeout(loop, 100);
      }
    }
    animId = requestAnimationFrame(loop);

    simRef.current = {
      W, H, SW, SH, h1, h2, ctx, simCtx, outImg,
      running: true, dpr, lastMoveTime: 0,
    };

    return () => {
      cancelAnimationFrame(animId);
      clearTimeout(idleTimer);
      window.removeEventListener('resize', resize);
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('pointerup', handlePointerUp);
      document.removeEventListener('pointermove', handlePointerMove);
      document.removeEventListener('touchmove', handleTouchMove);
    };
  }, []);

  return (
    <canvas
      ref={canvasRef}
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 1,
        pointerEvents: 'none',
        touchAction: 'none',
      }}
    />
  );
}