let clickAudio: HTMLAudioElement | null = null;
let successAudio: HTMLAudioElement | null = null;

function ensurePool() {
  if (clickAudio) return;
  clickAudio = new Audio('/soft_click.mp3');
  clickAudio.volume = 1.0; clickAudio.preload = 'auto'; clickAudio.load();
  successAudio = new Audio('/correct.mp3');
  successAudio.volume = 1.0; successAudio.preload = 'auto'; successAudio.load();
}

export function clickSound() {
  ensurePool();
  if (clickAudio) { clickAudio.currentTime = 0; clickAudio.play().catch(() => {}); }
}

export function tapSound() { clickSound(); }
export function popSound() { clickSound(); }
export function whooshSound() {}

export function successSound() {
  ensurePool();
  if (successAudio) { successAudio.currentTime = 0; successAudio.play().catch(() => {}); }
}

// ── BGM (disabled by default) ──
let bgmPlaying = false;
let bgmAudio: HTMLAudioElement | null = null;
export function startBGM() {
  if (bgmPlaying) return;
  bgmPlaying = true;
  if (!bgmAudio) {
    bgmAudio = new Audio('/bgm.mp3');
    bgmAudio.volume = 1.0; bgmAudio.loop = true;
  }
  bgmAudio.play().catch(() => {});
}
export function stopBGM() {
  bgmPlaying = false;
  if (bgmAudio) { bgmAudio.pause(); bgmAudio.currentTime = 0; }
}

// ── Haptic feedback ──
export function vibrate(ms = 10) { navigator.vibrate?.(ms); }

// ── Global hook ──
export function useGlobalSounds() {
  if (typeof window === 'undefined') return;
  ensurePool();
  const handler = (e: MouseEvent) => {
    const t = e.target as HTMLElement;
    if (t.closest('button, .btn, .btn-sm, .nav-item, .sidebar-header, .tab-strip button')) {
      clickSound();
      navigator.vibrate?.(30);
    }
  };
  document.addEventListener('click', handler);
  return () => document.removeEventListener('click', handler);
}
