import { useState, useEffect, useRef } from 'react';
import {
  listAccounts,
  addAccount,
  switchAccount,
  deleteAccount,  updateAccountNickname,
  refreshAccount,
  activeAccount,
  updateAccount,
  getSkinPresets,
  changeSkin,
  checkin,
  checkinStatus,
  getActiveSurvey,
  setServerOwner,
  listServerOwners,
  getOwnerServers,
  refreshOwnerServers,
  listTokens,
} from '../api';
import { useAuth } from '../AuthContext';
import AvatarModal from './AvatarModal';
import GuestLogin from './GuestLogin';
import PhoneLogin from './PhoneLogin';
import EmailLogin from './EmailLogin';
import SkinHead from './SkinHead';
import Modal from './Modal';
import ModalPortal from './ModalPortal';
import SurveyModal from './SurveyModal';
import { successSound } from '../sounds';
import { showToast } from './Toast';

function statusBadge(status?: string, isActive?: boolean, disabled?: boolean) {
  const map: Record<string, string> = { normal: '正常', banned: '封禁', offline: '离线', unknown: '未知' };
  const colors: Record<string, string> = { normal: '#4f9d53', banned: '#d9534f', offline: '#d6a21d', unknown: '#9f927d' };
  if (disabled) return <span className="status-pill danger">已停用</span>;
  const s = status || 'unknown';
  return <span className="status-pill" style={{ color: colors[s] }}>{isActive ? '当前 · ' : ''}{map[s] || s}</span>;
}

function GameAvatar({ acc }: { acc: any }) {
  const [failed, setFailed] = useState(false);
  if (acc.avatar_image_url && !failed) {
    return <img className="game-avatar" src={acc.avatar_image_url} alt={acc.display_name || ''}
      onError={() => setFailed(true)} />;
  }
  if (acc.avatar_image_url && failed) {
    return <div className="game-avatar fallback" style={{background:'#c4b8a0',color:'#fff',fontSize:11,fontWeight:600}}>审核中</div>;
  }
  return <div className="game-avatar fallback">{(acc.display_name || acc.uid || '?').slice(0, 1)}</div>;
}

function hasCustomSkin(acc: any) {
  return Boolean(acc.skin_url) && acc.skin_number !== '0' && acc.skin_number !== '-1';
}

