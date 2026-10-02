import { useState, useEffect, useRef } from 'react';
import { useAuth } from '../AuthContext';
import { updateProfile, updatePassword, challengeOverride, changeEmail, confirmEmail, growthOverride, getNuts, myInviteCode, getMyToolboxInfo, updateMyToolbox, startToolboxTrial, purchaseToolboxFeature, extendToolboxAccess } from '../api';
import Captcha from './Captcha';
import type { CaptchaHandle } from './Captcha';
// 支付功能暂未开放: createPaymentOrder, getPaymentOrderStatus
import { getTheme, cycleTheme, themeLabel, getStyle, cycleStyle, styleLabel } from '../theme';
import Modal from './Modal';
import NutsPolicy from './NutsPolicy';
import ApiDoc from './ApiDoc';
import TokenManager from './TokenManager';
import { successSound } from '../sounds';
import { showToast } from './Toast';

// 板栗(积分)交易原因中英对照表：用户可见的英文 reason 全部汉化，不显示任何英文。
const nutReasonText: Record<string, string> = {
  register: '注册奖励',
  add_account: '添加账号',
  share_account: '设为共享',
  unshare_account: '收回共享',
  delete_account: '删除账号',
  join_server: '进入服务器',
  shared_used: '共享账号被使用',
  guest_used: '游客账号被使用',
  redeem: '兑换激活码',
  survey_reward: '问卷奖励',
  to_code: '积分转兑换码',
  to_question_code: '积分转答题兑换码',
  create_redpacket: '发红包',
  redpacket_claim: '抢到红包',
  'toolbox:consumption': '工具箱消耗',
  admin_adjust: '管理员调整板栗',
};

function nutReasonLabel(t: any): string {
  const r = t.reason;
  if (r === 'join_server') return t.amount === -2 ? '进入服务器（共享账号）' : '进入服务器';
  if (typeof r === 'string' && r.startsWith('购买工具箱功能')) {
    if (r.includes('prompt')) return '购买工具箱功能（自定义提示词）';
    if (r.includes('name')) return '购买工具箱功能（自定义名字）';
    return '购买工具箱功能';
  }
  if (typeof r === 'string' && r.startsWith('续期工具箱')) return r; // 已是中文
  return nutReasonText[r] || r || '';
}

