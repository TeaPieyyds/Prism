import { useState } from 'react';
import { useAuth } from '../AuthContext';
import ModalPortal from './ModalPortal';

interface Props {
  onGoRegister: () => void;
}

export default function LoginPage({ onGoRegister }: Props) {
  const { login } = useAuth();
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [verifiedMsg, setVerifiedMsg] = useState('');
  const [showActivation, setShowActivation] = useState(false);
  const [activationCode, setActivationCode] = useState('');
  const [activationMsg, setActivationMsg] = useState('');
  const [activationOk, setActivationOk] = useState(true);

  // Check if redirected from verification
  useState(() => {
    if (window.location.hash.includes('verified=1')) {
      setVerifiedMsg('恭喜!邮箱验证成功,请登录~');
    }
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    if (!email || !password) { setError('请填写邮箱和密码~'); return; }
    setLoading(true);
    const res = await login(email, password);
    setLoading(false);
    if (!res.ok) {
      if ((res as any).require_activation) {
        setShowActivation(true);
      } else {
        setError(res.error || '糟糕,登录失败了,请核对账号密码后再试~');
      }
    }
  };

  const handleActivate = async () => {
    if (!activationCode.trim()) { setActivationMsg('请填写激活码~'); setActivationOk(false); return; }
    setActivationMsg('');
    const res = await fetch('/api/auth/activate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email.trim().toLowerCase(), code: activationCode.trim() }),
    }).then(r => r.json());
    if (res.ok) {
      setActivationOk(true);
      setActivationMsg('恭喜!激活成功,请重新登录~');
      setTimeout(() => { setShowActivation(false); setError(''); }, 1500);
    } else {
      setActivationOk(false);
      setActivationMsg(res.error || '糟糕,激活失败了,请稍后重试~');
    }
  };

  return (
    <div className="auth-page">
      <div className="auth-card">
        <div className="auth-header">
          <span className="auth-logo">🦉</span>
          <h1>Prism</h1>
          <p>账号管理后台</p>
        </div>

        {verifiedMsg && <div className="result-box ok">{verifiedMsg}</div>}

        <form onSubmit={handleSubmit}>
          <input
            type="email"
            className="input"
            placeholder="邮箱"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoFocus
          />
          <input
            type="password"
            className="input"
            placeholder="密码"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          {error && <div className="result-box err">{error}</div>}
          <button type="submit" className="btn btn-primary" disabled={loading}>
            {loading ? '登录中...' : '登录'}
          </button>
          <div style={{ textAlign: 'center', marginTop: 8 }}>
            <button className="btn-link" onClick={() => window.location.hash = '#/forgot-password'}>忘记密码</button>
          </div>
        </form>

        <div className="auth-footer">
          还没有账号？
          <button className="btn-link" onClick={onGoRegister}>注册</button>
        </div>
      </div>

      {showActivation && (
        <ModalPortal>
        <div className="modal-backdrop" style={{zIndex: 2000}}>
          <div className="modal-content" style={{maxWidth:400,padding:'24px'}}>
            <h3 style={{margin:'0 0 12px',fontSize:16}}>账号激活</h3>
            <p style={{fontSize:13,color:'#5d4b36',margin:'0 0 12px',lineHeight:1.6}}>
              您已超过试用次数，请加入
              <a href="https://example.com/support-group" target="_blank" rel="noopener" style={{color:'var(--phx-primary)',fontWeight:600}}>QQ群</a>
              查看群公告获取激活码。
            </p>
            <input className="input" value={activationCode} onChange={e => setActivationCode(e.target.value)} placeholder="输入激活码" autoFocus style={{width:'100%',textAlign:'center',marginBottom:8}} />
            {activationMsg && <div className={"result-box " + (activationOk ? "ok" : "err")} style={{marginBottom:8}}>{activationMsg}</div>}
            <button className="btn btn-primary" style={{width:'100%'}} onClick={handleActivate}>激活</button>
          </div>
        </div>
        </ModalPortal>
      )}
    </div>
  );
}