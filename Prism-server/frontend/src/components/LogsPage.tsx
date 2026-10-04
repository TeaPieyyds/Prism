import { useEffect, useState } from 'react';
import { myLogs } from '../api';
import UserStatsPanel from './UserStatsPanel';

const actionText: Record<string, string> = {
  register_email_sent: '发送验证邮件',
  register_verified: '邮箱验证成功',
  login: '网页用户登录',
  logout: '网页用户退出',
  add_account: '添加游戏账号',
  switch_account: '切换游戏账号',
  delete_account: '删除游戏账号',
  refresh_account: '刷新游戏账号',
  update_username: '修改用户名',
  update_password: '修改密码',
  reset_token: '重置密钥',
  join_server: '进入服务器',
  join_server_failed: '进服失败',
  challenge_override: '黑洞模式切换',
  update_account: '更新账号设置',
  update_account_nickname: '修改游戏昵称',
  update_account_shared: '变更共享状态',
  merge_account: '合并到共享池',
  share_account: '设为共享',
  unshare_account: '收回共享',
  admin_nuts: '调整板栗',
  growth_override: '等级覆盖',
  admin_add_preset: '添加实名预设',
  forgot_password_sent: '发送重置密码邮件',
  change_email_sent: '发送改邮箱验证',
  email_changed: '邮箱已修改',
  mpay_email_login: '邮箱登录游戏',
  mpay_phone_login: '手机登录游戏',
  guest_account: '游客账号创建',
  forgot_password: '忘记密码',
  reset_password: '重置密码',
  change_email: '修改邮箱',
  confirm_email: '验证邮箱',
  activate: '账号激活',
  admin_add_account: '管理员添加账号',
  admin_clone_account: '管理员认领账号',
  admin_claim_account: '管理员认领账号',
  admin_create_user: '管理员创建用户',
  admin_delete_user: '管理员删除用户',
  admin_gen_codes: '管理员生成激活码',
  admin_toggle_preset: '管理员切换预设',
  admin_update_user: '管理员修改用户',
  create_payment_order: '创建支付订单',
  redeem_code: '兑换激活码',
  redeem_redpacket: '抢到红包',
  create_redpacket: '创建红包',
  create_token: '创建密钥',
  update_token: '修改密钥',
  delete_token: '删除密钥',
  nuts_to_code: '积分转兑换码',
  nuts_to_question_code: '积分转答题兑换码',
  skin_preset_add: '添加皮肤预设',
  skin_preset_update: '修改皮肤预设',
  skin_preset_delete: '删除皮肤预设',
  password_test: '密码测试',
  password_test_abort: '密码测试已中止',
  toolbox_trial: '试用工具箱',
  toolbox_purchase: '购买工具箱功能',
  toolbox_extend: '续期工具箱',
};

const targetText: Record<string, string> = {
  web: '网页后台',
  email: '邮箱',
  web_user: '网页用户',
  game_account: '游戏账号',
};

function labelAction(action: string) {
  return actionText[action] || action || '未知操作';
}

function labelTarget(log: any) {
  if (log.action === 'join_server' || log.action === 'join_server_failed') return log.target || log.detail || '未知服务器';
  return targetText[log.target] || log.target || '-';
}

function formatBeijingTime(value?: string) {
  if (!value) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai',
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date).replace(/\//g, '-');
}

function accountLabel(log: any) {
  if (log.account_name) return log.account_id ? `${log.account_name} (#${log.account_id})` : log.account_name;
  return log.account_id ? `#${log.account_id}` : '-';
}

const TABS = [
  { key: 'logs', label: '操作日志' },
  { key: 'stats', label: '使用统计' },
];

const PAGE_SIZE = 40;

