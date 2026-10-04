import { useEffect, useState } from 'react';
import {
  adminMarketFiles, adminMarketFlagged, adminMarketFileStatus, adminMarketFileUnflag,
  adminMarketFileDelete, adminMarketConfigGet, adminMarketConfigUpdate,
  adminMarketCategoriesGet, adminMarketCategoryCreate, adminMarketCategoryRename, adminMarketCategoryDelete,
  adminMarketFileUpdate,
} from '../api';
import ModalPortal from './ModalPortal';
import { showToast } from './Toast';

const toCST = (utcStr: string) => {
  if (!utcStr) return '-';
  const d = new Date(utcStr);
  return isNaN(d.getTime()) ? utcStr : d.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
};

interface MarketFile {
  id: number;
  title: string;
  description?: string;
  categories: string[];
  tags: string[];
  price_pts: number;
  allow_anonymous: boolean;
  download_count: number;
  block_count: number;
  uploader_id: number;
  uploader_name?: string;
  flagged: boolean;
  report_count: number;
  status: number;
  created_at: string;
  cover_part_id?: number;
}

interface Category { id: number; name: string; sort_order?: number }

export default function MarketAdminPanel() {
  const [files, setFiles] = useState<MarketFile[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [q, setQ] = useState('');
  const [flagged, setFlagged] = useState<MarketFile[]>([]);
  const [reward, setReward] = useState(10);
  const [threshold, setThreshold] = useState(3);
  const [cats, setCats] = useState<Category[]>([]);
  const [newCat, setNewCat] = useState('');
  const [editing, setEditing] = useState<MarketFile | null>(null);
  const [editCat, setEditCat] = useState<Category | null>(null);

  const fetchFiles = async (p = page, kw = q) => {
    const res: any = await adminMarketFiles(p, 20, kw);
    if (res.ok) { setFiles(res.list || []); setTotal(res.total || 0); }
  };
  const fetchFlagged = async () => {
    const res: any = await adminMarketFlagged();
    if (res.ok) setFlagged(res.list || []);
  };
  const fetchConfig = async () => {
    const res: any = await adminMarketConfigGet();
    if (res.ok && res.config) { setReward(res.config.upload_reward_nuts ?? 10); setThreshold(res.config.report_auto_takedown ?? 3); }
  };
  const fetchCats = async () => {
    const res: any = await adminMarketCategoriesGet();
    if (res.ok) setCats(res.categories || []);
  };
  const fetchAll = () => { fetchFiles(); fetchFlagged(); fetchConfig(); fetchCats(); };
  useEffect(() => { fetchAll(); }, []);

  const setStatus = async (f: MarketFile, status: number) => {
    const res: any = await adminMarketFileStatus(f.id, status);
    if (res.ok) showToast(status === 1 ? '已重新上架' : '已下架', 'ok'); else showToast(res.error || '操作失败', 'err');
    fetchAll();
  };
  const unflag = async (f: MarketFile) => {
    if (!confirm(`确定解除文件「${f.title}」的举报标记并重新上架？`)) return;
    const res: any = await adminMarketFileUnflag(f.id);
    if (res.ok) showToast('已解除标记并上架', 'ok'); else showToast(res.error || '操作失败', 'err');
    fetchAll();
  };
  const del = async (f: MarketFile) => {
    if (!confirm(`确定永久删除文件「${f.title}」？此操作不可恢复。`)) return;
    const res: any = await adminMarketFileDelete(f.id);
    if (res.ok) showToast('已删除', 'ok'); else showToast(res.error || '删除失败', 'err');
    fetchAll();
  };
  const saveConfig = async () => {
    const res: any = await adminMarketConfigUpdate({ upload_reward_nuts: Math.max(0, reward | 0), report_auto_takedown: Math.max(1, threshold | 0) });
    if (res.ok) showToast('配置已保存', 'ok'); else showToast(res.error || '保存失败', 'err');
    fetchConfig();
  };

  // 分类管理
  const addCat = async () => {
    if (!newCat.trim()) return;
    const res: any = await adminMarketCategoryCreate(newCat.trim());
    if (res.ok) { showToast('分类已添加', 'ok'); setNewCat(''); fetchCats(); }
    else showToast(res.error || '添加失败', 'err');
  };
  const renameCat = async () => {
    if (!editCat || !editCat.name.trim()) return;
    const res: any = await adminMarketCategoryRename(editCat.id, editCat.name.trim());
    if (res.ok) { showToast('已重命名', 'ok'); setEditCat(null); fetchAll(); }
    else showToast(res.error || '重命名失败', 'err');
  };
  const delCat = async (c: Category) => {
    if (!confirm(`删除分类「${c.name}」？只属于该分类的文件将归入"未分类"（不会被删除）。`)) return;
    const res: any = await adminMarketCategoryDelete(c.id);
    if (res.ok) { showToast('分类已删除', 'ok'); fetchAll(); }
    else showToast(res.error || '删除失败', 'err');
  };

  // 文件编辑
  const openEdit = (f: MarketFile) => {
    setEditing({ ...f, tags: [...(f.tags || [])], categories: [...(f.categories || [])] });
  };
  const saveEdit = async () => {
    if (!editing) return;
    const res: any = await adminMarketFileUpdate(editing.id, {
      price_pts: Math.max(0, editing.price_pts | 0),
      allow_anonymous: !!editing.allow_anonymous,
      tags: (editing.tags || []).filter(t => t.trim() !== ''),
      description: editing.description || '',
      categories: editing.categories || [],
    });
    if (res.ok) { showToast('已保存', 'ok'); setEditing(null); fetchAll(); }
    else showToast(res.error || '保存失败', 'err');
  };
  const toggleEditCat = (name: string) => {
    if (!editing) return;
    const cur = editing.categories || [];
    setEditing({ ...editing, categories: cur.includes(name) ? cur.filter(x => x !== name) : [...cur, name] });
  };

  const coverURL = (f: MarketFile) => f.cover_part_id ? `/api/market/parts/${f.cover_part_id}/preview` : '';
  const catDisplay = (f: MarketFile) => (f.categories || []).length ? f.categories.join('、') : '未分类';

  const renderActions = (f: MarketFile, isFlagged: boolean) => (
    <div className="account-actions" style={{ marginTop: 8, flexWrap: 'wrap', gap: 4 }}>
      <button className="btn btn-sm btn-outline" onClick={() => openEdit(f)}>编辑</button>
      <button className="btn btn-sm btn-outline" onClick={() => setStatus(f, f.status === 1 ? 0 : 1)}>{f.status === 1 ? '下架' : '上架'}</button>
      {isFlagged && <button className="btn btn-sm btn-warning" onClick={() => unflag(f)}>解除标记</button>}
      <button className="btn btn-sm btn-danger" onClick={() => del(f)}>删除</button>
    </div>
  );

  const renderCard = (f: MarketFile, isFlagged: boolean) => (
    <article key={f.id} className="admin-card">
      <div style={{ display: 'flex', gap: 10 }}>
        {coverURL(f) ? (
          <img src={coverURL(f)} alt="" style={{ width: 72, height: 72, objectFit: 'cover', borderRadius: 8, flexShrink: 0, background: 'var(--phx-bg)' }} />
        ) : (
          <div style={{ width: 72, height: 72, borderRadius: 8, background: 'var(--phx-bg)', display: 'flex', alignItems: 'center', justifyContent: 'center', color: 'var(--phx-text-secondary)', fontSize: 11, flexShrink: 0 }}>无预览</div>
        )}
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="admin-title" style={{ flexWrap: 'wrap' }}>
            <strong>#{f.id} {f.title}</strong>
            <span className={f.status === 1 ? 'status-pill' : 'status-pill danger'}>{f.status === 1 ? '上架' : '已下架'}</span>
            {f.flagged && <span className="status-pill danger">举报{f.report_count}</span>}
            {f.price_pts > 0 && <span className="status-pill">{f.price_pts} 🌰</span>}
          </div>
          <p style={{ margin: '2px 0' }}>{f.uploader_name ? `作者：${f.uploader_name}` : `#${f.uploader_id}`} · 分类：{catDisplay(f)} · 方块：{f.block_count || 0}</p>
          <p style={{ margin: '2px 0', fontSize: 12 }}>标签：{(f.tags || []).length ? f.tags.join('、') : '-'}</p>
          <p style={{ margin: '2px 0', fontSize: 12, color: 'var(--phx-text-secondary)' }}>下载 {f.download_count} · 匿名下载：{f.allow_anonymous ? '允许' : '禁止'} · {toCST(f.created_at)}</p>
        </div>
      </div>
      {f.description && <p style={{ fontSize: 12, color: 'var(--phx-text-secondary)', margin: '6px 0 0' }}>{f.description}</p>}
      {renderActions(f, isFlagged)}
    </article>
  );

  return (
    <>
      <section className="hero-panel compact">
        <h2 style={{ margin: 0 }}>文件市场管理</h2>
        <button className="btn btn-sm btn-outline" onClick={fetchAll}>刷新全部</button>
      </section>

      <section className="card" style={{ marginBottom: 16 }}>
        <h3 style={{ margin: '0 0 10px' }}>配置</h3>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
          <label style={{ fontSize: 13 }}>上传奖励(🌰)：<input className="input" type="number" value={reward} onChange={e => setReward(+e.target.value)} style={{ width: 90 }} /></label>
          <label style={{ fontSize: 13 }}>举报数超过即自动下架：<input className="input" type="number" value={threshold} onChange={e => setThreshold(+e.target.value)} style={{ width: 70 }} /></label>
          <button className="btn btn-primary btn-sm" onClick={saveConfig}>保存配置</button>
        </div>
      </section>

      <section className="card" style={{ marginBottom: 16 }}>
        <h3 style={{ margin: '0 0 10px' }}>分类管理 <span style={{ fontSize: 12, color: 'var(--phx-text-secondary)' }}>共 {cats.length} 个 · 一个文件可属于多个分类</span></h3>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 10 }}>
          <input className="input" placeholder="新分类名" value={newCat} onChange={e => setNewCat(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') addCat(); }} style={{ width: 180 }} />
          <button className="btn btn-primary btn-sm" onClick={addCat}>添加分类</button>
        </div>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
          {cats.map(c => (
            <span key={c.id} style={{ display: 'inline-flex', alignItems: 'center', gap: 6, background: 'var(--phx-bg)', borderRadius: 16, padding: '4px 10px', fontSize: 13 }}>
              {c.name}
              <button className="btn-icon" title="重命名" onClick={() => setEditCat({ ...c })} style={{ fontSize: 12 }}>✏️</button>
              <button className="btn-icon" title="删除" onClick={() => delCat(c)} style={{ fontSize: 12 }}>🗑️</button>
            </span>
          ))}
        </div>
      </section>

      <section className="card" style={{ marginBottom: 16 }}>
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginBottom: 10 }}>
          <h3 style={{ margin: 0 }}>受标记文件</h3>
          <span className="status-pill danger">{flagged.length}</span>
        </div>
        {flagged.length === 0 ? (
          <p className="empty-state" style={{ color: 'var(--phx-text-secondary)', fontSize: 13 }}>暂无受举报标记的文件</p>
        ) : (
          <div className="admin-grid">{flagged.map(f => renderCard(f, true))}</div>
        )}
      </section>

      <section className="card" style={{ marginBottom: 16 }}>
        <h3 style={{ margin: '0 0 10px' }}>全部文件 <span style={{ fontSize: 12, color: 'var(--phx-text-secondary)' }}>共 {total}</span></h3>
        <div style={{ display: 'flex', gap: 8, marginBottom: 10 }}>
          <input className="input" placeholder="搜索标题" value={q} onChange={e => setQ(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') { setPage(1); fetchFiles(1, q); } }} style={{ flex: 1 }} />
          <button className="btn btn-sm btn-outline" onClick={() => { setPage(1); fetchFiles(1, q); }}>搜索</button>
        </div>
        {files.length === 0 ? (
          <p className="empty-state" style={{ color: 'var(--phx-text-secondary)', fontSize: 13 }}>暂无文件</p>
        ) : (
          <div className="admin-grid">{files.map(f => renderCard(f, false))}</div>
        )}
        <div style={{ display: 'flex', gap: 8, alignItems: 'center', marginTop: 12 }}>
          <button className="btn btn-sm btn-outline" disabled={page <= 1} onClick={() => { setPage(page - 1); fetchFiles(page - 1, q); }}>上一页</button>
          <span style={{ fontSize: 13 }}>第 {page} 页</span>
          <button className="btn btn-sm btn-outline" disabled={page * 20 >= total} onClick={() => { setPage(page + 1); fetchFiles(page + 1, q); }}>下一页</button>
        </div>
      </section>

      {/* 文件编辑弹窗 */}
      {editing && (
        <ModalPortal>
          <div className="modal-backdrop" style={{ zIndex: 2000 }}>
            <div className="modal-content" style={{ maxWidth: 520, padding: 24 }}>
              <h3 style={{ margin: '0 0 14px', fontSize: 16 }}>编辑文件 #{editing.id}：{editing.title}</h3>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 12, fontSize: 13 }}>
                <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input type="checkbox" checked={editing.price_pts > 0} onChange={e => { setEditing({ ...editing, price_pts: e.target.checked ? 1 : 0 }); }} />
                  是否付费
                </label>
                {editing.price_pts > 0 && (
                  <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    价格(🌰)：
                    <input className="input" type="number" value={editing.price_pts} onChange={e => setEditing({ ...editing, price_pts: +e.target.value })} style={{ width: 100 }} />
                  </label>
                )}
                <label style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <input type="checkbox" checked={editing.allow_anonymous} onChange={e => setEditing({ ...editing, allow_anonymous: e.target.checked })} />
                  允许匿名下载
                </label>
                <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  标签（逗号分隔）
                  <input className="input" value={(editing.tags || []).join(',')} onChange={e => setEditing({ ...editing, tags: e.target.value.split(',').map(s => s.trim()).filter(Boolean) })} />
                </label>
                <label style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                  说明
                  <textarea className="input" rows={3} value={editing.description || ''} onChange={e => setEditing({ ...editing, description: e.target.value })} />
                </label>
                <div>
                  <div style={{ marginBottom: 6 }}>分类（可多选）</div>
                  <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
                    {cats.map(c => (
                      <label key={c.id} style={{ display: 'inline-flex', alignItems: 'center', gap: 4, background: 'var(--phx-bg)', borderRadius: 14, padding: '3px 9px' }}>
                        <input type="checkbox" checked={(editing.categories || []).includes(c.name)} onChange={() => toggleEditCat(c.name)} />
                        {c.name}
                      </label>
                    ))}
                  </div>
                  {(editing.categories || []).length === 0 && <div style={{ color: 'var(--phx-text-secondary)', fontSize: 12, marginTop: 4 }}>未分类</div>}
                </div>
              </div>
              <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 18 }}>
                <button className="btn btn-sm btn-outline" onClick={() => setEditing(null)}>取消</button>
                <button className="btn btn-primary btn-sm" onClick={saveEdit}>保存</button>
              </div>
            </div>
          </div>
        </ModalPortal>
      )}

      {/* 分类重命名弹窗 */}
      {editCat && (
        <ModalPortal>
          <div className="modal-backdrop" style={{ zIndex: 2000 }}>
            <div className="modal-content" style={{ maxWidth: 360, padding: 24 }}>
              <h3 style={{ margin: '0 0 14px', fontSize: 16 }}>重命名分类</h3>
              <input className="input" value={editCat.name} onChange={e => setEditCat({ ...editCat, name: e.target.value })} style={{ width: '100%' }} autoFocus />
              <div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 18 }}>
                <button className="btn btn-sm btn-outline" onClick={() => setEditCat(null)}>取消</button>
                <button className="btn btn-primary btn-sm" onClick={renameCat}>保存</button>
              </div>
            </div>
          </div>
        </ModalPortal>
      )}
    </>
  );
}
