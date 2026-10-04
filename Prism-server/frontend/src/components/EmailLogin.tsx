import { useState } from 'react';
import { emailLogin } from '../api';
import type { ApiResponse } from '../types';
import ResultBox from './ResultBox';

interface Props {
  onSuccess?: () => void;
}

export default function EmailLogin({ onSuccess }: Props) {
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<ApiResponse | null>(null);

  const handleSubmit = async () => {
    if (!email || !password) {
      setResult({ ok: false, error: '请填写邮箱和密码~' });
      return;
    }
    setLoading(true);
    setResult(null);
    const d = await emailLogin(email, password);
    setResult(d);
    setLoading(false);
    if (d.ok && onSuccess) onSuccess();
  };

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-icon">📧</span>
        <h2>网易邮箱登录</h2>
      </div>
      <p className="card-desc">用网易邮箱账号登录，获取游戏凭证</p>
      <input
        type="email"
        className="input"
        placeholder="邮箱地址"
        value={email}
        onChange={(e) => setEmail(e.target.value)}
      />
      <input
        type="password"
        className="input"
        placeholder="密码"
        value={password}
        onChange={(e) => setPassword(e.target.value)}
      />
      <button
        className="btn btn-primary"
        onClick={handleSubmit}
        disabled={loading}
      >
        {loading ? '登录中...' : '登录'}
      </button>
      {result && <ResultBox data={result} />}
    </div>
  );
}
