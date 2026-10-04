import { useEffect, useRef, useState } from 'react';
import steveSkin from '../assets/steve.png';
import steveHead from '../assets/steve_head.png';

type Props = {
  skinUrl?: string;
  label?: string;
  className?: string;
  hideOnFallback?: boolean;
  body?: boolean;
  size?: number;
};

function drawHead(ctx: CanvasRenderingContext2D, img: HTMLImageElement, outSize: number) {
  const isHd = img.naturalWidth >= 128;
  const u = isHd ? 16 : 8;
  ctx.canvas.width = outSize;
  ctx.canvas.height = outSize;
  ctx.clearRect(0, 0, outSize, outSize);
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(img, u, u, u, u, 0, 0, outSize, outSize);
  ctx.drawImage(img, u * 5, u, u, u, 0, 0, outSize, outSize);
}

export default function SkinHead({ skinUrl, label, className = '', hideOnFallback = false, body = false, size = 64 }: Props) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [fallback, setFallback] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const canvas = canvasRef.current;
    const ctx = canvas?.getContext('2d');
    if (!canvas || !ctx) return;

    const loadSkin = (src: string, allowFallback: boolean) => {
      const img = new Image();
      img.onload = () => {
        if (cancelled) return;
        try {
          if (body) {
            const isHd = img.naturalWidth >= 128;
            const u = isHd ? 16 : 8;
            const s = 6;
            const w = u * s;
            const h = u * 2 * s;
            ctx.canvas.width = w;
            ctx.canvas.height = h;
            ctx.clearRect(0, 0, w, h);
            ctx.imageSmoothingEnabled = false;
            ctx.drawImage(img, u, u, u, u, 0, 0, w, w);
            ctx.drawImage(img, u * 5, u, u, u, 0, 0, w, w);
            ctx.drawImage(img, u * 2, u * 2, u, u * 1.5, 0, w, w, w * 1.5);
            ctx.drawImage(img, u * 5.5, u * 2, u * 0.5, u * 1.5, -u * 0.5 * s, w, u * 0.5 * s, w * 1.5);
            ctx.drawImage(img, u * 4, u * 2, u * 0.5, u * 1.5, w, w, u * 0.5 * s, w * 1.5);
          } else {
            drawHead(ctx, img, size);
          }
          setFallback(false);
        } catch {
          if (allowFallback) { setFallback(true); loadSkin(steveSkin, false); }
        }
      };
      img.onerror = () => {
        if (!cancelled && allowFallback) {
          setFallback(true);
          if (!hideOnFallback) loadSkin(steveSkin, false);
        }
      };
      img.src = src;
    };

    loadSkin(skinUrl || steveSkin, Boolean(skinUrl));
    return () => { cancelled = true; };
  }, [skinUrl, body, size]);

  const isBg = className === 'game-card-skin-bg';
  const canvasSize = body ? { width: size * 1.5, height: size * 3 } : { width: size, height: size };
  const containerStyle = isBg ? {
    position: 'absolute' as const, inset: 0,
    overflow: 'hidden',
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    opacity: 0.32, filter: 'saturate(1.2) contrast(1.08)',
    pointerEvents: 'none' as const,
  } : {
    width: canvasSize.width,
    height: canvasSize.height,
    borderRadius: Math.round(size / 6),
    overflow: 'hidden',
    background: '#2b2118',
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
  };

  return (
    <div
      className={`${className} ${fallback && hideOnFallback ? 'skin-head-hidden' : ''}`.trim()}
      title={fallback ? '皮肤加载失败' : label}
      style={containerStyle}
    >
      <canvas
        ref={canvasRef}
        width={canvasSize.width}
        height={canvasSize.height}
        style={{ display: 'block', imageRendering: 'pixelated', width: '100%', height: '100%' }}
      />
      <img src={steveHead} alt="fallback" style={{ display: 'none' }} />
    </div>
  );
}
