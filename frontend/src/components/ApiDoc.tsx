import { useState, useCallback } from 'react';
import ModalPortal from './ModalPortal';

interface Param { name: string; desc: string; example?: string }
interface RespField { name: string; desc: string }
interface EP {
  method: string; path: string; name: string; desc: string; rate: string;
  params?: Param[]; body?: Param[]; okFields?: RespField[]; note?: string;
  noTest?: boolean;
}

const ENDPOINTS: EP[] = [
  { method:'GET', path:'/api/auth/me', name:'当前用户', desc:'获取已登录用户的完整信息', rate:'无限制',
    okFields:[
      {name:'id',desc:'用户ID'},{name:'username',desc:'用户名'},{name:'email',desc:'邮箱'},
      {name:'role',desc:'admin/user'},{name:'token',desc:'API令牌'},{name:'nuts_balance',desc:'板栗余额'},
      {name:'subscription_start',desc:'订阅开始时间'},{name:'subscription_until',desc:'订阅到期时间'},
    ]},
  { method:'GET', path:'/api/auth/nuts', name:'板栗余额', desc:'板栗余额 + 最近30条交易', rate:'无限制',
    okFields:[{name:'balance',desc:'板栗数量'},{name:'transactions',desc:'交易数组'}],
    note:'交易 record 含 id/amount/reason/balance_after/created_at' },
  { method:'POST', path:'/api/auth/redeem', name:'兑换激活码', desc:'使用激活码兑换板栗', rate:'3次/60秒',
    body:[{name:'code',desc:'激活码',example:'XXXX-XXXX-XXXX'}],
    okFields:[{name:'amount',desc:'获得板栗数'},{name:'message',desc:'提示'}] },
  { method:'POST', path:'/api/auth/nuts/to-code', name:'积分转兑换码', desc:'把自身板栗按 10:9 向下取整生成兑换码（差额不返还）', rate:'1次/3秒',
    body:[{name:'amount',desc:'要转换的积分数量(≥10)',example:'10'}],
    okFields:[{name:'code',desc:'生成的兑换码'},{name:'code_value',desc:'兑换码面值(floor(amount*0.9))'}] },
  { method:'GET', path:'/api/auth/tokens', name:'令牌列表', desc:'我的子令牌列表 + 每令牌调用次数 + 总调用次数', rate:'无限制',
    okFields:[{name:'tokens',desc:'子令牌数组(id/name/max_calls/max_nuts/call_count/nuts_consumed/disabled)'},{name:'total_calls',desc:'总调用次数'},{name:'primary_token',desc:'主令牌'}] },
  { method:'POST', path:'/api/auth/tokens', name:'创建令牌', desc:'创建一个可限制的子令牌（主令牌之外的令牌）', rate:'无限制',
    body:[{name:'name',desc:'令牌名称',example:'工作机'},{name:'max_calls',desc:'累计调用上限，-1=不限',example:'-1'},{name:'max_nuts',desc:'累计消耗积分上限，-1=不限',example:'-1'}],
    okFields:[{name:'token',desc:'完整令牌值（仅此一次返回，请立即保存）'}] },
  { method:'POST', path:'/api/auth/tokens/{id}/reset', name:'重置令牌', desc:'重新生成指定子令牌的值', rate:'无限制',
    okFields:[{name:'token',desc:'新的完整令牌值'}] },
  { method:'POST', path:'/api/auth/tokens/{id}/update', name:'更新令牌', desc:'修改名称/上限/停用状态', rate:'无限制',
    body:[{name:'name',desc:'新名称(可选)'},{name:'max_calls',desc:'调用上限(可选)'},{name:'max_nuts',desc:'积分上限(可选)'},{name:'disabled',desc:'停用(可选)'}],
    okFields:[{name:'ok',desc:'成功为true'}] },
  { method:'DELETE', path:'/api/auth/tokens/{id}/delete', name:'删除令牌', desc:'删除指定子令牌', rate:'无限制',
    okFields:[{name:'ok',desc:'成功为true'}] },

  { method:'GET', path:'/api/server/ls', name:'服务器列表', desc:'浏览租赁服，排序+分页', rate:'20次/10秒',
    params:[{name:'sort_type',desc:'0综合/1在线/2最新/3最受欢迎',example:'3'},{name:'order_type',desc:'0降/1升',example:'0'},{name:'offset',desc:'分页，每页30',example:'0'}],
    okFields:[{name:'total',desc:'总数'},{name:'servers',desc:'列表'}] },
  { method:'GET', path:'/api/server/find', name:'搜索服务器', desc:'按房间号搜索', rate:'20次/10秒',
    params:[{name:'keyword',desc:'服务器房间号',example:'1234567'}], okFields:[{name:'servers',desc:'结果'}] },
  { method:'GET', path:'/api/server/detail', name:'服务器详情', desc:'单个服务器完整信息', rate:'20次/10秒',
    params:[{name:'server_id',desc:'19位ID',example:'1234567890123456789'}], okFields:[{name:'server',desc:'详情对象'}] },
  { method:"GET", path:"/api/server/owner", name:"玩家信息", desc:"查看指定玩家的公开资料", rate:"20次/10秒", params:[{name:"uid",desc:"玩家UID",example:"1234567890"}], okFields:[{name:"user",desc:"nickname/headImage/pe_growth/signature"}] },
  { method:'GET', path:'/api/server/players', name:'房间玩家', desc:'在线玩家列表', rate:'20次/10秒',
    params:[{name:'server_id',desc:'19位ID',example:'1234567890123456789'},{name:'offset',desc:'分页',example:'0'},{name:'length',desc:'最大50',example:'20'}],
    okFields:[{name:'players',desc:'玩家数组(name/level/head_image)'}] },
  { method:'GET', path:'/api/social/search', name:'搜索玩家', desc:'昵称或邮箱搜索', rate:'20次/10秒',
    params:[{name:'keyword',desc:'昵称或邮箱',example:'test'}], okFields:[{name:'users',desc:'结果'}] },
  { method:'GET', path:'/api/social/requests', name:'好友申请', desc:'待处理好友申请', rate:'20次/10秒',
    okFields:[{name:'requests',desc:'申请列表'}] },
  { method:'POST', path:'/api/social/apply', name:'好友申请', desc:'向指定玩家发送好友申请', rate:'20次/10秒',
    body:[{name:'uid',desc:'目标玩家UID',example:'1234567890'}],
    okFields:[{name:'code',desc:'状态码'},{name:'message',desc:'返回信息'}] },
  { method:'POST', path:'/api/social/reply', name:'处理好友', desc:'同意或拒绝好友申请', rate:'20次/10秒',
    body:[{name:'uid',desc:'目标玩家UID',example:'1234567890'},{name:'accept',desc:'true同意/false拒绝',example:'true'}],
    okFields:[{name:'code',desc:'状态码'},{name:'message',desc:'返回信息'}] },
  { method:'GET', path:'/api/social/messages', name:'查看私信', desc:'查看与指定玩家的私信记录', rate:'20次/10秒',
    params:[{name:'uid',desc:'对方UID(可选)',example:'1234567890'},{name:'page',desc:'页码',example:'1'},{name:'num',desc:'每页数量',example:'10'}],
    okFields:[{name:'messages',desc:'消息列表'}] },
  { method:'POST', path:'/api/social/moment', name:'发动态', desc:'发送一条游戏动态', rate:'20次/10秒',
    body:[{name:'content',desc:'动态内容',example:'今天玩得很开心！'}],
    okFields:[{name:'code',desc:'状态码'},{name:'message',desc:'返回信息'}] },
  { method:'POST', path:'/api/social/like', name:'点赞动态', desc:'给指定动态点赞', rate:'20次/10秒',
    body:[{name:'msg_id',desc:'动态ID',example:'abc123'},{name:'push_id',desc:'推送ID(可选)',example:'0'}],
    okFields:[{name:'code',desc:'状态码'},{name:'message',desc:'返回信息'}] },
  { method:'GET', path:'/api/lobby/search', name:'联机大厅', desc:'搜索大厅房间', rate:'20次/10秒',
    params:[{name:'keyword',desc:'搜索词',example:'生存'}, {name:'offset',desc:'分页',example:'0'}], okFields:[{name:'rooms',desc:'房间列表(room_name/cur_num/max_count)'}] },
  { method:'GET', path:'/api/lobby/room', name:'房间详情', desc:'查看联机大厅房间详情', rate:'20次/10秒',
    params:[{name:'room_id',desc:'房间ID',example:'1234567890123456789'}],
    okFields:[{name:'room',desc:'房间详情对象'}] },
  { method:'POST', path:'/api/lobby/enter', name:'进入房间', desc:'进入联机大厅房间', rate:'20次/10秒',
    body:[{name:'room_id',desc:'房间ID'},{name:'password',desc:'密码(可选)'}],
    okFields:[{name:'code',desc:'状态码'},{name:'room',desc:'房间信息'}] },
  { method:'GET', path:'/api/hot', name:'热门话题', desc:'获取当前热门话题/分类', rate:'20次/10秒',
    okFields:[{name:'categories',desc:'热门分类列表'}] },
  { method:'GET', path:'/api/skin/presets', name:'皮肤预设', desc:'预设皮肤列表（需登录）', rate:'无限制',
    okFields:[{name:'presets',desc:'预设数组(id/name/item_id/preview_url)'}] },
  { method:'POST', path:'/api/skin/change', name:'更换皮肤', desc:'更换账号皮肤', rate:'6次/7秒',
    body:[{name:'item_id',desc:'19位组件ID',example:'1234567890123456789'},{name:'account_id',desc:'目标账号(可选)',example:'1'}],
    okFields:[{name:'skin_url',desc:'新皮肤纹理地址'},{name:'message',desc:'成功提示'}] },
  { method:'POST', path:'/api/server/like', name:'点赞服务器', desc:'给指定租赁服点赞或取消', rate:'20次/10秒',
    body:[{name:'server_id',desc:'服务器房间号',example:'1234567'},{name:'is_like',desc:'true点赞/false取消',example:'true'}],
    okFields:[{name:'code',desc:'0=成功'},{name:'message',desc:'返回信息'}] },
  { method:'POST', path:'/api/accounts/rotate', name:'轮换账号', desc:'切换到下一个可用的私有账号，全部切完才重置', rate:'6次/7秒',
    okFields:[{name:'account_id',desc:'新活跃账号ID'},{name:'display_name',desc:'账号名'},{name:'uid',desc:'游戏UID'}] },
  { method:'GET', path:'/api/accounts', name:'私有账号列表', desc:'返回当前用户的所有私有和共享池账号', rate:'无限制',
    okFields:[{name:'accounts',desc:'账号数组'},{name:'accounts[].id',desc:'账号ID'},{name:'accounts[].display_name',desc:'显示名'},{name:'accounts[].uid',desc:'游戏UID'},{name:'accounts[].status',desc:'状态 normal/banned/offline'},{name:'accounts[].is_shared',desc:'是否共享池'},{name:'accounts[].is_active',desc:'是否当前活跃'},{name:'accounts[].auto_refresh_enabled',desc:'自动刷新开关'}] },
  { method:'GET', path:'/api/accounts/active', name:'当前活跃账号', desc:'返回当前活跃账号的完整信息(含access_game_flag/realname)', rate:'无限制',
    okFields:[{name:'id',desc:'账号ID'},{name:'display_name',desc:'显示名'},{name:'uid',desc:'游戏UID'},{name:'status',desc:'状态'},{name:'growth_level',desc:'成长等级'},{name:'access_game_flag',desc:'封禁标记(非0=封禁)'}] },
  { method:'POST', path:'/api/accounts/{id}/switch', name:'切换活跃账号', desc:'切换到指定ID的游戏账号为当前活跃', rate:'6次/7秒',
    okFields:[{name:'message',desc:'结果消息'}] },
  { method:'POST', path:'/api/accounts/add', name:'添加账号', desc:'添加游戏账号(Cookie)', rate:'无限制',
    body:[{name:'cookie',desc:'Cookie JSON字符串',example:'{"sauth_json":"..."}'},{name:'shared',desc:'是否共享池',example:'false'}],
    okFields:[{name:'ok',desc:'成功为true'},{name:'id',desc:'新账号ID'},{name:'display_name',desc:'显示名'},{name:'uid',desc:'游戏UID'}] },
  { method:'DELETE', path:'/api/accounts/{id}', name:'删除账号', desc:'删除指定游戏账号', rate:'无限制',
    okFields:[{name:'ok',desc:'成功为true'}] },
  { method:'POST', path:'/api/accounts/{id}/refresh', name:'刷新账号', desc:'重新验证并刷新账号信息', rate:'1次/4秒',
    okFields:[{name:'ok',desc:'成功为true'},{name:'display_name',desc:'显示名'},{name:'status',desc:'最新状态'}] },
  { method:'POST', path:'/api/accounts/{id}/update', name:'更新账号', desc:'更新账号显示名/共享状态/自动刷新开关', rate:'无限制',
    body:[{name:'display_name',desc:'新显示名(可选)'},{name:'shared',desc:'是否共享池(可选)'},{name:'auto_refresh_enabled',desc:'自动刷新开关(可选)'}],
    okFields:[{name:'ok',desc:'成功为true'}] },
  { method:'POST', path:'/api/accounts/{id}/nickname', name:'修改昵称', desc:'修改游戏内昵称', rate:'1次/5分钟',
    body:[{name:'nickname',desc:'新昵称',example:'Steve'}],
    okFields:[{name:'ok',desc:'成功为true'}] },

  // ── 服主 ──
  { method:'GET', path:'/api/accounts/server-owners', name:'服主账号列表', desc:'查看当前用户的所有服主账号', rate:'无限制',
    okFields:[{name:'accounts',desc:'服主账号数组(id/display_name/uid)'}] },
  { method:'POST', path:'/api/accounts/{id}/server-owner', name:'设置服主', desc:'将账号标记为服主/取消服主', rate:'无限制',
    body:[{name:'enabled',desc:'true设为服主/false取消',example:'true'}],
    okFields:[{name:'ok',desc:'成功为true'}] },
  { method:'GET', path:'/api/owner/servers', name:'服主服务器列表', desc:'获取服主账号的租赁服和山头服列表', rate:'无限制',
    okFields:[{name:'accounts',desc:'服主账号及服务器列表'}] },
  { method:'POST', path:'/api/owner/servers/refresh', name:'刷新服主服务器', desc:'强制重新认证并刷新服主服务器列表', rate:'无限制',
    okFields:[{name:'accounts',desc:'刷新后的服主列表'}] },

  // ── 租赁服管理 ──
  { method:'GET', path:'/api/rental/list', name:'租赁服列表', desc:'获取我的租赁服列表', rate:'无限制',
    params:[{name:'account_id',desc:'目标账号ID(可选)',example:'1'}],
    okFields:[{name:'entities',desc:'租赁服数组(entity_id/name/status/player_count/capacity)'}] },
  { method:'GET', path:'/api/rental/status', name:'租赁服状态', desc:'查看单个租赁服状态', rate:'无限制',
    params:[{name:'server_id',desc:'19位ID',example:'1234567890123456789'},{name:'account_id',desc:'目标账号ID(可选)',example:'1'}],
    okFields:[{name:'status',desc:'服务器状态'}] },
  { method:'POST', path:'/api/rental/control', name:'租赁服控制', desc:'启动/关闭租赁服', rate:'无限制',
    body:[{name:'server_id',desc:'19位ID',example:'1234567890123456789'},{name:'status',desc:'1=启动/0=关闭',example:'1'},{name:'account_id',desc:'目标账号ID(可选)',example:'1'}],
    okFields:[{name:'ok',desc:'成功为true'},{name:'message',desc:'操作结果'}] },
  { method:'POST', path:'/api/rental/update', name:'租赁服更新', desc:'更新租赁服配置（名称/简介/等级/可见性/密码等）', rate:'无限制',
    body:[{name:'server_id',desc:'19位ID',example:'1234567890123456789'},{name:'server_name',desc:'服务器名称(可选)',example:'我的租赁服'},{name:'brief_summary',desc:'简介(可选)',example:'欢迎来玩'},{name:'min_level',desc:'最低进入等级(可选)',example:'0'},{name:'visibility',desc:'可见性(可选)',example:'0'},{name:'pwd',desc:'密码(可选)',example:'123456'},{name:'image_url',desc:'图片URL(可选)'}],
    okFields:[{name:'ok',desc:'成功为true'},{name:'entity',desc:'更新后的实体信息'}] },

  // ── 头像 ──
  { method:'GET', path:'/api/avatar/list', name:'头像列表', desc:'获取可用头像列表', rate:'无限制',
    okFields:[{name:'avatars',desc:'头像数组'}] },
  { method:'POST', path:'/api/avatar/change', name:'更换头像', desc:'更换账号头像', rate:'无限制',
    body:[{name:'head_image',desc:'头像图片URL',example:'https://example.com/avatar.png'},{name:'account_id',desc:'目标账号ID(可选)',example:'1'}],
    okFields:[{name:'ok',desc:'成功为true'}] },
  { method:'POST', path:'/api/avatar/upload', name:'上传头像', desc:'上传自定义头像图片（multipart/form-data）', rate:'无限制',
    body:[{name:'file',desc:'图片文件',example:'@avatar.png'},{name:'account_id',desc:'目标账号ID(可选)',example:'1'}],
    note:'使用 multipart/form-data 上传，非 JSON 格式',
    okFields:[{name:'ok',desc:'成功为true'},{name:'url',desc:'头像URL'}] },
];

