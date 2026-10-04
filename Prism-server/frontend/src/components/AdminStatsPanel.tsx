import { useEffect, useState, useRef } from 'react';
import { Chart, registerables } from 'chart.js';
import { getAdminStatsOverview, getAdminEndpointStats, getAdminPerUserStats, getAdminRegistrationStats, getAdminUserDetailStats, getAdminErrorStats, getAdminBusyHours, getAdminMethodStats } from '../api';
import ModalPortal from './ModalPortal';

Chart.register(...registerables);

type Granularity = 'minute' | '15min' | 'hour' | 'day' | 'week' | 'month';

const COLORS = ['#4f9d53', '#3b82f6', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#f97316', '#6366f1'];
const GRAN_OPTIONS: { v: Granularity; l: string }[] = [
  { v: 'minute', l: '分' }, { v: '15min', l: '15分' }, { v: 'hour', l: '时' }, { v: 'day', l: '天' }, { v: 'week', l: '周' }, { v: 'month', l: '月' },
];
const RANGE_OPTIONS = [
  { v: 1, l: '1h' }, { v: 24, l: '24h' }, { v: 168, l: '7天' }, { v: 720, l: '30天' },
];

export default function AdminStatsPanel() {
  const [overview, setOverview] = useState<any>(null);
  const [endpoints, setEndpoints] = useState<any[]>([]);
  const [perUser, setPerUser] = useState<any[]>([]);
  const [registrations, setRegistrations] = useState<any[]>([]);
  const [errorStats, setErrorStats] = useState<any[]>([]);
  const [busyHours, setBusyHours] = useState<any[]>([]);
  const [methodStats, setMethodStats] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [granularity, setGranularity] = useState<Granularity>('hour');
  const [timeRange, setTimeRange] = useState(168);
  const [regGranularity, setRegGranularity] = useState<'day' | 'week'>('day');
  const [sortField, setSortField] = useState<'user_id' | 'total_calls' | 'success' | 'success_rate' | 'join_count' | 'nuts_used'>('total_calls');
  const [sortAsc, setSortAsc] = useState(false);
  const [detailUser, setDetailUser] = useState<any>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [excludeTest, setExcludeTest] = useState(true);

  const openUserDetail = async (userId: number) => {
    setDetailUser(null);
    setDetailLoading(true);
    const res = await getAdminUserDetailStats(userId);
    if ((res as any).ok) setDetailUser((res as any).user);
    setDetailLoading(false);
  };

  const epChartRef = useRef<HTMLCanvasElement>(null);
  const epChartInst = useRef<Chart | null>(null);
  const regChartRef = useRef<HTMLCanvasElement>(null);
  const regChartInst = useRef<Chart | null>(null);
  const busyChartRef = useRef<HTMLCanvasElement>(null);
  const busyChartInst = useRef<Chart | null>(null);

  // ── Split data fetches: each only depends on its own params ──

  // overview: timeRange + excludeTest
  useEffect(() => {
    (async () => {
      const ov = await getAdminStatsOverview(timeRange, excludeTest);
      if ((ov as any).ok) setOverview((ov as any).overview);
    })();
  }, [timeRange, excludeTest]);

  // endpoints: timeRange + granularity + excludeTest
  useEffect(() => {
    (async () => {
      const eps = await getAdminEndpointStats(timeRange, granularity, excludeTest);
      if ((eps as any).ok) setEndpoints((eps as any).endpoints || []);
    })();
  }, [timeRange, granularity, excludeTest]);

  // perUser: all-time + excludeTest
  useEffect(() => {
    (async () => {
      const pu = await getAdminPerUserStats(87600, excludeTest);
      if ((pu as any).ok) setPerUser((pu as any).users || []);
    })();
  }, [excludeTest]);

  // registrations: timeRange + regGranularity + excludeTest
  useEffect(() => {
    (async () => {
      const reg = await getAdminRegistrationStats(timeRange === 24 ? 720 : timeRange, regGranularity, excludeTest);
      if ((reg as any).ok) setRegistrations((reg as any).registrations || []);
    })();
  }, [timeRange, regGranularity, excludeTest]);

  // errorStats: timeRange + excludeTest
  useEffect(() => {
    (async () => {
      const es = await getAdminErrorStats(timeRange, excludeTest);
      if ((es as any).ok) setErrorStats((es as any).errors || []);
    })();
  }, [timeRange, excludeTest]);

  // busyHours: excludeTest
  useEffect(() => {
    (async () => {
      const bh = await getAdminBusyHours(excludeTest);
      if ((bh as any).ok) setBusyHours((bh as any).hours || []);
    })();
  }, [excludeTest]);

  // methodStats: excludeTest
  useEffect(() => {
    (async () => {
      const ms = await getAdminMethodStats(excludeTest);
      if ((ms as any).ok) setMethodStats((ms as any).methods || []);
    })();
  }, [excludeTest]);

  // Initial loading state
  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 300);
    return () => clearTimeout(timer);
  }, []);

  // ── Charts ──

  // Format chart label based on granularity
  const formatLabel = (time: string, g: Granularity) => {
    if (g === 'month') return time.slice(0, 7);
    if (g === 'week') return time;
    if (g === 'day') return time.slice(5);
    if (g === 'hour') return time.slice(11, 16);
    if (g === '15min') return time.slice(11, 16);
    if (g === 'minute') return time.slice(11, 16);
    return time;
  };

  // Endpoint trend chart
  useEffect(() => {
    if (!epChartRef.current) return;
    if (epChartInst.current) epChartInst.current.destroy();
    epChartInst.current = null;
    if (endpoints.length === 0 || !endpoints[0]?.data?.length) return;

    const labels = endpoints[0].data.map((d: any) => formatLabel(d.time, granularity));
    const datasets = endpoints.map((ep: any, i: number) => ({
      label: ep.endpoint, data: ep.data.map((d: any) => d.total),
      borderColor: COLORS[i % COLORS.length], backgroundColor: COLORS[i % COLORS.length] + '20', tension: 0.3, fill: false,
    }));
    epChartInst.current = new Chart(epChartRef.current, {
      type: 'line', data: { labels, datasets },
      options: { responsive: true, plugins: { legend: { position: 'bottom', labels: { boxWidth: 12, font: { size: 11 } } } }, scales: { y: { beginAtZero: true, ticks: { stepSize: 1 } } } },
    });
    return () => { if (epChartInst.current) epChartInst.current.destroy(); };
  }, [endpoints, granularity]);

  // Registration chart
  useEffect(() => {
    if (!regChartRef.current) return;
    if (regChartInst.current) regChartInst.current.destroy();
    regChartInst.current = null;
    if (registrations.length === 0) return;

    regChartInst.current = new Chart(regChartRef.current, {
      type: 'bar', data: { labels: registrations.map((r: any) => regGranularity === 'day' ? r.time.slice(5) : r.time), datasets: [{ label: '新注册', data: registrations.map((r: any) => r.count), backgroundColor: '#3b82f680', borderColor: '#3b82f6', borderWidth: 1 }] },
      options: { responsive: true, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true, ticks: { stepSize: 1 } } } },
    });
    return () => { if (regChartInst.current) regChartInst.current.destroy(); };
  }, [registrations, regGranularity]);

  // Busy hours chart
  useEffect(() => {
    if (!busyChartRef.current) return;
    if (busyChartInst.current) busyChartInst.current.destroy();
    busyChartInst.current = null;
    if (busyHours.length === 0) return;

    busyChartInst.current = new Chart(busyChartRef.current, {
      type: 'bar', data: { labels: busyHours.map((h: any) => h.time), datasets: [{ label: '调用次数', data: busyHours.map((h: any) => h.count), backgroundColor: '#f59e0b80', borderColor: '#f59e0b', borderWidth: 1 }] },
      options: { responsive: true, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true, ticks: { stepSize: 1 } } } },
    });
    return () => { if (busyChartInst.current) busyChartInst.current.destroy(); };
  }, [busyHours]);

  const sortedUsers = [...perUser].sort((a, b) => {
    const diff = (a[sortField] || 0) - (b[sortField] || 0);
    return sortAsc ? diff : -diff;
  });

  // Unified loading skeleton (shown only on first load before any data arrives)
  if (loading && !overview && endpoints.length === 0) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div style={{display:'grid',gridTemplateColumns:'repeat(auto-fit,minmax(120px,1fr))',gap:10}}>
          {Array.from({length:5}).map((_,i)=><div key={i} className="skeleton-card" style={{height:78,borderRadius:14}} />)}
        </div>
        <div className="stats-controls"><div className="skeleton-line" style={{width:200,height:28,borderRadius:14}} /></div>
        <div className="skeleton-card" style={{height:240,borderRadius:14}} />
        <div className="skeleton-card" style={{height:200,borderRadius:14}} />
        <div className="stats-controls"><div className="skeleton-line" style={{width:160,height:28,borderRadius:14}} /></div>
        <div className="skeleton-card" style={{height:200,borderRadius:14}} />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
      {/* Overview cards */}
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: 10 }}>
        <StatCard label="总调用" value={overview?.total_calls ?? 0} />
        <StatCard label="成功率" value={overview?.success_rate?.toFixed(1) ?? '0'} unit="%" color={overview?.success_rate > 90 ? 'var(--phx-green)' : 'var(--phx-red)'} />
        <StatCard label="总进服" value={overview?.total_joins ?? 0} />
        <StatCard label="活跃用户" value={overview?.active_users ?? 0} />
        <StatCard label="总用户" value={overview?.total_users ?? 0} />
        <StatCard label="今日注册" value={overview?.today_regs ?? 0} />
      </div>

      {/* Controls */}
      <div className="stats-controls">
        <span className="control-label">粒度</span>
        <div className="capsule-switch">
          {GRAN_OPTIONS.map(g => (
            <button key={g.v} className={granularity === g.v ? 'active' : ''} onClick={() => setGranularity(g.v)}>{g.l}</button>
          ))}
        </div>
        <span className="control-label">范围</span>
        <div className="capsule-switch">
          {RANGE_OPTIONS.map(r => (
            <button key={r.v} className={timeRange === r.v ? 'active' : ''} onClick={() => setTimeRange(r.v)}>{r.l}</button>
          ))}
        </div>
        <span className="control-label">排除测试号</span>
        <div className="capsule-switch">
          <button className={excludeTest ? 'active' : ''} onClick={() => setExcludeTest(true)}>是</button>
          <button className={!excludeTest ? 'active' : ''} onClick={() => setExcludeTest(false)}>否</button>
        </div>
      </div>

      {/* Endpoint trend chart */}
      {endpoints.length > 0 ? (
        <section className="card"><h3 style={{ margin: '0 0 10px' }}>全局端点调用趋势</h3><canvas ref={epChartRef} /></section>
      ) : (
        <section className="card"><div className="empty-state">暂无端点调用数据，开始使用后这里会显示趋势图</div></section>
      )}

      {/* Error Stats */}
      {errorStats.length > 0 && (
        <section className="card">
          <h3 style={{ margin: '0 0 10px' }}>错误端点排行</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
            {errorStats.map((e: any) => (
              <div key={e.endpoint} style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 10px', background: 'var(--phx-bg)', borderRadius: 8, fontSize: 13 }}>
                <span>{e.endpoint}</span>
                <span><strong style={{color:'var(--phx-red)'}}>{e.errors}</strong> / {e.total} ({e.err_rate?.toFixed(1)}%)</span>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Busy Hours + Method Stats side by side */}
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 16 }}>
        {busyHours.length > 0 && (
          <section className="card">
            <h3 style={{ margin: '0 0 10px', fontSize: 14 }}>高峰时段</h3>
            <canvas ref={busyChartRef} />
          </section>
        )}
        {methodStats.length > 0 && (
          <section className="card">
            <h3 style={{ margin: '0 0 10px', fontSize: 14 }}>HTTP 方法分布</h3>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
              {methodStats.map((m: any) => (
                <div key={m.time} style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 10px', background: 'var(--phx-bg)', borderRadius: 8, fontSize: 13 }}>
                  <span style={{ fontWeight: 700 }}>{m.time}</span>
                  <span>{m.count}</span>
                </div>
              ))}
            </div>
          </section>
        )}
      </div>

      {/* Per-user ranking */}
      <section className="card">
        <h3 style={{ margin: '0 0 10px' }}>用户统计排行</h3>
        {perUser.length === 0 ? <div className="empty-state">暂无数据</div> : (
          <div style={{ overflowX: 'auto' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
              <thead>
                <tr style={{ borderBottom: '2px solid var(--phx-border-light)' }}>
                  {([
                    { k: 'user_id' as const, l: 'ID' },
                    { k: 'total_calls' as const, l: '调用' },
                    { k: 'success' as const, l: '成功' },
                    { k: 'success_rate' as const, l: '成功率' },
                    { k: 'join_count' as const, l: '进服' },
                    { k: 'nuts_used' as const, l: '板栗' },
                  ]).map(col => (
                    <th key={col.k} style={{ padding: '8px 10px', textAlign: 'left', cursor: 'pointer', color: sortField === col.k ? 'var(--phx-primary)' : undefined, fontSize: 12, fontWeight: 800, userSelect: 'none' }} onClick={() => {
                      if (sortField === col.k) setSortAsc(!sortAsc);
                      else { setSortField(col.k); setSortAsc(false); }
                    }}>
                      {col.l}{sortField === col.k ? (sortAsc ? ' ▲' : ' ▼') : ''}
                    </th>
                  ))}
                  <th style={{ padding: '8px 10px', width: 60 }}></th>
                </tr>
              </thead>
              <tbody>
                {sortedUsers.map((u: any) => (
                  <tr key={u.user_id} style={{ borderBottom: '1px solid var(--phx-border-light)' }}>
                    <td style={{ padding: '8px 10px', fontSize: 11, color: 'var(--phx-text-secondary)' }}>#{u.user_id} {u.username || ''}</td>
                    <td style={{ padding: '8px 10px' }}>{u.total_calls}</td>
                    <td style={{ padding: '8px 10px' }}>{u.success}</td>
                    <td style={{ padding: '8px 10px', color: u.success_rate > 90 ? 'var(--phx-green)' : u.success_rate > 50 ? 'inherit' : 'var(--phx-red)' }}>{u.success_rate?.toFixed(1)}%</td>
                    <td style={{ padding: '8px 10px' }}>{u.join_count}</td>
                    <td style={{ padding: '8px 10px' }}>{u.nuts_used}</td>
                    <td style={{ padding: '8px 6px' }}><button className="btn btn-sm btn-outline" onClick={() => openUserDetail(u.user_id)}>详情</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* User detail modal */}
      {detailUser !== null || detailLoading ? (
        <ModalPortal>
        <div className="modal-backdrop" onClick={() => { setDetailUser(null); setDetailLoading(false); }}>
          <div className="modal-content" style={{ maxWidth: 560 }} onClick={e => e.stopPropagation()}>
            <div className="modal-header" style={{ position: 'relative' }}>
              <h2 className="modal-title">{detailLoading ? '加载中…' : `${detailUser?.username || '#' + detailUser?.user_id} 的统计`}</h2>
              <button className="btn-icon" style={{ position: 'absolute', top: -4, right: -4 }} onClick={() => { setDetailUser(null); setDetailLoading(false); }}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
              </button>
            </div>
            <div className="modal-body">
              {detailLoading ? (
                <div style={{display:'flex',flexDirection:'column',gap:8}}>
                  {Array.from({length:4}).map((_,i)=><div key={i} className="skeleton-line" style={{height:14,borderRadius:7}} />)}
                </div>
              ) : detailUser ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 10, fontSize: 13 }}>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8 }}>
                    <div className="game-stat" style={{padding:12}}><span>总调用</span><strong>{detailUser.total_calls}</strong></div>
                    <div className="game-stat" style={{padding:12}}><span>成功率</span><strong style={{color: detailUser.success_rate > 90 ? 'var(--phx-green)' : 'var(--phx-red)'}}>{detailUser.success_rate?.toFixed(1)}%</strong></div>
                    <div className="game-stat" style={{padding:12}}><span>进服次数</span><strong>{detailUser.join_count}</strong></div>
                    <div className="game-stat" style={{padding:12}}><span>板栗余额 / 消耗</span><strong>{detailUser.nuts_balance} / {detailUser.nuts_used}</strong></div>
                  </div>
                  {detailUser.servers?.length > 0 && (
                    <div>
                      <div style={{fontWeight:700,marginBottom:6}}>常进服务器</div>
                      {detailUser.servers.slice(0, 10).map((s: any) => (
                        <div key={s.server_code} style={{display:'flex',justifyContent:'space-between',padding:'4px 8px',background:'var(--phx-bg)',borderRadius:6,marginBottom:4}}>
                          <span>{s.server_code}</span><span style={{fontWeight:700}}>{s.count}次</span>
                        </div>
                      ))}
                    </div>
                  )}
                  {detailUser.endpoints?.length > 0 && (
                    <div>
                      <div style={{fontWeight:700,marginBottom:6}}>端点分布</div>
                      {detailUser.endpoints.map((ep: any) => {
                        const total = ep.data.reduce((sum: number, d: any) => sum + d.total, 0);
                        const errors = ep.data.reduce((sum: number, d: any) => sum + d.errors, 0);
                        return (
                          <div key={ep.endpoint} style={{display:'flex',justifyContent:'space-between',padding:'4px 8px',background:'var(--phx-bg)',borderRadius:6,marginBottom:4}}>
                            <span style={{fontSize:12}}>{ep.endpoint}</span>
                            <span style={{fontWeight:700}}>{total}<span style={{color:'var(--phx-red)',fontSize:11,marginLeft:4}}>{errors > 0 ? `(${errors})` : ''}</span></span>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              ) : <div className="empty-state">暂无数据</div>}
            </div>
          </div>
        </div>
        </ModalPortal>
      ) : null}

      {/* Registration */}
      <div className="stats-controls">
        <span className="control-label">注册粒度</span>
        <div className="capsule-switch">
          <button className={regGranularity === 'day' ? 'active' : ''} onClick={() => setRegGranularity('day')}>天</button>
          <button className={regGranularity === 'week' ? 'active' : ''} onClick={() => setRegGranularity('week')}>周</button>
        </div>
      </div>

      {registrations.length > 0 ? (
        <section className="card"><h3 style={{ margin: '0 0 10px' }}>用户注册趋势</h3><canvas ref={regChartRef} /></section>
      ) : (
        <section className="card"><div className="empty-state">暂无注册数据</div></section>
      )}
    </div>
  );
}

function StatCard({ label, value, unit, color }: { label: string; value: number | string; unit?: string; color?: string }) {
  return (
    <div className="card" style={{ padding: '14px 16px', textAlign: 'center' }}>
      <div style={{ fontSize: 12, color: 'var(--phx-text-secondary)', marginBottom: 6 }}>{label}</div>
      <div style={{ fontSize: 26, fontWeight: 700, color: color || 'var(--phx-text)' }}>
        {typeof value === 'number' ? value.toLocaleString() : value}
        {unit && <span style={{ fontSize: 13, fontWeight: 400, marginLeft: 3 }}>{unit}</span>}
      </div>
    </div>
  );
}