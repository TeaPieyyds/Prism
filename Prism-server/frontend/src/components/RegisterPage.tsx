import { useState, useRef } from 'react';
import { register } from '../api';
import Captcha from './Captcha';
import type { CaptchaHandle } from './Captcha';

interface Props {
  onGoLogin: () => void;
  inviteCode?: string;
}

export default function RegisterPage({ onGoLogin, inviteCode: initialInvite = '' }: Props) {
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPwd, setConfirmPwd] = useState('');
  const [inviteCode, setInviteCode] = useState(initialInvite);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [loading, setLoading] = useState(false);
  const captchaRef = useRef<CaptchaHandle>(null);
  const captchaKey = useRef(0);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess('');

    if (!email || !username || !password) {
      setError('请把信息都填完整~');
      return;
    }
    if (password !== confirmPwd) {
      setError('两次输入的密码不一致,请检查~');
      return;
    }
    if (password.length < 6) {
      setError('密码至少需要 6 位~');
      return;
    }

    const c = captchaRef.current?.getChallenge() || '';
    const a = captchaRef.current?.getAnswer() || '';
    if (!c || !a) {
      setError('请先完成验证码再提交~');
      return;
    }

    setLoading(true);
    const res = await register(email, username, password, inviteCode, c, a);
    setLoading(false);
    if (res.ok) {
      setSuccess((res.message as string) || '恭喜!注册成功,请查收验证邮件~');
    } else {
      setError(res.error || '糟糕,注册失败了,请稍后重试~');
      captchaRef.current?.reset();
      captchaKey.current++;
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <span className="auth-logo">🦉</span>
          <h1>注册账号</h1>
          <p>创建你的 Prism 账号</p>
        </div>

        <form onSubmit={handleSubmit}>
          <input type="email" className="input" placeholder="邮箱" value={email}
            onChange={(e) => setEmail(e.target.value)} autoFocus />
          <input type="text" className="input" placeholder="用户名" value={username}
            onChange={(e) => setUsername(e.target.value)} />
          <input type="password" className="input" placeholder="密码（至少6位）" value={password}
            onChange={(e) => setPassword(e.target.value)} />
          <input type="password" className="input" placeholder="确认密码" value={confirmPwd}
            onChange={(e) => setConfirmPwd(e.target.value)} />
          <input type="text" className="input" placeholder="邀请码（选填）" value={inviteCode}
            onChange={(e) => setInviteCode(e.target.value)} maxLength={6} />
          <Captcha key={captchaKey.current} ref={captchaRef} />
          {error && <div className="result-box err">{error}</div>}
          {success && <div className="result-box ok">{success}</div>}
          <button type="submit" className="btn btn-success" disabled={loading}>
            {loading ? '注册中...' : '注册'}
          </button>
        </form>

        <div className="auth-footer">
          已有账号？
          <button className="btn-link" onClick={onGoLogin}>登录</button>
        </div>
      </div>
    </div>
  );
}
