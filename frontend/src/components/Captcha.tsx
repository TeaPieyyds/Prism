import { useState, useRef, useCallback, useEffect, forwardRef, useImperativeHandle } from 'react';
import { api } from '../api';

export interface CaptchaHandle {
  getChallenge: () => string;
  getAnswer: () => string;
  reset: () => void;
}

const Captcha = forwardRef<CaptchaHandle, object>(function Captcha(_props, ref) {
  const [image, setImage] = useState('');
  const [userInput, setUserInput] = useState('');
  const [loading, setLoading] = useState(true);
  const [cooldown, setCooldown] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const cidRef = useRef('');

  useImperativeHandle(ref, () => ({
    getChallenge: () => cidRef.current,
    getAnswer: () => inputRef.current?.value || '',
    reset: () => { fetchChallenge(); },
  }));

  const fetchChallenge = useCallback(async () => {
    setLoading(true);
    setUserInput('');
    const res = await api<{ ok: boolean; id: string; image: string }>('POST', '/api/auth/captcha/new');
    if (res.ok && res.id && res.image) {
      cidRef.current = res.id;
      setImage(res.image);
    }
    setLoading(false);
  }, []);

  const handleRefresh = () => {
    if (cooldown) return;
    setCooldown(true);
    fetchChallenge();
    setUserInput('');
    inputRef.current?.focus();
    setTimeout(() => setCooldown(false), 2000);
  };

  useEffect(() => { fetchChallenge(); }, [fetchChallenge]);

  if (loading) {
    return <div style={{ marginBottom: 10, padding: 12, textAlign: 'center', color: '#9f927d', border: '2px solid #e8e2d6', borderRadius: 12 }}>验证码加载中...</div>;
  }

  if (!image) {
    return <div style={{ marginBottom: 10, padding: 12, textAlign: 'center', color: '#e05a5a', border: '2px solid #e8e2d6', borderRadius: 12 }}>
      验证码加载失败<button type="button" className="btn btn-sm btn-outline" onClick={handleRefresh} style={{ marginLeft: 8 }}>重试</button>
    </div>;
  }

  return (
    <div style={{ marginBottom: 10 }}>
      <div style={{ border: '2px solid #e8e2d6', borderRadius: 12, overflow: 'hidden', marginBottom: 6 }}>
        <img src={image} alt="验证码" style={{ display: 'block', width: '100%', height: 54, objectFit: 'cover' }} />
      </div>
      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        <input
          ref={inputRef}
          className="input"
          placeholder="输入计算结果"
          value={userInput}
          onChange={(e) => setUserInput(e.target.value)}
          style={{ margin: 0, flex: 1, textAlign: 'center', fontSize: 16, letterSpacing: 2 }}
          autoComplete="off"
        />
        <button type="button" className="btn btn-sm btn-outline" onClick={handleRefresh}
          disabled={cooldown}
          style={{ width: 'auto', margin: 0, whiteSpace: 'nowrap', flexShrink: 0 }}>
          {cooldown ? '2s' : '换一张'}
        </button>
      </div>
    </div>
  );
});

export default Captcha;
