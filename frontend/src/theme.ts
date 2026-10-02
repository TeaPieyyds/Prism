type Theme = 'light' | 'dark' | 'auto';

// 视觉风格主题：glass=液态玻璃(默认)，animal=动物岛(原默认)
type Style = 'glass' | 'animal';

const KEY = 'prism-theme';
const STYLE_KEY = 'prism-style';

export function getStyle(): Style {
  return (localStorage.getItem(STYLE_KEY) as Style) || 'glass';
}

export function setStyle(s: Style) {
  localStorage.setItem(STYLE_KEY, s);
  applyStyle(s);
}

function applyStyle(s: Style) {
  document.documentElement.setAttribute('data-style', s);
  if (s === 'glass') {
    // 流霞红蓝流光：随机旋转速度，每次加载/切换都不同
    const dur = 24 + Math.random() * 24; // 24~48 秒一轮
    document.documentElement.style.setProperty('--lg-aurora-dur', dur.toFixed(1) + 's');
  }
}

export function cycleStyle(): Style {
  const current = getStyle();
  const next: Style = current === 'glass' ? 'animal' : 'glass';
  setStyle(next);
  return next;
}

export function styleLabel(s: Style): string {
  return s === 'glass' ? '液态玻璃' : '动物岛';
}

export function getTheme(): Theme {
  return (localStorage.getItem(KEY) as Theme) || 'auto';
}

export function setTheme(t: Theme) {
  localStorage.setItem(KEY, t);
  applyTheme(t);
}

function applyTheme(t: Theme) {
  const resolved = t === 'auto' ? systemTheme() : t;
  document.documentElement.setAttribute('data-theme', resolved);
}

function systemTheme(): 'light' | 'dark' {
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function initTheme() {
  applyTheme(getTheme());
  applyStyle(getStyle());
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (getTheme() === 'auto') {
      applyTheme('auto');
    }
  });
}

export function cycleTheme(): Theme {
  const current = getTheme();
  const next: Theme = current === 'light' ? 'dark' : current === 'dark' ? 'auto' : 'light';
  setTheme(next);
  return next;
}

export function themeLabel(t: Theme): string {
  return t === 'light' ? '浅色' : t === 'dark' ? '深色' : '自动';
}
