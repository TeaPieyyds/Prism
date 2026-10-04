import { useState, useRef } from 'react';
import { forgotPassword, resetPassword } from '../api';
import Captcha from './Captcha';
import type { CaptchaHandle } from './Captcha';

interface Props {
  onGoLogin: () => void;
}

export function ForgotPasswordPage({ onGoLogin }: Props) {
  const params = new URLSearchParams(window.location.hash.split('?')[1] || '');
  const prefillEmail = params.get('email') || '';
  const prefillCode = params.get('code') || '';

  const [step, setStep] = useState(prefillCode ? 2 : 1);
  const [email, setEmail] = useState(prefillEmail);
  const [code, setCode] = useState(prefillCode);
  const [password, setPassword] = useState('');
  const [confirmPwd, setConfirmPwd] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const captchaRef = useRef<CaptchaHandle>(null);
  const captchaKey = useRef(0);

  const handleSendCode = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email) { setError('请填写邮箱~'); return; }

    const c = captchaRef.current?.getChallenge() || '';
    const a = captchaRef.current?.getAnswer() || '';
    if (!c || !a) { setError('请先完成验证码再提交~'); return; }

    setLoading(true);
    setError('');
    setSuccess('');
    const res = await forgotPassword(email, c, a);
    setLoading(false);
    if (res.ok) {
      setSuccess('如果该邮箱已注册，重置密码邮件已发送');
      setStep(2);
    } else {
      setError(res.error || '哎呀,发送失败了,请稍后重试~');
      captchaRef.current?.reset();
      captchaKey.current++;
    }
  };

  const handleReset = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!code) { setError('请填写验证码~'); return; }
    if (password.length < 6) { setError('密码至少需要 6 位~'); return; }
    if (password !== confirmPwd) { setError('两次输入的密码不一致,请检查~'); return; }
    setLoading(true);
    setError('');
    const res = await resetPassword(email, code, password);
    setLoading(false);
    if (res.ok) {
      setSuccess('密码已重置，即将跳转到登录页');
      setTimeout(() => onGoLogin(), 2000);
    } else {
      setError(res.error || '糟糕,重置失败了,请稍后重试~');
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <span className="auth-logo">🦉</span>
          <h1>忘记密码</h1>
          <p>{step === 1 ? '输入邮箱接收重置链接' : '输入验证码和新密码'}</p>
        </div>

        {step === 1 ? (
          <form onSubmit={handleSendCode}>
            <input className="input" type="email" placeholder="邮箱地址" value={email}
              onChange={e => setEmail(e.target.value)} autoFocus />
            <Captcha key={captchaKey.current} ref={captchaRef} />
            {error && <div className="result-box err">{error}</div>}
            {success && <div className="result-box ok">{success}</div>}
            <button type="submit" className="btn btn-primary" disabled={loading}>
              {loading ? '发送中...' : '发送重置邮件'}
            </button>
          </form>
        ) : (
          <form onSubmit={handleReset}>
            <input className="input" type="text" placeholder="验证码" value={code}
              onChange={e => setCode(e.target.value)} autoFocus />
            <input className="input" type="password" placeholder="新密码（至少6位）" value={password}
              onChange={e => setPassword(e.target.value)} style={{ marginTop: 10 }} />
            <input className="input" type="password" placeholder="确认新密码" value={confirmPwd}
              onChange={e => setConfirmPwd(e.target.value)} style={{ marginTop: 10 }} />
            {error && <div className="result-box err">{error}</div>}
            {success && <div className="result-box ok">{success}</div>}
            <button type="submit" className="btn btn-primary" disabled={loading}>
              {loading ? '重置中...' : '重置密码'}
            </button>
          </form>
        )}

        <div className="auth-footer">
          <button className="btn-link" onClick={onGoLogin}>返回登录</button>
        </div>
      </div>
    </div>
  );
}
