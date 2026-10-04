import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../api';
import Modal from './Modal';

interface Props {
  open: boolean;
  onClose: () => void;
  accountId?: number;
}

interface HeadItem {
  id: number;
  url: string;
  name: string;
  item_id: number;
}

export default function AvatarModal({ open, onClose, accountId }: Props) {
  const [tab, setTab] = useState<'preset' | 'upload'>('preset');
  const [heads, setHeads] = useState<HeadItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [msg, setMsg] = useState('');
  const [msgOk, setMsgOk] = useState(true);
  const [confirmItem, setConfirmItem] = useState<HeadItem | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [sourceImg, setSourceImg] = useState<HTMLImageElement | null>(null);
  const [zoom, setZoom] = useState(1);
  const [panX, setPanX] = useState(0);
  const [panY, setPanY] = useState(0);
  const [isDragging, setIsDragging] = useState(false);
  const [dragStart, setDragStart] = useState({ x: 0, y: 0 });
  const [uploading, setUploading] = useState(false);

  const loadHeads = useCallback(async (acctId?: number) => {
    setLoading(true);
    setMsg('');
    const path = acctId ? `/api/avatar/list?account_id=${acctId}` : '/api/avatar/list';
    const res = await api<{ ok: boolean; entities: HeadItem[] }>('GET', path);
    if (res.ok && res.entities) {
      setHeads(res.entities.filter(h => h.url).slice(0, 10));
    } else if (!res.ok) {
      setMsg((res as any).error || '哎呀,加载失败了,请刷新页面再试~');
      setMsgOk(false);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    if (open) {
      setTab('preset');
      setMsg('');
      setSourceImg(null);
      setZoom(1);
      setPanX(0);
      setPanY(0);
      loadHeads(accountId);
    }
  }, [open, accountId, loadHeads]);

  // Draw crop preview
  useEffect(() => {
    if (!sourceImg || !canvasRef.current) return;
    const canvas = canvasRef.current;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const size = 280;
    canvas.width = size;
    canvas.height = size;

    ctx.clearRect(0, 0, size, size);

    // Dark overlay outside crop area
    ctx.fillStyle = 'rgba(0,0,0,0.5)';
    ctx.fillRect(0, 0, size, size);

    const cw = size;
    const ch = size;
    const imgW = sourceImg.naturalWidth;
    const imgH = sourceImg.naturalHeight;

    const scale = zoom * Math.max(cw / imgW, ch / imgH);
    const drawW = imgW * scale;
    const drawH = imgH * scale;
    const drawX = (cw - drawW) / 2 + panX;
    const drawY = (ch - drawH) / 2 + panY;

    ctx.save();
    ctx.beginPath();
    ctx.rect(0, 0, size, size);
    ctx.clip();
    ctx.clearRect(0, 0, size, size);
    ctx.drawImage(sourceImg, drawX, drawY, drawW, drawH);
    ctx.restore();

    // Grid overlay
    ctx.strokeStyle = 'rgba(255,255,255,0.3)';
    ctx.lineWidth = 1;
    ctx.strokeRect(size / 3, 0, size / 3, size);
    ctx.strokeRect(0, size / 3, size, size / 3);

    // White border
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 2;
    ctx.strokeRect(0, 0, size, size);
  }, [sourceImg, zoom, panX, panY]);

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const img = new Image();
      img.onload = () => {
        setSourceImg(img);
        setZoom(1);
        setPanX(0);
        setPanY(0);
      };
      img.src = ev.target?.result as string;
    };
    reader.readAsDataURL(file);
  };

  const handleCropMouseDown = (e: React.MouseEvent) => {
    setIsDragging(true);
    setDragStart({ x: e.clientX - panX, y: e.clientY - panY });
  };

  const handleCropMouseMove = (e: React.MouseEvent) => {
    if (!isDragging) return;
    setPanX(e.clientX - dragStart.x);
    setPanY(e.clientY - dragStart.y);
  };

  const handleCropMouseUp = () => setIsDragging(false);

  const doUpload = async () => {
    if (!sourceImg || !canvasRef.current) return;
    setUploading(true);
    setMsg('');

    const canvas = canvasRef.current;
    const size = 280;
    const ctx = canvas.getContext('2d');
    if (!ctx) { setUploading(false); return; }

    canvas.width = size;
    canvas.height = size;
    const cw = size, ch = size;
    const imgW = sourceImg.naturalWidth;
    const imgH = sourceImg.naturalHeight;
    const scale = zoom * Math.max(cw / imgW, ch / imgH);
    const drawW = imgW * scale;
    const drawH = imgH * scale;
    const drawX = (cw - drawW) / 2 + panX;
    const drawY = (ch - drawH) / 2 + panY;
    ctx.clearRect(0, 0, size, size);
    ctx.drawImage(sourceImg, drawX, drawY, drawW, drawH);

    canvas.toBlob(async (blob) => {
      if (!blob) { setUploading(false); return; }
      const formData = new FormData();
      formData.append('file', blob, 'avatar.jpg');
      if (accountId) formData.append('account_id', String(accountId));

      const res = await api('POST', '/api/avatar/upload', formData);
      if ((res as any).ok) {
        setMsg('头像已更新');
        setMsgOk(true);
        setTimeout(() => onClose(), 1500);
      } else {
        setMsg((res as any).error || '哎呀,上传失败了,请稍后重试~');
        setMsgOk(false);
      }
      setUploading(false);
    }, 'image/jpeg', 0.92);
  };

  const selectPreset = async (item: HeadItem) => {
    setMsg('');
    setConfirmItem(null);
    const res = await api('POST', '/api/avatar/change', accountId ? { head_image: item.url, account_id: accountId } : { head_image: item.url });
    if ((res as any).ok) {
      setMsg('头像已更新');
      setMsgOk(true);
      setTimeout(() => onClose(), 1500);
    } else {
      setMsg((res as any).error || '哎呀,设置失败了,请稍后重试~');
      setMsgOk(false);
    }
  };

  return (
    <Modal open={open} title="更换头像" message="" onCancel={onClose}>
      <div style={{ minWidth: 340 }}>
        <div style={{ display: 'flex', gap: 0, marginBottom: 14, borderBottom: '2px solid #e8e2d6' }}>
          <button
            onClick={() => { setTab('preset'); setConfirmItem(null); }}
            style={{
              flex: 1, padding: '8px 0', border: 'none', background: 'none',
              fontWeight: tab === 'preset' ? 700 : 400,
              color: tab === 'preset' ? '#d48a0e' : '#9f927d',
              borderBottom: tab === 'preset' ? '2px solid #d48a0e' : '2px solid transparent',
              marginBottom: -2, cursor: 'pointer', fontSize: 14,
            }}
          >预设头像</button>
          <button
            onClick={() => { setTab('upload'); setConfirmItem(null); }}
            style={{
              flex: 1, padding: '8px 0', border: 'none', background: 'none',
              fontWeight: tab === 'upload' ? 700 : 400,
              color: tab === 'upload' ? '#d48a0e' : '#9f927d',
              borderBottom: tab === 'upload' ? '2px solid #d48a0e' : '2px solid transparent',
              marginBottom: -2, cursor: 'pointer', fontSize: 14,
            }}
          >上传裁剪</button>
        </div>

        {tab === 'preset' && (
          <div style={{ maxHeight: 350, overflowY: 'auto' }}>
            {confirmItem && (
              <div style={{ display: 'flex', gap: 8, marginBottom: 10, padding: 8, background: '#fef7e6', borderRadius: 8, alignItems: 'center' }}>
                <img src={confirmItem.url} alt="" style={{ width: 36, height: 36, borderRadius: 8, objectFit: 'cover' }} />
                <span style={{ flex: 1, fontSize: 13, color: '#5d4b36' }}>确认使用此头像？</span>
                <button className="btn btn-sm btn-primary" onClick={() => selectPreset(confirmItem)}>确认</button>
                <button className="btn btn-sm btn-outline" onClick={() => setConfirmItem(null)}>取消</button>
              </div>
            )}
            {loading ? (
              <p style={{ textAlign: 'center', color: '#9f927d', padding: 20 }}>加载中...</p>
            ) : (
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(4, 1fr)', gap: 8 }}>
                {heads.map((h) => (
                  <div key={h.id} onClick={() => setConfirmItem(h)}
                    style={{
                      cursor: 'pointer', borderRadius: 10, overflow: 'hidden',
                      border: '2px solid ' + (confirmItem?.id === h.id ? '#d48a0e' : '#e8e2d6'),
                      transition: 'border-color 0.15s',
                    }}
                    onMouseEnter={e => (e.currentTarget.style.borderColor = '#d48a0e')}
                    onMouseLeave={e => (e.currentTarget.style.borderColor = confirmItem?.id === h.id ? '#d48a0e' : '#e8e2d6')}
                  >
                    <img src={h.url} alt="" style={{ display: 'block', width: '100%', aspectRatio: '1/1', objectFit: 'cover' }} />
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {tab === 'upload' && (
          <div>
            {!sourceImg ? (
              <div
                onClick={() => fileInputRef.current?.click()}
                style={{
                  border: '2px dashed #d0c8b8', borderRadius: 16, padding: 40,
                  textAlign: 'center', cursor: 'pointer', color: '#9f927d',
                }}
              >
                <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="#9f927d" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" style={{ marginBottom: 8 }}>
                  <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
                  <circle cx="12" cy="13" r="4" />
                </svg>
                <div>点击选择图片</div>
                <div style={{ fontSize: 12, marginTop: 4 }}>将自动裁剪为正方形</div>
              </div>
            ) : (
              <div>
                <div
                  style={{ position: 'relative', width: 280, height: 280, margin: '0 auto', cursor: isDragging ? 'grabbing' : 'grab', overflow: 'hidden', borderRadius: 12 }}
                  onMouseDown={handleCropMouseDown}
                  onMouseMove={handleCropMouseMove}
                  onMouseUp={handleCropMouseUp}
                  onMouseLeave={handleCropMouseUp}
                >
                  <canvas ref={canvasRef} width={280} height={280} style={{ display: 'block', width: 280, height: 280, borderRadius: 12 }} />
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginTop: 10 }}>
                  <span style={{ fontSize: 12, color: '#9f927d' }}>缩放</span>
                  <input
                    type="range" min="0.5" max="3" step="0.05"
                    value={zoom}
                    onChange={(e) => setZoom(parseFloat(e.target.value))}
                    style={{ flex: 1 }}
                  />
                  <span style={{ fontSize: 12, color: '#9f927d', minWidth: 30 }}>{zoom.toFixed(1)}x</span>
                </div>
                <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
                  <button className="btn btn-outline" style={{ flex: 1 }} onClick={() => { setSourceImg(null); fileInputRef.current?.click(); }}>重新选择</button>
                  <button className="btn btn-primary" style={{ flex: 1 }} onClick={doUpload} disabled={uploading}>
                    {uploading ? '上传中...' : '确认上传'}
                  </button>
                </div>
              </div>
            )}
            <input ref={fileInputRef} type="file" accept="image/jpeg,image/png,image/gif" style={{ display: 'none' }} onChange={handleFileSelect} />
          </div>
        )}

        {msg && (
          <div className={`result-box ${msgOk ? 'ok' : 'err'}`} style={{ marginTop: 12 }}>
            {msg}
          </div>
        )}
      </div>
    </Modal>
  );
}
