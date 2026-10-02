import { useState } from 'react';
import { sendSms, verifyPhone } from '../api';
import type { ApiResponse } from '../types';
import ResultBox from './ResultBox';

interface Props {
  onSuccess?: () => void;
}

export default function PhoneLogin({ onSuccess }: Props) {
  const [phone, setPhone] = useState('');
  const [code, setCode] = useState('');
  const [showVerify, setShowVerify] = useState(false);
  const [loading, setLoading] = useState(false);
  const [result, setResult] = useState<ApiResponse | null>(null);

  const handleSendSms = async () => {
    if (!phone) {
      setResult({ ok: false, error: '请填写手机号~' });
      return;
    }
    setLoading(true);
    setResult(null);
    const d = await sendSms(phone);
    setResult(d);
    if (d.ok) setShowVerify(true);
    setLoading(false);
  };

  const handleVerify = async () => {
    if (!phone || !code) {
      setResult({ ok: false, error: '请填写手机号和验证码~' });
      return;
    }
    setLoading(true);
    const d = await verifyPhone(phone, code);
    setResult(d);
    setLoading(false);
    if (d.ok && onSuccess) onSuccess();
  };

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-icon">📱</span>
        <h2>手机号登录</h2>
      </div>
      <p className="card-desc">发送验证码，验证后自动登录</p>
      <input
        type="tel"
        className="input"
        placeholder="手机号"
        value={phone}
        onChange={(e) => setPhone(e.target.value)}
      />
      <button
        className="btn btn-primary"
        onClick={handleSendSms}
        disabled={loading}
      >
        {loading ? '发送中...' : '发送验证码'}
      </button>
      {showVerify && (
        <>
          <input
            type="text"
            className="input"
            placeholder="短信验证码"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            style={{ marginTop: 8 }}
          />
          <button
            className="btn btn-success"
            onClick={handleVerify}
            disabled={loading}
          >
            {loading ? '验证中...' : '验证并登录'}
          </button>
        </>
      )}
      {result && <ResultBox data={result} />}
    </div>
  );
}
