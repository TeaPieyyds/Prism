import { useEffect, useState } from 'react';
import './MusicPlayer.css';

/**
 * MusicPlayer - 浮窗音乐播放器
 * 基于 APlayer + Meting2，从 theme-sky-blog-1 移植
 * 歌单: 网易云 8854508490
 */
export default function MusicPlayer() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    // 加载 APlayer 和 Meting2 脚本
    const loadScripts = async () => {
      const scripts = [
        '/assets/js/APlayer.min.js',
        '/assets/js/Meting2.min.js',
      ];
      for (const src of scripts) {
        if (!document.querySelector(`script[src="${src}"]`)) {
          await new Promise<void>((resolve, reject) => {
            const s = document.createElement('script');
            s.src = src;
            s.async = true;
            s.onload = () => resolve();
            s.onerror = () => reject(new Error(`Failed to load ${src}`));
            document.head.appendChild(s);
          });
        }
      }
    };

    loadScripts()
      .then(() => setReady(true))
      .catch(console.error);
  }, []);

  // 当脚本就绪且 DOM 渲染后，初始化播放器逻辑
  useEffect(() => {
    if (!ready) return;

    const el = document.getElementById('nav-music');
    if (!el) return;

    const tips = document.getElementById('nav-music-hoverTips')!;
    const panel = document.getElementById('music-panel')!;
    let ap: any = null;
    let playing = false;
    let panelOpen = false;
    const MIN_PANEL_WIDTH = 280;

    // ===== 工具函数 =====
    const fmtTime = (s: number) => {
      if (!s || isNaN(s)) return '0:00';
      s = Math.floor(s);
      return Math.floor(s / 60) + ':' + (s % 60 < 10 ? '0' : '') + (s % 60);
    };

    const getPrimaryColor = () => {
      return getComputedStyle(document.documentElement)
        .getPropertyValue('--phx-primary').trim() || '#19c8b9';
    };

    const applyThemeColor = (inst: any) => {
      if (inst) inst.theme(getPrimaryColor());
    };

    // 保存播放状态
    const savePlayState = () => {
      if (!ap) return;
      try {
        localStorage.setItem('musicPlayerState', JSON.stringify({
          index: ap.list.index,
          time: ap.audio ? ap.audio.currentTime : 0,
          volume: ap.audio ? ap.audio.volume : 0.7,
          timestamp: Date.now(),
        }));
      } catch (e) { /* ignore */ }
    };

    // 恢复播放状态
    const restorePlayState = () => {
      if (!ap || !ap.list.audios || !ap.list.audios.length) return;
      try {
        const saved = localStorage.getItem('musicPlayerState');
        if (!saved) return;
        const state = JSON.parse(saved);
        if (Date.now() - state.timestamp > 24 * 60 * 60 * 1000) {
          localStorage.removeItem('musicPlayerState');
          return;
        }
        if (state.volume !== undefined) {
          ap.volume(state.volume);
          const levelEl = document.getElementById('mpVolumeLevel');
          if (levelEl) levelEl.style.width = (state.volume * 100) + '%';
        }
        if (state.index !== undefined && state.index >= 0 && state.index < ap.list.audios.length) {
          const needSwitch = ap.list.index !== state.index;
          const savedTime = state.time || 0;
          if (needSwitch) ap.list.switch(state.index);
          const seekOnLoad = () => {
            if (savedTime > 0 && ap.audio && ap.audio.duration) ap.seek(savedTime);
            if (ap.audio) ap.audio.removeEventListener('canplay', seekOnLoad);
          };
          if (ap.audio && ap.audio.readyState >= 2) {
            if (savedTime > 0) ap.seek(savedTime);
          } else if (ap.audio) {
            ap.audio.addEventListener('canplay', seekOnLoad);
          }
        }
      } catch (e) { /* ignore */ }
    };

    // ===== 面板状态同步 =====
    let progressDragging = false;
    let volumeDragging = false;

    const syncPanelState = () => {
      if (!ap || !panelOpen) return;
      if (progressDragging) return;
      const audio = ap.audio;
      const cur = ap.list.audios[ap.list.index];

      const coverEl = document.getElementById('mpCover') as HTMLImageElement;
      if (coverEl && cur) coverEl.src = cur.cover || '';

      const navCover = document.getElementById('navMusicCover');
      if (navCover && cur?.cover) {
        navCover.style.backgroundImage = 'url(' + cur.cover + ')';
      }

      const titleEl = document.getElementById('mpTitle');
      const artistEl = document.getElementById('mpArtist');
      if (titleEl && cur) titleEl.textContent = cur.name || 'Unknown';
      if (artistEl && cur) artistEl.textContent = cur.artist || 'Unknown';

      const currentEl = document.getElementById('mpCurrent');
      const durationEl = document.getElementById('mpDuration');
      if (currentEl) currentEl.textContent = fmtTime(audio.currentTime);
      if (durationEl) durationEl.textContent = fmtTime(audio.duration);

      const pct = audio.duration ? (audio.currentTime / audio.duration * 100) : 0;
      const playedEl = document.getElementById('mpPlayed');
      if (playedEl) playedEl.style.width = pct + '%';

      const iconPlay = document.querySelector('.mp-icon-play') as HTMLElement;
      const iconPause = document.querySelector('.mp-icon-pause') as HTMLElement;
      if (iconPlay && iconPause) {
        iconPlay.style.display = audio.paused ? '' : 'none';
        iconPause.style.display = audio.paused ? 'none' : '';
      }

      const volPct = Math.round(audio.volume * 100);
      const volLevel = document.getElementById('mpVolumeLevel');
      if (volLevel) volLevel.style.width = volPct + '%';

      const volIcon = document.querySelector('.mp-icon-vol') as HTMLElement;
      const muteIcon = document.querySelector('.mp-icon-mute') as HTMLElement;
      if (volIcon && muteIcon) {
        volIcon.style.display = audio.volume > 0 ? '' : 'none';
        muteIcon.style.display = audio.volume > 0 ? 'none' : '';
      }
    };

    // ===== 歌词 =====
    let lrcData: Array<{ time: number; text: string }> = [];
    let lrcIndex = -1;
    let lrcVisible = false;

    const parseLrc = (lrc: string) => {
      if (!lrc) return [];
      const result: Array<{ time: number; text: string }> = [];
      lrc.split('\n').forEach(line => {
        const timeRegex = /\[(\d{2}):(\d{2})\.(\d{2,3})\]/g;
        let match;
        const times: number[] = [];
        while ((match = timeRegex.exec(line)) !== null) {
          const t = parseInt(match[1]) * 60 + parseInt(match[2]) +
            parseInt(match[3]) / (match[3].length === 2 ? 100 : 1000);
          times.push(t);
        }
        const text = line.replace(/\[\d{2}:\d{2}\.\d{2,3}\]/g, '').trim();
        if (!text) return;
        times.forEach(t => result.push({ time: t, text }));
      });
      return result.sort((a, b) => a.time - b.time);
    };

    const updateLrc = (currentTime: number) => {
      if (!lrcData.length || !lrcVisible) return;
      let newIndex = -1;
      for (let i = 0; i < lrcData.length; i++) {
        if (lrcData[i].time <= currentTime) newIndex = i;
        else break;
      }
      if (newIndex !== lrcIndex) {
        lrcIndex = newIndex;
        const lrcEl = document.getElementById('musicLrcContent');
        if (lrcEl) {
          lrcEl.textContent = (newIndex >= 0 && lrcData[newIndex])
            ? lrcData[newIndex].text : '';
        }
      }
    };

    const toggleLrc = () => {
      const lrcEl = document.getElementById('music-lrc');
      const btn = document.getElementById('mpLrcToggle');
      lrcVisible = !lrcVisible;
      if (lrcVisible) {
        if (lrcEl) lrcEl.style.display = '';
        if (btn) btn.classList.add('active');
        loadCurrentLrc();
        if (ap) updateLrc(ap.audio.currentTime);
      } else {
        if (lrcEl) lrcEl.style.display = 'none';
        if (btn) btn.classList.remove('active');
      }
    };

    const loadCurrentLrc = () => {
      if (!ap) return;
      const cur = ap.list.audios[ap.list.index];
      const lrcEl = document.getElementById('musicLrcContent');
      if (cur && cur.lrc) {
        const lrcText = cur.lrc;
        if (lrcText.startsWith('http')) {
          fetch(lrcText)
            .then(r => r.text())
            .then(text => {
              lrcData = parseLrc(text);
              if (lrcEl && lrcData.length > 0) lrcEl.textContent = lrcData[0].text;
            })
            .catch(() => { lrcData = []; if (lrcEl) lrcEl.textContent = '♪ 暂无歌词 ♪'; });
        } else {
          lrcData = parseLrc(lrcText);
          if (lrcEl) lrcEl.textContent = lrcData.length > 0 ? lrcData[0].text : '♪ 暂无歌词 ♪';
        }
      } else {
        lrcData = [];
        if (lrcEl) lrcEl.textContent = '♪ 暂无歌词 ♪';
      }
      lrcIndex = -1;
    };

    // ===== 播放列表 =====
    const renderPlaylist = () => {
      if (!ap) return;
      const ul = document.getElementById('mpPlaylist');
      if (!ul) return;
      ul.innerHTML = '';
      ap.list.audios.forEach((song: any, i: number) => {
        const li = document.createElement('li');
        if (i === ap.list.index) li.classList.add('mp-list-active');
        const idx = document.createElement('span');
        idx.className = 'mp-list-idx';
        idx.textContent = String(i + 1);
        const name = document.createElement('span');
        name.className = 'mp-list-name';
        name.textContent = song.name || 'Unknown';
        const artist = document.createElement('span');
        artist.className = 'mp-list-artist';
        artist.textContent = song.artist || '';
        li.appendChild(idx);
        li.appendChild(name);
        li.appendChild(artist);
        li.onclick = (e) => {
          e.preventDefault();
          e.stopPropagation();
          ap.list.switch(i);
          ap.play();
        };
        ul.appendChild(li);
      });
    };

    // ===== 面板操作 =====
    const openPanel = () => {
      if (panelOpen) return;
      panel.style.display = '';
      void panel.offsetHeight;
      panel.classList.add('active');
      panelOpen = true;
      syncPanelState();
    };

    const closePanel = () => {
      if (!panelOpen) return;
      panel.classList.remove('active');
      panelOpen = false;
      const handler = () => {
        if (!panel.classList.contains('active')) panel.style.display = 'none';
        panel.removeEventListener('transitionend', handler);
      };
      panel.addEventListener('transitionend', handler);
    };

    const togglePanel = () => {
      if (panelOpen) closePanel();
      else openPanel();
    };

    // ===== 进度条拖动 =====
    const initProgressDrag = () => {
      const progressBar = document.getElementById('mpProgress');
      if (!progressBar) return;
      let lastPct = 0;

      const getPct = (e: MouseEvent | TouchEvent, rect: DOMRect) => {
        const clientX = 'touches' in e ? e.touches[0].clientX : (e as MouseEvent).clientX;
        return Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
      };

      const onStart = (e: MouseEvent | TouchEvent) => {
        e.preventDefault();
        e.stopPropagation();
        progressDragging = true;
        const rect = progressBar.getBoundingClientRect();
        lastPct = getPct(e, rect);

        const onMove = (e: MouseEvent | TouchEvent) => {
          if (!progressDragging) return;
          e.preventDefault();
          lastPct = getPct(e, rect);
          const playedEl = document.getElementById('mpPlayed');
          if (playedEl) playedEl.style.width = (lastPct * 100) + '%';
        };
        const onEnd = () => {
          if (!progressDragging) return;
          if (ap && ap.audio && ap.audio.duration) ap.seek(lastPct * ap.audio.duration);
          setTimeout(() => { progressDragging = false; savePlayState(); }, 500);
          document.removeEventListener('mousemove', onMove);
          document.removeEventListener('mouseup', onEnd);
          document.removeEventListener('touchmove', onMove);
          document.removeEventListener('touchend', onEnd);
        };
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onEnd);
        document.addEventListener('touchmove', onMove, { passive: false });
        document.addEventListener('touchend', onEnd);
      };

      progressBar.addEventListener('mousedown', onStart);
      progressBar.addEventListener('touchstart', onStart, { passive: false });
      progressBar.addEventListener('click', (e) => {
        e.stopPropagation();
        progressDragging = true;
        const rect = progressBar.getBoundingClientRect();
        const pct = getPct(e, rect);
        const playedEl = document.getElementById('mpPlayed');
        if (playedEl) playedEl.style.width = (pct * 100) + '%';
        if (ap && ap.audio && ap.audio.duration) ap.seek(pct * ap.audio.duration);
        setTimeout(() => { progressDragging = false; savePlayState(); }, 500);
      });
    };

    // ===== 音量条拖动 =====
    const initVolumeDrag = () => {
      const volBar = document.getElementById('mpVolumeBar');
      if (!volBar) return;
      let lastVol = 0.7;

      const getPct = (e: MouseEvent | TouchEvent, rect: DOMRect) => {
        const clientX = 'touches' in e ? e.touches[0].clientX : (e as MouseEvent).clientX;
        return Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
      };

      const onStart = (e: MouseEvent | TouchEvent) => {
        e.preventDefault();
        e.stopPropagation();
        volumeDragging = true;
        const rect = volBar.getBoundingClientRect();
        lastVol = getPct(e, rect);
        const levelEl = document.getElementById('mpVolumeLevel');
        if (levelEl) levelEl.style.width = (lastVol * 100) + '%';
        if (ap) ap.volume(lastVol);

        const onMove = (e: MouseEvent | TouchEvent) => {
          if (!volumeDragging) return;
          e.preventDefault();
          lastVol = getPct(e, rect);
          const levelEl = document.getElementById('mpVolumeLevel');
          if (levelEl) levelEl.style.width = (lastVol * 100) + '%';
          if (ap) ap.volume(lastVol);
        };
        const onEnd = () => {
          volumeDragging = false;
          document.removeEventListener('mousemove', onMove);
          document.removeEventListener('mouseup', onEnd);
          document.removeEventListener('touchmove', onMove);
          document.removeEventListener('touchend', onEnd);
        };
        document.addEventListener('mousemove', onMove);
        document.addEventListener('mouseup', onEnd);
        document.addEventListener('touchmove', onMove, { passive: false });
        document.addEventListener('touchend', onEnd);
      };

      volBar.addEventListener('mousedown', onStart);
      volBar.addEventListener('touchstart', onStart, { passive: false });
      volBar.addEventListener('click', (e) => {
        e.stopPropagation();
        const rect = volBar.getBoundingClientRect();
        const pct = getPct(e, rect);
        const levelEl = document.getElementById('mpVolumeLevel');
        if (levelEl) levelEl.style.width = (pct * 100) + '%';
        if (ap) ap.volume(pct);
      });
    };

    // ===== 模式图标更新 =====
    const updateModeIcons = () => {
      if (!ap) return;
      const modeBtn = document.getElementById('mpMode');
      const loopBtn = document.getElementById('mpLoop');
      if (modeBtn) {
        const isRandom = ap.options?.order === 'random';
        modeBtn.classList.toggle('active', isRandom);
        const orderIcon = modeBtn.querySelector('.mp-icon-order') as HTMLElement;
        const randomIcon = modeBtn.querySelector('.mp-icon-random') as HTMLElement;
        if (orderIcon) orderIcon.style.display = isRandom ? 'none' : '';
        if (randomIcon) randomIcon.style.display = isRandom ? '' : 'none';
      }
      if (loopBtn) {
        const loop = ap.options?.loop || 'all';
        const icons = {
          all: loopBtn.querySelector('.mp-icon-loop-all') as HTMLElement,
          one: loopBtn.querySelector('.mp-icon-loop-one') as HTMLElement,
          none: loopBtn.querySelector('.mp-icon-loop-none') as HTMLElement,
        };
        if (icons.all) icons.all.style.display = loop === 'all' ? '' : 'none';
        if (icons.one) icons.one.style.display = loop === 'one' ? '' : 'none';
        if (icons.none) icons.none.style.display = loop === 'none' ? '' : 'none';
      }
    };

    // ===== 绑定面板事件 =====
    const bindPanelEvents = () => {
      document.getElementById('mpPlay')?.addEventListener('click', (e) => {
        e.stopPropagation();
        ap?.toggle();
      });
      document.getElementById('mpPrev')?.addEventListener('click', (e) => {
        e.stopPropagation();
        ap?.skipBack();
      });
      document.getElementById('mpNext')?.addEventListener('click', (e) => {
        e.stopPropagation();
        ap?.skipForward();
      });
      document.getElementById('mpMode')?.addEventListener('click', (e) => {
        e.stopPropagation();
        if (!ap) return;
        ap.options.order = ap.options.order === 'random' ? 'list' : 'random';
        updateModeIcons();
      });
      document.getElementById('mpLoop')?.addEventListener('click', (e) => {
        e.stopPropagation();
        if (!ap) return;
        const cur = ap.options.loop || 'all';
        ap.options.loop = cur === 'all' ? 'one' : (cur === 'one' ? 'none' : 'all');
        updateModeIcons();
      });
      document.getElementById('mpListToggle')?.addEventListener('click', (e) => {
        e.stopPropagation();
        e.preventDefault();
        const listArea = document.getElementById('mpListArea');
        const btn = document.getElementById('mpListToggle');
        if (!listArea) return;
        if (listArea.style.display === 'none' || !listArea.style.display) {
          listArea.style.display = 'block';
          btn?.classList.add('active');
          renderPlaylist();
        } else {
          listArea.style.display = 'none';
          btn?.classList.remove('active');
        }
      });
      document.getElementById('mpLrcToggle')?.addEventListener('click', (e) => {
        e.stopPropagation();
        e.preventDefault();
        toggleLrc();
      });
      let savedVol = 0.7;
      document.getElementById('mpVolBtn')?.addEventListener('click', (e) => {
        e.stopPropagation();
        if (!ap) return;
        if (ap.audio.volume > 0) {
          savedVol = ap.audio.volume;
          ap.volume(0);
        } else {
          ap.volume(savedVol);
        }
        syncPanelState();
      });
      initProgressDrag();
      initVolumeDrag();
    };

    // ===== 定时同步 =====
    let syncTimer: ReturnType<typeof setInterval> | null = null;
    let saveCounter = 0;

    const startSync = () => {
      if (syncTimer) return;
      syncTimer = setInterval(() => {
        if (panelOpen && !progressDragging) syncPanelState();
        if (lrcVisible && ap && !progressDragging) updateLrc(ap.audio.currentTime);
        saveCounter++;
        if (saveCounter >= 25) { saveCounter = 0; if (playing) savePlayState(); }
      }, 200);
    };

    // ===== 监听主题变化 =====
    const observer = new MutationObserver(() => {
      if (ap) applyThemeColor(ap);
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme', 'data-color-scheme'],
    });

    // ===== 等待 APlayer 初始化 =====
    const waitForAPlayer = setInterval(() => {
      const metingEl = el.querySelector('meting-js') as any;
      if (metingEl && metingEl.aplayer) {
        clearInterval(waitForAPlayer);
        ap = metingEl.aplayer;
        restorePlayState();
        applyThemeColor(ap);

        // 更新迷你悬浮窗封面（不受面板打开/关闭影响）
        const updateMiniCover = () => {
          if (!ap) return;
          const cur = ap.list.audios[ap.list.index];
          const navCover = document.getElementById('navMusicCover');
          if (navCover && cur?.cover) {
            navCover.style.backgroundImage = 'url(' + cur.cover + ')';
          }
        };

        ap.on('play', () => {
          if (!playing) {
            el.classList.add('playing');
            tips.textContent = '暂停音乐';
            playing = true;
          }
          updateMiniCover();
          // 歌词只在用户手动开启时显示，不再自动弹出
          if (lrcVisible) {
            const lrcEl = document.getElementById('music-lrc');
            if (lrcEl) lrcEl.style.display = '';
          }
          syncPanelState();
          renderPlaylist();
          loadCurrentLrc();
          updateModeIcons();
          savePlayState();
        });

        ap.on('pause', () => {
          if (playing) {
            el.classList.remove('playing');
            tips.textContent = '播放音乐';
            playing = false;
          }
          // 暂停时隐藏歌词
          if (lrcVisible) {
            const lrcEl = document.getElementById('music-lrc');
            if (lrcEl) lrcEl.style.display = 'none';
          }
          syncPanelState();
          savePlayState();
        });

        ap.on('listswitch', () => {
          setTimeout(() => {
            updateMiniCover();
            syncPanelState();
            renderPlaylist();
            loadCurrentLrc();
            savePlayState();
          }, 100);
        });

        bindPanelEvents();
        startSync();

        // 强制自动播放：模拟点击迷你播放器（和用户点击行为完全一致）
        if (el.getAttribute('data-autoplay') === 'true') {
          (function tryPlay(retries) {
            if (retries > 20) return;
            if (!ap || !ap.audio) return setTimeout(tryPlay, 500, retries);
            tips.click();
            setTimeout(function() {
              if (ap.audio.paused) tryPlay(retries + 1);
            }, 1500);
          })(0);
        }
      }
    }, 500);

    // ===== 迷你按钮事件 =====
    tips.addEventListener('click', (e) => {
      e.stopPropagation();
      if (panelOpen) {
        closePanel();
      } else if (!playing) {
        if (ap) ap.toggle();
        openPanel();
      } else {
        if (ap) ap.toggle();
      }
    });

    el.addEventListener('dblclick', (e) => {
      e.preventDefault();
      e.stopPropagation();
      togglePanel();
    });

    el.addEventListener('click', (e) => {
      if ((e.target as HTMLElement).closest('.nav-music-tips')) return;
      if ((e.target as HTMLElement).closest('.aplayer-button')) return;
      if ((e.target as HTMLElement).closest('.mp-btn')) return;
      togglePanel();
    });

    // ===== 构建面板宽度 =====
    const syncPanelWidth = () => {
      const w = el.offsetWidth;
      panel.style.width = Math.max(w, MIN_PANEL_WIDTH) + 'px';
    };
    const resizeObs = new ResizeObserver(() => syncPanelWidth());
    resizeObs.observe(el);

    // ===== 清理 =====
    return () => {
      clearInterval(waitForAPlayer);
      if (syncTimer) clearInterval(syncTimer);
      observer.disconnect();
      resizeObs.disconnect();
      tips.replaceWith(tips.cloneNode(true));
      el.replaceWith(el.cloneNode(true));
    };
  }, [ready]);

  return (
    <>
      {/* 迷你播放器按钮 */}
      <div id="nav-music" data-autoplay="false">
        <a id="nav-music-hoverTips" className="nav-music-tips">播放音乐</a>
        <div className="nav-music-cover" id="navMusicCover"></div>
        <div className="nav-music-icon">
          <svg className="icon-pause" viewBox="0 0 24 24" fill="currentColor">
            <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
          </svg>
          <svg className="icon-play" viewBox="0 0 24 24" fill="currentColor">
            <path d="M8 5v14l11-7z" />
          </svg>
        </div>
        {/* @ts-expect-error Meting 自定义元素 */}
        <meting-js
          server="netease"
          type="playlist"
          id="8854508490"
          mutex="true"
          preload="auto"
          order="random" />
      </div>

      {/* 展开式播放面板 */}
      <div id="music-panel" className="music-panel" style={{ display: 'none' }}>
        <div className="music-panel-inner">
          {/* 顶部 */}
          <div className="mp-header">
            <div className="mp-cover">
              <img id="mpCover" src="" alt="cover" />
            </div>
            <div className="mp-song-info">
              <div className="mp-title" id="mpTitle">正在加载...</div>
              <div className="mp-artist" id="mpArtist">-</div>
              <div className="mp-time"><span id="mpCurrent">0:00</span> / <span id="mpDuration">0:00</span></div>
            </div>
            <div className="mp-header-actions">
              <button className="mp-btn mp-btn-lrc" id="mpLrcToggle" title="歌词" aria-label="歌词">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <path d="M9 18V5l12-2v13" />
                  <circle cx="6" cy="18" r="3" />
                  <circle cx="18" cy="16" r="3" />
                </svg>
              </button>
              <button className="mp-btn mp-btn-list" id="mpListToggle" title="播放列表" aria-label="播放列表">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <line x1="8" y1="6" x2="21" y2="6" />
                  <line x1="8" y1="12" x2="21" y2="12" />
                  <line x1="8" y1="18" x2="21" y2="18" />
                  <line x1="3" y1="6" x2="3.01" y2="6" />
                  <line x1="3" y1="12" x2="3.01" y2="12" />
                  <line x1="3" y1="18" x2="3.01" y2="18" />
                </svg>
              </button>
            </div>
          </div>

          {/* 播放列表 */}
          <div className="mp-list-area" id="mpListArea" style={{ display: 'none' }}>
            <ul className="mp-playlist" id="mpPlaylist"></ul>
          </div>

          {/* 进度条 */}
          <div className="mp-progress-wrap">
            <div className="mp-progress" id="mpProgress">
              <div className="mp-progress-played" id="mpPlayed">
                <div className="mp-progress-thumb" id="mpThumb"></div>
              </div>
            </div>
          </div>

          {/* 控制按钮 */}
          <div className="mp-controls">
            <button className="mp-btn mp-btn-mode" id="mpMode" title="播放模式" aria-label="播放模式">
              <svg className="mp-icon-order" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="8" y1="6" x2="21" y2="6" />
                <line x1="8" y1="12" x2="21" y2="12" />
                <line x1="8" y1="18" x2="21" y2="18" />
                <line x1="3" y1="6" x2="3.01" y2="6" />
                <line x1="3" y1="12" x2="3.01" y2="12" />
                <line x1="3" y1="18" x2="3.01" y2="18" />
              </svg>
              <svg className="mp-icon-random" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ display: 'none' }}>
                <polyline points="16 3 21 3 21 8" />
                <line x1="4" y1="20" x2="21" y2="3" />
                <polyline points="21 16 21 21 16 21" />
                <line x1="15" y1="15" x2="21" y2="21" />
                <line x1="4" y1="4" x2="9" y2="9" />
                <line x1="9" y1="4" x2="4" y2="9" />
              </svg>
            </button>
            <button className="mp-btn mp-btn-prev" id="mpPrev" title="上一首" aria-label="上一首">
              <svg viewBox="0 0 24 24" fill="currentColor">
                <path d="M6 6h2v12H6zm3.5 6l8.5 6V6z" />
              </svg>
            </button>
            <button className="mp-btn mp-btn-play" id="mpPlay" title="播放/暂停" aria-label="播放或暂停">
              <svg className="mp-icon-play" viewBox="0 0 24 24" fill="currentColor">
                <path d="M8 5v14l11-7z" />
              </svg>
              <svg className="mp-icon-pause" viewBox="0 0 24 24" fill="currentColor" style={{ display: 'none' }}>
                <path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z" />
              </svg>
            </button>
            <button className="mp-btn mp-btn-next" id="mpNext" title="下一首" aria-label="下一首">
              <svg viewBox="0 0 24 24" fill="currentColor">
                <path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z" />
              </svg>
            </button>
            <button className="mp-btn mp-btn-loop" id="mpLoop" title="循环模式" aria-label="循环模式">
              <svg className="mp-icon-loop-all" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <polyline points="17 1 21 5 17 9" />
                <path d="M3 11V9a4 4 0 0 1 4-4h14" />
                <polyline points="7 23 3 19 7 15" />
                <path d="M21 13v2a4 4 0 0 1-4 4H3" />
              </svg>
              <svg className="mp-icon-loop-one" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ display: 'none' }}>
                <polyline points="17 1 21 5 17 9" />
                <path d="M3 11V9a4 4 0 0 1 4-4h14" />
                <polyline points="7 23 3 19 7 15" />
                <path d="M21 13v2a4 4 0 0 1-4 4H3" />
                <text x="12" y="14" fontSize="7" fill="currentColor" stroke="none" textAnchor="middle" fontWeight="bold">1</text>
              </svg>
              <svg className="mp-icon-loop-none" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ display: 'none' }}>
                <polyline points="17 1 21 5 17 9" />
                <path d="M3 11V9a4 4 0 0 1 4-4h14" />
                <polyline points="7 23 3 19 7 15" />
                <path d="M21 13v2a4 4 0 0 1-4 4H3" />
                <line x1="1" y1="1" x2="23" y2="23" strokeWidth="2.5" />
              </svg>
            </button>
          </div>

          {/* 音量控制 */}
          <div className="mp-footer">
            <button className="mp-btn mp-btn-vol" id="mpVolBtn" title="音量" aria-label="音量">
              <svg className="mp-icon-vol" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
                <path d="M19.07 4.93a10 10 0 0 1 0 14.14M15.54 8.46a5 5 0 0 1 0 7.07" />
              </svg>
              <svg className="mp-icon-mute" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ display: 'none' }}>
                <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
                <line x1="23" y1="9" x2="17" y2="15" />
                <line x1="17" y1="9" x2="23" y2="15" />
              </svg>
            </button>
            <div className="mp-volume-wrap">
              <div className="mp-volume-bar" id="mpVolumeBar">
                <div className="mp-volume-level" id="mpVolumeLevel">
                  <div className="mp-volume-thumb" id="mpVolumeThumb"></div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* 歌词 */}
      <div id="music-lrc" className="music-lrc" style={{ display: 'none' }}>
        <div className="music-lrc-content" id="musicLrcContent"></div>
      </div>
    </>
  );
}