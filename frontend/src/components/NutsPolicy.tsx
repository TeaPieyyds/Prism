import ModalPortal from './ModalPortal';

export default function NutsPolicy({ open, onClose }: { open: boolean; onClose: () => void }) {
  if (!open) return null;

  return (
    <ModalPortal>
    <div className="modal-backdrop" onClick={onClose} style={{zIndex: 2000}}>
      <div className="modal-content" onClick={e => e.stopPropagation()} style={{
        maxWidth: 520, maxHeight: '90dvh', overflowY: 'auto', padding: '28px 24px 96px',
      }}>
        <div style={{display:'flex',alignItems:'center',justifyContent:'space-between',marginBottom:4}}>
          <h2 style={{margin:0,fontSize:20,color:'var(--phx-text)'}}>板栗规则说明</h2>
          <button className="modal-close" onClick={onClose} aria-label="关闭" title="关闭">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>
        <p style={{fontSize: 13, color: 'var(--phx-text-secondary)', margin: '0 0 20px'}}>
          板栗 是系统的虚拟货币，用于进服消耗和管理奖励。
        </p>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-success)'}}>获取方式</h3>
          <div style={{display: 'flex', flexDirection: 'column', gap: 6}}>
            <RuleRow label="注册账号" value="+50 " />
            <RuleRow label="每日签到" value="+1，连续第7天+3" />
            <RuleRow label="添加游戏账号" value="+5 " />
            <RuleRow label="账号设为共享" value="+5 " />
            <RuleRow label="共享账号被他人使用" value="+1 次" />
            <RuleRow label="上传建筑" value="按后台配置" />
            <RuleRow label="在线购买板栗" value="1元=10颗" />
            <RuleRow label="兑换激活码" value="数额不定" />
          </div>
        </section>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-success)'}}>邀请返利</h3>
          <div style={{display: 'flex', flexDirection: 'column', gap: 6}}>
            <RuleRow label="被邀请人注册（邀请者）" value="+120 " />
            <RuleRow label="被邀请人注册（被邀请者）" value="+20 " />
            <RuleRow label="被邀请人首次进服（邀请者）" value="+6 " />
            <RuleRow label="被邀请人进服 10 次（邀请者）" value="+10 " />
          </div>
          <p style={{fontSize:12, color:'var(--phx-text-secondary)', margin:'6px 0 0'}}>邀请码在"网页用户"页面查看和复制</p>
        </section>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-error)'}}>消耗 / 扣除</h3>
          <div className="nuts-policy-bg" style={{borderRadius: 12, padding: 12, marginBottom: 12}}>
            <div style={{display: 'flex', justifyContent: 'space-between', fontSize: 13, padding: '4px 0'}}>
              <span>使用<strong>自己</strong>的账号进服</span>
              <span style={{color: 'var(--phx-error)', fontWeight: 600}}>-1 </span>
            </div>
            <div style={{display: 'flex', justifyContent: 'space-between', fontSize: 13, padding: '4px 0'}}>
              <span>使用<strong>共享池</strong>的账号进服</span>
              <span style={{color: 'var(--phx-error)', fontWeight: 600}}>-2 </span>
            </div>
            <div style={{display: 'flex', justifyContent: 'space-between', fontSize: 13, padding: '4px 0'}}>
              <span>取消账号共享</span>
              <span style={{color: 'var(--phx-error)', fontWeight: 600}}>-5 </span>
            </div>
          </div>
          <p style={{fontSize:12, color:'var(--phx-text-secondary)', margin:0}}>被封禁或重复的账号不奖励板栗</p>
        </section>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-primary)'}}>订阅期（管理员授予）</h3>
          <div className="nuts-policy-teal" style={{fontSize: 13, lineHeight: 1.8, borderRadius: 10, padding: 12, color: 'var(--phx-text)'}}>
            <p style={{margin: 0}}>订阅期内所有内容<strong>不消耗板栗</strong>，由管理员授予，用户不可自行购买或修改：</p>
            <ul style={{margin: '8px 0 0', paddingLeft: 18, lineHeight: 1.9}}>
              <li>进服、市场下载、工具箱等均<strong>免费</strong>（不扣板栗）</li>
              <li>不可<strong>兑换激活码</strong>、<strong>抢/发红包</strong>、<strong>生成兑换码/答题码</strong></li>
              <li>订阅期内<strong>不显示</strong>板栗余额与积分变更记录</li>
            </ul>
          </div>
        </section>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-primary)'}}>10分钟窗口机制</h3>
          <div style={{fontSize: 13, lineHeight: 1.8, color: 'var(--phx-text)'}}>
            <p style={{margin: '0 0 8px'}}>在 <strong>10 分钟</strong>内重复进入<strong>同一个</strong>服务器，享受折扣：</p>
            <div className="nuts-policy-teal" style={{borderRadius: 10, padding: 12}}>
              <div style={{display: 'flex', justifyContent: 'space-between', padding: '4px 0', fontSize: 13}}>
                <span>自己的账号</span>
                <span style={{fontWeight: 600, color: 'var(--phx-success)'}}>0 </span>
              </div>
              <div style={{display: 'flex', justifyContent: 'space-between', padding: '4px 0', fontSize: 13}}>
                <span>共享池账号</span>
                <span style={{color: 'var(--phx-error)', fontWeight: 600}}>-1 </span>
              </div>
            </div>
            <p style={{margin: '8px 0 0'}}>如果在窗口期间<strong>进了其他服务器</strong>，原服务器的窗口立即失效，恢复原价。</p>
          </div>
        </section>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-text)'}}>举例</h3>
          <div className="nuts-policy-bg" style={{fontSize: 12, borderRadius: 10, padding: 12, fontFamily: 'monospace', lineHeight: 2}}>
            <div>14:00 进服 A（自己）&nbsp;&nbsp; <span style={{color:'var(--phx-error)'}}>-1</span></div>
            <div>14:05 进服 A（自己）&nbsp;&nbsp; <span style={{color:'var(--phx-success)'}}>0</span> &nbsp;窗口内重进</div>
            <div>14:07 进服 B（自己）&nbsp;&nbsp; <span style={{color:'var(--phx-error)'}}>-1</span> &nbsp;新服全价，A窗口失效</div>
            <div>14:08 进服 A（自己）&nbsp;&nbsp; <span style={{color:'var(--phx-error)'}}>-1</span> &nbsp;窗口已被B打破</div>
          </div>
        </section>

        <section style={{marginBottom: 20}}>
          <h3 style={{fontSize: 15, margin: '0 0 8px', color: 'var(--phx-text)'}}>注意事项</h3>
          <ul style={{fontSize: 13, color: 'var(--phx-text)', lineHeight: 1.8, paddingLeft: 20, margin: 0}}>
            <li>余额不足无法进服</li>
            <li>进服失败不扣除板栗</li>
            <li>板栗余额为负数不会发生</li>
            <li>获取激活码请加入 QQ 群（点击用户页兑换旁的 ?）</li>
          </ul>
        </section>

        <button className="btn btn-primary" style={{width: '100%'}} onClick={onClose}>知道了</button>
      </div>
    </div>
    </ModalPortal>
  );
}

function RuleRow({ label, value }: { label: string; value: string }) {
  return (
    <div style={{display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 12px', background: 'var(--phx-bg-card)', borderRadius: 8, fontSize: 13}}>
      <span>{label}</span>
      <span style={{fontWeight: 700, color: 'var(--phx-success)'}}>{value}</span>
    </div>
  );
}
