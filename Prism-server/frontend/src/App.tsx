import { useState, useEffect } from 'react';
import { AuthProvider, useAuth } from './AuthContext';
import LoginPage from './components/LoginPage';
import RegisterPage from './components/RegisterPage';
import Dashboard from './components/Dashboard';
import AdminPage from './components/AdminPage';
import UserSettingsPage from './components/UserSettingsPage';
import LogsPage from './components/LogsPage';
import { ForgotPasswordPage } from './components/ForgotPasswordPage';
import ParticleField from './components/ParticleField';
import ModalPortal from './components/ModalPortal';
import MusicPlayer from './components/MusicPlayer';

const navIcons: Record<string, React.ReactNode> = {
  home:  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>,
  user:  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>,
  log:   <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>,
  admin: <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06A1.65 1.65 0 004.68 15a1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06A1.65 1.65 0 009 4.68a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z"/></svg>,
};

function Shell({ route, children }: { route: string; children: React.ReactNode }) {
  const { user } = useAuth();
  const nav = [
    { path: '/', icon: navIcons.home,  label: '游戏账号' },
    { path: '/settings', icon: navIcons.user, label: '网页用户' },
    { path: '/logs', icon: navIcons.log,    label: '操作日志' },
  ];
  if (user?.role === 'admin') nav.push({ path: '/admin', icon: navIcons.admin, label: '后台管理' });

  return (
    <div className="app-layout">
      <main className="main-content-full">{children}</main>
      <MusicPlayer />
      <nav className="bottom-nav">
        {nav.map((item) => (
          <button key={item.path}
            className={`bottom-nav-item${route === item.path ? ' active' : ''}`}
            onClick={() => window.location.hash = `#${item.path}`}>
            <span className="bottom-nav-icon">{item.icon}</span>
            <span className="bottom-nav-label">{item.label}</span>
          </button>
        ))}
      </nav>
    </div>
  );
}

function ActivationModal({ onActivated }: { onActivated: () => void }) {
  const { user } = useAuth();
  const [code, setCode] = useState('');
  const [msg, setMsg] = useState('');
  const [ok, setOk] = useState(true);

  const handleActivate = async () => {
    if (!code.trim()) { setMsg('请填写激活码~'); setOk(false); return; }
    setMsg('');
    const res = await fetch('/api/auth/activate', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: user?.email || '', code: code.trim() }),
    }).then(r => r.json());
    if (res.ok) { setOk(true); setMsg('恭喜!激活成功~'); setTimeout(onActivated, 1000); }
    else { setOk(false); setMsg(res.error || '糟糕,激活失败了,请稍后重试~'); }
  };

  return (
    <ModalPortal>
    <div className="modal-backdrop" style={{zIndex: 2000}}>
      <div className="modal-content" style={{maxWidth:400,padding:'24px'}}>
        <h3 style={{margin:'0 0 12px',fontSize:16}}>账号激活</h3>
        <p style={{fontSize:13,color:'#5d4b36',margin:'0 0 12px',lineHeight:1.6}}>
          试用次数已用完，请加入
          <a href="https://example.com/support-group" target="_blank" rel="noopener" style={{color:'var(--phx-primary)',fontWeight:600}}>QQ群</a>
          查看群公告获取激活码。
        </p>
        <input className="input" value={code} onChange={e => setCode(e.target.value)} placeholder="输入激活码" autoFocus style={{width:'100%',textAlign:'center',marginBottom:8}} />
        {msg && <div className={"result-box " + (ok ? "ok" : "err")} style={{marginBottom:8}}>{msg}</div>}
        <button className="btn btn-primary" style={{width:'100%'}} onClick={handleActivate}>激活</button>
      </div>
    </div>
    </ModalPortal>
  );
}

function Router() {
  const { user, loading, checkAuth } = useAuth();
  const [hash, setHash] = useState(() => window.location.hash.slice(1) || '/');

  useEffect(() => {
    const onHashChange = () => setHash(window.location.hash.slice(1) || '/');
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  if (loading) return <div className="boot-screen"><div style={{color:'var(--phx-text-secondary)'}}>正在准备...</div></div>;

  const route = hash.split('?')[0];
  const params = new URLSearchParams(hash.includes('?') ? hash.split('?')[1] : '');
  if (!user) {
    if (route === '/register') return <LoginShell><RegisterPage inviteCode={params.get('invite') || ''} onGoLogin={() => window.location.hash = '#/login'} /></LoginShell>;
    if (route === '/forgot-password') return <LoginShell><ForgotPasswordPage onGoLogin={() => window.location.hash = '#/login'} /></LoginShell>;
    return <LoginShell><LoginPage onGoRegister={() => window.location.hash = '#/register'} /></LoginShell>;
  }

  let page = <Dashboard />;
  if (route === '/settings') page = <UserSettingsPage />;
  if (route === '/logs') page = <LogsPage />;
  if (route === '/admin' && user.role === 'admin') page = <AdminPage />;

  return <Shell route={route}>
    {(user as any)?.require_activation && <ActivationModal onActivated={() => checkAuth()} />}
    <div key={route} className="page-enter">{page}</div>
  </Shell>;
}

function LoginShell({ children }: { children: React.ReactNode }) {
  return <div className="app-layout"><main className="main-content-full">{children}</main></div>;
}

export default function App() {
  return <AuthProvider>
    <ParticleField />
    <Router />
  </AuthProvider>;
}