export default function UserSettingsPage() {
  const { user, logout, checkAuth } = useAuth();
  const [editingName, setEditingName] = useState(false);
  const [editingPassword, setEditingPassword] = useState(false);
  const [username, setUsername] = useState(user?.username || '');
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [challengeOn, setChallengeOn] = useState(user?.challenge_override ?? false);
  const [challengePending, setChallengePending] = useState(false);
  const [showChallengeHelp, setShowChallengeHelp] = useState(false);
  const [growthOn, setGrowthOn] = useState(user?.growth_override ?? false);
  const [growthValue, setGrowthValue] = useState(user?.growth_override_value ?? 0);
  const [growthPending, setGrowthPending] = useState(false);
  const [showGrowthModal, setShowGrowthModal] = useState(false);
  const [growthModalValue, setGrowthModalValue] = useState('');
  const [nutsBalance, setNutsBalance] = useState(user?.nuts_balance ?? 0);
  const [nutsTxns, setNutsTxns] = useState<any[]>([]);
  const subUntil = user?.subscription_until;
  const subStart = user?.subscription_start;
  const subActive = !!subUntil && new Date(subUntil).getTime() > Date.now();
  const [invCode, setInvCode] = useState('');
  const [invCount, setInvCount] = useState(0);
  const [invTotalNuts, setInvTotalNuts] = useState(0);
  const [redeemCode, setRedeemCode] = useState('');
  const [redeemMsg, setRedeemMsg] = useState('');
  const [redeemOk, setRedeemOk] = useState(true);
  const [showRedeem, setShowRedeem] = useState(false);
  const [redeemQuestion, setRedeemQuestion] = useState('');
  const [redeemAnswer, setRedeemAnswer] = useState('');
  const captchaRef = useRef<CaptchaHandle>(null);
  const [captchaKey, setCaptchaKey] = useState(0);
  /* 支付功能暂未开放
  const [showPay, setShowPay] = useState(false);
  const [payStep, setPayStep] = useState<'input'|'qr'|'done'>('input');
  const [payAmount, setPayAmount] = useState('');
  const [payType, setPayType] = useState('wxpay');
  const [payQRCode, setPayQRCode] = useState('');
  const [payURL, setPayURL] = useState('');
  const [payOrderNo, setPayOrderNo] = useState('');
  const [payRate] = useState(10);
  const [payMinAmount] = useState(1);
  const [payCodes, setPayCodes] = useState<string[]>([]);
  const [payMsg, setPayMsg] = useState('');
  const [payMsgOk, setPayMsgOk] = useState(true);
  const [payPolling, setPayPolling] = useState<ReturnType<typeof setInterval> | null>(null);
  */
  const [showPolicy, setShowPolicy] = useState(false);
  const [showApiDoc, setShowApiDoc] = useState(false);
  const [theme, setTheme] = useState(getTheme);
  const [style, setStyleState] = useState(getStyle);
  const [editingEmail, setEditingEmail] = useState(false);
  const [emailStep2, setEmailStep2] = useState(false);
  const [newEmail, setNewEmail] = useState('');
  const [emailCode, setEmailCode] = useState('');
  const [emailError, setEmailError] = useState('');
  const [emailSuccess, setEmailSuccess] = useState('');

  useEffect(() => { setUsername(user?.username || ''); }, [user?.username]);
  useEffect(() => { setChallengeOn(user?.challenge_override ?? false); }, [user?.challenge_override]);
  useEffect(() => { setGrowthOn(user?.growth_override ?? false); setGrowthValue(user?.growth_override_value ?? 0); }, [user?.growth_override, user?.growth_override_value]);

  // 板栗余额/记录实时刷新：挂载拉取 + 每 20s 轮询 + 窗口聚焦/回到前台时刷新
  const loadNuts = async () => {
    const r: any = await getNuts();
    if (r.ok) { setNutsBalance(r.balance); setNutsTxns(r.transactions || []); }
  };
  useEffect(() => {
    loadNuts();
    const iv = setInterval(loadNuts, 20000);
    const onFocus = () => loadNuts();
    window.addEventListener('focus', onFocus);
    const onVis = () => { if (!document.hidden) loadNuts(); };
    document.addEventListener('visibilitychange', onVis);
    return () => { clearInterval(iv); window.removeEventListener('focus', onFocus); document.removeEventListener('visibilitychange', onVis); };
  }, []);
  useEffect(() => {
    myInviteCode().then((r: any) => {
      if (r.ok) { setInvCode(r.code); setInvCount(r.count); setInvTotalNuts(r.total_nuts); }
    });
  }, []);

  const handleRedeem = async () => {
    if (!redeemCode.trim()) { setRedeemMsg('请填写激活码~'); setRedeemOk(false); return; }
    const c = captchaRef.current?.getChallenge() || '';
    const a = captchaRef.current?.getAnswer() || '';
    if (!c || !a) { setRedeemMsg('请先完成验证码~'); setRedeemOk(false); return; }
    setRedeemMsg('');
    const body: any = { code: redeemCode.trim(), c, a };
    if (redeemQuestion) body.answer = redeemAnswer.trim();
    const res = await fetch('/api/auth/redeem', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + localStorage.getItem('session_token') },
      body: JSON.stringify(body),
    }).then(r => r.json());
    if (res.need_answer) {
      // 答题码：第一步返回问题，等待作答后再提交
      setRedeemQuestion(res.question || '');
      setRedeemAnswer('');
      setRedeemMsg('请先把上面的问题答完~');
      setRedeemOk(true);
      captchaRef.current?.reset();
      setCaptchaKey(k => k + 1);
      return;
    }
    if (res.ok) { successSound(); setRedeemMsg(res.message); setRedeemOk(true); setShowRedeem(false); setRedeemCode(''); setRedeemQuestion(''); setRedeemAnswer(''); captchaRef.current?.reset(); getNuts().then((r: any) => { if (r.ok) { setNutsBalance(r.balance); setNutsTxns(r.transactions || []); } }); }
    else { setRedeemMsg(res.error || '糟糕,兑换没成功,请核对后重试~'); setRedeemOk(false); captchaRef.current?.reset(); setCaptchaKey(k => k + 1); }
  };

  /* 支付功能暂未开放
  const handleCreatePayOrder = async () => {
    const amount = parseFloat(payAmount);
    if (!amount || amount < payMinAmount) { setPayMsg(`最低支付 ${payMinAmount} 元`); setPayMsgOk(false); return; }
    setPayMsg('');
    const res: any = await createPaymentOrder(amount, payType);
    if (!res.ok) { setPayMsg(res.error || '糟糕,创建订单失败了,请稍后重试~'); setPayMsgOk(false); return; }
    setPayOrderNo(res.order?.order_no || '');
    if (res.qrcode) { setPayQRCode(res.qrcode); setPayStep('qr'); }
    else if (res.payurl) { setPayURL(res.payurl); window.open(res.payurl, '_blank'); setPayStep('qr'); }
    else if (res.submit_url) { setPayURL(res.submit_url); window.open(res.submit_url, '_blank'); setPayStep('qr'); }
    const interval = setInterval(async () => {
      const s: any = await getPaymentOrderStatus(res.order?.order_no);
      if (s.ok && s.order?.status === 'paid') {
        clearInterval(interval);
        setPayPolling(null);
        setPayCodes(s.order.codes ? JSON.parse(s.order.codes) : []);
        setPayStep('done');
        successSound();
        getNuts().then((r: any) => { if (r.ok) { setNutsBalance(r.balance); setNutsTxns(r.transactions || []); } });
      }
    }, 3000);
    setPayPolling(interval);
  };

  const closePayModal = () => {
    if (payPolling) { clearInterval(payPolling); setPayPolling(null); }
    setShowPay(false);
    setPayQRCode('');
    setPayURL('');
    setPayOrderNo('');
    setPayCodes([]);
  };
  */

  const saveName = async () => {
    const res = await updateProfile(username.trim());
    if ((res as any).ok) {
      setEditingName(false);
      await checkAuth();
    } else {
      alert((res as any).error || '哎呀,更新失败了,请稍后重试~');
    }
  };

  const savePassword = async () => {
    const res = await updatePassword(oldPassword, newPassword);
    if ((res as any).ok) {
      setEditingPassword(false);
      setOldPassword('');
      setNewPassword('');
      // 改密后主令牌轮换，保存新令牌；其他设备会话已全部下线
      if ((res as any).token) {
        localStorage.setItem('session_token', (res as any).token);
        await checkAuth();
      }
      alert((res as any).message || '密码已更新，其他设备已全部下线');
    } else {
      alert((res as any).error || '哎呀,更新失败了,请稍后重试~');
    }
  };

  const handleToggleChallenge = async () => {
    const next = !challengeOn;
    setChallengePending(true);
    const res = await challengeOverride(next);
    setChallengePending(false);
    if ((res as any).ok) {
      setChallengeOn(next);
      await checkAuth();
    } else {
      alert((res as any).error || '糟糕,操作没成功,请稍后重试~');
    }
  };

  const handleToggleGrowth = async () => {
    if (!growthOn) {
      setGrowthModalValue(String(growthValue || 1));
      setShowGrowthModal(true);
      return;
    }
    setGrowthPending(true);
    const res = await growthOverride(false, 0);
    setGrowthPending(false);
    if ((res as any).ok) { setGrowthOn(false); await checkAuth(); }
    else { alert((res as any).error || '糟糕,操作没成功,请稍后重试~'); }
  };
  const confirmGrowthOverride = async () => {
    const val = parseInt(growthModalValue) || 1;
    setShowGrowthModal(false);
    setGrowthPending(true);
    const res = await growthOverride(true, val);
    setGrowthPending(false);
    if ((res as any).ok) { setGrowthOn(true); setGrowthValue(val); await checkAuth(); }
    else { alert((res as any).error || '糟糕,操作没成功,请稍后重试~'); }
  };



  const emailCaptchaRef = useRef<CaptchaHandle>(null);
  const [emailCaptchaKey, setEmailCaptchaKey] = useState(0);

  const sendChangeEmail = async () => {
    if (!newEmail) { setEmailError('请填写新邮箱~'); return; }
    const c = emailCaptchaRef.current?.getChallenge() || '';
    const a = emailCaptchaRef.current?.getAnswer() || '';
    if (!c || !a) { setEmailError('请先完成验证码再提交~'); return; }
    setEmailError(''); setEmailSuccess('');
    const res = await changeEmail(newEmail, c, a);
    if ((res as any).ok) {
      setEmailStep2(true);
      setEmailSuccess('验证码已发送');
    } else {
      setEmailError((res as any).error || '哎呀,发送失败了,请稍后重试~');
      emailCaptchaRef.current?.reset();
      setEmailCaptchaKey(k => k + 1);
    }
  };

  const confirmChangeEmail = async () => {
    if (!emailCode) { setEmailError('请填写验证码~'); return; }
    setEmailError('');
    const res = await confirmEmail(newEmail, emailCode);
    if ((res as any).ok) {
      setEditingEmail(false);
      setEmailStep2(false);
      setEmailSuccess('');
      await checkAuth();
    } else {
      setEmailError((res as any).error || '糟糕,验证失败了,请检查后重试~');
    }
  };

  return (
    <div className="page-stack narrow-page">
      <section className="hero-panel compact">
        <div>
          <div className="eyebrow">用户中心</div>
          <h1>账号设置</h1>
          <p>管理你的账户信息、密码和安全设置。</p>
        </div>
        <div className="settings-actions" style={{justifyContent:'flex-end', rowGap: 8}}>
          <button className="btn btn-sm btn-primary" onClick={() => setShowRedeem(true)}>兑换</button>
          <button className="btn btn-sm btn-outline" onClick={() => setShowApiDoc(true)}>API 文档</button>
          <button className="btn btn-sm btn-danger" onClick={logout}>退出登录</button>
          <button className="btn btn-sm btn-outline" style={{minWidth:0,padding:'4px 9px',fontSize:14}} title="兑换码获取方式" aria-label="激活码获取方式" onClick={() => window.open('https://example.com/support-group', '_blank')}>?</button>
        </div>
      </section>

      {subActive ? (
        <section className="card" style={{marginBottom:16,background:'var(--phx-warm-bg)'}}>
          <div style={{fontSize:13,color:'var(--phx-text-secondary)'}}>订阅中</div>
          <div style={{fontSize:34,fontWeight:800,color:'var(--phx-accent, #4c9aff)'}}>
            剩 {Math.max(0, Math.ceil((new Date(subUntil).getTime() - Date.now()) / 86400000))} 天
          </div>
          {(() => {
            const total = new Date(subUntil).getTime() - (subStart ? new Date(subStart).getTime() : Date.now());
            const remain = new Date(subUntil).getTime() - Date.now();
            const pct = total > 0 ? Math.max(0, Math.min(100, remain / total * 100)) : 100;
            return (
              <>
                <div style={{height:10,background:'var(--phx-warm-border)',borderRadius:5,margin:'8px 0',overflow:'hidden'}}>
                  <div style={{height:'100%',width:pct+'%',background:'var(--phx-accent, #4c9aff)'}} />
                </div>
                <div style={{display:'flex',justifyContent:'space-between',fontSize:12,color:'var(--phx-text-secondary)'}}>
                  <span>{subStart ? new Date(subStart).toLocaleDateString('zh-CN') : ''}</span>
                  <span>{new Date(subUntil).toLocaleDateString('zh-CN')}</span>
                </div>
              </>
            );
          })()}
        </section>
      ) : (
        <section className="card" style={{marginBottom:16,background:'var(--phx-warm-bg)'}}>
          <div style={{display:'flex',alignItems:'center',justifyContent:'space-between'}}>
            <div>
              <div style={{fontSize:13,color:'var(--phx-text-secondary)'}}>板栗余额</div>
            <div style={{display:'flex',alignItems:'center',gap:10}}>
              <span className="num" style={{fontSize:34,fontWeight:800,color:'var(--phx-text)'}}>{nutsBalance}</span>
              <span style={{fontSize:22,lineHeight:1}} aria-hidden="true">🌰</span>
              <button className="btn btn-sm btn-outline" style={{fontSize:11,padding:'2px 8px',minWidth:0}} onClick={() => setShowPolicy(true)} title="板栗规则">规则</button>
            </div>
          </div>
        </div>
        {nutsTxns.length > 0 && (
          <div style={{marginTop:12,borderTop:'1px dashed var(--phx-warm-border)',paddingTop:8}}>
            <div style={{fontSize:12,color:'var(--phx-text-secondary)',marginBottom:4}}>最近记录</div>
            {(() => {
              // 合并连续的零值 join_server 记录
              const merged: any[] = [];
              let zeroCount = 0;
              for (let i = 0; i < nutsTxns.length; i++) {
                const t = nutsTxns[i];
                if (t.reason === 'join_server' && t.amount === 0) {
                  zeroCount++;
                } else {
                  if (zeroCount > 0) {
                    merged.push({ _merged: true, _count: zeroCount, _reason: 'join_server', _time: nutsTxns[i-1]?.created_at });
                    zeroCount = 0;
                  }
                  merged.push(t);
                }
              }
              if (zeroCount > 0) {
                merged.push({ _merged: true, _count: zeroCount, _reason: 'join_server', _time: nutsTxns[nutsTxns.length-1]?.created_at });
              }
              return merged.slice(0,5).map((t: any, idx: number) => {
                if (t._merged) return (
                  <div key={'m'+idx} style={{display:'flex',justifyContent:'space-between',fontSize:12,padding:'2px 0'}}>
                    <div style={{flex:1,minWidth:0}}>
                      <span style={{color:'var(--phx-text-secondary)',fontSize:10,marginRight:4}}>{t._time ? new Date(t._time).getDate()+' '+new Date(t._time).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'}) : ''}</span>
                      <span style={{color:'var(--phx-text)'}}>进入服务器{t._count > 1 ? <span style={{color:'var(--phx-text-secondary)'}}> ×{t._count}</span> : ''}</span>
                    </div>
                    <span style={{color:'var(--phx-text-secondary)',fontWeight:500}}>0</span>
                  </div>
                );
                return (
                  <div key={t.id} style={{display:'flex',justifyContent:'space-between',fontSize:12,padding:'2px 0'}}>
                    <div style={{flex:1,minWidth:0}}>
                      <span style={{color:'var(--phx-text-secondary)',fontSize:10,marginRight:4}}>{t.created_at ? new Date(t.created_at).getDate()+' '+new Date(t.created_at).toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit'}) : ''}</span>
                      <span style={{color:'var(--phx-text)'}}>{nutReasonLabel(t)}</span>
                    </div>
                    <span style={{color:t.amount>0?'#4f9d53':'#d9534f',fontWeight:600}}>{t.amount>0?'+':''}{t.amount}</span>
                  </div>
                );
              });
            })()}
          </div>
        )}
        </section>
      )}

      {/* Toolbox Status */}
      <ToolboxUserSection />

      {/* Multi-token management */}
      <TokenManager />

      <section className="card settings-card">
        <div className="setting-row">
          <div><span>用户名</span><strong>{user?.username || '-'}</strong></div>
          {!editingName ? <button className="btn btn-sm btn-outline" onClick={() => setEditingName(true)}>编辑</button> : null}
        </div>
        {editingName && (
          <div className="edit-drawer">
            <input className="input" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="新用户名" />
            <div className="inline-actions"><button className="btn btn-primary" onClick={saveName}>保存</button><button className="btn btn-outline" onClick={() => setEditingName(false)}>取消</button></div>
          </div>
        )}

        <div className="setting-row"><div><span>邮箱</span><strong>{user?.email || '-'}</strong></div>{!editingEmail ? <button className="btn btn-sm btn-outline" onClick={() => setEditingEmail(true)}>修改</button> : null}</div>
        {editingEmail && (
          <div className="edit-drawer">
            {!emailStep2 ? (
              <>
                <input className="input" value={newEmail} onChange={(e) => setNewEmail(e.target.value)} placeholder="新邮箱地址" />
                <Captcha key={emailCaptchaKey} ref={emailCaptchaRef} />
                {emailError && <div className="result-box err">{emailError}</div>}
                {emailSuccess && <div className="result-box ok">{emailSuccess}</div>}
                <div className="inline-actions"><button className="btn btn-primary" onClick={sendChangeEmail}>发送验证码</button><button className="btn btn-outline" onClick={() => setEditingEmail(false)}>取消</button></div>
              </>
            ) : (
              <>
                <p style={{fontSize:14,color:'#6d5a41',margin:'0 0 10px'}}>验证码已发送到 <strong>{newEmail}</strong></p>
                <input className="input" value={emailCode} onChange={(e) => setEmailCode(e.target.value)} placeholder="输入验证码" />
                {emailError && <div className="result-box err">{emailError}</div>}
                {emailSuccess && <div className="result-box ok">{emailSuccess}</div>}
                <div className="inline-actions"><button className="btn btn-primary" onClick={confirmChangeEmail}>确认修改</button><button className="btn btn-outline" onClick={() => { setEditingEmail(false); setEmailStep2(false); }}>取消</button></div>
              </>
            )}
          </div>
        )}
        <div className="setting-row"><div><span>角色</span><strong>{user?.role === 'admin' ? '管理员' : '用户'}</strong></div></div>

        <div className="setting-row">
          <div>
            <span>邀请码</span>
            <strong style={{letterSpacing:3,fontSize:16,color:'#c8a45c'}}>{invCode || '加载中...'}</strong>
          </div>
          <div style={{display:'flex',gap:6,flexShrink:0}}>
            {invCode && <button className="btn btn-sm btn-outline" onClick={() => { navigator.clipboard.writeText(invCode); alert('邀请码已复制'); }}>复制码</button>}
            {invCode && <button className="btn btn-sm btn-primary" onClick={() => { navigator.clipboard.writeText(window.location.origin + '/invite/' + invCode); alert('邀请链接已复制，分享给朋友即可自动填码'); }}>复制链接</button>}
          </div>
        </div>
        <div className="setting-row" style={{borderBottom:0,paddingTop:0}}>
          <div><span>已邀请 {invCount} 人 · 累计获得 {invTotalNuts} 板栗</span></div>
        </div>

        <div className="setting-row">
          <div><span>主题</span><strong>{themeLabel(theme)}</strong></div>
          <button className="btn btn-sm btn-outline" onClick={() => { const t = cycleTheme(); setTheme(t); }}>切换</button>
        </div>
        <div className="setting-row">
          <div><span>风格</span><strong>{styleLabel(style)}</strong></div>
          <button className="btn btn-sm btn-outline" onClick={() => { const s = cycleStyle(); setStyleState(s); }}>切换</button>
        </div>

        <div className="setting-row">
          <div><span>密码</span><strong>已加密保存</strong></div>
          {!editingPassword ? <button className="btn btn-sm btn-outline" onClick={() => setEditingPassword(true)}>修改</button> : null}
        </div>
        {editingPassword && (
          <div className="edit-drawer">
            <input className="input" type="password" value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} placeholder="原密码" />
            <input className="input" type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} placeholder="新密码" />
            <div className="inline-actions"><button className="btn btn-primary" onClick={savePassword}>保存密码</button><button className="btn btn-outline" onClick={() => setEditingPassword(false)}>取消</button></div>
          </div>
        )}

        <div className="setting-row">
          <div className="setting-label-with-help">
            <span>黑洞模式</span>
            <span className="help-icon" onClick={() => setShowChallengeHelp(!showChallengeHelp)}>?</span>
          </div>
          <label className="toggle-switch">
            <input type="checkbox" checked={challengeOn} onChange={handleToggleChallenge} disabled={challengePending} />
            <span className="toggle-slider"></span>
          </label>
        </div>
        {showChallengeHelp && (
          <div className="help-popup">
            <p>开启黑洞模式后，机器人进入服务器将卡在加载阶段，无法正常执行后续操作。</p>
            <p>此时机器人不会主动请求权限，而是持续停留在服务器中，直到机器人被关闭或被意外踢出。</p>
            <p>适用于需要临时冻结机器人活动但保持在线占位的场景。</p>
            <p className="help-note">此开关仅对当前用户生效，不影响其他用户。</p>
          </div>
        )}

        <div className="setting-row">
          <div className="setting-label-with-help">
            <span>成长等级覆盖</span>
          </div>
          <div style={{display:'flex',alignItems:'center',gap:8}}>
            {growthOn && <span style={{fontWeight:600,color:'#4f9d53',fontSize:15}}>Lv.{growthValue}</span>}
            <label className="toggle-switch">
              <input type="checkbox" checked={growthOn} onChange={handleToggleGrowth} disabled={growthPending} />
              <span className="toggle-slider"></span>
            </label>
          </div>
        </div>
        {growthOn && <p className="card-desc" style={{fontSize:12,margin:0}}>当前显示等级已固定为 Lv.{growthValue}</p>}
      <Modal open={showRedeem} title={redeemQuestion ? "答题兑换" : "兑换板栗"} message={redeemQuestion ? "答对该问题才能兑换" : "输入激活码兑换板栗，或在线购买"}
        confirmLabel={redeemQuestion ? "提交答案" : "兑换"} cancelLabel="取消"
        onConfirm={handleRedeem}
        onCancel={() => { setShowRedeem(false); setRedeemCode(""); setRedeemQuestion(""); setRedeemAnswer(""); setRedeemMsg(""); }}>
        <Captcha key={captchaKey} ref={captchaRef} />
        <input className="input" value={redeemCode} onChange={e => setRedeemCode(e.target.value)} placeholder="输入激活码" style={{width:"100%",textAlign:"center",marginTop:8}} autoFocus />
        {redeemQuestion && (
          <>
            <div style={{ marginTop: 8, textAlign: 'center', fontWeight: 600, color: 'var(--phx-text)', padding: '8px', background: '#f5ecd7', borderRadius: 8 }}>{redeemQuestion}</div>
            <input className="input" value={redeemAnswer} onChange={e => setRedeemAnswer(e.target.value)} placeholder="输入答案" style={{width:"100%",textAlign:"center",marginTop:8}} autoFocus />
          </>
        )}
{/* <button className="btn btn-sm btn-primary" style={{width:'100%',marginTop:8,background:'#e87d2f'}} onClick={() => { setShowRedeem(false); setShowPay(true); }}>在线购买激活码</button> */}
        {redeemMsg && <div className={"result-box " + (redeemOk ? "ok" : "err")} style={{marginTop:8}}>{redeemMsg}</div>}
      </Modal>

      {/* 支付功能暂未开放
      <Modal open={showPay}
        title={payStep === 'done' ? '恭喜!支付成功~' : payStep === 'qr' ? '扫码支付' : '购买板栗'}
        message={payStep === 'done' ? '恭喜!支付成功!以下是你的激活码,可兑换板栗或分享给朋友~' : payStep === 'qr' ? '请使用支付宝/微信扫描二维码支付' : `1元 = ${payRate}板栗，最低 ${payMinAmount} 元`}
        confirmLabel={payStep === 'done' ? '完成' : undefined}
        cancelLabel={payStep === 'done' ? undefined : '取消'}
        onConfirm={payStep === 'done' ? closePayModal : undefined}
        onCancel={payStep === 'done' ? undefined : closePayModal}>
        {payStep === 'input' && (
          <div style={{marginTop:8}}>
            <input className="input" type="number" min={payMinAmount} step="0.01" value={payAmount}
              onChange={e => setPayAmount(e.target.value)}
              placeholder={`最低 ${payMinAmount} 元`}
              style={{width:'100%',textAlign:'center',marginBottom:8}} autoFocus />
            <div style={{textAlign:'center',fontSize:14,color:'#6d5a41',marginBottom:8}}>
              预计获得 <strong>{payAmount ? Math.floor(parseFloat(payAmount) * payRate) : 0}</strong> 板栗
            </div>
            <div style={{display:'flex',gap:8,justifyContent:'center',marginBottom:8}}>
              <label style={{display:'flex',alignItems:'center',gap:4,cursor:'pointer',fontWeight:payType==='alipay'?700:400}}>
                <input type="radio" name="paytype" value="alipay" checked={payType==='alipay'} onChange={() => setPayType('alipay')} /> 支付宝
              </label>
              <label style={{display:'flex',alignItems:'center',gap:4,cursor:'pointer',fontWeight:payType==='wxpay'?700:400}}>
                <input type="radio" name="paytype" value="wxpay" checked={payType==='wxpay'} onChange={() => setPayType('wxpay')} /> 微信
              </label>
            </div>
            {payMsg && <div className={"result-box " + (payMsgOk ? "ok" : "err")} style={{marginBottom:8}}>{payMsg}</div>}
            <button className="btn btn-primary" style={{width:'100%'}} onClick={handleCreatePayOrder}>立即支付</button>
          </div>
        )}
        {payStep === 'qr' && (
          <div style={{textAlign:'center',marginTop:8}}>
            {payQRCode && <img src={payQRCode} alt="支付二维码" style={{maxWidth:200,maxHeight:200,margin:'0 auto',display:'block'}} />}
            {payURL && !payQRCode && (
              <div style={{margin:'12px 0'}}>
                <p style={{fontSize:13,color:'#8b7a61'}}>支付页面已在新窗口打开</p>
                <button className="btn btn-outline btn-sm" onClick={() => window.open(payURL, '_blank')}>重新打开支付页面</button>
              </div>
            )}
            <p style={{fontSize:12,color:'#8b7a61',marginTop:8}}>支付完成后请稍候，系统会自动确认</p>
            <p style={{fontSize:12,color:'#8b7a61'}}>订单号: {payOrderNo}</p>
          </div>
        )}
        {payStep === 'done' && (
          <div style={{textAlign:'center',marginTop:8}}>
            <div style={{fontSize:48,marginBottom:12}}>🎉</div>
            {payCodes.map((c: string, i: number) => (
              <div key={i} style={{background:'#f5ecd7',padding:'12px',borderRadius:8,fontFamily:'monospace',fontSize:18,fontWeight:700,letterSpacing:2,marginBottom:8,wordBreak:'break-all'}}>
                {c}
              </div>
            ))}
            <p style={{fontSize:12,color:'#8b7a61'}}>激活码已生成，可分享给朋友使用</p>
          </div>
        )}
      </Modal>
      */}

        <Modal open={showGrowthModal} title="成长等级覆盖" message="输入要显示的游戏等级（整数），设置后该账号在房间内显示的等级将改变，但不会因此进入高等级限制的服务器。" detail={`当前设定值：${growthValue}`}
          confirmLabel="确认" cancelLabel="取消"
          onConfirm={confirmGrowthOverride}
          onCancel={() => setShowGrowthModal(false)}>
          <input className="input" type="number" min="0" max="999" value={growthModalValue}
            onChange={e => setGrowthModalValue(e.target.value)}
            style={{width:'100%',textAlign:'center'}} autoFocus />
        </Modal>
      </section>
      <div className="cs-row">
        <a className="cs-link cs-link-left"
          href="https://example.com/archives/prism"
          target="_blank" rel="noopener noreferrer">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
            <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"/><polyline points="14 2 14 8 20 8"/>
          </svg>
          查看文档
        </a>
        <a className="cs-link cs-link-right"
          href={`https://example.com/qa?token=${encodeURIComponent(user?.token || '')}`}
          target="_blank" rel="noopener noreferrer">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
            <path d="M4 14v-2a8 8 0 0 1 16 0v2"/>
            <rect x="2" y="13" width="4" height="6" rx="1.5"/>
            <rect x="18" y="13" width="4" height="6" rx="1.5"/>
            <path d="M20 19a4 4 0 0 1-4 4h-3"/>
          </svg>
          联系客服
        </a>
      </div>
      <NutsPolicy open={showPolicy} onClose={() => setShowPolicy(false)} />
      <ApiDoc open={showApiDoc} onClose={() => setShowApiDoc(false)} />
    </div>
  );
}

