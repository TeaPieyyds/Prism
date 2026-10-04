import { useEffect, useRef } from 'react';

/* 可交互粒子场：柔和光点缓慢上浮，靠近光标轻轻让开。
 * 预渲染径向光晕 sprite + additive 混合，单层 canvas 由 GPU 合成，极轻量。 */

type P = {
  x: number; y: number; vx: number; vy: number;
  r: number; hue: number; alpha: number; tw: number; phase: number;
};

const COUNT = 64;
const REPEL = 130;

/* 点击迸发粒子：点击处生成，向外散开并衰减消失 */
type B = {
  x: number; y: number; vx: number; vy: number;
  r: number; hue: number; alpha: number; life: number; maxLife: number;
};

function makeGlow(size: number): HTMLCanvasElement {
  const c = document.createElement('canvas');
  c.width = c.height = size;
  const ctx = c.getContext('2d')!;
  const g = ctx.createRadialGradient(size / 2, size / 2, 0, size / 2, size / 2, size / 2);
  g.addColorStop(0, 'rgba(255,255,255,1)');
  g.addColorStop(0.22, 'rgba(255,255,255,0.55)');
  g.addColorStop(1, 'rgba(255,255,255,0)');
  ctx.fillStyle = g;
  ctx.fillRect(0, 0, size, size);
  return c;
}

export default function ParticleField() {
  const ref = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = ref.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const dpr = Math.min(2, window.devicePixelRatio || 1);
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const glow = makeGlow(96);
    let W = 0, H = 0;
    let parts: P[] = [];
    let bursts: B[] = [];
    let mx = -9999, my = -9999;
    let raf = 0, t = 0;

    function spawn(anywhere: boolean): P {
      return {
        x: Math.random() * W,
        y: anywhere ? Math.random() * H : H + 12,
        vx: (Math.random() - 0.5) * 0.16,
        vy: -(0.05 + Math.random() * 0.22),
        r: 1 + Math.random() * 2.4,
        hue: [210, 265, 190][(Math.random() * 3) | 0],
        alpha: 0.25 + Math.random() * 0.4,
        tw: Math.random() * Math.PI * 2,
        phase: Math.random() * Math.PI * 2,
      };
    }

    function resize() {
      W = window.innerWidth; H = window.innerHeight;
      canvas!.width = W * dpr; canvas!.height = H * dpr;
      canvas!.style.width = W + 'px'; canvas!.style.height = H + 'px';
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0);
      parts = Array.from({ length: COUNT }, () => spawn(true));
    }

    // 点击处迸发 6~8 颗临时粒子
    function spawnBurst(x: number, y: number) {
      const hue = [210, 265, 190][(Math.random() * 3) | 0];
      const n = 6 + ((Math.random() * 3) | 0);
      for (let i = 0; i < n; i++) {
        const angle = Math.random() * Math.PI * 2;
        const speed = 0.8 + Math.random() * 2.4;
        bursts.push({
          x, y,
          vx: Math.cos(angle) * speed,
          vy: Math.sin(angle) * speed,
          r: 1 + Math.random() * 2.2,
          hue,
          alpha: 0.8,
          life: 0,
          maxLife: 0.5 + Math.random() * 0.4,
        });
      }
    }

    function draw() {
      ctx!.clearRect(0, 0, W, H);
      ctx!.globalCompositeOperation = 'lighter';
      const mouseActive = mx > -9990;
      for (let i = 0; i < parts.length; i++) {
        const p = parts[i];
        p.x += p.vx + Math.sin(t * 0.6 + p.phase) * 0.05;
        p.y += p.vy;

        if (mouseActive) {
          const dx = p.x - mx, dy = p.y - my;
          const d2 = dx * dx + dy * dy;
          if (d2 < REPEL * REPEL && d2 > 0.01) {
            const d = Math.sqrt(d2), f = (1 - d / REPEL) * 1.5;
            p.x += (dx / d) * f; p.y += (dy / d) * f;
          }
        }

        if (p.y < -14) Object.assign(p, spawn(false));
        if (p.x < -14) p.x = W + 12;
        else if (p.x > W + 14) p.x = -12;

        const tw = 0.55 + 0.45 * Math.sin(t * 1.3 + p.tw);
        const a = p.alpha * tw;
        const g = p.r * 6;

        ctx!.globalAlpha = a;
        ctx!.fillStyle = 'hsl(' + p.hue + ', 90%, 72%)';
        ctx!.beginPath();
        ctx!.arc(p.x, p.y, p.r, 0, Math.PI * 2);
        ctx!.fill();

        ctx!.globalAlpha = a * 0.55;
        ctx!.drawImage(glow, p.x - g / 2, p.y - g / 2, g, g);
      }

      // 点击迸发：散开 + 减速 + 衰减，寿命到移除
      for (let i = bursts.length - 1; i >= 0; i--) {
        const b = bursts[i];
        b.life += 0.016;
        if (b.life >= b.maxLife) { bursts.splice(i, 1); continue; }
        b.vx *= 0.94; b.vy *= 0.94;
        b.x += b.vx; b.y += b.vy;
        const a = b.alpha * (1 - b.life / b.maxLife);
        const g = b.r * 7;
        ctx!.globalAlpha = a;
        ctx!.fillStyle = 'hsl(' + b.hue + ', 90%, 72%)';
        ctx!.beginPath();
        ctx!.arc(b.x, b.y, b.r, 0, Math.PI * 2);
        ctx!.fill();
        ctx!.globalAlpha = a * 0.6;
        ctx!.drawImage(glow, b.x - g / 2, b.y - g / 2, g, g);
      }

      ctx!.globalAlpha = 1;
      ctx!.globalCompositeOperation = 'source-over';
    }

    function frame() { t += 0.016; draw(); raf = requestAnimationFrame(frame); }
    function onMove(e: PointerEvent) { mx = e.clientX; my = e.clientY; }
    function onLeave() { mx = -9999; my = -9999; }
    function onDown(e: PointerEvent) {
      // 点击处迸发反馈（reduced-motion 下不做动画，忽略）
      if (reduced) return;
      spawnBurst(e.clientX, e.clientY);
    }

    resize();
    window.addEventListener('resize', resize);
    document.addEventListener('pointermove', onMove, { passive: true });
    document.addEventListener('pointerdown', onDown, { passive: true });
    document.documentElement.addEventListener('mouseleave', onLeave);

    if (reduced) draw(); else raf = requestAnimationFrame(frame);

    return () => {
      cancelAnimationFrame(raf);
      window.removeEventListener('resize', resize);
      document.removeEventListener('pointermove', onMove);
      document.removeEventListener('pointerdown', onDown);
      document.documentElement.removeEventListener('mouseleave', onLeave);
    };
  }, []);

  return (
    <canvas
      ref={ref}
      style={{ position: 'fixed', inset: 0, zIndex: 0, pointerEvents: 'none' }}
    />
  );
}
