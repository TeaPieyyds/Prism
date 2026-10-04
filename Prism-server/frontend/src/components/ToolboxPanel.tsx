import { useState, useEffect } from 'react';
import ModalPortal from './ModalPortal';

// 将 UTC 时间戳转为北京时间 (CST, UTC+8)
const toCST = (utcStr: string) => {
  if (!utcStr) return '';
  const d = new Date(utcStr);
  return d.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
};
const toCSTTime = (utcStr: string) => {
  if (!utcStr) return '';
  const d = new Date(utcStr);
  return d.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
};
import { updateVersionConfig, createAnnouncement, updateAnnouncement, deleteAnnouncement, toggleAnnouncement, addSignature, deleteSignature, getAppVersions, switchAppVersion, deleteAppVersion, scanReleases, getAdminToolboxUsers, getAdminToolboxDefaults, updateAdminToolboxDefaults, updateAdminToolboxUserConfig, sendAdminPush, getAdminOnlineUsers, getAdminUserDetail } from '../api';

interface Props {
  versionCfg: any;
  announcements: any[];
  signatures: any[];
  onRefresh: () => void;
  showToast: (msg: string, type: string) => void;
  showConfirm: (title: string, msg: string, cb: () => void) => void;
  closeModal: () => void;
}

export default function ToolboxPanel({ versionCfg, announcements, signatures, onRefresh, showToast, showConfirm, closeModal }: Props) {
  const [appVersions, setAppVersions] = useState<any[]>([]);

  // Load versions on mount
  const loadVersions = async () => {
    const r: any = await getAppVersions();
    if (r.ok) setAppVersions(r.versions || []);
  };
  useEffect(() => { loadVersions(); }, []);
  // Version form
  const [ver, setVer] = useState(versionCfg.latest_version || '');
  const [verCode, setVerCode] = useState(versionCfg.latest_version_code || 0);
  const [forceUpdate, setForceUpdate] = useState(versionCfg.force_update || false);
  const [updateUrl, setUpdateUrl] = useState(versionCfg.update_url || '');
  const [updateMsg, setUpdateMsg] = useState(versionCfg.update_message || '');
  // Sync version form fields when prop changes (after switch/save)
  useEffect(() => {
    setVer(versionCfg.latest_version || '');
    setVerCode(versionCfg.latest_version_code || 0);
    setForceUpdate(versionCfg.force_update || false);
    setUpdateUrl(versionCfg.update_url || '');
    setUpdateMsg(versionCfg.update_message || '');
  }, [versionCfg]);

  // Announcement form
  const [annForm, setAnnForm] = useState<any>(null);
  const [annTitle, setAnnTitle] = useState('');
  const [annContent, setAnnContent] = useState('');
  const [annSeverity, setAnnSeverity] = useState('info');
  const [annDisplay, setAnnDisplay] = useState('modal');
  const [annDismiss, setAnnDismiss] = useState(true);
  const [annActive, setAnnActive] = useState(true);
  const [annVerTarget, setAnnVerTarget] = useState('');
  const [annSvrTarget, setAnnSvrTarget] = useState('');
  const [annContentType, setAnnContentType] = useState('markdown');
  const [annExpiresAt, setAnnExpiresAt] = useState('');

  // Signature form
  const [sigPkg, setSigPkg] = useState('com.prismtool.box');
  const [sigHash, setSigHash] = useState('');
  const [sigLabel, setSigLabel] = useState('');

  // Toolbox user management
  const [tbUsers, setTbUsers] = useState<any[]>([]);
  const [tbDefaults, setTbDefaults] = useState<any>({default_prompt:'',default_name:''});
  const [tbEditUser, setTbEditUser] = useState<any>(null);
  const [tbUserForm, setTbUserForm] = useState<any>({});
  const loadTbUsers = async () => {
    const r: any = await getAdminToolboxUsers();
    if (r.ok) setTbUsers(r.users || []);
  };
  const loadTbDefaults = async () => {
    const r: any = await getAdminToolboxDefaults();
    if (r.ok) setTbDefaults(r.defaults || {default_prompt:'',default_name:''});
  };
  const saveTbUser = async () => {
    const r: any = await updateAdminToolboxUserConfig(tbUserForm);
    if (r.ok) { showToast('已保存', 'ok'); setTbEditUser(null); loadTbUsers(); }
    else showToast(r.error || '糟糕,保存没成功,请稍后重试~', 'err');
  };
  const saveTbDefaults = async () => {
    const r: any = await updateAdminToolboxDefaults(tbDefaults);
    if (r.ok) { showToast('默认配置已保存', 'ok'); }
    else showToast(r.error || '糟糕,保存没成功,请稍后重试~', 'err');
  };

  // Push notification — online users list
  const [onlineUsers, setOnlineUsers] = useState<any[]>([]);
  const [selUser, setSelUser] = useState<any>(null);
  const [selUserDetail, setSelUserDetail] = useState<any>(null);
  const [selUserToolbox, setSelUserToolbox] = useState<any>(null);
  const [pushForm, setPushForm] = useState<any>(null);
  const [pushTitle, setPushTitle] = useState('');
  const [pushBody, setPushBody] = useState('');
  const [pushType, setPushType] = useState('admin_push');
  const [pushDisplayMode, setPushDisplayMode] = useState('toast');
  const [pushAction, setPushAction] = useState('none');
  const [pushActionData, setPushActionData] = useState('');
  const [pushTargetUser, setPushTargetUser] = useState<number|null>(null);
  const loadOnlineUsers = async () => {
    const r: any = await getAdminOnlineUsers();
    if (r.ok) setOnlineUsers(r.users || []);
  };
  // 自动刷新在线用户列表
  useEffect(() => { loadOnlineUsers(); const t = setInterval(loadOnlineUsers, 30000); return () => clearInterval(t); }, []);
  const loadUserDetail = async (uid: number) => {
    const r: any = await getAdminUserDetail(uid);
    if (r.ok) { setSelUserDetail(r.user); setSelUserToolbox(r.toolbox_config || null); }
  };
  const openPushForm = (userId: number|null) => {
    setPushTargetUser(userId);
    setPushTitle(''); setPushBody(''); setPushType('admin_push');
    setPushDisplayMode('toast');
    setPushAction('none'); setPushActionData('');
    setPushForm('new');
  };
  const savePush = async () => {
    const data: any = {
      type: pushType, title: pushTitle, body: pushBody,
      display_mode: pushDisplayMode,
      action: pushAction, action_data: pushActionData,
    };
    if (pushTargetUser) data.target_user_id = pushTargetUser;
    const r: any = await sendAdminPush(data);
    if (r.ok) { showToast('推送已发送', 'ok'); setPushForm(null); }
    else showToast(r.error || '哎呀,发送失败了,请稍后重试~', 'err');
  };

  const saveVersion = async () => {
    const r: any = await updateVersionConfig({
      latest_version: ver,
      latest_version_code: verCode,
      force_update: forceUpdate,
      update_url: updateUrl,
      update_message: updateMsg,
    });
    if (r.ok) { showToast('版本配置已保存', 'ok'); onRefresh(); }
    else showToast(r.error || '糟糕,保存没成功,请稍后重试~', 'err');
  };

  const openAnnForm = (a?: any) => {
    if (a) {
      setAnnForm(a); setAnnTitle(a.title); setAnnContent(a.content);
      setAnnSeverity(a.severity || 'info'); setAnnDisplay(a.display_mode || 'modal');
      setAnnDismiss(a.can_dismiss); setAnnActive(a.is_active);
      setAnnContentType(a.content_type || 'markdown'); setAnnExpiresAt(a.expires_at || '');
      setAnnVerTarget((a.target_versions || []).join(','));
      setAnnSvrTarget((a.target_servers || []).join(','));
    } else {
      setAnnForm('new'); setAnnTitle(''); setAnnContent('');
      setAnnSeverity('info'); setAnnDisplay('modal');
      setAnnDismiss(true); setAnnActive(true);
      setAnnContentType('markdown'); setAnnExpiresAt('');
      setAnnVerTarget(''); setAnnSvrTarget('');
    }
  };

  const saveAnn = async () => {
    const data = {
      title: annTitle, content: annContent, content_type: annContentType,
      severity: annSeverity, display_mode: annDisplay,
      can_dismiss: annDismiss, is_active: annActive,
      expires_at: annExpiresAt,
      target_versions: annVerTarget ? annVerTarget.split(',').map((s: string) => s.trim()).filter(Boolean) : [],
      target_servers: annSvrTarget ? annSvrTarget.split(',').map((s: string) => s.trim()).filter(Boolean) : [],
    };
    const r: any = annForm === 'new' ? await createAnnouncement(data) : await updateAnnouncement(annForm.id, data);
    if (r.ok) { showToast(annForm === 'new' ? '公告已创建' : '公告已更新', 'ok'); setAnnForm(null); onRefresh(); }
    else showToast(r.error || '糟糕,保存没成功,请稍后重试~', 'err');
  };

  return (
    <div style={{display:'flex',flexDirection:'column',gap:16}}>
      {/* App Versions */}
      <section className="card">
        <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:12}}>
          <h3 style={{margin:0}}>发布版本</h3>
          <div style={{display:'flex',gap:6}}>
            <button className="btn btn-sm btn-primary" onClick={async () => {
              const r: any = await scanReleases();
              if (r.ok) { showToast(r.added?.length ? `新增 ${r.added.length} 个版本` : '无新版本', 'ok'); loadVersions(); }
              else showToast(r.error || '哎呀,扫描失败了,请稍后重试~', 'err');
            }}>扫描新版本</button>
            <button className="btn btn-sm btn-outline" onClick={loadVersions}>刷新</button>
          </div>
        </div>
        <p style={{fontSize:11,color:'var(--phx-text-secondary)',marginBottom:10}}>
          将 APK 文件放入 <code>data/releases/</code> 目录，命名格式: <code>prism-版本号.apk</code> 或 <code>prism-版本号_版本代号.apk</code>
        </p>
        {appVersions.length === 0 ? <div className="empty-state">暂无已发布版本</div> : appVersions.map((v: any) => (
          <div key={v.id} style={{display:'flex',justifyContent:'space-between',alignItems:'center',padding:'8px 12px',background:'var(--phx-bg)',borderRadius:10,marginBottom:6,fontSize:13}}>
            <div style={{flex:1,minWidth:0}}>
              <span style={{fontWeight:700}}>{v.version}</span>
              <span style={{marginLeft:8,fontSize:11,color:'var(--phx-text-secondary)'}}>code={v.version_code}</span>
              <span style={{marginLeft:8,fontSize:11,color:'var(--phx-text-secondary)'}}>{(v.file_size / 1024 / 1024).toFixed(1)}MB</span>
              {v.is_current && <span className="status-pill" style={{marginLeft:8,fontSize:10}}>当前版本</span>}
            </div>
            <div style={{display:'flex',gap:4,flexShrink:0}}>
              {!v.is_current && (
                <button className="btn btn-sm btn-primary" style={{fontSize:11,padding:'4px 8px'}} onClick={async () => {
                  const r: any = await switchAppVersion(v.id);
                  if (r.ok) { showToast(`已切换到 ${v.version}`, 'ok'); loadVersions(); onRefresh(); }
                  else showToast(r.error || '糟糕,切换失败了,请稍后重试~', 'err');
                }}>切换</button>
              )}
              {!v.is_current && (
                <button className="btn btn-sm btn-danger" style={{fontSize:11,padding:'4px 8px'}} onClick={() => showConfirm('删除版本', `确定删除 ${v.version}？`, async () => { closeModal(); await deleteAppVersion(v.id); loadVersions(); })}>删除</button>
              )}
            </div>
          </div>
        ))}
      </section>

      {/* Version Config */}
      <section className="card">
        <h3 style={{margin:'0 0 12px'}}>版本配置</h3>
        <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:8}}>
          <input className="input" placeholder="最新版本号" value={ver} onChange={e => setVer(e.target.value)} />
          <input className="input" type="number" placeholder="版本号(整数)" value={verCode || ''} onChange={e => setVerCode(parseInt(e.target.value) || 0)} />
        </div>
        <input className="input" placeholder="下载地址" value={updateUrl} onChange={e => setUpdateUrl(e.target.value)} />
        <textarea className="input textarea" placeholder="更新说明" value={updateMsg} onChange={e => setUpdateMsg(e.target.value)} style={{minHeight:80}} />
        <label style={{display:'flex',alignItems:'center',gap:8,marginBottom:8,cursor:'pointer'}}>
          <input type="checkbox" checked={forceUpdate} onChange={e => setForceUpdate(e.target.checked)} /> 强制更新
        </label>
        <button className="btn btn-primary btn-sm" onClick={saveVersion}>保存版本配置</button>
      </section>

      {/* Announcements */}
      <section className="card">
        <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:12}}>
          <h3 style={{margin:0}}>公告管理</h3>
          <button className="btn btn-sm btn-primary" onClick={() => openAnnForm()}>新建公告</button>
        </div>
        {announcements.length === 0 ? <div className="empty-state">暂无公告</div> : announcements.map((a: any) => (
          <div key={a.id} style={{display:'flex',justifyContent:'space-between',alignItems:'center',padding:'8px 12px',background:'var(--phx-bg)',borderRadius:10,marginBottom:6,fontSize:13}}>
            <div style={{flex:1,minWidth:0}}>
              <span style={{fontWeight:700}}>{a.title}</span>
              <span style={{marginLeft:8,fontSize:11,color:'var(--phx-text-secondary)'}}>{a.severity}/{a.display_mode}</span>
              <span className={a.is_active ? 'status-pill' : 'status-pill danger'} style={{marginLeft:8,fontSize:10}}>{a.is_active ? '启用' : '停用'}</span>
            </div>
            <div style={{display:'flex',gap:4,flexShrink:0}}>
              <button className="btn btn-sm btn-outline" style={{fontSize:11,padding:'4px 8px'}} onClick={() => { toggleAnnouncement(a.id); onRefresh(); }}>{a.is_active ? '停用' : '启用'}</button>
              <button className="btn btn-sm btn-outline" style={{fontSize:11,padding:'4px 8px'}} onClick={() => openAnnForm(a)}>编辑</button>
              <button className="btn btn-sm btn-danger" style={{fontSize:11,padding:'4px 8px'}} onClick={() => showConfirm('删除公告', `确定删除「${a.title}」？`, async () => { closeModal(); await deleteAnnouncement(a.id); onRefresh(); })}>删除</button>
            </div>
          </div>
        ))}

        {/* Ann form modal */}
        {annForm && (
          <ModalPortal>
          <div className="modal-backdrop" onClick={() => setAnnForm(null)}>
            <div className="modal-content" onClick={e => e.stopPropagation()} style={{maxWidth:520,maxHeight:'90dvh',overflowY:'auto'}}>
              <h3 style={{margin:'0 0 12px'}}>{annForm === 'new' ? '新建公告' : '编辑公告'}</h3>
              <input className="input" placeholder="标题" value={annTitle} onChange={e => setAnnTitle(e.target.value)} />
              <textarea className="input textarea" placeholder="内容 (Markdown)" value={annContent} onChange={e => setAnnContent(e.target.value)} style={{minHeight:120}} />
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:8}}>
                <select className="input" value={annSeverity} onChange={e => setAnnSeverity(e.target.value)}>
                  <option value="info">info 信息</option>
                  <option value="warning">warning 警告</option>
                  <option value="critical">critical 严重</option>
                </select>
                <select className="input" value={annDisplay} onChange={e => setAnnDisplay(e.target.value)}>
                  <option value="modal">modal 弹窗</option>
                  <option value="modal_once">modal_once 一次弹窗</option>
                  <option value="banner">banner 横幅</option>
                  <option value="inline">inline 内嵌</option>
                </select>
              </div>
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:8,marginTop:8}}>
                <select className="input" value={annContentType} onChange={e => setAnnContentType(e.target.value)}>
                  <option value="markdown">markdown</option>
                  <option value="text">text 纯文本</option>
                  <option value="html">html</option>
                </select>
                <input className="input" type="datetime-local" value={annExpiresAt ? annExpiresAt.replace('Z','') : ''} onChange={e => setAnnExpiresAt(e.target.value ? e.target.value + ':00Z' : '')} placeholder="过期时间(可选)" />
              </div>
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:8,marginTop:8}}>
                <input className="input" placeholder="目标版本(逗号分隔)" value={annVerTarget} onChange={e => setAnnVerTarget(e.target.value)} />
                <input className="input" placeholder="目标服务器(逗号分隔)" value={annSvrTarget} onChange={e => setAnnSvrTarget(e.target.value)} />
              </div>
              <div style={{display:'flex',gap:16,margin:'8px 0'}}>
                <label style={{display:'flex',alignItems:'center',gap:6,cursor:'pointer',fontSize:13}}><input type="checkbox" checked={annDismiss} onChange={e => setAnnDismiss(e.target.checked)} /> 可关闭</label>
                <label style={{display:'flex',alignItems:'center',gap:6,cursor:'pointer',fontSize:13}}><input type="checkbox" checked={annActive} onChange={e => setAnnActive(e.target.checked)} /> 立即启用</label>
              </div>
              <div style={{display:'flex',gap:8}}>
                <button className="btn btn-primary btn-sm" onClick={saveAnn}>保存</button>
                <button className="btn btn-outline btn-sm" onClick={() => setAnnForm(null)}>取消</button>
              </div>
            </div>
          </div>
          </ModalPortal>
        )}
      </section>

      {/* Toolbox User Management */}
      <section className="card">
        <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:12}}>
          <h3 style={{margin:0}}>工具箱用户管理</h3>
          <div style={{display:'flex',gap:6}}>
            <button className="btn btn-sm btn-primary" onClick={() => { loadTbUsers(); loadTbDefaults(); }}>刷新用户</button>
          </div>
        </div>
        {/* System defaults */}
        <div style={{padding:'8px 12px',background:'var(--phx-bg)',borderRadius:10,marginBottom:10,fontSize:13}}>
          <div style={{fontWeight:600,marginBottom:6}}>系统默认值</div>
          <div style={{display:'flex',gap:8,marginBottom:4}}>
            <input className="input" placeholder="默认提示词" value={tbDefaults.default_prompt} onChange={e => setTbDefaults({...tbDefaults,default_prompt:e.target.value})} style={{flex:1}} />
            <input className="input" placeholder="默认名称" value={tbDefaults.default_name} onChange={e => setTbDefaults({...tbDefaults,default_name:e.target.value})} style={{flex:1}} />
            <button className="btn btn-sm btn-outline" onClick={saveTbDefaults}>保存</button>
          </div>
        </div>
        {tbUsers.length === 0 ? <div className="empty-state">暂无工具箱用户</div> : tbUsers.map((u: any) => (
          <div key={u.user_id} style={{display:'flex',justifyContent:'space-between',alignItems:'center',padding:'8px 12px',background:'var(--phx-bg)',borderRadius:10,marginBottom:6,fontSize:13}}>
            <div style={{flex:1,minWidth:0}}>
              <span style={{fontWeight:600}}>{u.username || u.email || `ID:${u.user_id}`}</span>
              <span className={u.enabled ? 'status-pill' : 'status-pill danger'} style={{marginLeft:8,fontSize:10}}>{u.enabled ? '已开通' : '未开通'}</span>
              {u.expires_at && u.enabled && <span style={{marginLeft:6,fontSize:11,color:'var(--phx-text-secondary)'}}>到期: {u.expires_at?.substring(0,10)}</span>}
              <div style={{fontSize:11,color:'var(--phx-text-secondary)',marginTop:2}}>
                {u.prompt_purchased && <span style={{marginRight:8}}>✓ 提示词</span>}
                {u.name_purchased && <span style={{marginRight:8}}>✓ 命名</span>}
                {u.trial_used && <span>试用已用</span>}
              </div>
            </div>
            <button className="btn btn-sm btn-outline" style={{fontSize:11,padding:'4px 8px'}} onClick={() => {
              setTbUserForm({
                user_id: u.user_id,
                enabled: u.enabled,
                expires_at: u.expires_at || '',
                prompt_purchased: u.prompt_purchased,
                name_purchased: u.name_purchased,
                custom_prompt: u.custom_prompt || '',
                custom_name: u.custom_name || '',
              });
              setTbEditUser(u);
            }}>编辑</button>
          </div>
        ))}
        {/* Edit user modal */}
        {tbEditUser && (
          <ModalPortal>
          <div className="modal-backdrop" onClick={() => setTbEditUser(null)}>
            <div className="modal-content" onClick={e => e.stopPropagation()} style={{maxWidth:480}}>
              <h3 style={{margin:'0 0 12px'}}>编辑用户: {tbEditUser.username || tbEditUser.email || `ID:${tbEditUser.user_id}`}</h3>
              <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:8}}>
                <label style={{display:'flex',alignItems:'center',gap:6,cursor:'pointer',fontSize:13}}>
                  <input type="checkbox" checked={tbUserForm.enabled} onChange={e => setTbUserForm({...tbUserForm,enabled:e.target.checked})} /> 开通工具箱
                </label>
              </div>
              <input className="input" placeholder="到期时间 (RFC3339)" value={tbUserForm.expires_at} onChange={e => setTbUserForm({...tbUserForm,expires_at:e.target.value})} style={{marginBottom:8}} />
              {tbUserForm.enabled && !tbUserForm.expires_at && <div style={{fontSize:11,color:'var(--phx-text-warning)',marginBottom:8}}>到期时间为空 = 永久有效</div>}
              <div style={{display:'flex',gap:16,marginBottom:8}}>
                <label style={{display:'flex',alignItems:'center',gap:6,cursor:'pointer',fontSize:13}}>
                  <input type="checkbox" checked={tbUserForm.prompt_purchased} onChange={e => setTbUserForm({...tbUserForm,prompt_purchased:e.target.checked})} /> 已购提示词
                </label>
                <label style={{display:'flex',alignItems:'center',gap:6,cursor:'pointer',fontSize:13}}>
                  <input type="checkbox" checked={tbUserForm.name_purchased} onChange={e => setTbUserForm({...tbUserForm,name_purchased:e.target.checked})} /> 已购命名
                </label>
              </div>
              {tbUserForm.prompt_purchased && (
                <input className="input" placeholder="自定义提示词" value={tbUserForm.custom_prompt} onChange={e => setTbUserForm({...tbUserForm,custom_prompt:e.target.value})} style={{marginBottom:8}} />
              )}
              {tbUserForm.name_purchased && (
                <input className="input" placeholder="自定义名称" value={tbUserForm.custom_name} onChange={e => setTbUserForm({...tbUserForm,custom_name:e.target.value})} style={{marginBottom:8}} />
              )}
              <div style={{display:'flex',gap:8}}>
                <button className="btn btn-primary btn-sm" onClick={saveTbUser}>保存</button>
                <button className="btn btn-outline btn-sm" onClick={() => setTbEditUser(null)}>取消</button>
              </div>
            </div>
          </div>
          </ModalPortal>
        )}
      </section>

      {/* Online Users + Push */}
      <section className="card">
        <div style={{display:'flex',justifyContent:'space-between',alignItems:'center',marginBottom:12}}>
          <h3 style={{margin:0}}>在线用户</h3>
          <div style={{display:'flex',gap:6}}>
            <button className="btn btn-sm btn-primary" onClick={() => openPushForm(null)}>通知全体</button>
            <button className="btn btn-sm btn-outline" onClick={loadOnlineUsers}>刷新</button>
          </div>
        </div>
        {onlineUsers.length === 0 ? <div className="empty-state">暂无在线用户</div> : onlineUsers.map((u: any) => (
          <div key={u.user_id} style={{display:'flex',justifyContent:'space-between',alignItems:'center',padding:'8px 12px',background:'var(--phx-bg)',borderRadius:10,marginBottom:6,fontSize:13,cursor:'pointer'}}
            onClick={() => { setSelUser(u); loadUserDetail(u.user_id); }}>
            <div style={{flex:1,minWidth:0}}>
              <span style={{fontWeight:600}}>{u.username || u.email || `ID:${u.user_id}`}</span>
              <span style={{marginLeft:8,fontSize:11,color:'var(--phx-text-secondary)'}}>{u.last_server_code ? `服 ${u.last_server_code}` : ''}</span>
              <span style={{marginLeft:8,fontSize:11,color:'var(--phx-text-secondary)'}}>{u.device_model || ''}</span>
              <span style={{marginLeft:8,fontSize:11,color:'var(--phx-text-secondary)'}}>{toCSTTime(u.last_heartbeat_at)}</span>
              <div style={{fontSize:11,color:'var(--phx-text-secondary)',marginTop:2}}>
                方块 {u.blocks_imported?.toLocaleString() || 0} | 导入 {u.import_sessions || 0} | 在线 {Math.floor((u.total_online_minutes || 0)/60)}h
              </div>
            </div>
            <button className="btn btn-sm btn-outline" style={{fontSize:11,padding:'4px 8px'}} onClick={e => { e.stopPropagation(); openPushForm(u.user_id); }}>通知</button>
          </div>
        ))}
        {/* User detail modal */}
        {selUser && (
          <ModalPortal>
          <div className="modal-backdrop" onClick={() => { setSelUser(null); setSelUserDetail(null); }}>
            <div className="modal-content" onClick={e => e.stopPropagation()} style={{maxWidth:480}}>
              <h3 style={{margin:'0 0 12px'}}>{selUserDetail?.username || selUser.email || `ID:${selUser.user_id}`}</h3>
              {selUserDetail ? (
                <div style={{fontSize:13,lineHeight:1.8}}>
                  <div>邮箱: {selUserDetail.email}</div>
                  <div>用户名: {selUserDetail.username}</div>
                  <div>累计方块: {selUserDetail.blocks_imported?.toLocaleString()}</div>
                  <div>建筑导入: {selUserDetail.buildings_imported}</div>
                  <div>建筑导出: {selUserDetail.buildings_exported}</div>
                  <div>地图画: {selUserDetail.mapart_completed}</div>
                  <div>皮肤: {selUserDetail.skin_completed}</div>
                  <div>导入次数: {selUserDetail.import_sessions}</div>
                  <div>导出次数: {selUserDetail.export_sessions}</div>
                  <div>在线时长: {Math.floor((selUserDetail.total_online_minutes || 0)/60)}h{(selUserDetail.total_online_minutes || 0)%60}m</div>
                  <div>设备型号: {selUserDetail.device_model || '-'}</div>
                  <div>最后心跳: {toCST(selUserDetail.last_heartbeat_at)}</div>
                  <div>最后服务器: {selUserDetail.last_server_code}</div>
                {selUserToolbox && (
                  <div style={{marginTop:8,paddingTop:8,borderTop:"1px solid var(--phx-border)"}}>
                    <div style={{fontWeight:600,marginBottom:4}}>工具箱配置</div>
                    <div>状态: {selUserToolbox.enabled ? "已开通" : "未开通"}{selUserToolbox.expires_at ? " 到期: " + selUserToolbox.expires_at.substring(0,10) : ""}</div>
                    <div>提示词: {selUserToolbox.prompt_purchased ? (selUserToolbox.custom_prompt || "已购(未设置)") : "未购买"}</div>
                    <div>命名: {selUserToolbox.name_purchased ? (selUserToolbox.custom_name || "已购(未设置)") : "未购买"}</div>
                    <div style={{fontSize:11,color:"var(--phx-text-secondary)"}}>默认提示词: {selUserToolbox.default_prompt || "(空)"}</div>
                    <div style={{fontSize:11,color:"var(--phx-text-secondary)"}}>默认名称: {selUserToolbox.default_name || "(空)"}</div>
                  </div>
                )}
                </div>
              ) : <div className="empty-state">加载中...</div>}
              <div style={{display:'flex',gap:8,marginTop:12}}>
                <button className="btn btn-primary btn-sm" onClick={() => { setSelUser(null); setSelUserDetail(null); openPushForm(selUser.user_id); }}>发送通知</button>
                <button className="btn btn-outline btn-sm" onClick={() => { setSelUser(null); setSelUserDetail(null); }}>关闭</button>
              </div>
            </div>
          </div>
          </ModalPortal>
        )}
        {/* Push form modal */}
        {pushForm && (
          <ModalPortal>
          <div className="modal-backdrop" onClick={() => setPushForm(null)}>
            <div className="modal-content" onClick={e => e.stopPropagation()} style={{maxWidth:480}}>
              <h3 style={{margin:'0 0 12px'}}>{pushTargetUser ? '发送通知给指定用户' : '发送通知给全体用户'}</h3>
              {pushTargetUser && <div style={{fontSize:12,color:'var(--phx-text-secondary)',marginBottom:8}}>目标用户 ID: {pushTargetUser}</div>}
              <select className="input" value={pushType} onChange={e => setPushType(e.target.value)} style={{marginBottom:8}}>
                <option value="admin_push">admin_push 管理员推送</option>
                <option value="version_update">version_update 版本更新</option>
                <option value="command">command 静默指令</option>
                <option value="announcement">announcement 公告</option>
              </select>
              <select className="input" value={pushDisplayMode} onChange={e => setPushDisplayMode(e.target.value)} style={{marginBottom:8}}>
                <option value="toast">toast 一次性提示</option>
                <option value="popup">popup 临时弹窗</option>
                <option value="banner">banner 常驻横幅</option>
                <option value="modal">modal 弹窗</option>
                <option value="marquee">marquee 游戏内顶部滚动</option>
                <option value="chat">chat 游戏聊天</option>
                <option value="tellraw">tellraw 游戏彩色消息</option>
                <option value="none">none 静默</option>
              </select>
              <input className="input" placeholder="标题" value={pushTitle} onChange={e => setPushTitle(e.target.value)} style={{marginBottom:8}} />
              <textarea className="input textarea" placeholder="正文" value={pushBody} onChange={e => setPushBody(e.target.value)} style={{minHeight:80,marginBottom:8}} />
              <div style={{display:'grid',gridTemplateColumns:'1fr 1fr',gap:8,marginBottom:8}}>
                <select className="input" value={pushAction} onChange={e => setPushAction(e.target.value)}>
                  <option value="none">none 无动作</option>
                  <option value="open_url">open_url 打开链接</option>
                  <option value="open_toolbox">open_toolbox 打开工具箱</option>
                  <option value="refresh_config">refresh_config 刷新配置</option>
                  <option value="command">command 游戏内指令</option>
                  <option value="launch_app">launch_app 启动应用</option>
                </select>
                <input className="input" placeholder={pushAction==='open_url'?'URL':pushAction==='command'?'游戏内指令':pushAction==='launch_app'?'Android 包名':'动作数据(URL等)'} value={pushActionData} onChange={e => setPushActionData(e.target.value)} />
              </div>
              <div style={{display:'flex',gap:8}}>
                <button className="btn btn-primary btn-sm" onClick={savePush}>发送</button>
                <button className="btn btn-outline btn-sm" onClick={() => setPushForm(null)}>取消</button>
              </div>
            </div>
          </div>
          </ModalPortal>
        )}
      </section>

      {/* Signatures */}
      <section className="card">
        <h3 style={{margin:'0 0 12px'}}>签名管理</h3>
        <div style={{display:'flex',gap:8,marginBottom:12}}>
          <input className="input" placeholder="包名" value={sigPkg} onChange={e => setSigPkg(e.target.value)} style={{flex:2}} />
          <input className="input" placeholder="SHA256 哈希" value={sigHash} onChange={e => setSigHash(e.target.value)} style={{flex:3}} />
          <input className="input" placeholder="标签" value={sigLabel} onChange={e => setSigLabel(e.target.value)} style={{flex:1}} />
          <button className="btn btn-primary btn-sm" style={{flexShrink:0}} onClick={async () => {
            if (!sigPkg || !sigHash) { showToast('请填写包名和哈希~', 'err'); return; }
            const r: any = await addSignature(sigPkg, sigHash, sigLabel);
            if (r.ok) { showToast('已添加', 'ok'); onRefresh(); setSigHash(''); setSigLabel(''); }
            else showToast(r.error || '哎呀,添加失败了,请检查后重试~', 'err');
          }}>添加</button>
        </div>
        {signatures.length === 0 ? <div className="empty-state">暂无签名记录</div> : signatures.map((s: any) => (
          <div key={s.id} style={{display:'flex',justifyContent:'space-between',alignItems:'center',padding:'6px 10px',background:'var(--phx-bg)',borderRadius:8,marginBottom:4,fontSize:12}}>
            <div style={{flex:1,minWidth:0}}>
              <span style={{fontWeight:600}}>{s.package_name}</span>
              {s.label && <span style={{marginLeft:6,color:'var(--phx-text-secondary)'}}>({s.label})</span>}
              <span style={{marginLeft:8,fontFamily:'monospace',fontSize:10,color:'var(--phx-text-secondary)'}}>{s.signature_hash?.substring(0,16)}...</span>
            </div>
            <button className="btn btn-sm btn-danger" style={{fontSize:10,padding:'2px 8px'}} onClick={() => showConfirm('删除签名', '确定删除此签名？', async () => { closeModal(); await deleteSignature(s.id); onRefresh(); })}>删除</button>
          </div>
        ))}
      </section>
    </div>
  );
}