// ── Toolbox User Section ──
function ToolboxUserSection() {
  const [info, setInfo] = useState<any>(null);
  const [loading, setLoading] = useState(true);
  const [promptText, setPromptText] = useState('');
  const [nameText, setNameText] = useState('');
  const [saving, setSaving] = useState(false);

  const loadInfo = async () => {
    setLoading(true);
    const r: any = await getMyToolboxInfo();
    if (r.ok && r.data) {
      setInfo(r.data);
      setPromptText(r.data.custom_prompt || '');
      setNameText(r.data.custom_name || '');
    }
    setLoading(false);
  };
  useEffect(() => { loadInfo(); }, []);

  const handleStartTrial = async () => {
    if (!confirm('确定激活工具箱 90 天免费试用？（仅一次）')) return;
    const r: any = await startToolboxTrial();
    if (r.ok) { showToast('恭喜!试用已激活~', 'ok'); loadInfo(); }
    else showToast(r.error || '糟糕,激活失败了,请稍后重试~', 'err');
  };

  const handlePurchase = async (feature: string) => {
    const names: any = { prompt: '自定义提示词', name: '自定义命名' };
    if (!confirm(`确定花费板栗购买「${names[feature] || feature}」？`)) return;
    const r: any = await purchaseToolboxFeature(feature);
    if (r.ok) { showToast('恭喜!购买成功~', 'ok'); loadInfo(); }
    else showToast(r.error || '糟糕,购买失败了,请稍后重试~', 'err');
  };

  const handleExtend = async () => {
    const days = parseInt(prompt('续期天数？（30天 = 100板栗，60天 = 200板栗）', '30') || '0');
    if (days <= 0) return;
    const r: any = await extendToolboxAccess(days);
    if (r.ok) { showToast(`恭喜!续期 ${days} 天成功~`, 'ok'); loadInfo(); }
    else showToast(r.error || '糟糕,续期失败了,请稍后重试~', 'err');
  };

  const handleSaveCustom = async () => {
    setSaving(true);
    const r: any = await updateMyToolbox({ custom_prompt: promptText, custom_name: nameText });
    if (r.ok) { showToast('已保存', 'ok'); loadInfo(); }
    else showToast(r.error || '糟糕,保存没成功,请稍后重试~', 'err');
    setSaving(false);
  };

  if (loading) return (
    <section className="card" style={{marginBottom:16}}>
      <div className="skeleton-line" style={{width:120,height:18,marginBottom:10}} />
      <div className="skeleton-line" style={{width:200,height:14}} />
    </section>
  );

  return (
    <section className="card" style={{marginBottom:16}}>
      <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:10}}>
        <h3 style={{margin:0}}>工具箱</h3>
        <button className="btn btn-sm btn-outline" onClick={loadInfo}>刷新</button>
      </div>

      {/* Status badge */}
      <div style={{display:'flex',alignItems:'center',gap:10,marginBottom:10,flexWrap:'wrap'}}>
        <span className={info?.enabled && !info?.expired ? 'status-pill' : 'status-pill danger'} style={{fontSize:12}}>
          {info?.enabled ? (info?.expired ? '已过期' : '已开通') : '未开通'}
        </span>
        {info?.expires_at && (
          <span style={{fontSize:12,color:'var(--phx-text-secondary)'}}>
            到期: {new Date(info.expires_at).toLocaleDateString('zh-CN')}
            {info.days_remaining > 0 && <span style={{marginLeft:6}}>(剩余 {info.days_remaining} 天)</span>}
          </span>
        )}
        <span style={{fontSize:12,color:'var(--phx-text-secondary)'}}>
          提示词: {info?.prompt_purchased ? '✓ 已购' : '未购'}
          <span style={{margin: '0 8px'}}>|</span>
          命名: {info?.name_purchased ? '✓ 已购' : '未购'}
        </span>
      </div>

      {/* Action buttons */}
      {!info?.trial_used && !info?.enabled && (
        <button className="btn btn-sm btn-primary" onClick={handleStartTrial} style={{marginBottom:8}}>
          激活免费试用（90 天）
        </button>
      )}
      <button className="btn btn-sm btn-outline" onClick={() => window.open('/api/toolbox/download', '_blank')} style={{marginBottom:8}}>下载工具箱</button>
      {info?.enabled && !info?.expired && (
        <div style={{display:'flex',gap:8,flexWrap:'wrap',marginBottom:10}}>
          <button className="btn btn-sm btn-primary" onClick={() => window.open('/api/toolbox/download', '_blank')}>下载工具箱</button>
          <button className="btn btn-sm btn-outline" onClick={handleExtend}>续期（板栗）</button>
          {!info?.prompt_purchased && (
            <button className="btn btn-sm btn-outline" onClick={() => handlePurchase('prompt')}>
              购买自定义提示词
            </button>
          )}
          {!info?.name_purchased && (
            <button className="btn btn-sm btn-outline" onClick={() => handlePurchase('name')}>
              购买自定义命名
            </button>
          )}
        </div>
      )}

      {/* Current values */}
      <div style={{fontSize:12,color:'var(--phx-text-secondary)',marginBottom:8,background:'var(--phx-bg)',padding:'8px 10px',borderRadius:8}}>
        <div>当前提示词: <strong>{info?.effective_prompt || '(空)'}</strong></div>
        <div>当前名称: <strong>{info?.effective_name || '(空)'}</strong></div>
      </div>

      {/* Edit custom prompt/name (only if purchased) */}
      {(info?.prompt_purchased || info?.name_purchased) && (
        <div style={{display:'flex',flexDirection:'column',gap:6}}>
          {info?.prompt_purchased && (
            <input className="input" placeholder="自定义提示词（机器人进服发送的消息）" value={promptText} onChange={e => setPromptText(e.target.value)} />
          )}
          {info?.name_purchased && (
            <input className="input" placeholder="自定义名称（导入进度条显示的名称）" value={nameText} onChange={e => setNameText(e.target.value)} />
          )}
          <button className="btn btn-sm btn-primary" onClick={handleSaveCustom} disabled={saving} style={{alignSelf:'flex-start'}}>
            {saving ? '保存中...' : '保存自定义设置'}
          </button>
        </div>
      )}
    </section>
  );
}
