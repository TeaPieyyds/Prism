import { useState, useEffect } from 'react';
import { adminUsers, adminAccounts, adminUpdateUser, adminDeleteUser, adminAddUser, adminUpdateAccount, adminAdjustNuts, adminGrantSubscription, adminRevokeSubscription, addRealnamePreset, deleteRealnamePreset, toggleRealnamePreset, getAllRealnamePresets, getSkinPresets, addSkinPreset, deleteSkinPreset, updateSkinPreset, getAdminPerUserStats, adminPaymentOrders, adminRetryPayment, getVersionConfig, getAnnouncements, getSignatures, adminListSurveys, adminCreateSurvey, adminUpdateSurvey, adminUpdateSurveyQuestions, adminToggleSurvey, adminDeleteSurvey, adminGetSurveyQuestions, adminClearSurveyAnswers, adminGetSurveyStats, adminGetUserSurveyAnswers, adminAuditLogs, adminSystemLogs, adminCreateRedPacket, adminGetMessages, adminSetMessages } from '../api';
import Modal from './Modal';
import AdminStatsPanel from './AdminStatsPanel';
import SkinHead from './SkinHead';
import ToolboxPanel from './ToolboxPanel';
import MarketAdminPanel from './MarketAdminPanel';
import { showToast } from './Toast';

const actionText: Record<string, string> = {
  register_email_sent: '发送验证邮件',
  register_verified: '邮箱验证成功',
  login: '网页登录',
  logout: '网页退出',
  add_account: '添加游戏账号',
  switch_account: '切换游戏账号',
  delete_account: '删除游戏账号',
  refresh_account: '刷新游戏账号',
  update_username: '修改用户名',
  update_password: '修改密码',
  reset_token: '重置令牌',
  join_server: '进入租赁服',
  join_server_failed: '进服失败',
  challenge_override: '黑洞模式切换',
  update_account_nickname: '修改游戏昵称',
  update_account_shared: '变更账号共享状态',
  admin_claim_account: '管理员认领账号',
  admin_update_user: '管理员修改用户',
  admin_delete_user: '管理员删除用户',
  admin_add_user: '管理员创建用户',
  admin_update_account: '管理员修改账号',
  change_email: '修改邮箱',
  confirm_email: '验证邮箱',
  forgot_password: '忘记密码',
  reset_password: '重置密码',
  mpay_email_login: '邮箱登录游戏',
  mpay_phone_login: '手机登录游戏',
  guest_account: '游客账号创建',
  forgot_password_sent: '发送重置密码邮件',
  change_email_sent: '发送改邮箱验证',
  email_changed: '邮箱已修改',
  merge_account: '合并账号到共享池',
  update_account: '更新账号设置',
  share_account: '设为共享',
  unshare_account: '收回共享',
  admin_nuts: '调整板栗',
  growth_override: '等级覆盖',
  admin_adjust: '管理员调整',
  admin_add_preset: '添加实名预设',
  activate: '账号激活',
  admin_add_account: '管理员添加账号',
  admin_gen_codes: '管理员生成激活码',
  admin_toggle_preset: '管理员切换预设',
  create_payment_order: '创建支付订单',
  redeem_code: '兑换激活码',
};

const targetText: Record<string, string> = { web: '网页后台', email: '邮箱', web_user: '网页用户', game_account: '游戏账号' };

// 可编辑的消息配置项(key + 中文说明),供"消息配置"分区展示与编辑。
const MSG_META: { key: string; label: string; desc: string }[] = [
  { key: 'unauthorized', label: '未登录', desc: '未登录或登录态失效时提示' },
  { key: 'method_not_allowed', label: '请求方式错误', desc: '请求方法不对(POST/GET等)' },
  { key: 'too_frequent', label: '操作太频繁', desc: '触发限流冷却时提示' },
  { key: 'invalid_json', label: '提交格式错误', desc: '请求数据不是合法 JSON' },
  { key: 'invalid_id', label: '编号无效', desc: '传入了无效的 ID' },
  { key: 'param_error', label: '参数错误', desc: '提交的参数有误' },
  { key: 'missing_id', label: '缺少必要信息', desc: '缺少必要的参数' },
  { key: 'system_error', label: '系统错误', desc: '内部错误/异常的兜底' },
  { key: 'account_not_found', label: '账号不存在', desc: '找不到对应的账号' },
  { key: 'no_active_account', label: '无活跃账号', desc: '当前没有可用的活跃账号' },
  { key: 'no_cookie', label: '无 Cookie', desc: '账号未绑定 Cookie' },
];

function labelAction(action: string) { return actionText[action] || action || '未知操作'; }
function labelTarget(log: any) { if (log.action === 'join_server' || log.action === 'join_server_failed') return log.target || log.detail || '未知服务器'; return targetText[log.target] || log.target || '-'; }

function formatBeijingTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date).replace(/\//g, '-');
}

function userLabel(log: any) {
  if (log.username) return log.user_id ? `${log.username} (#${log.user_id})` : log.username;
  return log.user_id ? `#${log.user_id}` : '-';
}
function accountLabel(log: any) {
  if (log.account_name) return log.account_id ? `${log.account_name} (#${log.account_id})` : log.account_name;
  return log.account_id ? `#${log.account_id}` : '-';
}

function levelClass(level: string) {
  const l = (level || '').toUpperCase();
  if (l === 'ERROR' || l === 'FATAL') return 'log-level error';
  if (l === 'WARN' || l === 'WARNING') return 'log-level warn';
  return 'log-level info';
}

