import { useState } from 'react';
import { addCookie } from '../api';
import type { ApiResponse } from '../types';
import ResultBox from './ResultBox';

interface Props {
  onSuccess?: () => void;
}

export default function CookieLogin({ onSuccess }: Props) {
  const [cookieText, setCookieText] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<ApiResponse | null>(null);

  const handleSubmit = async () => {
    if (!cookieText.trim()) {
      setResult({ ok: false, error: '请粘贴 Cookie~' });
      return;
    }
    setLoading(true);
    setResult(null);
    const d = await addCookie(cookieText.trim());
    setResult(d);
    setLoading(false);
    if (d.ok && onSuccess) onSuccess();
  };

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-icon">🍪</span>
        <h2>Cookie 登录</h2>
      </div>
      <p className="card-desc">
        直接粘贴从浏览器或工具中复制的 Cookie 数据，支持多种格式自动识别
      </p>
      <textarea
        className="input textarea"
        rows={4}
        placeholder='{"sauth_json":"{\"gameid\":\"x19\"...}"}'
        value={cookieText}
        onChange={(e) => setCookieText(e.target.value)}
      />
      <button
        className="btn btn-success"
        onClick={handleSubmit}
        disabled={loading}
      >
        {loading ? '解析中...' : '解析并登录'}
      </button>
      {result && <ResultBox data={result} showCookieInfo />}
    </div>
  );
}