function dedupeAccounts(list: any[]) {
  const seen = new Set<string>();
  return list.filter((acc) => {
    const key = acc.uid || acc.display_name || String(acc.id);
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export default function Dashboard() {
  const { user } = useAuth();
  const [accounts, setAccounts] = useState<any[]>([]);
  const [active, setActive] = useState<any>(null);
  const [tokens, setTokens] = useState<any[]>([]);
  const [currentTokenId, setCurrentTokenId] = useState<number>(0); // 0 = primary token
  const [addMode, setAddMode] = useState<'guest' | 'phone' | 'email' | 'cookie' | null>(null);
  const [menuOpenId, setMenuOpenId] = useState<number | null>(null);
  const [editingNickname, setEditingNickname] = useState<number | null>(null);
  // sharedUIDs removed - using has_shared_copy from API
  const [nicknameInput, setNicknameInput] = useState("");
  const [nicknameErr, setNicknameErr] = useState("");
  const [cookieText, setCookieText] = useState('');
  const [addShared, setAddShared] = useState(false);
  const [loading, setLoading] = useState(true);
  const [modal, setModal] = useState<{open:boolean;title:string;message:string;onConfirm?:()=>void;variant?:'info'|'danger'|'warn';confirmLabel?:string}>({open:false,title:'',message:''});
  const [showSkinModal, setShowSkinModal] = useState(false);
  const skinModalAccRef = useRef<number | null>(null);
  const [skinPresetList, setSkinPresetList] = useState<any[]>([]);
  const [selectedSkinId, setSelectedSkinId] = useState("");
  const [customSkinInput, setCustomSkinInput] = useState("");
  const [skinMsg, setSkinMsg] = useState("");
  const [skinMsgOk, setSkinMsgOk] = useState(true);
  const [showAvatar, setShowAvatar] = useState(false);
  const [avatarAccountId, setAvatarAccountId] = useState<number | null>(null);
  const [serverOwners, setServerOwners] = useState<any[]>([]);
  const [ownerServers, setOwnerServers] = useState<any[]>([]);
  const [showOwnerModal, setShowOwnerModal] = useState(false);
  const [selectedOwner, setSelectedOwner] = useState<any>(null);
  const [ownerLoading, setOwnerLoading] = useState(false);
  const [switchingId, setSwitchingId] = useState<number | null>(null);
  const [refreshingId, setRefreshingId] = useState<number | null>(null);
  const [fetchError, setFetchError] = useState<string | null>(null);
  const [surveysLoading, setSurveysLoading] = useState(false);
  const [deleteLoading, setDeleteLoading] = useState(false);
  const fetchServerOwners = async (refresh = false) => {
    setOwnerLoading(true);
    const [ownerRes, serversRes] = await Promise.all([listServerOwners(), refresh ? refreshOwnerServers() : getOwnerServers()]);
    if ((ownerRes as any).ok) setServerOwners((ownerRes as any).accounts || []);
    if ((serversRes as any).ok) setOwnerServers((serversRes as any).accounts || []);
    setOwnerLoading(false);
  };

  const fetchAll = async (tokenId: number = currentTokenId) => {
    setLoading(true);
    setFetchError(null);
    let accErr = false;
    try {
      const [accRes, actRes] = await Promise.all([listAccounts(), activeAccount(tokenId || undefined)]);
      if ((accRes as any).ok) setAccounts((accRes as any).accounts || []);
      else accErr = true;
      if ((actRes as any).ok) setActive(actRes);
      else {
        setActive(null);
        accErr = true;
      }
    } catch {
      accErr = true;
    }
    if (accErr) setFetchError('账号列表加载失败，请检查网络后重试');
    setLoading(false);
  };

  const loadTokens = async () => {
    const r: any = await listTokens();
    if (r.ok) {
      setTokens(r.tokens || []);
      const saved = parseInt(localStorage.getItem('prism_active_token') || '0', 10);
      const valid = saved === 0 || (r.tokens || []).some((t: any) => t.id === saved);
      setCurrentTokenId(valid ? saved : 0);
    }
  };

  const selectToken = async (id: number) => {
    localStorage.setItem('prism_active_token', String(id));
    setCurrentTokenId(id);
    await fetchAll(id);
  };

  const [chkStatus, setChkStatus] = useState<{checked_in:boolean;streak:number}>({checked_in:false,streak:0});
  const [chkLoading, setChkLoading] = useState(false);
  const [chkJustDone, setChkJustDone] = useState(false);

  const [activeSurveys, setActiveSurveys] = useState<any[]>([]);
  const [currentSurvey, setCurrentSurvey] = useState<any>(null);
  const [currentQuestions, setCurrentQuestions] = useState<any[]>([]);
  const [showSurveyModal, setShowSurveyModal] = useState(false);

  useEffect(() => { loadTokens(); fetchAll(); fetchServerOwners(); }, []);
  useEffect(() => {
    checkinStatus().then((cs: any) => {
      if (cs.ok) setChkStatus({checked_in: cs.checked_in, streak: cs.streak});
    });
    setSurveysLoading(true);
    getActiveSurvey().then((r: any) => {
      if (r.ok && r.surveys) { setActiveSurveys(r.surveys); }
      else setActiveSurveys([]);
      setSurveysLoading(false);
    });
  }, []);

  const openSurvey = (s: any) => {
    setCurrentSurvey(s.survey);
    setCurrentQuestions(s.questions);
    setShowSurveyModal(true);
  };

  const handleCheckin = async () => {
    setChkLoading(true);
    const r: any = await checkin();
    setChkLoading(false);
    if (r.ok) {
      setChkStatus({checked_in:true,streak:r.streak});
      setChkJustDone(true);
      showToast(`恭喜!签到成功,+${r.reward} 板栗,连续 ${r.streak} 天~`, 'ok');
      setTimeout(() => setChkJustDone(false), 1400);
    }
    else showToast(r.error || '哎呀,签到没成功,请稍后重试~', 'err');
  };

  const handleAdd = async () => {
    if (!cookieText.trim()) return;
    const lines = cookieText.trim().split('\n').map(s => s.trim()).filter(s => s.startsWith('{') && s.includes('sauth_json'));
    const cookies = lines.length > 0 ? lines : [cookieText.trim()];
    let ok = 0, fail = 0;
    for (const c of cookies) {
      const res = await addAccount(c, addShared);
      if ((res as any).ok) ok++; else fail++;
    }
    setCookieText(''); setAddMode(null); setAddShared(false); fetchAll();
    showToast(`成功添加 ${ok} 个${fail > 0 ? `，${fail} 个失败` : ''}`, fail === 0 ? 'ok' : 'err');
  };

  const handleSwitch = async (id: number) => {
    setSwitchingId(id);
    const res = await switchAccount(id, currentTokenId || undefined);
    setSwitchingId(null);
    if (!(res as any).ok) { setModal({open:true,title:'糟糕,切换失败了,请稍后重试~',message:(res as any).error || '糟糕,切换失败了,请稍后重试~',variant:'danger'}); return; }
    const acc = accounts.find(a => a.id === id);
    showToast(`已切换到「${acc?.display_name || acc?.uid || id}」`, 'ok');
    fetchAll();
    loadTokens(); // 切换会改 token 的绑定，需刷新令牌状态以保证卡片 token 徽标准确
  };

  const handleDelete = async (acc: any) => {
    const name = acc?.display_name || acc?.uid || '这个账号';
    setModal({open:true,title:'确认删除',message:`确定删除「${name}」？此操作不可撤销，删除后需重新登录。`,variant:'danger',confirmLabel:'删除',onConfirm:async()=>{setDeleteLoading(true);await deleteAccount(acc.id);setDeleteLoading(false);setModal({open:false,title:'',message:''});fetchAll();}});return;
  };

  const handleRefresh = async (id: number) => {
    setRefreshingId(id);
    const res = await refreshAccount(id);
    setRefreshingId(null);
    if (!(res as any).ok) { setModal({open:true,title:'哎呀,刷新失败了,请稍后重试~',message:(res as any).error || '哎呀,刷新失败了,请稍后重试~',variant:'danger'}); return; }
    showToast('刷新成功', 'ok');
    fetchAll();
  };

  const handleToggleShared = async (acc: any) => {
    const res = await updateAccount(acc.id, { shared: !acc.is_shared });
    if (!(res as any).ok) { setModal({open:true,title:'哎呀,更新失败了,请稍后重试~',message:(res as any).error || '哎呀,更新失败了,请稍后重试~',variant:'danger'}); return; }
    fetchAll();
  };

  const [togglingId, setTogglingId] = useState<number | null>(null);

  const handleAutoRefreshToggle = async (acc: any) => {
    const next = !acc.auto_refresh_enabled;
    setTogglingId(acc.id);
    const res = await updateAccount(acc.id, { auto_refresh_enabled: next });
    if ((res as any).ok) { await fetchAll(); }
    else { setModal({open:true,title:'糟糕,操作没成功,请稍后重试~',message:(res as any).error || '糟糕,操作没成功,请稍后重试~',variant:'danger'}); }
    setTogglingId(null);
  };

  const [autoRefreshTip, setAutoRefreshTip] = useState(false);
  const [listMode, setListMode] = useState(false);
  const firstPhoneIdx = useRef(-1);

  const addButtons = (
    <>
      <div className="add-methods">
        {[
          { key: 'guest', label: '游客登录' },
          { key: 'phone', label: '手机号登录' },
          { key: 'email', label: '邮箱登录' },
          { key: 'cookie', label: 'Cookie 登录' },
        ].map((m) => (
          <button key={m.key}
            className={`add-method-tab${addMode === m.key ? ' active' : ''}`}
            onClick={() => setAddMode(addMode === m.key ? null : (m.key as any))}>
            {m.label}
          </button>
        ))}
      </div>
      {addMode === 'guest' && <div className="add-panel"><GuestLogin onSuccess={fetchAll} /></div>}
      {addMode === 'phone' && <div className="add-panel"><PhoneLogin onSuccess={fetchAll} /></div>}
      {addMode === 'email' && <div className="add-panel"><EmailLogin onSuccess={fetchAll} /></div>}
      {addMode === 'cookie' && (
        <div className="add-panel">
          <div className="mini-panel">
            <div className="panel-title">Cookie 登录</div>
            <textarea className="input textarea" rows={4} placeholder="粘贴 cookie JSON..." value={cookieText} onChange={(e) => setCookieText(e.target.value)} />
            <label className="check-line">
              <input type="checkbox" checked={addShared} onChange={(e) => setAddShared(e.target.checked)} />
              添加到共享账号池
            </label>
            <div className="inline-actions">
              <button className="btn btn-success" onClick={handleAdd}>添加并验证</button>
              <button className="btn btn-outline" onClick={() => setAddMode(null)}>取消</button>
            </div>
          </div>
        </div>
      )}
    </>
  );

  const myAccounts = dedupeAccounts(accounts.filter((a: any) => !a.is_shared && !a.is_server_owner));
  const sharedAccounts = dedupeAccounts(accounts.filter((a: any) => a.is_shared));


  const handleNicknameSave = async (accId: number) => {
    if (!nicknameInput.trim()) { setNicknameErr('昵称不能为空'); return; }
    setNicknameErr('');
    const res = await updateAccountNickname(accId, nicknameInput.trim());
    if ((res as any).ok) { setEditingNickname(null); setMenuOpenId(null); fetchAll(); }
    else { setNicknameErr((res as any).error || "修改失败"); }
  };

  // 主令牌(全局)绑定账号 = listAccounts 中 is_active 的那个；子令牌绑定见 tokens
  const primaryAccountId = accounts?.find(a => a.is_active)?.id ?? null;
  const tokenIdsFor = (accId: number): number[] => (tokens || []).filter(t => t.active_account_id === accId).map(t => t.id);

  // 令牌配色：按令牌 id 确定性取色，保证每个令牌一个固定颜色；主令牌用金色。
  const TOKEN_COLORS = ['#4a90d9', '#e8772e', '#7a4dd6', '#d94f6f', '#3ca06b', '#c8a45c', '#4db6c9', '#9c6b3f'];
  const tokenColor = (id: number) => TOKEN_COLORS[Math.abs(id) % TOKEN_COLORS.length];
  const PRIMARY_COLOR = '#c8a45c';
  // 账号卡片外阴影颜色：异常(封禁/停用)优先取红色，否则按使用该账号的令牌颜色；无则 undefined。
  const cardShadow = (acc: any, ids: number[], isPrimary: boolean): string | undefined => {
    if (acc.disabled || acc.status === 'banned') return '#d9534f'; // 账号异常优先于令牌阴影
    if (ids.length > 0) return tokenColor(currentTokenId && ids.includes(currentTokenId) ? currentTokenId : ids[0]);
    if (isPrimary) return PRIMARY_COLOR;
    return undefined;
  };

  const renderAccountCard = (acc: any, shared = false, _index = -1) => {
    const ids = tokenIdsFor(acc.id);
    const isPrimary = primaryAccountId === acc.id;
    const shadowColor = cardShadow(acc, ids, isPrimary);
    return (
    <div key={acc.id} className={`game-card${acc.is_active ? ' active' : ''}${acc.disabled ? ' disabled' : ''}`}
      style={{ position: 'relative', ...(shadowColor ? { boxShadow: `0 0 0 1px ${shadowColor}40, 0 8px 22px ${shadowColor}55`, borderColor: shadowColor, animation: 'none' } : {}) }}>
      {hasCustomSkin(acc) && <SkinHead skinUrl={acc.skin_url} label={acc.display_name || acc.uid} className="game-card-skin-bg" hideOnFallback />}
      <div className="game-card-visual">
        <GameAvatar acc={acc} />
      </div>
      <div className="game-card-body">
        <div className="game-card-head">
          <div>
            <div className="game-name">{acc.display_name || '未知游戏账号'}</div>
            <div className="game-uid">UID {acc.uid || '-'}</div>
          </div>
          {statusBadge(acc.status, acc.is_active, acc.disabled)}
          {(() => {
            const total = ids.length + (isPrimary ? 1 : 0);
            if (total === 0) return null;
            return (
              <div style={{ display: 'flex', alignItems: 'center', gap: 3, marginLeft: 4, flexShrink: 0, flexWrap: 'wrap' }}>
                {isPrimary && <span title="主令牌正在使用" style={{ width: 16, height: 16, borderRadius: 5, background: PRIMARY_COLOR, color: '#fff', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 11, lineHeight: 1 }}>★</span>}
                {ids.map(id => <span key={id} title={`令牌 #${id} 正在使用`} style={{ height: 16, minWidth: 18, padding: '0 3px', borderRadius: 5, background: tokenColor(id), color: '#fff', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', fontSize: 10, lineHeight: 1 }}>#{id}</span>)}
                {total >= 2 && <span title="多个令牌共用此账号" style={{ width: 16, height: 16, borderRadius: 5, background: '#e8b04b', color: '#fff', display: 'inline-flex', alignItems: 'center', justifyContent: 'center' }}><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg></span>}
              </div>
            );
          })()}
          {(!shared || user?.role === 'admin') && (
            <div className="card-menu" style={{position:"relative",marginLeft:4}}>
              <button className="btn-dots" onClick={(e) => { e.stopPropagation(); setMenuOpenId(menuOpenId === acc.id ? null : acc.id); }} style={{background:"none",border:"none",cursor:"pointer",fontSize:18,lineHeight:1,padding:"0 4px",color:"#8b7a61"}}>&#8942;</button>
              {menuOpenId === acc.id && (
                <div className="card-menu-dropdown" style={{position:"absolute",right:0,top:22,background:"#fffaf0",border:"1px solid #eadfc9",borderRadius:8,padding:4,minWidth:100,boxShadow:"0 4px 12px rgba(0,0,0,0.1)",zIndex:10}}
                  onClick={(e) => e.stopPropagation()}>
                  {!shared && !acc.is_server_owner && (
                    <button className="btn-menu-item" onClick={() => {
                      setMenuOpenId(null);
                      setModal({
                        open: true, title: '设为服主账号',
                        message: `将「${acc.display_name}」设为服主账号后：\n\n• 您可以使用该账号管理其名下的租赁服务器\n• 该账号将不再参与在线心跳保活和自动刷新\n• 该账号将在首页单独显示\n• 存在封号风险，请谨慎操作\n\n确认设置？`,
                        variant: 'warn',
                        onConfirm: async () => {
                          setModal({ open: false, title: '', message: '' });
                          const res = await setServerOwner(acc.id, true);
                          if ((res as any).ok) { showToast('已设为服主账号', 'ok'); fetchAll(); fetchServerOwners(); }
                          else { showToast((res as any).error || '哎呀,设置失败了,请稍后重试~', 'err'); }
                        },
                      });
                    }} style={{ display: "block", width: "100%", background: "none", border: "none", padding: "6px 12px", textAlign: "left", cursor: "pointer", fontSize: 13, color: "#d4a017", borderRadius: 4 }}>
                      设为服主账号
                    </button>
                  )}
                  {!shared && acc.is_server_owner && (
                    <button className="btn-menu-item" onClick={async () => {
                      setMenuOpenId(null);
                      const res = await setServerOwner(acc.id, false);
                      if ((res as any).ok) { showToast('已取消服主关联', 'ok'); fetchAll(); fetchServerOwners(); }
                      else { showToast((res as any).error || '哎呀,取消失败了,请稍后重试~', 'err'); }
                    }} style={{ display: "block", width: "100%", background: "none", border: "none", padding: "6px 12px", textAlign: "left", cursor: "pointer", fontSize: 13, color: "#d9534f", borderRadius: 4 }}>
                      取消服主关联
                    </button>
                  )}
                  <button className="btn-menu-item" onClick={() => { setEditingNickname(acc.id); setNicknameInput(acc.display_name || ""); setNicknameErr(""); setMenuOpenId(null); }} style={{display:"block",width:"100%",background:"none",border:"none",padding:"6px 12px",textAlign:"left",cursor:"pointer",fontSize:13,color:"#5d4b36",borderRadius:4}}>修改昵称</button>
                  <button className="btn-menu-item" onClick={() => { skinModalAccRef.current = acc.id; setSelectedSkinId(""); setCustomSkinInput(""); setSkinMsg(""); getSkinPresets().then((r:any) => { if (r.ok) setSkinPresetList(r.presets || []); }); setMenuOpenId(null); setShowSkinModal(true); }} style={{display:"block",width:"100%",background:"none",border:"none",padding:"6px 12px",textAlign:"left",cursor:"pointer",fontSize:13,color:"#5d4b36",borderRadius:4}}>更换皮肤</button>
                  <button className="btn-menu-item" onClick={() => { setMenuOpenId(null); setAvatarAccountId(acc.id); setShowAvatar(true); }} style={{display:"block",width:"100%",background:"none",border:"none",padding:"6px 12px",textAlign:"left",cursor:"pointer",fontSize:13,color:"#5d4b36",borderRadius:4}}>更换头像</button>
                  {shared && user?.role === 'admin' && (
                    <button className="btn-menu-item" onClick={() => { setMenuOpenId(null); handleDelete(acc); }} style={{display:"block",width:"100%",background:"none",border:"none",padding:"6px 12px",textAlign:"left",cursor:"pointer",fontSize:13,color:"#d9534f",borderRadius:4}}>从共享池删除</button>
                  )}
                </div>
              )}
            </div>
          )}
        </div>
        <div className="game-stat-row">
          <div className="game-stat"><span>等级</span><strong>{acc.growth_level || '0'}</strong></div>
          <div className="game-stat"><span>归属</span><strong>{shared ? '共享池' : '私有'}</strong></div>
          <div className="game-stat"><span>来源</span><strong>{acc.source || '-'}</strong></div>
        </div>
        {!shared && (acc.source === 'phone' || acc.source === 'email') && (
          <div className="auto-refresh-line" style={{display:'flex',alignItems:'center',gap:6,margin:'6px 0',fontSize:13,color:'#8b7a61'}}>
            <input type="checkbox" checked={togglingId === acc.id ? !acc.auto_refresh_enabled : !!acc.auto_refresh_enabled} onChange={() => handleAutoRefreshToggle(acc)} style={{cursor:'pointer',accentColor:'var(--phx-primary)'}} />
            <span style={{cursor:'default'}}>自动刷新</span>
            <span style={{position:'relative',display:'inline-flex'}}>
              <span onClick={(e) => { e.stopPropagation(); e.preventDefault(); setAutoRefreshTip(!autoRefreshTip); }} style={{display:'inline-flex',alignItems:'center',justifyContent:'center',width:16,height:16,borderRadius:'50%',background:'#d9c9a5',color:'#fff',fontSize:11,fontWeight:700,cursor:'help'}}>?</span>
              {autoRefreshTip && (
                <span style={{position:'absolute',bottom:22,left:0,background:'#fffaf0',border:'1px solid #eadfc9',borderRadius:8,padding:'8px 12px',fontSize:12,color:'#5d4b36',whiteSpace:'nowrap',zIndex:10,boxShadow:'0 4px 12px rgba(0,0,0,0.1)'}}>
                  手机/邮箱登录的账号默认关闭此功能，开启后参与在线心跳保活。
                </span>
              )}
            </span>
          </div>
        )}
        <div className="account-actions">
          {!acc.is_active && acc.status !== 'banned' && !acc.disabled && <button className="btn btn-sm btn-primary" onClick={() => handleSwitch(acc.id)} disabled={switchingId === acc.id}>{switchingId === acc.id ? '切换中...' : '切换使用'}</button>}
          <button className="btn btn-sm btn-outline" onClick={() => handleRefresh(acc.id)} disabled={refreshingId === acc.id}>{refreshingId === acc.id ? '刷新中...' : '刷新'}</button>
          {!shared && (acc.has_shared_copy ? <button className="btn btn-sm btn-outline" disabled>已共享</button> : <button className="btn btn-sm btn-outline" onClick={() => handleToggleShared(acc)}>设为共享</button>)}
          {shared && acc.can_reclaim && <button className="btn btn-sm btn-outline" onClick={() => handleToggleShared(acc)} title="收回自己共享出去的账号">收回私有</button>}
          {!shared && <button className="btn btn-sm btn-danger" onClick={() => handleDelete(acc)}>删除</button>}
        </div>
        {editingNickname === acc.id && (
          <div className="edit-drawer" style={{marginTop:8}}>
            <input className="input" value={nicknameInput} onChange={e => setNicknameInput(e.target.value)} placeholder="新游戏昵称" autoFocus />
            {nicknameErr && <div className="result-box err" style={{margin:"6px 0 0"}}>{nicknameErr}</div>}
            <div className="inline-actions" style={{marginTop:6}}>
              <button className="btn btn-primary" onClick={() => handleNicknameSave(acc.id)}>保存</button>
              <button className="btn btn-outline" onClick={() => { setEditingNickname(null); setNicknameErr(""); }}>取消</button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
  };

  if (loading && accounts.length === 0) {
    return (
      <div className="page-stack">
        {/* Hero skeleton — 匹配 eyebrow/h1/p + active-snapshot */}
        <section className="hero-panel compact">
          <div>
            <div className="skeleton-line" style={{width:64,height:11,marginBottom:10}} />
            <div className="skeleton-line" style={{width:200,height:26,marginBottom:10}} />
            <div className="skeleton-line" style={{width:260,height:13}} />
          </div>
          <div className="skeleton-line" style={{width:120,height:64,borderRadius:22}} />
        </section>
        {/* 添加账号卡片 — 匹配 4 个堆叠登录按钮 */}
        <section className="card">
          <div className="skeleton-line" style={{width:140,height:18,marginBottom:8}} />
          <div className="skeleton-line" style={{width:240,height:13,marginBottom:16}} />
          <div className="add-methods">
            {Array.from({length:4}).map((_,i)=><div key={i} className="skeleton-line" style={{height:40,borderRadius:12,flex:1}} />)}
          </div>
        </section>
        {/* 我的游戏账号 — 匹配 game-card 结构 */}
        <section className="card">
          <div className="skeleton-line" style={{width:140,height:18,marginBottom:16}} />
          <div className="game-grid">
            {Array.from({length:3}).map((_,i)=>(
              <div key={i} className="game-card">
                <div className="game-card-visual">
                  <div className="skeleton-avatar" style={{width:60,height:60,borderRadius:18,marginBottom:0}} />
                </div>
                <div className="game-card-body">
                  <div className="skeleton-line" style={{width:'55%',height:17,marginBottom:6}} />
                  <div className="skeleton-line" style={{width:'35%',height:12,marginBottom:14}} />
                  <div className="game-stat-row">
                    {Array.from({length:3}).map((_,j)=><div key={j} className="game-stat"><div className="skeleton-line" style={{height:10,marginBottom:6}} /><div className="skeleton-line" style={{height:14,width:'60%'}} /></div>)}
                  </div>
                  <div className="account-actions">
                    <div className="skeleton-line" style={{flex:1,height:30,borderRadius:10,marginBottom:0}} />
                    <div className="skeleton-line" style={{flex:1,height:30,borderRadius:10,marginBottom:0}} />
                  </div>
                </div>
              </div>
            ))}
          </div>
        </section>
      </div>
    );
  }

  return (
    <div className="page-stack">
      <section className="hero-panel">
        <div>
          <div className="eyebrow">游戏账号</div>
          <h1>游戏账号控制台</h1>
          <p>管理你的游戏账号，添加、切换或进入服务器。</p>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12, alignItems: 'flex-start', minWidth: 0 }}>
          <div className="token-selector">
            <span className="token-selector-label">当前令牌</span>
            {[{ id: 0, name: '主', title: '主令牌' }, ...tokens.map(t => ({ id: t.id, name: t.name || ('#' + t.id), title: t.name ? `令牌#${t.id}(${t.name})` : `令牌#${t.id}` }))].map(t => (
              <button key={t.id} className={`token-pill${currentTokenId === t.id ? ' active' : ''}`} title={t.title} onClick={() => selectToken(t.id)}>
                {t.name}
              </button>
            ))}
          </div>
          {active ? (
            <div className="active-snapshot">
              <GameAvatar acc={active} />
              <div>
                <strong>{active.display_name || '当前账号'}</strong>
                <span>等级 {active.growth_level || '0'} · UID {active.uid || '-'}</span>
              </div>
            </div>
          ) : <span className="status-pill">暂无活跃账号</span>}
        </div>
      </section>

      {fetchError && (
        <section className="card" style={{borderColor:'var(--phx-error)'}}>
          <div className="card-header" style={{gap:8}}>
            <span style={{fontSize:18}}>⚠️</span>
            <h2>加载失败</h2>
          </div>
          <p style={{color:'var(--phx-text-secondary)',fontSize:13,margin:'0 0 10px',lineHeight:1.6}}>{fetchError}</p>
          <button className="btn btn-primary btn-sm" onClick={() => fetchAll()} style={{width:'auto'}}>重新加载</button>
        </section>
      )}

      {serverOwners.length > 0 && (
        <section className="card">
          <div className="card-header"><h2>
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{verticalAlign:'middle',marginRight:6}}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
                服主账号
              </h2></div>
          <div className="owner-list">
            {serverOwners.map((owner: any) => (
              <div key={owner.id} className="owner-row" style={{
                display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                padding: '8px 0', borderBottom: '1px solid var(--phx-border-light)',
              }}>
                <span style={{ fontWeight: 600, fontSize: 14 }}>{owner.display_name}</span>
                <div style={{ display: 'flex', gap: 8 }}>
                  <button className="btn btn-sm btn-primary" onClick={async () => {
                    setSelectedOwner(owner);
                    setShowOwnerModal(true);
                    fetchServerOwners();
                  }}>管理</button>
                  <button className="btn btn-sm btn-outline" onClick={async () => {
                    const res = await setServerOwner(owner.id, false);
                    if ((res as any).ok) { showToast('已取消关联', 'ok'); fetchAll(); fetchServerOwners(); }
                    else { showToast((res as any).error || '哎呀,取消失败了,请稍后重试~', 'err'); }
                  }}>取消关联</button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {(surveysLoading || activeSurveys.length > 0 || !chkStatus.checked_in || chkJustDone) && (
        <section className="card">
          <div className="card-header"><h2>今日活动</h2></div>
          {(!chkStatus.checked_in || chkJustDone) && (
            <div className="activity-block">
              <div className="activity-body">
                {chkJustDone ? (
                  <p>已连续签到 <strong className="num">{chkStatus.streak}</strong> 天</p>
                ) : (
                  <p>连续 <strong className="num">{chkStatus.streak}</strong> 天，今日签到可得 <strong className="num">{chkStatus.streak === 6 ? 3 : 1}</strong> 板栗</p>
                )}
              </div>
              <div className="activity-actions">
                {chkJustDone ? <span className="chk-done">已签到 ✓</span> : <button className="btn btn-primary btn-sm" onClick={handleCheckin} disabled={chkLoading}>{chkLoading ? '签到中...' : '签到'}</button>}
              </div>
            </div>
          )}
          {surveysLoading ? (
            <div className="activity-block">
              <div className="activity-body"><p>正在加载问卷…</p></div>
            </div>
          ) : activeSurveys.map((s: any) => (
            <div key={s.survey.id} className="activity-block">
              <div className="activity-body">
                <div className="activity-title">{s.survey.title}</div>
                <p>奖励 <strong className="num" style={{color:'var(--phx-success)'}}>{s.survey.reward_nuts}</strong> 板栗</p>
              </div>
              <div className="activity-actions">
                <button className="btn btn-primary btn-sm" onClick={() => openSurvey(s)}>填写</button>
              </div>
            </div>
          ))}
        </section>
      )}

      <section className="card add-card">
        <div className="card-header"><h2>添加游戏账号</h2></div>
        <p className="card-desc">选择登录方式添加游戏账号，支持邮箱、手机、游客和 Cookie 直接导入。</p>
        {addButtons}
      </section>

      <section className="card">
        <div className="card-header" style={{display:'flex',alignItems:'center',justifyContent:'space-between'}}>
          <h2>我的游戏账号</h2>
          <label style={{display:'flex',alignItems:'center',gap:6,fontSize:13,color:'#8b7a61',cursor:'pointer'}}>
            <span>极简列表</span>
            <input type="checkbox" checked={listMode} onChange={() => setListMode(!listMode)} style={{cursor:'pointer',accentColor:'var(--phx-primary)'}} />
          </label>
        </div>
        {myAccounts.length === 0 ? <div className="empty-state">还没有私人账号，用上方「添加游戏账号」登入一个</div> :
          listMode ? (
            <div className="account-list">
              {myAccounts.map((acc) => (
                <div key={acc.id} className="account-list-row" style={{
                  display:'flex',alignItems:'center',justifyContent:'space-between',gap:12,
                  padding:'8px 0',borderBottom:'1px solid var(--phx-border-light)',fontSize:14
                }}>
                  <div style={{display:'flex',alignItems:'center',gap:8,minWidth:0}}>
                    <GameAvatar acc={acc} />
                    <span style={{fontWeight:600,whiteSpace:'nowrap',overflow:'hidden',textOverflow:'ellipsis'}}>{acc.display_name || '未知'}</span>
                    {statusBadge(acc.status, acc.is_active, acc.disabled)}
                  </div>
                  <div style={{display:'flex',alignItems:'center',gap:8,flexShrink:0}}>
                    <span style={{color:'#8b7a61',fontSize:12}}>Lv{acc.growth_level||0}</span>
                    {!acc.is_active && acc.status !== 'banned' && !acc.disabled && <button className="btn btn-sm btn-primary" onClick={() => handleSwitch(acc.id)} disabled={switchingId === acc.id}>{switchingId === acc.id ? '切换中...' : '切换'}</button>}
                    <button className="btn btn-sm btn-outline" onClick={() => handleDelete(acc)}>删除</button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="game-grid">{(firstPhoneIdx.current = myAccounts.findIndex(a => a.source === 'phone' || a.source === 'email'), myAccounts.map((acc, i) => renderAccountCard(acc, false, i)))}</div>
          )}
      </section>

      <section className="card">
        <div className="card-header"><h2>共享账号池</h2></div>
        {sharedAccounts.length === 0 ? <div className="empty-state">共享账号会出现在这里，添加后可以「设为共享」</div> :
          listMode ? (
            <div className="account-list">
              {sharedAccounts.map((acc) => (
                <div key={acc.id} className="account-list-row" style={{
                  display:'flex',alignItems:'center',justifyContent:'space-between',gap:12,
                  padding:'8px 0',borderBottom:'1px solid var(--phx-border-light)',fontSize:14
                }}>
                  <div style={{display:'flex',alignItems:'center',gap:8,minWidth:0}}>
                    <GameAvatar acc={acc} />
                    <span style={{fontWeight:600}}>{acc.display_name || '未知'}</span>
                    {statusBadge(acc.status, acc.is_active, acc.disabled)}
                  </div>
                  <div style={{display:'flex',alignItems:'center',gap:8,flexShrink:0}}>
                    <span style={{color:'#8b7a61',fontSize:12}}>Lv{acc.growth_level||0}</span>
                    {!acc.is_active && acc.status !== 'banned' && !acc.disabled && <button className="btn btn-sm btn-primary" onClick={() => handleSwitch(acc.id)} disabled={switchingId === acc.id}>{switchingId === acc.id ? '切换中...' : '切换'}</button>}
                    {acc.can_reclaim && <button className="btn btn-sm btn-outline" onClick={() => handleToggleShared(acc)}>收回</button>}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="game-grid">{sharedAccounts.map((acc) => renderAccountCard(acc, true))}</div>
          )}
      </section>
      {showOwnerModal && selectedOwner && (() => {
        const ownerData = ownerServers.find((o: any) => o.id === selectedOwner.id);
        const rentals = ownerData?.rental_servers || [];
        const domains = ownerData?.domain_servers || [];
        return (
          <ModalPortal>
          <div className="modal-backdrop" onClick={() => setShowOwnerModal(false)}>
            <div className="modal-content" onClick={e => e.stopPropagation()} style={{ maxWidth: 520, padding: '20px 24px' }}>
              <h3 style={{ margin: '0 0 12px', fontSize: 16 }}>
                <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{verticalAlign:'middle',marginRight:6}}><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
                {selectedOwner.display_name}
              </h3>
              <div style={{ maxHeight: 400, overflowY: 'auto' }}>
                {/* 租赁服 */}
                {rentals.length > 0 && (
                  <>
                    <div style={{ fontSize: 13, fontWeight: 600, color: '#8b7a61', margin: '8px 0 4px' }}>
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{verticalAlign:'middle',marginRight:4}}><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
                      租赁服
                    </div>
                    {rentals.map((s: any) => (
                      <div key={s.server_id} className="server-row" style={{
                        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                        padding: '10px 12px', marginBottom: 4, borderRadius: 8,
                        background: 'var(--phx-bg-secondary)', fontSize: 13,
                      }}>
                        <div>
                          <div style={{ fontWeight: 500 }}>{s.name}</div>
                          <div style={{ color: '#8b7a61', fontSize: 12 }}>
                            {s.status === 1 ? (
                              <><svg width="10" height="10" viewBox="0 0 10 10" style={{verticalAlign:'middle',marginRight:3}}><circle cx="5" cy="5" r="4" fill="#4f9d53"/></svg> 在线</>
                            ) : (
                              <><svg width="10" height="10" viewBox="0 0 10 10" style={{verticalAlign:'middle',marginRight:3}}><circle cx="5" cy="5" r="4" fill="#d9534f"/></svg> 已关闭</>
                            )} · {s.player_count}/{s.capacity}人 · {s.mc_version || ''}
                          </div>
                        </div>
                        <div style={{ display: 'flex', gap: 6 }}>
                          {s.status === 1 && (
                            <button className="btn btn-sm btn-outline" onClick={async () => {
                              const res = await fetch(`/api/rental/control`, {
                                method: 'POST',
                                headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + localStorage.getItem('session_token') },
                                body: JSON.stringify({ server_id: s.server_id, status: 2, account_id: selectedOwner.id }),
                              }).then(r => r.json());
                              showToast((res as any).ok ? '重启指令已发送' : (res as any).error || '糟糕,操作没成功,请稍后重试~', (res as any).ok ? 'ok' : 'err');
                              if ((res as any).ok) { setShowOwnerModal(false); await fetchServerOwners(); }
                            }}>重启</button>
                          )}
                          {s.status === 1 ? (
                            <button className="btn btn-sm btn-outline" onClick={async () => {
                              const res = await fetch(`/api/rental/control`, {
                                method: 'POST',
                                headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + localStorage.getItem('session_token') },
                                body: JSON.stringify({ server_id: s.server_id, status: 0, account_id: selectedOwner.id }),
                              }).then(r => r.json());
                              showToast((res as any).ok ? '已关闭' : (res as any).error || '糟糕,操作没成功,请稍后重试~', (res as any).ok ? 'ok' : 'err');
                              if ((res as any).ok) { setShowOwnerModal(false); await fetchServerOwners(); }
                            }}>关闭</button>
                          ) : (
                            <button className="btn btn-sm btn-primary" onClick={async () => {
                              const res = await fetch(`/api/rental/control`, {
                                method: 'POST',
                                headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + localStorage.getItem('session_token') },
                                body: JSON.stringify({ server_id: s.server_id, status: 1, account_id: selectedOwner.id }),
                              }).then(r => r.json());
                              showToast((res as any).ok ? '已开启' : (res as any).error || '糟糕,操作没成功,请稍后重试~', (res as any).ok ? 'ok' : 'err');
                              if ((res as any).ok) await fetchServerOwners();
                            }}>开启</button>
                          )}
                        </div>
                      </div>
                    ))}
                  </>
                )}
                {/* 山头服 */}
                {domains.length > 0 && (
                  <>
                    <div style={{ fontSize: 13, fontWeight: 600, color: '#8b7a61', margin: '8px 0 4px' }}>
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{verticalAlign:'middle',marginRight:4}}><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>
                      山头服
                    </div>
                    {domains.map((s: any) => (
                      <div key={s.sid} className="server-row" style={{
                        display: 'flex', alignItems: 'center', justifyContent: 'space-between',
                        padding: '10px 12px', marginBottom: 4, borderRadius: 8,
                        background: 'var(--phx-bg-secondary)', fontSize: 13,
                      }}>
                        <div>
                          <div style={{ fontWeight: 500 }}>{s.name}</div>
                          <div style={{ color: '#8b7a61', fontSize: 12 }}>
                            {s.status === 1 ? (
                              <><svg width="10" height="10" viewBox="0 0 10 10" style={{verticalAlign:'middle',marginRight:3}}><circle cx="5" cy="5" r="4" fill="#4f9d53"/></svg> 在线</>
                            ) : (
                              <><svg width="10" height="10" viewBox="0 0 10 10" style={{verticalAlign:'middle',marginRight:3}}><circle cx="5" cy="5" r="4" fill="#d9534f"/></svg> 已关闭</>
                            )} · {s.online_count}/{s.capacity}人
                          </div>
                        </div>
                        <div style={{ display: 'flex', gap: 6 }}>
                          {s.status === 1 ? (
                            <button className="btn btn-sm btn-outline" onClick={async () => {
                              await fetch(`/api/domain/leave`, {
                                method: 'POST',
                                headers: { 'Authorization': 'Bearer ' + localStorage.getItem('session_token') },
                              });
                              showToast('已关闭', 'ok');
                              await fetchServerOwners();
                            }}>关闭</button>
                          ) : (
                            <button className="btn btn-sm btn-primary" onClick={async () => {
                              await fetch(`/api/domain/enter`, {
                                method: 'POST',
                                headers: { 'Authorization': 'Bearer ' + localStorage.getItem('session_token') },
                              });
                              showToast('已开启', 'ok');
                              await fetchServerOwners();
                            }}>开启</button>
                          )}
                        </div>
                      </div>
                    ))}
                  </>
                )}
                {!ownerLoading && rentals.length === 0 && domains.length === 0 && (
                  <div className="empty-state" style={{ padding: 24 }}>暂无服务器</div>
                )}
                {ownerLoading && rentals.length === 0 && domains.length === 0 && (
                  <div style={{ textAlign: 'center', padding: 24, color: '#8b7a61', fontSize: 13 }}>正在刷新...</div>
                )}
              </div>
              <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
                <button className="btn btn-outline" onClick={async () => { await fetchServerOwners(true); showToast('刷新成功', 'ok'); }} disabled={ownerLoading}>{ownerLoading ? '刷新中...' : '刷新列表'}</button>
                <button className="btn btn-primary" onClick={() => setShowOwnerModal(false)}>关闭</button>
              </div>
            </div>
          </div>
          </ModalPortal>
        );
      })()}
      {showSurveyModal && currentSurvey && (
        <SurveyModal survey={currentSurvey} questions={currentQuestions}
          onClose={() => setShowSurveyModal(false)}
          onSubmitted={(reward) => {
            setShowSurveyModal(false);
            setActiveSurveys(activeSurveys.filter(s => s.survey.id !== currentSurvey.id));
            showToast(`恭喜!问卷提交成功,获得 ${reward} 板栗~`, 'ok');
          }} />
      )}
      <Modal {...modal} cancelLabel="取消" onCancel={() => setModal({open:false,title:"",message:""})} confirmLoading={deleteLoading} />
      {showSkinModal && (() => {
        const firstWithPreview = skinPresetList.find((p: any) => p.preview_url) || skinPresetList[0];
        const initId = firstWithPreview?.item_id || "";
        if (initId && !selectedSkinId) { setTimeout(() => setSelectedSkinId(initId), 0); }
        const previewUrl = skinPresetList.find((p: any) => p.item_id === selectedSkinId)?.preview_url || "";
        return (
        <ModalPortal>
        <div className="modal-backdrop" onClick={() => { setShowSkinModal(false); skinModalAccRef.current = null; }}>
          <div className="modal-content" onClick={e => e.stopPropagation()} style={{maxWidth:400,padding:'20px 24px'}}>
            <h3 style={{margin:'0 0 12px',fontSize:16}}>更换皮肤</h3>
            {/* Preview of selected preset */}
            <div style={{background:'#1e1e1e',borderRadius:14,padding:16,textAlign:'center',marginBottom:12,minHeight:120,display:'flex',alignItems:'center',justifyContent:'center'}}>
              {previewUrl ? (
                <SkinHead skinUrl={previewUrl} />
              ) : (
                <div style={{color:'#666',fontSize:13}}>{selectedSkinId ? '暂无预览图' : '请选择一个预设'}</div>
              )}
            </div>
            {/* Preset list with single select */}
            <div style={{maxHeight:180,overflowY:'auto',marginBottom:12}}>
              {skinPresetList.length === 0 ? (
                <div style={{color:'#8b7a61',fontSize:13,textAlign:'center',padding:16}}>暂无预设皮肤，请手动输入 ID</div>
              ) : (
                <div style={{display:'flex',flexDirection:'column',gap:4}}>
                  {skinPresetList.map((p: any) => (
                    <div key={p.id}
                      onClick={() => { setSelectedSkinId(p.item_id); setCustomSkinInput(''); setSkinMsg(''); }}
                      style={{
                        display:'flex',alignItems:'center',gap:8,padding:'8px 12px',
                        borderRadius:10,cursor:'pointer',fontSize:13,
                        background: selectedSkinId === p.item_id ? 'rgba(25,200,185,0.12)' : 'transparent',
                        border: selectedSkinId === p.item_id ? '1px solid var(--phx-primary)' : '1px solid var(--phx-border-light)',
                        transition: 'background .15s, border-color .15s',
                      }}>
                      {p.preview_url ? (
                        <div style={{width:36,height:36,borderRadius:8,overflow:'hidden',flexShrink:0,background:'var(--phx-bg-secondary)'}}>
                          <SkinHead skinUrl={p.preview_url} size={32} />
                        </div>
                      ) : null}
                      <div style={{flex:1}}>
                        <div style={{fontWeight:500}}>{p.name}</div>
                        <div style={{fontSize:10,color:'#8b7a61'}}>{p.item_id}</div>
                      </div>
                      {selectedSkinId === p.item_id && <span style={{fontSize:16,color:'var(--phx-primary)'}}>&#10003;</span>}
                    </div>
                  ))}
                </div>
              )}
            </div>
            {/* Custom input */}
            <div style={{marginBottom:6}}>
              <input className="input" placeholder="粘贴分享链接或输入 19 位组件 ID" value={customSkinInput}
                onChange={e => {
                  const v = e.target.value; setCustomSkinInput(v);
                  const m = v.match(/id=(\d{10,})/) || v.match(/(\d{15,})/);
                  if (m) { setSelectedSkinId(m[1]); setSkinMsg(''); }
                }} style={{width:'100%',fontSize:13}} />
            </div>
            {skinMsg && <div className={"result-box " + (skinMsgOk ? "ok" : "err")} style={{marginBottom:6}}>{skinMsg}</div>}
            <button className="btn btn-primary" style={{width:'100%'}} disabled={!selectedSkinId} onClick={async () => {
              if (!selectedSkinId) { setSkinMsg('请选择皮肤'); setSkinMsgOk(false); return; }
              if (selectedSkinId.length < 10) { setSkinMsg('组件 ID 格式不正确'); setSkinMsgOk(false); return; }
              setSkinMsg(''); setSkinMsgOk(true);
              const res = await changeSkin(selectedSkinId, skinModalAccRef.current);
              if ((res as any).ok) { successSound(); setSkinMsg('恭喜!皮肤更换成功~'); setSkinMsgOk(true); setTimeout(() => { setShowSkinModal(false); fetchAll(); }, 1200); }
              else { setSkinMsg((res as any).error || '哎呀,更换失败了,请稍后重试~'); setSkinMsgOk(false); }
            }}>确认更换</button>
          </div>
        </div>
        </ModalPortal>
        );})()}
      <AvatarModal open={showAvatar} onClose={() => { setShowAvatar(false); setAvatarAccountId(null); }} accountId={avatarAccountId || undefined} />
    </div>
  );
}