export default function AdminPage() {
  const [users, setUsers] = useState<any[]>([]);
  const [accounts, setAccounts] = useState<any[]>([]);
  const [auditLogs, setAuditLogs] = useState<any[]>([]);
  const [systemLogs, setSystemLogs] = useState<any[]>([]);
  const [presets, setPresets] = useState<any[]>([]);
  const [tab, setTab] = useState<'users' | 'accounts' | 'audit' | 'system' | 'presets' | 'apistats' | 'payorders' | 'toolbox' | 'surveys' | 'messages' | 'market'>('users');
  const [msgConfig, setMsgConfig] = useState<Record<string, string>>({});
  const [msgDirty, setMsgDirty] = useState(false);
  const [editingUser, setEditingUser] = useState<any>(null);
  const [editEmail, setEditEmail] = useState("");
  const [editUsername, setEditUsername] = useState("");
  const [editPassword, setEditPassword] = useState("");
  const [editMsg, setEditMsg] = useState("");
  const [addEmail, setAddEmail] = useState("");
  const [addUser, setAddUser] = useState("");
  const [addPass, setAddPass] = useState("");
  const [addMsg, setAddMsg] = useState("");
  const [modal, setModal] = useState<{open:boolean;title:string;message:string;detail?:string;onConfirm?:()=>void;variant?:'info'|'danger'|'warn'}>({open:false,title:'',message:''});
  const [auditHasMore, setAuditHasMore] = useState(true);
  const [systemHasMore, setSystemHasMore] = useState(true);
  const [auditTotal, setAuditTotal] = useState(0);
  const [systemTotal, setSystemTotal] = useState(0);
  const [auditPage, setAuditPage] = useState(0);
  const [systemPage, setSystemPage] = useState(0);
  const [auditKeyword, setAuditKeyword] = useState('');
  const [auditAction, setAuditAction] = useState('');
  const [auditDateFrom, setAuditDateFrom] = useState('');
  const [auditDateTo, setAuditDateTo] = useState('');
  const [systemKeyword, setSystemKeyword] = useState('');
  const [systemLevel, setSystemLevel] = useState('');
  const [systemDateFrom, setSystemDateFrom] = useState('');
  const [systemDateTo, setSystemDateTo] = useState('');
  const [presetName, setPresetName] = useState("");
  const [presetIDNum, setPresetIDNum] = useState("");
  const [presetMsg, setPresetMsg] = useState("");
  const [skinPresets, setSkinPresets] = useState<any[]>([]);
  const [skinPresetName, setSkinPresetName] = useState("");
  const [skinPresetID, setSkinPresetID] = useState("");
  const [skinPresetMsg, setSkinPresetMsg] = useState("");
  const [editingSkinPreset, setEditingSkinPreset] = useState<number | null>(null);
  const [editSkinName, setEditSkinName] = useState("");
  const [editSkinItemID, setEditSkinItemID] = useState("");
  const [actAmount, setActAmount] = useState("10");
  const [actCodes, setActCodes] = useState<any[]>([]);
  const [rpTotal, setRpTotal] = useState("100");
  const [rpCount, setRpCount] = useState("10");
  const [rpCode, setRpCode] = useState("");
  const [userStats, setUserStats] = useState<any[]>([]);
  const [payOrders, setPayOrders] = useState<any[]>([]);
  const [payOrdersLoading, setPayOrdersLoading] = useState(false);
  const [versionCfg, setVersionCfg] = useState<any>({});
  const [announcements, setAnnouncements] = useState<any[]>([]);
  const [signatures, setSignatures] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [userSearch, setUserSearch] = useState("");
  const [accountSearch, setAccountSearch] = useState("");
  const [accountStatusFilter, setAccountStatusFilter] = useState("");
  const PAGE = 50;

  const fetchAll = async () => {
    const [u, a] = await Promise.all([adminUsers(), adminAccounts()]);
    if ((u as any).ok) setUsers((u as any).users || []);
    if ((a as any).ok) setAccounts((a as any).accounts || []);
    const pData = await getAllRealnamePresets();
    if ((pData as any).ok) setPresets((pData as any).presets || []);
    const spData = await getSkinPresets();
    if ((spData as any).ok) setSkinPresets((spData as any).presets || []);
    const st = await getAdminPerUserStats(87600); // all-time for cards
    if ((st as any).ok) setUserStats((st as any).users || []);
    setLoading(false);
  };

  const fetchAuditLogs = async (page = 0) => {
    const to = auditDateTo ? auditDateTo + 'T23:59:59Z' : undefined;
    const al = await adminAuditLogs(PAGE, page * PAGE, auditKeyword || undefined, auditAction || undefined, auditDateFrom || undefined, to);
    if ((al as any).ok) { setAuditLogs((al as any).logs || []); setAuditPage(page); setAuditHasMore(((al as any).logs || []).length >= PAGE); setAuditTotal((al as any).total || 0); }
  };
  const fetchSystemLogs = async (page = 0) => {
    const to = systemDateTo ? systemDateTo + 'T23:59:59Z' : undefined;
    const sl = await adminSystemLogs(PAGE, page * PAGE, systemKeyword || undefined, systemLevel || undefined, systemDateFrom || undefined, to);
    if ((sl as any).ok) { setSystemLogs((sl as any).logs || []); setSystemPage(page); setSystemHasMore(((sl as any).logs || []).length >= PAGE); setSystemTotal((sl as any).total || 0); }
  };

  // ── Survey state ──
  const [surveyList, setSurveyList] = useState<any[]>([]);
  const [surveyFormMode, setSurveyFormMode] = useState<'list' | 'create' | 'edit' | 'stats' | 'user-detail'>('list');
  const [currentSurvey, setCurrentSurvey] = useState<any>(null);
  const [surveyTitle, setSurveyTitle] = useState('');
  const [surveyReward, setSurveyReward] = useState(5);
  const [surveyQuestions, setSurveyQuestions] = useState<any[]>([]);
  const [surveyMsg, setSurveyMsg] = useState('');
  const [surveyMsgOk, setSurveyMsgOk] = useState(true);
  const [statsData, setStatsData] = useState<any>(null);
  const clearAllAnswers = async (surveyId: number) => {
    if (!confirm('确定清除该问卷的所有答题记录吗？已答用户将被标记为未答。')) return;
    const r: any = await adminClearSurveyAnswers(surveyId);
    if (r.ok) { showToast(`已清除 ${r.cleared} 条答题记录`, 'ok'); fetchSurveys(); }
    else showToast(r.error || '糟糕,操作没成功,请稍后重试~', 'err');
  };
  const [statsUserAnswers, setStatsUserAnswers] = useState<any>(null);
  const [statsUserName, setStatsUserName] = useState('');

  const fetchSurveys = async () => {
    const r: any = await adminListSurveys();
    if (r.ok) setSurveyList(r.surveys || []);
  };

  useEffect(() => { fetchAll(); }, []);
  useEffect(() => { if (tab === 'audit') fetchAuditLogs(0); }, [tab]);
  useEffect(() => { if (tab === 'system') fetchSystemLogs(0); }, [tab]);

  const fetchMessages = async () => {
    const r: any = await adminGetMessages();
    if (r && r.ok) {
      const merged: Record<string, string> = {};
      MSG_META.forEach(m => { merged[m.key] = (r.messages || {})[m.key] || ''; });
      setMsgConfig(merged);
      setMsgDirty(false);
    }
  };
  const saveMessages = async () => {
    const r: any = await adminSetMessages(msgConfig);
    if (r?.ok) { showToast('恭喜,消息配置已保存并立即生效~', 'ok'); setMsgDirty(false); }
    else showToast(r?.error || '糟糕,保存失败了,请稍后重试~', 'err');
  };

  const fetchPayOrders = async () => {
    setPayOrdersLoading(true);
    const r: any = await adminPaymentOrders(100, 0);
    if (r.ok) setPayOrders(r.orders || []);
    setPayOrdersLoading(false);
  };

  const fetchToolbox = async () => {
    const [v, a, s] = await Promise.all([getVersionConfig(), getAnnouncements(), getSignatures()]);
    if ((v as any).ok) setVersionCfg((v as any).config || {});
    if ((a as any).ok) setAnnouncements((a as any).announcements || []);
    if ((s as any).ok) setSignatures((s as any).signatures || []);
  };

  const showConfirm = (title:string, msg:string, onOk:()=>void, v:'info'|'danger'='danger') => setModal({open:true,title,message:msg,onConfirm:onOk,variant:v});
  const userStat = (uid: number) => userStats.find((s: any) => s.user_id === uid);
  const removeUser = (u: any) => { showConfirm('确认删除', '确定删除网页用户 ' + (u.username || u.email) + '？', async () => { setModal({open:false,title:'',message:''}); await adminDeleteUser(u.id); fetchAll(); }); };
  const toggleAccountDisabled = async (a: any) => { await adminUpdateAccount(a.id, { disabled: !a.disabled }); fetchAll(); };
  const toggleAccountShared = async (a: any) => { await adminUpdateAccount(a.id, { shared: !!a.owner_id, owner_id: a.owner_id }); fetchAll(); };
  const filteredUsers = users.filter((u: any) => {
    if (!userSearch) return true;
    const q = userSearch.toLowerCase();
    return (u.username || '').toLowerCase().includes(q) || (u.email || '').toLowerCase().includes(q) || String(u.id) === q;
  });
  const filteredAccounts = accounts.filter((a: any) => {
    if (!accountSearch && !accountStatusFilter) return true;
    const q = accountSearch.toLowerCase();
    const matchSearch = !accountSearch || (a.display_name || '').toLowerCase().includes(q) || (a.uid || '').toLowerCase().includes(q) || String(a.id) === q;
    const matchStatus = !accountStatusFilter || (a.status || '') === accountStatusFilter;
    return matchSearch && matchStatus;
  });

  if (loading && users.length === 0) {
    return (
      <div className="page-stack">
        <section className="hero-panel compact">
          <div>
            <div className="skeleton-line" style={{width:80,height:12,marginBottom:8}} />
            <div className="skeleton-line" style={{width:160,height:22,marginBottom:8}} />
            <div className="skeleton-line" style={{width:260,height:14}} />
          </div>
          <div className="skeleton-line" style={{width:80,height:28,borderRadius:14}} />
        </section>
        <div className="skeleton-line" style={{height:34,borderRadius:10,marginBottom:16}} />
        <section className="card">
          <div className="skeleton-line" style={{width:120,height:18,marginBottom:14}} />
          <div className="skeleton-line" style={{width:'100%',height:14,marginBottom:6}} />
          <div className="skeleton-line" style={{width:'100%',height:14,marginBottom:6}} />
          <div className="skeleton-line" style={{width:'60%',height:14}} />
        </section>
      </div>
    );
  }

  return (
    <div className="page-stack">
      <section className="hero-panel compact">
        <div>
          <div className="eyebrow">后台工作台</div>
          <h1>后台管理</h1>
          <p>网页用户、G79 游戏账号、总日志和详细日志分区管理。</p>
        </div>
        <button className="btn btn-sm btn-outline" onClick={fetchAll}>刷新全部</button>
      </section>

      <div className="tab-strip">
        <button className={tab === 'users' ? 'active' : ''} onClick={() => setTab('users')}>网页用户</button>
        <button className={tab === 'accounts' ? 'active' : ''} onClick={() => setTab('accounts')}>G79账号</button>
        <button className={tab === 'audit' ? 'active' : ''} onClick={() => { setTab('audit'); setAuditPage(0); setAuditHasMore(true); fetchAuditLogs(0); }}>总日志</button>
        <button className={tab === 'system' ? 'active' : ''} onClick={() => { setTab('system'); setSystemPage(0); setSystemHasMore(true); fetchSystemLogs(0); }}>详细日志</button>
        <button className={tab === 'presets' ? 'active' : ''} onClick={() => setTab('presets')}>预设列表</button>
        <button className={tab === 'apistats' ? 'active' : ''} onClick={() => setTab('apistats')}>API 统计</button>
        <button className={tab === 'payorders' ? 'active' : ''} onClick={() => { setTab('payorders'); fetchPayOrders(); }}>支付订单</button>
        <button className={tab === 'toolbox' ? 'active' : ''} onClick={() => { setTab('toolbox'); fetchToolbox(); }}>工具箱</button>
        <button className={tab === 'surveys' ? 'active' : ''} onClick={() => { setTab('surveys'); fetchSurveys(); }}>问卷调查</button>
        <button className={tab === 'messages' ? 'active' : ''} onClick={() => { setTab('messages'); fetchMessages(); }}>消息配置</button>
        <button className={tab === 'market' ? 'active' : ''} onClick={() => setTab('market')}>文件市场</button>
      </div>

      {tab === 'users' && (<>
        <section className="card" style={{marginBottom:16}}>
          <h3 style={{margin:"0 0 10px"}}>创建网页用户</h3>
          <div style={{display:"flex",flexDirection:"column",gap:8}}>
            <input className="input" placeholder="邮箱" value={addEmail} onChange={e => setAddEmail(e.target.value)} />
            <div style={{display:"flex",gap:8}}>
              <input className="input" placeholder="用户名" value={addUser} onChange={e => setAddUser(e.target.value)} style={{flex:1}} />
              <input className="input" type="password" placeholder="密码" value={addPass} onChange={e => setAddPass(e.target.value)} style={{flex:1}} />
            </div>
            {addMsg && <div className={"result-box " + (addMsg.startsWith("ok:") ? "ok" : "err")}>{addMsg.replace(/^ok:/, "")}</div>}
            <div><button className="btn btn-primary" onClick={async () => {
              if (!addEmail || !addUser || !addPass) { setAddMsg("err:请填写完整"); return; }
              setAddMsg("");
              const res = await adminAddUser(addEmail, addUser, addPass);
              if ((res as any).ok) { setAddEmail(""); setAddUser(""); setAddPass(""); setAddMsg("ok:用户已创建"); fetchAll(); }
              else { setAddMsg("err:" + ((res as any).error || "创建失败")); }
            }}>创建用户</button></div>
          </div>
        </section>
        <section className="card" style={{marginBottom:16}}>
          <h3 style={{margin:"0 0 10px"}}>板栗激活码</h3>
          <div style={{display:"flex",gap:8,marginBottom:10}}>
            <input className="input" type="number" placeholder="板栗数量(默认10)" value={actAmount} onChange={e => setActAmount(e.target.value)} style={{width:150}} />
            <button className="btn btn-primary btn-sm" onClick={async () => {
              const a = parseInt(actAmount) || 10;
              const res = await fetch('/api/admin/activation-codes', {
                method:'POST', headers:{'Content-Type':'application/json', Authorization:'Bearer '+localStorage.getItem('session_token')},
                body: JSON.stringify({amount:a})
              }).then(r=>r.json());
              if (res.ok) { setActCodes(res.codes||[]); }
            }}>生成激活码</button>
            <button className="btn btn-sm btn-outline" onClick={async () => {
              const res = await fetch('/api/admin/activation-codes', {
                headers:{ Authorization:'Bearer '+localStorage.getItem('session_token')}
              }).then(r=>r.json());
              if (res.ok) setActCodes(res.codes||[]);
            }}>刷新</button>
          </div>
          {actCodes.length > 0 && (
            <div style={{display:"flex",flexDirection:"column",gap:4}}>
              {actCodes.map((c: any) => (
                <div key={c.id||c.code} style={{display:"flex",justifyContent:"space-between",padding:"4px 8px",background:'var(--phx-bg)',borderRadius:6,fontSize:13,fontFamily:'monospace'}}>
                  <span>{c.code}</span>
                  <span style={{color:'var(--phx-text-secondary)'}}>{c.amount} 🌰</span>
                </div>
              ))}
            </div>
          )}
        </section>
        <section className="card" style={{marginBottom:16}}>
          <h3 style={{margin:"0 0 10px"}}>红包</h3>
          <div style={{display:"flex",gap:8,marginBottom:10}}>
            <input className="input" type="number" placeholder="总积分" value={rpTotal} onChange={e => setRpTotal(e.target.value)} style={{width:120}} />
            <input className="input" type="number" placeholder="份数" value={rpCount} onChange={e => setRpCount(e.target.value)} style={{width:100}} />
            <button className="btn btn-primary btn-sm" onClick={async () => {
              const t = parseInt(rpTotal) || 0;
              const c = parseInt(rpCount) || 0;
              const res: any = await adminCreateRedPacket(t, c);
              if (res.ok) { setRpCode(res.code || ""); showToast(res.message || "红包已创建", "ok"); }
              else showToast(res.error || "创建失败", "err");
            }}>创建红包</button>
          </div>
          {rpCode && (
            <div style={{display:"flex",justifyContent:"space-between",padding:"4px 8px",background:'var(--phx-bg)',borderRadius:6,fontSize:13,fontFamily:'monospace'}}>
              <span>{rpCode}</span>
              <button className="btn btn-sm btn-outline" style={{fontSize:11,padding:"2px 8px",minWidth:0}} onClick={() => { navigator.clipboard.writeText(rpCode); showToast("已复制","ok"); }}>复制</button>
            </div>
          )}
        </section>
        <section className="card" style={{marginBottom:16}}>
          <div style={{display:"flex",gap:8,alignItems:"center"}}>
            <input className="input" placeholder="搜索用户（用户名/邮箱/ID）" value={userSearch} onChange={e => setUserSearch(e.target.value)} style={{flex:1}} />
            <span style={{fontSize:13,color:"var(--phx-text-secondary)",whiteSpace:"nowrap"}}>共 {filteredUsers.length} / {users.length} 人</span>
          </div>
        </section>
        <section className="admin-grid">
          {filteredUsers.map((u) => (
          <article key={u.id} className="admin-card">
            <div className="admin-title"><strong>#{u.id} {u.username}</strong><span className={u.disabled ? 'status-pill danger' : 'status-pill'}>{u.disabled ? '已停用' : '正常'}</span></div>
            <p>{u.email}</p>
            <p>角色：{u.role === 'admin' ? '管理员' : '用户'} · 验证：{u.verified ? '已验证' : '未验证'}</p>
            <p>活跃：{u.active_account_name ? `${u.active_account_name} (${u.active_account_uid || '-'})` : '未选择'}</p>
            <p className="mono-line">{u.token}</p>
            <p>板栗：{(u.nuts_balance ?? 0)} 🌰 <button className="btn btn-sm btn-outline" style={{marginLeft:8}} onClick={() => {
              const amt = parseInt(prompt('调整数量（正数增加，负数扣除）:') || '0');
              if (!amt) return;
              const rsn = prompt('原因:') || 'admin_adjust';
              adminAdjustNuts(u.id, amt, rsn).then(() => fetchAll());
            }}>调整</button></p>
            <p>订阅：{(() => {
              const until = (u as any).subscription_until;
              const active = !!until && new Date(until).getTime() > Date.now();
              const grantBtn = (
                <button className="btn btn-sm btn-outline" style={{marginLeft:8}} onClick={() => {
                  const d = parseInt(prompt('授予订阅天数:') || '0');
                  if (!d || d <= 0) return;
                  adminGrantSubscription(u.id, d).then(() => fetchAll());
                }}>授予</button>
              );
              if (!active) return <span>未订阅{grantBtn}</span>;
              const start = (u as any).subscription_start;
              const total = new Date(until).getTime() - (start ? new Date(start).getTime() : Date.now());
              const remain = new Date(until).getTime() - Date.now();
              const days = Math.ceil(remain / 86400000);
              const pct = total > 0 ? Math.max(0, Math.min(100, remain / total * 100)) : 100;
              return (
                <span>
                  <span style={{fontSize:12}}>订阅中 · 剩 {days} 天</span>
                  {grantBtn}
                  <button className="btn btn-sm btn-warning" style={{marginLeft:6}} onClick={() => { adminRevokeSubscription(u.id).then(() => fetchAll()); }}>撤销</button>
                  <div style={{height:6,background:'var(--phx-warm-border)',borderRadius:3,marginTop:4,overflow:'hidden'}}>
                    <div style={{height:'100%',width:pct+'%',background:'var(--phx-accent, #4c9aff)'}} />
                  </div>
                </span>
              );
            })()}</p>
            {(() => { const s = userStat(u.id); return s ? <p style={{fontSize:12}}>API: {s.total_calls}次 · 成功: {s.success} · 进服: {s.join_count}次 · 成功率: {s.success_rate?.toFixed(1)}%</p> : null; })()}
            <div className="account-actions">
              <button className="btn btn-sm btn-outline" onClick={() => setEditingUser(editingUser?.id === u.id ? null : u)}>{editingUser?.id === u.id ? '关闭' : '编辑'}</button>
              <button className="btn btn-sm btn-warning" onClick={() => { adminUpdateUser(u.id, { disabled: !u.disabled }); fetchAll(); }}>{u.disabled ? '启用' : '停用'}</button>
              <button className="btn btn-sm btn-danger" onClick={() => removeUser(u)}>删除</button>
            </div>
            {editingUser?.id === u.id && (
              <div className="edit-drawer" style={{marginTop:10}}>
                <input className="input" placeholder="新邮箱" value={editEmail} onChange={e => setEditEmail(e.target.value)} />
                <input className="input" placeholder="新用户名" value={editUsername} onChange={e => setEditUsername(e.target.value)} style={{marginTop:6}} />
                <input className="input" type="password" placeholder="新密码（留空不修改）" value={editPassword} onChange={e => setEditPassword(e.target.value)} style={{marginTop:6}} />
                {editMsg && <div className={"result-box " + (editMsg.startsWith("ok:") ? "ok" : "err")} style={{marginTop:6}}>{editMsg.replace(/^ok:/, "")}</div>}
                <div className="inline-actions" style={{marginTop:6}}>
                  <button className="btn btn-primary" onClick={async () => {
                    setEditMsg("");
                    const data: any = {};
                    if (editEmail) data.email = editEmail;
                    if (editUsername) data.username = editUsername;
                    if (editPassword) data.password = editPassword;
                    if (!data.email && !data.username && !data.password) { setEditMsg("err:请至少填写一项"); return; }
                    const res = await adminUpdateUser(u.id, data);
                    if ((res as any).ok) { setEditEmail(""); setEditUsername(""); setEditPassword(""); setEditMsg("ok:保存成功"); fetchAll(); }
                    else { setEditMsg("err:" + ((res as any).error || "失败")); }
                  }}>保存</button>
                  <button className="btn btn-outline" onClick={() => { setEditingUser(null); setEditEmail(""); setEditUsername(""); setEditPassword(""); setEditMsg(""); }}>取消</button>
                </div>
              </div>
            )}
          </article>
          ))}
        </section>
        </>
      )}

      {tab === 'accounts' && (<>
        <section className="card" style={{marginBottom:16}}>
          <div style={{display:"flex",gap:8,alignItems:"center",flexWrap:"wrap"}}>
            <input className="input" placeholder="搜索账号（名称/UID/ID）" value={accountSearch} onChange={e => setAccountSearch(e.target.value)} style={{flex:1,minWidth:160}} />
            <select className="input" style={{width:'auto',minWidth:100}} value={accountStatusFilter} onChange={e => setAccountStatusFilter(e.target.value)}>
              <option value="">全部状态</option>
              <option value="normal">normal</option>
              <option value="offline">offline</option>
              <option value="banned">banned</option>
              <option value="unknown">unknown</option>
            </select>
            <span style={{fontSize:13,color:"var(--phx-text-secondary)",whiteSpace:"nowrap"}}>共 {filteredAccounts.length} / {accounts.length} 个</span>
          </div>
        </section>
        <section className="admin-grid">
          {filteredAccounts.map((a) => (
            <article key={a.id} className="admin-card">
              <div className="admin-title"><strong>#{a.id} {a.display_name || '未知游戏账号'}</strong><span className={'status-pill' + (a.disabled || a.status === 'banned' ? ' danger' : a.status === 'offline' ? ' warn' : '')} style={a.status === 'offline' ? {color:'#d6a21d'} : a.status === 'unknown' ? {color:'#9f927d'} : {}}>{a.disabled ? '已停用' : a.status}</span></div>
              <p>UID：{a.uid || '-'} · 所属：{a.owner_id || '共享池'} · 来源：{a.source || '-'}</p>
              <p>等级：{a.growth_level || '0'} · 皮肤：{a.skin_number || '默认'}</p>
              <div className="account-actions">
                <button className="btn btn-sm btn-outline" onClick={() => setModal({open:true,title:`账号 #${a.id}`,message:a.display_name || '未知',detail:JSON.stringify(a,null,2)})}>详情</button>
                <button className="btn btn-sm btn-primary" onClick={() => toggleAccountShared(a)}>{a.owner_id ? '设为共享' : '取消共享'}</button>
                <button className="btn btn-sm btn-warning" onClick={() => toggleAccountDisabled(a)}>{a.disabled ? '启用' : '停用'}</button>
                <button className="btn btn-sm btn-danger" onClick={() => showConfirm('删除账号', '确定删除此账号？', async () => { setModal({open:false,title:'',message:''}); await fetch('/api/admin/accounts/del', { method:'POST', body: JSON.stringify({id: a.id}), headers:{ Authorization:'Bearer ' + localStorage.getItem('session_token'), 'Content-Type': 'application/json' } }).then(() => fetchAll()); })}>删除账号</button>
                <button className="btn btn-sm btn-outline" onClick={() => { navigator.clipboard.writeText((a as any).cookie_data || ''); showToast('已复制','ok'); }}>复制Cookie</button>
              </div>
            </article>
          ))}
        </section>
        </>
      )}
      {tab === 'audit' && (
        <>
          <section className="card" style={{marginBottom:16}}>
            <div className="log-search-bar" style={{display:'flex',gap:8,flexWrap:'wrap',alignItems:'center'}}>
              <input className="input" placeholder="搜索关键词" value={auditKeyword} onChange={e => setAuditKeyword(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') fetchAuditLogs(0); }} style={{flex:1,minWidth:120}} />
              <input className="input" placeholder="动作筛选" value={auditAction} onChange={e => setAuditAction(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') fetchAuditLogs(0); }} style={{width:120}} />
              <input className="input" type="date" value={auditDateFrom} onChange={e => setAuditDateFrom(e.target.value)} style={{width:140}} />
              <input className="input" type="date" value={auditDateTo} onChange={e => setAuditDateTo(e.target.value)} style={{width:140}} />
              <button className="btn btn-sm btn-primary" onClick={() => fetchAuditLogs(0)}>搜索</button>
              <button className="btn btn-sm btn-outline" onClick={() => { setAuditKeyword(''); setAuditAction(''); setAuditDateFrom(''); setAuditDateTo(''); fetchAuditLogs(0); }}>重置</button>
            </div>
          </section>
          <LogList logs={auditLogs} kind="audit" hasMore={auditHasMore} total={auditTotal} page={auditPage} pageSize={PAGE} onPage={fetchAuditLogs} />
        </>
      )}
      {tab === 'system' && (
        <>
          <section className="card" style={{marginBottom:16}}>
            <div className="log-search-bar" style={{display:'flex',gap:8,flexWrap:'wrap',alignItems:'center'}}>
              <input className="input" placeholder="搜索关键词" value={systemKeyword} onChange={e => setSystemKeyword(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') fetchSystemLogs(0); }} style={{flex:1,minWidth:120}} />
              <select className="input" style={{width:100}} value={systemLevel} onChange={e => setSystemLevel(e.target.value)}>
                <option value="">全部级别</option>
                <option value="info">INFO</option>
                <option value="warn">WARN</option>
                <option value="error">ERROR</option>
              </select>
              <input className="input" type="date" value={systemDateFrom} onChange={e => setSystemDateFrom(e.target.value)} style={{width:140}} />
              <input className="input" type="date" value={systemDateTo} onChange={e => setSystemDateTo(e.target.value)} style={{width:140}} />
              <button className="btn btn-sm btn-primary" onClick={() => fetchSystemLogs(0)}>搜索</button>
              <button className="btn btn-sm btn-outline" onClick={() => { setSystemKeyword(''); setSystemLevel(''); setSystemDateFrom(''); setSystemDateTo(''); fetchSystemLogs(0); }}>重置</button>
            </div>
          </section>
          <LogList logs={systemLogs} kind="system" hasMore={systemHasMore} total={systemTotal} page={systemPage} pageSize={PAGE} onPage={fetchSystemLogs} />
        </>
      )}
      {tab === 'presets' && (
        <>
        <section className="card">
          <h3 style={{margin:"0 0 10px"}}>实名预设</h3>
          <div style={{display:"flex",flexDirection:"column",gap:8,marginBottom:16}}>
            <div style={{display:"flex",gap:8}}>
              <input className="input" placeholder="姓名" value={presetName} onChange={e => setPresetName(e.target.value)} style={{flex:1}} />
              <input className="input" placeholder="身份证号" value={presetIDNum} onChange={e => setPresetIDNum(e.target.value)} style={{flex:2}} />
            </div>
            {presetMsg && <div className={"result-box " + (presetMsg.startsWith("ok:") ? "ok" : "err")}>{presetMsg.replace(/^ok:/, "")}</div>}
            <div><button className="btn btn-primary" onClick={async () => {
              if (!presetName || !presetIDNum) { setPresetMsg("err:请填写完整"); return; }
              setPresetMsg("");
              const res = await addRealnamePreset(presetName, presetIDNum);
              if ((res as any).ok) { setPresetName(""); setPresetIDNum(""); setPresetMsg("ok:已添加"); fetchAll(); }
              else { setPresetMsg("err:" + ((res as any).error || "添加失败")); }
            }}>添加预设</button></div>
          </div>
          {presets.length === 0 ? <div className="empty-state">暂无预设</div> : (
            <div style={{display:"flex",flexDirection:"column",gap:8}}>
              {presets.map((p: any) => (
                <div key={p.id} className="admin-card" style={{display:"flex",justifyContent:"space-between",alignItems:"center",padding:"10px 14px"}}>
                  <div><strong>{p.name}</strong><span style={{marginLeft:12,color:"#8b7a61",fontSize:13}}>{p.id_number}</span><span className={"status-pill " + (p.enabled ? "" : "danger")} style={{marginLeft:8,fontSize:11}}>{p.enabled ? '启用' : '停用'}</span></div>
                  <div style={{display:"flex",gap:6}}>
                    <button className={"btn btn-sm " + (p.enabled ? "btn-warning" : "btn-primary")} onClick={() => { toggleRealnamePreset(p.id, !p.enabled); fetchAll(); }}>{p.enabled ? '停用' : '启用'}</button>
                    <button className="btn btn-sm btn-danger" onClick={() => showConfirm('确认删除', '确定删除此预设？', async () => { setModal({open:false,title:'',message:''}); await deleteRealnamePreset(p.id); fetchAll(); })}>删除</button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
        <section className="card">
          <h3 style={{margin:"0 0 10px"}}>皮肤预设</h3>
          <div style={{display:"flex",flexDirection:"column",gap:8,marginBottom:16}}>
            <div style={{display:"flex",gap:8}}>
              <input className="input" placeholder="预设名称" value={skinPresetName} onChange={e => setSkinPresetName(e.target.value)} style={{flex:1}} />
              <input className="input" placeholder="皮肤组件 ID（19位）" value={skinPresetID} onChange={e => setSkinPresetID(e.target.value)} style={{flex:2}} />
            </div>
            {skinPresetMsg && <div className={"result-box " + (skinPresetMsg.startsWith("ok:") ? "ok" : "err")}>{skinPresetMsg.replace(/^ok:/, "")}</div>}
            <div><button className="btn btn-primary" onClick={async () => {
              if (!skinPresetName || !skinPresetID) { setSkinPresetMsg("err:请填写完整"); return; }
              if (!/^\d{10,}$/.test(skinPresetID.trim())) { setSkinPresetMsg("err:组件 ID 需为 10 位以上数字"); return; }
              setSkinPresetMsg("");
              const res = await addSkinPreset(skinPresetName.trim(), skinPresetID.trim());
              if ((res as any).ok) { setSkinPresetName(""); setSkinPresetID(""); setSkinPresetMsg("ok:已添加"); fetchAll(); }
              else { setSkinPresetMsg("err:" + ((res as any).error || "添加失败")); }
            }}>添加预设</button></div>
          </div>
          {skinPresets.length === 0 ? <div className="empty-state">暂无皮肤预设</div> : (
            <div style={{display:"grid",gridTemplateColumns:"repeat(auto-fill, minmax(200px, 1fr))",gap:10}}>
              {skinPresets.map((p: any) => (
                <div key={p.id} className="admin-card" style={{padding:10}}>
                  {editingSkinPreset === p.id ? (
                    <div style={{display:"flex",flexDirection:"column",gap:6}}>
                      <input className="input" value={editSkinName} onChange={e => setEditSkinName(e.target.value)} placeholder="名称" style={{fontSize:12}} />
                      <input className="input" value={editSkinItemID} onChange={e => setEditSkinItemID(e.target.value)} placeholder="组件 ID" style={{fontSize:12}} />
                      <div style={{display:"flex",gap:4}}>
                        <button className="btn btn-sm btn-primary" onClick={async () => {
                          if (!editSkinName || !editSkinItemID) return;
                          const res = await updateSkinPreset(p.id, editSkinName.trim(), editSkinItemID.trim());
                          if ((res as any).ok) { setEditingSkinPreset(null); fetchAll(); }
                        }}>保存</button>
                        <button className="btn btn-sm btn-outline" onClick={() => setEditingSkinPreset(null)}>取消</button>
                      </div>
                    </div>
                  ) : (
                    <>
                      <div style={{display:"flex",alignItems:"center",gap:10,marginBottom:6}}>
                        <div style={{width:44,height:44,flexShrink:0,borderRadius:8,overflow:"hidden",background:"#2b2118"}}>
                          {p.preview_url ? <SkinHead skinUrl={p.preview_url} size={40} /> : <div style={{display:"flex",alignItems:"center",justifyContent:"center",width:"100%",height:"100%",fontSize:18}}>👕</div>}
                        </div>
                        <div style={{flex:1,minWidth:0}}>
                          <div style={{fontWeight:600,fontSize:13}}>{p.name}</div>
                          <div style={{fontSize:10,color:"#8b7a61",wordBreak:"break-all"}}>{p.item_id}</div>
                        </div>
                      </div>
                      <div style={{display:"flex",gap:4}}>
                        <button className="btn btn-sm btn-outline" onClick={() => { setEditingSkinPreset(p.id); setEditSkinName(p.name); setEditSkinItemID(p.item_id); }}>编辑</button>
                        <button className="btn btn-sm btn-danger" onClick={() => showConfirm('确认删除', '确定删除此皮肤预设？', async () => { setModal({open:false,title:'',message:''}); await deleteSkinPreset(p.id); fetchAll(); })}>删除</button>
                      </div>
                    </>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
        </>
      )}
      {tab === 'apistats' && <AdminStatsPanel />}
      {tab === 'payorders' && (
        <section className="card" style={{marginBottom:16}}>
          <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:10}}>
            <h3 style={{margin:0}}>支付订单</h3>
            <button className="btn btn-sm btn-outline" onClick={fetchPayOrders} disabled={payOrdersLoading}>刷新</button>
          </div>
          {payOrdersLoading ? <div className="empty-state">加载中...</div> :
           payOrders.length === 0 ? <div className="empty-state">暂无支付订单</div> : (
            <div style={{overflowX:'auto'}}>
              <table className="log-list" style={{width:'100%',fontSize:13}}>
                <thead><tr style={{borderBottom:'2px solid var(--phx-border)',textAlign:'left'}}>
                  <th style={{padding:'6px 8px'}}>ID</th><th style={{padding:'6px 8px'}}>订单号</th><th style={{padding:'6px 8px'}}>用户</th><th style={{padding:'6px 8px'}}>金额</th><th style={{padding:'6px 8px'}}>板栗</th><th style={{padding:'6px 8px'}}>支付方式</th><th style={{padding:'6px 8px'}}>状态</th><th style={{padding:'6px 8px'}}>时间</th><th style={{padding:'6px 8px'}}>操作</th>
                </tr></thead>
                <tbody>
                  {payOrders.map((o: any) => (
                    <tr key={o.id} style={{borderBottom:'1px solid var(--phx-border)'}}>
                      <td style={{padding:'6px 8px'}}>{o.id}</td>
                      <td style={{padding:'6px 8px',fontSize:11,fontFamily:'monospace',maxWidth:140,overflow:'hidden',textOverflow:'ellipsis'}} title={o.order_no}>{o.order_no}</td>
                      <td style={{padding:'6px 8px'}}>#{o.user_id}</td>
                      <td style={{padding:'6px 8px',fontWeight:600}}>¥{o.amount_yuan?.toFixed(2)}</td>
                      <td style={{padding:'6px 8px'}}>{o.nuts_amount}</td>
                      <td style={{padding:'6px 8px'}}>{o.pay_type === 'alipay' ? '支付宝' : o.pay_type === 'wxpay' ? '微信' : o.pay_type || '-'}</td>
                      <td style={{padding:'6px 8px'}}><span className={o.status === 'paid' ? 'status-pill' : 'status-pill warn'}>{o.status === 'paid' ? '已支付' : '待支付'}</span></td>
                      <td style={{padding:'6px 8px',fontSize:11}}>{o.paid_at ? formatBeijingTime(o.paid_at) : formatBeijingTime(o.created_at)}</td>
                      <td style={{padding:'6px 8px'}}>
                        {o.status !== 'paid' && (
                          <button className="btn btn-sm btn-outline" onClick={async () => {
                            const r: any = await adminRetryPayment(o.order_no);
                            if (r.ok) { showToast(r.order?.status === 'paid' ? '已补单成功' : '平台亦未支付', r.order?.status === 'paid' ? 'ok' : 'info'); fetchPayOrders(); }
                            else { showToast(r.error || '糟糕,补单失败了,请稍后重试~', 'err'); }
                          }}>补单</button>
                        )}
                        {o.codes && o.codes !== '[]' && (
                          <button className="btn btn-sm btn-outline" style={{marginLeft:4}} onClick={() => {
                            try { const codes = JSON.parse(o.codes); showToast(codes.join(', '), 'ok'); } catch(_) { showToast(o.codes, 'ok'); }
                          }}>查看激活码</button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}
      {tab === 'toolbox' && <ToolboxPanel
        versionCfg={versionCfg} announcements={announcements} signatures={signatures}
        onRefresh={fetchToolbox} showToast={(msg: string, type: string) => showToast(msg, type as any)}
        showConfirm={(t: string, m: string, cb: () => void) => setModal({open:true,title:t,message:m,onConfirm:cb,variant:'danger'})}
        closeModal={() => setModal({open:false,title:'',message:''})}
      />}
      {/* ── Market tab ── */}
      {tab === 'market' && <MarketAdminPanel />}
      {/* ── Surveys tab ── */}
      {tab === 'surveys' && surveyFormMode === 'list' && (<>
        <section className="card">
          <div className="admin-title">
            <h2 style={{margin:0}}>问卷调查</h2>
            <button className="btn btn-primary btn-sm" onClick={() => {
              setSurveyFormMode('create'); setSurveyTitle(''); setSurveyReward(5);
              setSurveyQuestions([{ question_text: '', question_type: 'choice', options: '[]', options_text: '' }]);
              setCurrentSurvey(null);
            }}>创建问卷</button>
          </div>
        </section>
        {surveyList.map((s: any) => (
          <section key={s.id} className="card" style={{padding:'16px 20px'}}>
            <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',flexWrap:'wrap',gap:8}}>
              <div>
                <strong style={{fontSize:15}}>{s.title}</strong>
                <div style={{fontSize:12,color:'var(--phx-text-secondary)',marginTop:4}}>
                  {s.question_count} 题 · 奖励 {s.reward_nuts} 板栗 · {s.answer_count} 人已答 ·
                  <span style={{color: s.is_active ? 'var(--phx-success)' : 'var(--phx-text-disabled)',marginLeft:4}}>
                    {s.is_active ? '启用中' : '已停用'}
                  </span>
                  {s.has_answers ? <span style={{color:'var(--phx-warning)',marginLeft:6}}>（已有回答）</span> : null}
                </div>
              </div>
              <div style={{display:'flex',gap:6,flexWrap:'wrap'}}>
                <button className="btn btn-sm btn-outline" onClick={() => {
                  adminToggleSurvey(s.id).then(() => fetchSurveys());
                }}>{s.is_active ? '停用' : '启用'}</button>
                <button className="btn btn-sm btn-outline" onClick={() => {
                  adminGetSurveyQuestions(s.id).then((qr: any) => {
                    setCurrentSurvey(s); setSurveyTitle(s.title); setSurveyReward(s.reward_nuts);
                    const qs = (qr.questions || []).map((q: any) => {
                      let optsText = '';
                      try { optsText = JSON.parse(q.options || '[]').join(','); } catch {}
                      return { ...q, options_text: optsText };
                    });
                    setSurveyQuestions(qs);
                    setSurveyFormMode('edit');
                  });
                  setSurveyTitle(s.title); setSurveyReward(s.reward_nuts);
                  setSurveyQuestions([]); setSurveyMsg(''); setSurveyMsgOk(true);
                }}>编辑</button>
                <button className="btn btn-sm btn-outline" onClick={() => {
                  setCurrentSurvey(s); setSurveyFormMode('stats'); setStatsData(null);
                  adminGetSurveyStats(s.id).then((r: any) => { if (r.ok) setStatsData(r.stats); });
                }}>统计</button>
                <button className="btn btn-sm btn-danger" onClick={() => {
                  if (!confirm(`确定删除问卷"${s.title}"？所有答案将被清除。`)) return;
                  adminDeleteSurvey(s.id).then(() => fetchSurveys());
                }}>删除</button>
              </div>
            </div>
          </section>
        ))}
        {surveyList.length === 0 && <section className="card"><div className="empty-state">暂无问卷</div></section>}
      </>)}
      {/* ── Create / Edit form ── */}
      {(tab === 'surveys' && (surveyFormMode === 'create' || surveyFormMode === 'edit')) && (<>
        <section className="card">
          <div className="admin-title">
            <h2 style={{margin:0}}>{surveyFormMode === 'edit' ? '编辑问卷' : '创建问卷'}</h2>
            <button className="btn btn-sm btn-outline" onClick={() => setSurveyFormMode('list')}>返回列表</button>
          </div>
        </section>
        <section className="card">

          <div style={{marginBottom:12}}>
            <label style={{fontSize:13,fontWeight:600,color:'var(--phx-text-secondary)',display:'block',marginBottom:4}}>问卷标题</label>
            <input className="input" value={surveyTitle} onChange={e => setSurveyTitle(e.target.value)} placeholder="问卷标题" style={{margin:0}} />
          </div>
          <div style={{marginBottom:16}}>
            <label style={{fontSize:13,fontWeight:600,color:'var(--phx-text-secondary)',display:'block',marginBottom:4}}>奖励板栗</label>
            <input className="input" type="number" min={0} value={surveyReward} onChange={e => setSurveyReward(parseInt(e.target.value)||0)} style={{margin:0,width:120}} />
          </div>
          {/* 问题编辑器 */}
          {(
            <div style={{marginBottom:16}}>
              <label style={{fontSize:13,fontWeight:600,color:'var(--phx-text-secondary)',display:'block',marginBottom:8}}>问题列表</label>
              {surveyQuestions.map((q: any, idx: number) => (
                <div key={idx} style={{padding:12,marginBottom:8,borderRadius:12,border:'1px solid var(--phx-border-light)',background:'var(--phx-bg)'}}>
                  <div style={{display:'flex',gap:8,alignItems:'center',marginBottom:6}}>
                    <span style={{fontWeight:700,fontSize:13,minWidth:20}}>{idx+1}.</span>
                    <input className="input" style={{margin:0,flex:1}} placeholder="问题内容" value={q.question_text}
                      onChange={e => { const c = [...surveyQuestions]; c[idx] = {...c[idx], question_text: e.target.value}; setSurveyQuestions(c); }} />
                    <select className="input" style={{margin:0,width:'auto',minWidth:80}} value={q.question_type}
                      onChange={e => { const c = [...surveyQuestions]; c[idx] = {...c[idx], question_type: e.target.value}; setSurveyQuestions(c); }}>
                      <option value="choice">单选</option>
                      <option value="multi_choice">多选</option>
                      <option value="text">文本</option>
                    </select>
                    <button className="btn btn-sm btn-danger" style={{margin:0}} onClick={() => setSurveyQuestions(surveyQuestions.filter((_, i) => i !== idx))}>删除</button>
                  </div>
                  {(q.question_type === 'choice' || q.question_type === 'multi_choice') && (
                    <input className="input" style={{margin:0}} placeholder="选项用逗号分隔，如：满意,一般,不满意"
                      value={q.options_text || ''} onChange={e => {
                        const c = [...surveyQuestions];
                        const opts = e.target.value.split(',').map((s: string) => s.trim()).filter(Boolean);
                        c[idx] = {...c[idx], options_text: e.target.value, options: JSON.stringify(opts)};
                        setSurveyQuestions(c);
                      }} />
                  )}
                </div>
              ))}
              <button className="btn btn-sm btn-outline" onClick={() => setSurveyQuestions([...surveyQuestions, { question_text: '', question_type: 'choice', sort_order: surveyQuestions.length, options: '[]', options_text: '' }])}>
                + 添加问题
              </button>
            </div>
          )}
          {surveyMsg && <div className={"result-box "+(surveyMsgOk?"ok":"err")} style={{marginBottom:10}}>{surveyMsg}</div>}
          <div style={{display:'flex',gap:8}}>
            <button className="btn btn-primary btn-sm"
              onClick={async () => {
                if (!surveyTitle.trim()) { setSurveyMsg('标题不能为空'); setSurveyMsgOk(false); return; }
                if (surveyQuestions.length === 0) { setSurveyMsg('至少需要一个问题'); setSurveyMsgOk(false); return; }
                for (const q of surveyQuestions) {
                  if (!q.question_text.trim()) { setSurveyMsg('问题内容不能为空'); setSurveyMsgOk(false); return; }
                  if (q.question_type === 'choice') {
                    try { if (JSON.parse(q.options).length === 0) { setSurveyMsg(`"${q.question_text||'问题'}" 至少需要一个选项`); setSurveyMsgOk(false); return; } }
                    catch { setSurveyMsg(`"${q.question_text||'问题'}" 选项格式错误`); setSurveyMsgOk(false); return; }
                  }
                }
                setSurveyMsg(''); setSurveyMsgOk(true);
                if (surveyFormMode === 'create') {
                  const r: any = await adminCreateSurvey({ title: surveyTitle.trim(), reward_nuts: surveyReward, questions: surveyQuestions.map((q, i) => ({ question_text: q.question_text, question_type: q.question_type, sort_order: i, options: q.options })) });
                  if (r.ok) { showToast('问卷已创建', 'ok'); setSurveyFormMode('list'); fetchSurveys(); }
                  else { setSurveyMsg(r.error || '糟糕,创建失败了,请检查后重试~'); setSurveyMsgOk(false); }
                } else {
                  const r1: any = await adminUpdateSurvey(currentSurvey.id, surveyTitle.trim(), surveyReward);
                  if (!r1.ok) { setSurveyMsg(r1.error || '哎呀,更新失败了,请稍后重试~'); setSurveyMsgOk(false); return; }
                  const r2: any = await adminUpdateSurveyQuestions(currentSurvey.id, surveyQuestions.map((q, i) => ({ question_text: q.question_text, question_type: q.question_type, sort_order: i, options: q.options })));
                  if (!r2.ok) { setSurveyMsg(r2.error || '更新问题失败'); setSurveyMsgOk(false); return; }
                  showToast('问卷已更新', 'ok'); setSurveyFormMode('list'); fetchSurveys();
                }
              }}>
              {surveyFormMode === 'create' ? '创建' : '保存'}
            </button>
            <button className="btn btn-outline btn-sm" onClick={() => setSurveyFormMode('list')}>取消</button>
          </div>
        </section>
      </>)}
      {/* ── Stats view ── */}
      {(tab === 'surveys' && surveyFormMode === 'stats' && statsData) && (<>
        <section className="card">
          <div className="admin-title">
            <h2 style={{margin:0}}>统计：{statsData.survey?.title}</h2>
            <div style={{display:'flex',gap:8}}>
              <button className="btn btn-sm btn-danger" onClick={() => clearAllAnswers(currentSurvey.id)}>清除全部答案</button>
              <button className="btn btn-sm btn-outline" onClick={() => { setSurveyFormMode('list'); setStatsData(null); }}>返回列表</button>
            </div>
          </div>
        </section>
        {/* 用户答题状态 */}
        <section className="card">
          <h3 style={{margin:'0 0 12px',fontSize:14}}>用户答题情况</h3>
          <div style={{maxHeight:300,overflowY:'auto'}}>
            <table style={{width:'100%',borderCollapse:'collapse',fontSize:13}}>
              <thead><tr style={{borderBottom:'2px solid var(--phx-border-light)'}}>
                <th style={{textAlign:'left',padding:'6px 8px',color:'var(--phx-text-secondary)'}}>用户</th>
                <th style={{textAlign:'center',padding:'6px 8px',color:'var(--phx-text-secondary)'}}>状态</th>
                <th style={{textAlign:'right',padding:'6px 8px',color:'var(--phx-text-secondary)'}}>操作</th>
              </tr></thead>
              <tbody>
                {(statsData.user_statuses || []).map((us: any) => (
                  <tr key={us.user_id} style={{borderBottom:'1px solid var(--phx-border-light)'}}>
                    <td style={{padding:'6px 8px'}}>{us.username} <span style={{color:'var(--phx-text-secondary)',fontSize:11}}>#{us.user_id}</span></td>
                    <td style={{textAlign:'center',padding:'6px 8px',color:us.answered?'var(--phx-success)':'var(--phx-text-disabled)',fontWeight:600}}>
                      {us.answered ? '已答' : '未答'}
                    </td>
                    <td style={{textAlign:'right',padding:'6px 8px'}}>
                      {us.answered && <button className="btn btn-sm btn-outline" style={{fontSize:11}} onClick={() => {
                        setSurveyFormMode('user-detail'); setStatsUserName(us.username); setStatsUserAnswers(null);
                        adminGetUserSurveyAnswers(currentSurvey.id, us.user_id).then((r: any) => {
                          if (r.ok) setStatsUserAnswers({ answers: r.answers, questions: r.questions });
                        });
                      }}>查看回答</button>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
        {/* 选择题统计 */}
        {statsData.choice_stats?.length > 0 && (
          <section className="card">
            <h3 style={{margin:'0 0 12px',fontSize:14}}>选择题统计</h3>
            {statsData.choice_stats.map((cs: any) => (
              <div key={cs.question_id} style={{marginBottom:16}}>
                <p style={{margin:'0 0 8px',fontWeight:600,fontSize:13}}>{cs.question_text}</p>
                <div style={{display:'flex',flexDirection:'column',gap:6}}>
                  {cs.options.map((opt: any, oi: number) => (
                    <div key={oi} style={{display:'flex',alignItems:'center',gap:8}}>
                      <span style={{minWidth:80,fontSize:12}}>{opt.option_text}</span>
                      <div style={{flex:1,background:'var(--phx-bg)',borderRadius:6,height:20,overflow:'hidden'}}>
                        <div style={{height:'100%',background:'var(--phx-primary)',borderRadius:6,width:`${Math.max(opt.percentage, 2)}%`,transition:'width .3s'}} />
                      </div>
                      <span style={{minWidth:80,textAlign:'right',fontSize:12,color:'var(--phx-text-secondary)'}}>
                        {opt.count} 票 ({opt.percentage.toFixed(1)}%)
                      </span>
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </section>
        )}
        {(!statsData.choice_stats || statsData.choice_stats.length === 0) && (
          <section className="card"><div className="empty-state">暂无选择题</div></section>
        )}
      </>)}
      {/* ── User answer detail ── */}
      {(tab === 'surveys' && surveyFormMode === 'user-detail' && statsUserAnswers) && (<>
        <section className="card">
          <div className="admin-title">
            <h3 style={{margin:0}}>{statsUserName} 的回答</h3>
            <button className="btn btn-sm btn-outline" onClick={() => setSurveyFormMode('stats')}>返回统计</button>
          </div>
        </section>
        <section className="card">
          {(statsUserAnswers.questions || []).map((q: any, idx: number) => {
            const ans = (statsUserAnswers.answers || []).find((a: any) => a.question_id === q.id);
            return (
              <div key={q.id} style={{marginBottom:16}}>
                <p style={{margin:'0 0 6px',fontWeight:600,fontSize:14}}>{idx+1}. {q.question_text}</p>
                <p style={{margin:0,padding:'8px 12px',background:'var(--phx-bg)',borderRadius:8,fontSize:13}}>
                  {ans ? ans.answer_text : <span style={{color:'var(--phx-text-disabled)'}}>未回答</span>}
                </p>
              </div>
            );
          })}
        </section>
      </>)}
      {/* ── Messages tab ── */}
      {tab === 'messages' && (<>
        <section className="card">
          <div className="admin-title">
            <h2 style={{margin:0}}>消息配置</h2>
            <button className="btn btn-primary btn-sm" disabled={!msgDirty} onClick={saveMessages}>保存并立即生效</button>
          </div>
          <p style={{fontSize:12,color:'var(--phx-text-secondary)',margin:'8px 0 0'}}>
            修改通用报错/提示文案,保存后立即生效(写入 messages.json 并热重载,无需重启)。留空 = 使用内置默认文案。
          </p>
        </section>
        <section className="card">
          {MSG_META.map(m => (
            <div key={m.key} style={{marginBottom:16}}>
              <label style={{fontSize:13,fontWeight:600,display:'block',marginBottom:4}}>
                {m.label}
                <span style={{fontSize:11,fontWeight:400,color:'var(--phx-text-secondary)',marginLeft:8}}>{m.key} · {m.desc}</span>
              </label>
              <textarea className="input" rows={2} value={msgConfig[m.key] || ''}
                onChange={e => { setMsgConfig(c => ({ ...c, [m.key]: e.target.value })); setMsgDirty(true); }}
                style={{margin:0,width:'100%',resize:'vertical'}} />
            </div>
          ))}
          <div style={{display:'flex',gap:8,flexWrap:'wrap'}}>
            <button className="btn btn-primary" disabled={!msgDirty} onClick={saveMessages}>保存并立即生效</button>
            <button className="btn btn-outline" onClick={() => {
              const c: Record<string, string> = {};
              MSG_META.forEach(m => { c[m.key] = ''; });
              setMsgConfig(c); setMsgDirty(true);
            }}>全部恢复为默认</button>
          </div>
        </section>
      </>)}
      <Modal {...modal} confirmLabel="确定" cancelLabel="取消" onCancel={() => setModal({open:false,title:"",message:""})} />
    </div>
  );
}

function LogList({ logs, kind, hasMore, total, page, pageSize, onPage }: {
  logs: any[]; kind: 'audit' | 'system'; hasMore: boolean; total: number; page: number; pageSize: number; onPage: (p: number) => void;
}) {
  if (logs.length === 0) return <section className="card"><div className="empty-state">暂无日志</div></section>;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <section className="card">
      <div className="log-list">
        {logs.map((l: any) => (
          <article key={l.id} className="log-item">
            <div className="log-dot" style={{ background: kind === 'audit' ? '#8b9e6b' : ((l.level||'').toUpperCase()==='ERROR'?'#d9534f':(l.level||'').toUpperCase()==='WARN'?'#f0a040':'#8b9e6b'), boxShadow: kind === 'audit' ? '0 0 4px rgba(139,158,107,0.4)' : ((l.level||'').toUpperCase()==='ERROR'?'0 0 6px rgba(217,83,79,0.5)':(l.level||'').toUpperCase()==='WARN'?'0 0 4px rgba(240,160,64,0.4)':'0 0 4px rgba(139,158,107,0.3)') }} />
            <div style={{flex:1}}>
              <div className="log-head"><strong>{kind === 'audit' ? labelAction(l.action) : (<span className={levelClass(l.level)}>{l.level||'INFO'}</span>)}</strong><span className="log-time">{formatBeijingTime(l.created_at)}</span></div>
              {kind === 'audit' ? (
                <><p className="log-meta">用户：{userLabel(l)} · 账号：{accountLabel(l)} · 对象：{labelTarget(l)}</p>{l.detail && l.detail !== l.target && <p className="log-detail">{l.detail}</p>}{l.ip && <span className="log-ip">{l.ip}</span>}</>
              ) : (
                <><p className="log-meta">{l.message || ''}</p>{l.detail && l.detail !== l.message && <p className="log-detail">{l.detail}</p>}</>
              )}
            </div>
          </article>
        ))}
      </div>
      {totalPages > 1 && (
        <div className="pagination" style={{display:'flex',justifyContent:'center',alignItems:'center',gap:6,marginTop:12,flexWrap:'wrap'}}>
          <button className="btn btn-sm btn-outline" disabled={page === 0} onClick={() => onPage(page - 1)}>上一页</button>
          {Array.from({length: Math.min(totalPages, 10)}).map((_, i) => {
            // Show pages around current page
            let pageNum = i;
            if (totalPages > 10) {
              const start = Math.max(0, Math.min(page - 5, totalPages - 10));
              pageNum = start + i;
            }
            return (
              <button key={pageNum} className={"btn btn-sm " + (pageNum === page ? 'btn-primary' : 'btn-outline')}
                onClick={() => onPage(pageNum)}>{pageNum + 1}</button>
            );
          })}
          <button className="btn btn-sm btn-outline" disabled={!hasMore} onClick={() => onPage(page + 1)}>下一页</button>
          <span style={{fontSize:12,color:'var(--phx-text-secondary)',marginLeft:8}}>共 {total} 条</span>
        </div>
      )}
    </section>
  );
}