export default function LogsPage() {
  const [tab, setTab] = useState('logs');
  const [logs, setLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(0);
  const [total, setTotal] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [actionFilter, setActionFilter] = useState('');

  const fetchLogs = async (p = 0) => {
    setLoading(true);
    const res = await myLogs(PAGE_SIZE, p * PAGE_SIZE, keyword || undefined, actionFilter || undefined);
    if ((res as any).ok) { setLogs((res as any).logs || []); setTotal((res as any).total || 0); setPage(p); }
    else alert((res as any).error || '哎呀,日志加载失败了,请稍后重试~');
    setLoading(false);
  };

  useEffect(() => { fetchLogs(0); }, []);

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="page-stack narrow-page">
      <section className="hero-panel compact">
        <div>
          <div className="eyebrow">我的记录</div>
          <h1>{TABS.find(t => t.key === tab)?.label}</h1>
          <p>{tab === 'logs' ? '查看你的登录记录、账号切换、密钥修改和进入服务器等操作历史。' : '查看你的 API 调用统计、进服频率和板栗使用情况。'}</p>
        </div>
        {tab === 'logs' && <button className="btn btn-sm btn-outline" onClick={() => fetchLogs(page)}>刷新</button>}
      </section>

      <div className="pill-slider">
        {TABS.map(t => (
          <button key={t.key} className={tab === t.key ? 'active' : ''} onClick={() => setTab(t.key)}>
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'logs' ? (
        <>
          <section className="card" style={{marginBottom:8}}>
            <div style={{display:'flex',gap:8,flexWrap:'wrap',alignItems:'center'}}>
              <input className="input" placeholder="搜索关键词" value={keyword} onChange={e => setKeyword(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') fetchLogs(0); }} style={{flex:1,minWidth:120}} />
              <input className="input" placeholder="动作筛选" value={actionFilter} onChange={e => setActionFilter(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') fetchLogs(0); }} style={{width:120}} />
              <button className="btn btn-sm btn-primary" onClick={() => fetchLogs(0)}>搜索</button>
              <button className="btn btn-sm btn-outline" onClick={() => { setKeyword(''); setActionFilter(''); fetchLogs(0); }}>重置</button>
            </div>
          </section>
          <section className="card">
            {loading ? <div style={{display:'flex',flexDirection:'column',gap:12}}>{Array.from({length:5}).map((_,i)=><div key={i} className="skeleton-card" style={{display:'flex',alignItems:'center',gap:12,padding:'12px 16px'}}><div style={{width:10,height:10,borderRadius:'50%',background:'var(--phx-border-light)'}} /><div style={{flex:1}}><div className="skeleton-line" /><div className="skeleton-line" style={{width:'60%'}} /></div></div>)}</div> : logs.length === 0 ? <div className="empty-state">暂无操作日志</div> : (
              <div className="log-list">
                {logs.map((l) => (
                  <article key={l.id} className="log-item">
                    <div className="log-dot" />
                    <div>
                      <div className="log-head"><strong>{labelAction(l.action)}</strong><span>{formatBeijingTime(l.created_at)}</span></div>
                      <p>对象：{labelTarget(l)} · 游戏账号：{accountLabel(l)}</p>
                      <p>{l.detail || '-'}</p>
                    </div>
                  </article>
                ))}
              </div>
            )}
            {totalPages > 1 && (
              <div className="pagination" style={{display:'flex',justifyContent:'center',alignItems:'center',gap:6,marginTop:12,flexWrap:'wrap'}}>
                <button className="btn btn-sm btn-outline" disabled={page === 0} onClick={() => fetchLogs(page - 1)}>上一页</button>
                {Array.from({length: Math.min(totalPages, 10)}).map((_, i) => {
                  let pageNum = i;
                  if (totalPages > 10) {
                    const start = Math.max(0, Math.min(page - 5, totalPages - 10));
                    pageNum = start + i;
                  }
                  return (
                    <button key={pageNum} className={"btn btn-sm " + (pageNum === page ? 'btn-primary' : 'btn-outline')}
                      onClick={() => fetchLogs(pageNum)}>{pageNum + 1}</button>
                  );
                })}
                <button className="btn btn-sm btn-outline" disabled={page >= totalPages - 1} onClick={() => fetchLogs(page + 1)}>下一页</button>
                <span style={{fontSize:12,color:'var(--phx-text-secondary)',marginLeft:8}}>共 {total} 条</span>
              </div>
            )}
          </section>
        </>
      ) : (
        <UserStatsPanel />
      )}
    </div>
  );
}