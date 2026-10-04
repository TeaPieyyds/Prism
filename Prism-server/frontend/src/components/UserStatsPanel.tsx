import { useEffect, useState, useRef } from 'react';
import { Chart, registerables } from 'chart.js';
import { getUserStatsSummary, getUserEndpointStats, getUserServerStats, getUserNutsTimeline, listTokens } from '../api';

Chart.register(...registerables);

type Granularity = 'minute' | '15min' | 'hour' | 'day' | 'week' | 'month';

const COLORS = ['#4f9d53', '#3b82f6', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#06b6d4', '#84cc16', '#f97316', '#6366f1'];

const GRAN_OPTIONS: { v: Granularity; l: string }[] = [
  { v: 'minute', l: '分' }, { v: '15min', l: '15分' }, { v: 'hour', l: '时' }, { v: 'day', l: '天' }, { v: 'week', l: '周' }, { v: 'month', l: '月' },
];
const RANGE_OPTIONS = [
  { v: 1, l: '1h' }, { v: 24, l: '24h' }, { v: 168, l: '7天' }, { v: 720, l: '30天' },
];

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

export default function UserStatsPanel() {
  const [summary, setSummary] = useState<any>(null);
  const [endpoints, setEndpoints] = useState<any[]>([]);
  const [servers, setServers] = useState<any[]>([]);
  const [nutsPoints, setNutsPoints] = useState<any[]>([]);
  const [tokenStats, setTokenStats] = useState<any[]>([]);
  const [tokenTotal, setTokenTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [granularity, setGranularity] = useState<Granularity>('hour');
  const [timeRange, setTimeRange] = useState(168);

  const epChartRef = useRef<HTMLCanvasElement>(null);
  const epChartInst = useRef<Chart | null>(null);
  const serverChartRef = useRef<HTMLCanvasElement>(null);
  const serverChartInst = useRef<Chart | null>(null);
  const nutsChartRef = useRef<HTMLCanvasElement>(null);
  const nutsChartInst = useRef<Chart | null>(null);

  // Split data fetching: summary only depends on timeRange
  useEffect(() => {
    (async () => {
      const sum = await getUserStatsSummary(timeRange);
      if ((sum as any).ok) setSummary((sum as any).summary);
    })();
  }, [timeRange]);

  // endpoints: timeRange + granularity
  useEffect(() => {
    (async () => {
      const eps = await getUserEndpointStats(timeRange, granularity);
      if ((eps as any).ok) setEndpoints((eps as any).endpoints || []);
    })();
  }, [timeRange, granularity]);

  // servers: timeRange
  useEffect(() => {
    (async () => {
      const srv = await getUserServerStats(timeRange);
      if ((srv as any).ok) setServers((srv as any).servers || []);
    })();
  }, [timeRange]);

  // nuts: timeRange
  useEffect(() => {
    (async () => {
      const nuts = await getUserNutsTimeline(timeRange);
      if ((nuts as any).ok) setNutsPoints((nuts as any).points || []);
    })();
  }, [timeRange]);

  // token call stats (independent of time range)
  useEffect(() => {
    (async () => {
      const r = await listTokens();
      if ((r as any).ok) {
        setTokenStats((r as any).tokens || []);
        setTokenTotal((r as any).total_calls ?? 0);
      }
    })();
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 300);
    return () => clearTimeout(timer);
  }, []);

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

  const serverChartHeight = Math.max(280, servers.length * 32);

  useEffect(() => {
    if (!serverChartRef.current || servers.length === 0) return;
    if (serverChartInst.current) serverChartInst.current.destroy();
    serverChartInst.current = new Chart(serverChartRef.current, {
      type: 'bar', data: { labels: servers.map((s: any) => s.server_code), datasets: [{ label: '进服次数', data: servers.map((s: any) => s.count), backgroundColor: COLORS.map(c => c + '80'), borderColor: COLORS, borderWidth: 1 }] },
      options: {
        indexAxis: 'y', responsive: true, maintainAspectRatio: false,
        plugins: { legend: { display: false } },
        scales: {
          x: { beginAtZero: true, ticks: { stepSize: 1 } },
          y: { ticks: { font: { size: 11 }, autoSkip: false } },
        },
      },
    });
    return () => { if (serverChartInst.current) serverChartInst.current.destroy(); };
  }, [servers]);

  useEffect(() => {
    if (!nutsChartRef.current || nutsPoints.length === 0) return;
    if (nutsChartInst.current) nutsChartInst.current.destroy();
    nutsChartInst.current = new Chart(nutsChartRef.current, {
      type: 'line', data: { labels: nutsPoints.map((p: any) => p.time.slice(0, 16).replace('T', ' ')), datasets: [{ label: '板栗余额', data: nutsPoints.map((p: any) => p.balance), borderColor: '#f59e0b', backgroundColor: 'rgba(245,158,11,0.1)', tension: 0.3, fill: true }] },
      options: { responsive: true, plugins: { legend: { display: false } }, scales: { y: { beginAtZero: true } } },
    });
    return () => { if (nutsChartInst.current) nutsChartInst.current.destroy(); };
  }, [nutsPoints]);

  if (loading && !summary && endpoints.length === 0) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
        <div className="stats-controls"><div className="skeleton-line" style={{width:200,height:28,borderRadius:14}} /></div>
        <div style={{display:'grid',gridTemplateColumns:'repeat(auto-fit,minmax(130px,1fr))',gap:10}}>
          {Array.from({length:4}).map((_,i)=><div key={i} className="skeleton-card" style={{height:78,borderRadius:14}} />)}
        </div>
        {Array.from({length:3}).map((_,i)=><div key={i} className="skeleton-card" style={{height:220,borderRadius:14}} />)}
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
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
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(130px, 1fr))', gap: 10 }}>
        <StatCard label="API 调用" value={summary?.total_calls ?? 0} unit="次" />
        <StatCard label="成功率" value={summary?.success_rate?.toFixed(1) ?? '0'} unit="%" color={summary?.success_rate > 90 ? 'var(--phx-green)' : 'var(--phx-red)'} />
        <StatCard label="进服次数" value={summary?.join_count ?? 0} unit="次" />
        <StatCard label="板栗消耗" value={summary?.nuts_used ?? 0} unit="个" />
      </div>

      {tokenStats.length > 0 && (
        <section className="card">
          <h3 style={{ margin: '0 0 10px' }}>令牌调用次数</h3>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 6, fontSize: 13 }}>
            {tokenStats.map((t: any) => (
              <div key={t.id} style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>#{t.id}{t.name ? ` ${t.name}` : ''}</span>
                <span><strong>{t.call_count}</strong> 次</span>
              </div>
            ))}
            <div style={{ borderTop: '1px dashed var(--phx-border)', paddingTop: 6, display: 'flex', justifyContent: 'space-between', fontWeight: 600 }}>
              <span>总调用次数</span><span>{tokenTotal} 次</span>
            </div>
          </div>
        </section>
      )}

      {endpoints.length > 0 ? (
        <section className="card"><h3 style={{ margin: '0 0 10px' }}>端点调用趋势</h3><canvas ref={epChartRef} /></section>
      ) : (
        <section className="card"><div className="empty-state">暂无端点调用数据，开始使用后这里会显示趋势图</div></section>
      )}

      {servers.length > 0 ? (
        <section className="card"><h3 style={{ margin: '0 0 10px' }}>服务器进入次数</h3><div style={{ height: serverChartHeight, maxHeight: 600, overflowY: 'auto' }}><canvas ref={serverChartRef} /></div></section>
      ) : (
        <section className="card"><div className="empty-state">暂无进服记录</div></section>
      )}

      {nutsPoints.length > 0 ? (
        <section className="card"><h3 style={{ margin: '0 0 10px' }}>板栗余额变化</h3><canvas ref={nutsChartRef} /></section>
      ) : (
        <section className="card"><div className="empty-state">暂无板栗交易记录</div></section>
      )}
    </div>
  );
}

function StatCard({ label, value, unit, color }: { label: string; value: number | string; unit: string; color?: string }) {
  return (
    <div className="card" style={{ padding: '14px 16px', textAlign: 'center' }}>
      <div style={{ fontSize: 12, color: 'var(--phx-text-secondary)', marginBottom: 6 }}>{label}</div>
      <div style={{ fontSize: 26, fontWeight: 700, color: color || 'var(--phx-text)' }}>{value}<span style={{ fontSize: 13, fontWeight: 400, marginLeft: 3 }}>{unit}</span></div>
    </div>
  );
}