export default function ApiDoc({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [expanded, setExpanded] = useState<number | null>(null);
  const [testing, setTesting] = useState<number | null>(null);
  const [copiedIdx, setCopiedIdx] = useState<number | null>(null);
  if (!open) return null;

  const copyCurl = async (ep: EP, i: number) => {
    const token = localStorage.getItem('session_token') || '';
    let curl = `curl -X ${ep.method} '${window.location.origin}${ep.path}`;
    if (ep.params?.length) {
      const qs = ep.params.map(p => `${p.name}=${p.example||''}`).join('&');
      curl += `?${qs}`;
    }
    curl += `'`;
    if (token) curl += ` \\\n  -H 'Authorization: Bearer ${token}'`;
    if (ep.method === 'POST') {
      curl += ` \\\n  -H 'Content-Type: application/json'`;
      const bodyObj: Record<string,string> = {};
      ep.body?.forEach(p => { bodyObj[p.name] = p.example || ''; });
      curl += ` \\\n  -d '${JSON.stringify(bodyObj)}'`;
    }
    await navigator.clipboard.writeText(curl);
    setCopiedIdx(i);
    setTimeout(() => setCopiedIdx(null), 1500);
  };

  return (
    <ModalPortal>
    <div className="modal-backdrop" onClick={onClose} style={{zIndex:2000}}>
      <div className="modal-content" onClick={e => e.stopPropagation()} style={{
        maxWidth:660, maxHeight:'94dvh', overflowY:'auto', padding:'20px 24px 96px',
      }}>
        <div style={{display:'flex',alignItems:'center',justifyContent:'space-between',marginBottom:12}}>
          <h2 style={{margin:0,fontSize:18}}>API 文档</h2>
          <button className="modal-close" onClick={onClose} aria-label="关闭" title="关闭">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <p style={{fontSize:12,color:'var(--phx-text-secondary)',margin:'0 0 16px'}}>
          需 <code>Authorization: Bearer &lt;token&gt;</code> 或 URL 参数 <code>login_token=</code>
        </p>

        {ENDPOINTS.map((ep, i) => {
          const isOpen = expanded === i;
          return (
          <div key={i} style={{
            border:'1px solid var(--phx-border-light)', borderRadius:10,
            padding:'10px 14px', marginBottom:8, background:'var(--phx-bg-card)',
            cursor:'pointer',
          }} onClick={() => setExpanded(isOpen ? null : i)}>

            <div style={{display:'flex',alignItems:'flex-start',justifyContent:'space-between'}}>
              <div style={{flex:1}}>
                <div style={{fontSize:13,fontWeight:700,color:'var(--phx-text)',marginBottom:4}}>{ep.name}</div>
                <div style={{display:'flex',alignItems:'center',gap:8}}>
                  <span style={{fontWeight:700,fontSize:11,color:ep.method==='GET'?'#4f9d53':'#d6a21d',minWidth:42}}>{ep.method}</span>
                  <code style={{fontSize:12}}>{ep.path}</code>
                  <span style={{fontSize:11,color:'#8b7a61',marginLeft:'auto'}}>{ep.rate}</span>
                </div>
                <div style={{fontSize:12,color:'var(--phx-text-secondary)',marginTop:4}}>{ep.desc}</div>
              </div>
              <button className="btn btn-sm" onClick={e => { e.stopPropagation(); copyCurl(ep, i); }}
                style={{minWidth:56,fontSize:11,marginLeft:8,background:copiedIdx===i?'#4f9d53':'var(--phx-bg-card)',color:copiedIdx===i?'#fff':'var(--phx-text)',border:'1px solid var(--phx-border)',borderRadius:6,padding:'4px 10px',cursor:'pointer'}}>
                {copiedIdx === i ? '已复制' : '复制 curl'}
              </button>
            </div>

            {isOpen && (
              <div style={{marginTop:10,paddingTop:10,borderTop:'1px solid var(--phx-border-light)'}} onClick={e => e.stopPropagation()}>
                {ep.note && <p style={{fontSize:11,color:'#9f927d',margin:'0 0 8px'}}>{ep.note}</p>}
                {ep.params && (
                  <div style={{marginBottom:8}}>
                    <div style={{fontSize:12,fontWeight:600,color:'#5d4b36',marginBottom:4}}>URL 参数</div>
                    {ep.params.map(p => (
                      <div key={p.name} style={{fontSize:12,marginLeft:8,marginBottom:2}}>
                        <code>{p.name}</code> <span style={{color:'#8b7a61'}}>{p.desc}</span>
                        {p.example && <code style={{color:'#cf3f7a',marginLeft:4}}>例:{p.example}</code>}
                      </div>
                    ))}
                  </div>
                )}
                {ep.body && (
                  <div style={{marginBottom:8}}>
                    <div style={{fontSize:12,fontWeight:600,color:'#5d4b36',marginBottom:4}}>请求体</div>
                    {ep.body.map(p => (
                      <div key={p.name} style={{fontSize:12,marginLeft:8,marginBottom:2}}>
                        <code>{p.name}</code> <span style={{color:'#8b7a61'}}>{p.desc}</span>
                        {p.example && <code style={{color:'#cf3f7a',marginLeft:4}}>例:{p.example}</code>}
                      </div>
                    ))}
                  </div>
                )}
                {ep.okFields && (
                  <div style={{marginBottom:8}}>
                    <div style={{fontSize:12,fontWeight:600,color:'#4f9d53',marginBottom:4}}>响应字段</div>
                    {ep.okFields.map(f => (
                      <div key={f.name} style={{fontSize:12,marginLeft:8,marginBottom:2}}>
                        <code>{f.name}</code> <span style={{color:'#8b7a61'}}>{f.desc}</span>
                      </div>
                    ))}
                  </div>
                )}
                {!ep.noTest && <button className="btn btn-sm btn-outline" onClick={(e) => { e.stopPropagation(); setTesting(testing === i ? null : i); }}>
                  {testing === i ? '收起测试' : '测试'}
                </button>}
                {testing === i && !ep.noTest && <Tester ep={ep} />}
              </div>
            )}
          </div>
        )})}

        <h3 style={{fontSize:14,margin:'20px 0 10px',color:'#e05a5a'}}>常见错误</h3>
        <div style={{fontSize:12,lineHeight:1.8,color:'#5d4b36'}}>
          <div><code>凭据已过期</code> — Cookie 失效，重新登录</div>
          <div><code>活跃账号已失效</code> — 切换其他账号</div>
          <div><code>服务器繁忙</code> — 稍后重试</div>
          <div><code>操作太频繁</code> — 等待限制</div>
          <div><code>板栗不足</code> — 兑换或等待</div>
          <div><code>该账号已被封禁</code> — 更换账号</div>
        </div>
        <button className="btn btn-outline" style={{width:'100%',marginTop:16}} onClick={onClose}>关闭</button>
      </div>
    </div>
    </ModalPortal>
  );
}

/* ── tester ── */
function Tester({ ep }: { ep: EP }) {
  const token = localStorage.getItem('session_token') || '';

  const initValues = useCallback(() => {
    const v: Record<string,string> = {};
    ep.params?.forEach(p => { if (p.example) v[p.name] = p.example; });
    return v;
  }, [ep]);

  const initBody = useCallback(() => {
    if (!ep.body?.length) return '';
    const obj: Record<string,string> = {};
    ep.body.forEach(p => { if (p.example) obj[p.name] = p.example; });
    return JSON.stringify(obj, null, 2);
  }, [ep]);

  const [values, setValues] = useState<Record<string,string>>(initValues);
  const [bodyJson, setBodyJson] = useState(initBody);
  const [authOverride, setAuthOverride] = useState('');
  const [resp, setResp] = useState('');
  const [status, setStatus] = useState('');
  const [sending, setSending] = useState(false);

  const activeToken = authOverride || token;

  const buildURL = () => {
    let url = ep.path;
    const qs = Object.entries(values).filter(([,v])=>v).map(([k,v])=>k+'='+encodeURIComponent(v)).join('&');
    if (qs) url += '?' + qs;
    return url;
  };

  const send = async () => {
    setSending(true); setResp(''); setStatus('');
    try {
      const url = buildURL();
      const headers: Record<string,string> = { 'Content-Type': 'application/json' };
      if (activeToken) headers['Authorization'] = 'Bearer ' + activeToken;
      const opts: RequestInit = { method: ep.method, headers };
      if (ep.method === 'POST') opts.body = bodyJson || '{}';
      const res = await fetch(url, opts);
      setStatus(res.status + ' ' + res.statusText);
      const text = await res.text();
      try { setResp(JSON.stringify(JSON.parse(text), null, 2)); } catch { setResp(text); }
    } catch (e: any) { setStatus('错误'); setResp(e.message); }
    setSending(false);
  };

  return (
    <div style={{border:'1px dashed var(--phx-primary)',borderRadius:10,padding:10,marginTop:8,background:'#f8fdf9'}} onClick={e => e.stopPropagation()}>
      <div style={{fontSize:11,color:'#8b7a61',marginBottom:8,wordBreak:'break-all'}}>
        <span style={{fontWeight:700,color:ep.method==='GET'?'#4f9d53':'#d6a21d'}}>{ep.method}</span>{' '}
        <code style={{fontSize:11}}>{buildURL()}</code>
      </div>

      {/* Auth override */}
      <div style={{display:'flex',alignItems:'center',gap:8,marginBottom:6}}>
        <code style={{fontSize:11,minWidth:80,color:'#8b7a61'}}>Authorization</code>
        <input className="input" style={{flex:1,fontSize:12,height:30}}
          placeholder={token ? `当前: ${token.slice(0,20)}...` : '输入 Bearer token 覆盖默认'}
          value={authOverride}
          onChange={e => setAuthOverride(e.target.value)} />
      </div>

      {/* Params */}
      {ep.params?.map(p => (
        <div key={p.name} style={{display:'flex',alignItems:'center',gap:8,marginBottom:4}}>
          <code style={{fontSize:11,minWidth:80}}>{p.name}</code>
          <input className="input" style={{flex:1,fontSize:12,height:30}}
            placeholder={p.example || p.desc}
            value={values[p.name] ?? ''}
            onChange={e => setValues({...values, [p.name]: e.target.value})} />
        </div>
      ))}

      {/* Body */}
      {ep.method === 'POST' && !!ep.body?.length && (
        <textarea className="input textarea" rows={3} style={{width:'100%',fontSize:12,marginBottom:6}}
          value={bodyJson} onChange={e => setBodyJson(e.target.value)} />
      )}

      <button className="btn btn-primary btn-sm" onClick={send} disabled={sending} style={{width:'100%'}}>
        {sending ? '发送中...' : '发送'}
      </button>

      {status && (
        <div style={{marginTop:8,fontSize:12}}>
          <span style={{fontWeight:600,color:status.startsWith('2')?'#4f9d53':'#d9534f'}}>HTTP {status}</span>
        </div>
      )}
      {resp && (
        <pre style={{background:'#1e1e1e',color:'#d4d4d4',borderRadius:8,padding:10,marginTop:4,fontSize:11,whiteSpace:'pre-wrap',maxHeight:200,overflow:'auto'}}>{resp}</pre>
      )}
    </div>
  );
}
