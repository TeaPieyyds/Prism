import { useState, useEffect, useRef, useCallback } from 'react';
import { listTokens, createToken, resetToken, resetSubToken, updateToken, deleteToken, nutsToCode, nutsToQuestionCode, createRedPacket, listRedPackets, listMyCodes } from '../api';
import type { ApiToken } from '../types';
import Captcha from './Captcha';
import Modal from './Modal';
import { successSound } from '../sounds';
import { showToast } from './Toast';

function fmtLimit(v: number, unit: string) {
  return v === -1 ? '不限' : `${v}${unit}`;
}

function copyText(v: string, msg = '已复制') {
  navigator.clipboard.writeText(v).then(() => showToast(msg, 'ok'));
}

export default function TokenManager() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [totalCalls, setTotalCalls] = useState(0);
  const [primaryToken, setPrimaryToken] = useState('');
  const [loading, setLoading] = useState(true);

  // add modal
  const [showAdd, setShowAdd] = useState(false);
  const [addName, setAddName] = useState('');
  const [addMaxCalls, setAddMaxCalls] = useState('');
  const [addMaxNuts, setAddMaxNuts] = useState('');

  // result modal (new / regenerated token, shown once)
  const [result, setResult] = useState<{ title: string; value: string } | null>(null);

  // to-code modal
  const [showToCode, setShowToCode] = useState(false);
  const [toCodeAmount, setToCodeAmount] = useState('');
  const [toCodeResult, setToCodeResult] = useState<{ ok: boolean; msg: string; code?: string } | null>(null);
  const captchaRef = useRef<any>(null);
  const [captchaKey, setCaptchaKey] = useState(0);
  const [myCodes, setMyCodes] = useState<any[]>([]);
  const [showCodes, setShowCodes] = useState(false);

  // question-code modal
  const [showQCode, setShowQCode] = useState(false);
  const [qcAmount, setQcAmount] = useState('');
  const [qcQuestion, setQcQuestion] = useState('');
  const [qcAnswer, setQcAnswer] = useState('');
  const [qcResult, setQcResult] = useState<{ ok: boolean; msg: string; code?: string } | null>(null);
  const qcCaptchaRef = useRef<any>(null);
  const [qcCaptchaKey, setQcCaptchaKey] = useState(0);

  // red-packet modal
  const [showRP, setShowRP] = useState(false);
  const [rpAmount, setRpAmount] = useState('');
  const [rpCount, setRpCount] = useState('');
  const [rpResult, setRpResult] = useState<{ ok: boolean; msg: string; code?: string } | null>(null);
  const rpCaptchaRef = useRef<any>(null);
  const [rpCaptchaKey, setRpCaptchaKey] = useState(0);
  const [myPackets, setMyPackets] = useState<any[]>([]);
  const [showPackets, setShowPackets] = useState(false);

  const load = useCallback(async () => {
    const r: any = await listTokens();
    if (r.ok) {
      setTokens(r.tokens || []);
      setTotalCalls(r.total_calls ?? 0);
      setPrimaryToken(r.primary_token || '');
    }
    setLoading(false);
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    const mc = addMaxCalls.trim() === '' ? -1 : parseInt(addMaxCalls) || -1;
    const mn = addMaxNuts.trim() === '' ? -1 : parseInt(addMaxNuts) || -1;
    const r: any = await createToken({ name: addName.trim(), max_calls: mc, max_nuts: mn });
    if (r.ok) {
      successSound();
      setShowAdd(false);
      setResult({ title: '令牌已创建', value: r.token });
      setAddName(''); setAddMaxCalls(''); setAddMaxNuts('');
      load();
    } else {
      showToast(r.error || '糟糕,创建失败了,请检查后重试~', 'err');
    }
  };

  const handleReset = async (t: ApiToken) => {
    if (!confirm(`确定重置令牌 #${t.id}？重置后旧值立即失效。`)) return;
    const r: any = await resetSubToken(t.id);
    if (r.ok) { setResult({ title: `令牌 #${t.id} 已重置`, value: r.token }); showToast('已重置', 'ok'); load(); }
    else showToast(r.error || '糟糕,重置失败了,请稍后重试~', 'err');
  };

  const handleResetPrimary = async () => {
    if (!confirm('重置后旧主令牌 adb// Token 会立即失效，确定继续？')) return;
    const r: any = await resetToken();
    if (r.ok && r.token) {
      localStorage.setItem('session_token', r.token);
      setResult({ title: '主令牌已重置', value: r.token });
      showToast('主令牌已重置', 'ok');
      load();
    } else {
      showToast(r.error || '糟糕,重置失败了,请稍后重试~', 'err');
    }
  };

  const handleToggle = async (t: ApiToken) => {
    const r: any = await updateToken(t.id, { disabled: !t.disabled });
    if (r.ok) { showToast(!t.disabled ? '已停用' : '已启用', 'ok'); load(); }
    else showToast(r.error || '糟糕,操作没成功,请稍后重试~', 'err');
  };

  const handleDelete = async (t: ApiToken) => {
    if (!confirm(`确定删除令牌 #${t.id}${t.name ? '（' + t.name + '）' : ''}？删除后不可恢复。`)) return;
    const r: any = await deleteToken(t.id);
    if (r.ok) { showToast('已删除', 'ok'); load(); }
    else showToast(r.error || '删除失败', 'err');
  };

  const handleToCode = async () => {
    const amount = parseInt(toCodeAmount);
    if (!amount || amount <= 0) { setToCodeResult({ ok: false, msg: '请输入有效的积分数量' }); return; }
    const c = captchaRef.current?.getChallenge() || '';
    const a = captchaRef.current?.getAnswer() || '';
    if (!c || !a) { setToCodeResult({ ok: false, msg: '请先完成验证码~' }); return; }
    setToCodeResult(null);
    const r: any = await nutsToCode(amount, c, a);
    if (r.ok) {
      successSound();
      setToCodeResult({ ok: true, msg: `已生成 ${r.code_value} 积分兑换码（消耗 ${amount} 积分）`, code: r.code });
      setToCodeAmount('');
      captchaRef.current?.reset();
    } else {
      setToCodeResult({ ok: false, msg: r.error || '转换失败' });
      captchaRef.current?.reset();
      setCaptchaKey(k => k + 1);
    }
  };

  const handleCreateQuestionCode = async () => {
    const amount = parseInt(qcAmount);
    if (!amount || amount <= 0) { setQcResult({ ok: false, msg: '请输入有效的积分数量' }); return; }
    if (!qcQuestion.trim()) { setQcResult({ ok: false, msg: '请填写问题~' }); return; }
    if (!qcAnswer.trim()) { setQcResult({ ok: false, msg: '请填写答案~' }); return; }
    const c = qcCaptchaRef.current?.getChallenge() || '';
    const a = qcCaptchaRef.current?.getAnswer() || '';
    if (!c || !a) { setQcResult({ ok: false, msg: '请先完成验证码~' }); return; }
    setQcResult(null);
    const r: any = await nutsToQuestionCode(amount, qcQuestion.trim(), qcAnswer.trim(), c, a);
    if (r.ok) {
      successSound();
      setQcResult({ ok: true, msg: `已生成 ${r.code_value} 积分答题码（消耗 ${amount} 积分）`, code: r.code });
      setQcAmount(''); setQcQuestion(''); setQcAnswer('');
      qcCaptchaRef.current?.reset();
      loadCodes();
    } else {
      setQcResult({ ok: false, msg: r.error || '糟糕,创建失败了,请检查后重试~' });
      qcCaptchaRef.current?.reset();
      setQcCaptchaKey(k => k + 1);
    }
  };

  const handleCreateRedPacket = async () => {
    const amount = parseInt(rpAmount);
    const count = parseInt(rpCount);
    if (!amount || amount <= 0) { setRpResult({ ok: false, msg: '请输入有效的积分数量' }); return; }
    if (!count || count <= 0) { setRpResult({ ok: false, msg: '请填写份数~' }); return; }
    const c = rpCaptchaRef.current?.getChallenge() || '';
    const a = rpCaptchaRef.current?.getAnswer() || '';
    if (!c || !a) { setRpResult({ ok: false, msg: '请先完成验证码~' }); return; }
    setRpResult(null);
    const r: any = await createRedPacket(amount, count, c, a);
    if (r.ok) {
      successSound();
      setRpResult({ ok: true, msg: r.message || `红包已创建（池 ${r.pool} 板栗，${r.count} 份）`, code: r.code });
      setRpAmount(''); setRpCount('');
      rpCaptchaRef.current?.reset();
      loadPackets();
    } else {
      setRpResult({ ok: false, msg: r.error || '糟糕,创建失败了,请检查后重试~' });
      rpCaptchaRef.current?.reset();
      setRpCaptchaKey(k => k + 1);
    }
  };

  const loadCodes = async () => {
    const r: any = await listMyCodes();
    if (r.ok) setMyCodes(r.codes || []);
  };

  const loadPackets = async () => {
    const r: any = await listRedPackets();
    if (r.ok) setMyPackets(r.packets || []);
  };

  const handleShowCodes = async () => {
    setShowCodes(!showCodes);
    if (!showCodes && myCodes.length === 0) await loadCodes();
  };

  const handleShowPackets = async () => {
    setShowPackets(!showPackets);
    if (!showPackets && myPackets.length === 0) await loadPackets();
  };

  if (loading) {
    return (
      <section className="card" style={{ marginBottom: 16 }}>
        <div className="skeleton-line" style={{ width: 140, height: 18, marginBottom: 10 }} />
        <div className="skeleton-line" style={{ width: 220, height: 14 }} />
      </section>
    );
  }

  return (
    <section className="card" style={{ marginBottom: 16 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
        <h3 style={{ margin: 0 }}>令牌管理</h3>
        <div style={{ display: 'flex', gap: 6, alignItems: 'center', flexWrap: 'wrap' }}>
          <button className="btn btn-sm btn-outline" style={{ fontSize: 12, padding: '2px 8px', minWidth: 0 }} onClick={() => setShowToCode(true)}>积分转码</button>
          <button className="btn btn-sm btn-outline" style={{ fontSize: 12, padding: '2px 8px', minWidth: 0 }} onClick={() => setShowQCode(true)}>创建答题码</button>
          <button className="btn btn-sm btn-outline" style={{ fontSize: 12, padding: '2px 8px', minWidth: 0 }} onClick={() => setShowRP(true)}>发红包</button>
          <button className="btn btn-sm btn-primary" style={{ fontSize: 12, padding: '2px 10px', minWidth: 0 }} onClick={() => setShowAdd(true)}>+ 添加令牌</button>
        </div>
      </div>

      {/* 主令牌 — 突出展示 */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap',
        background: 'var(--phx-warm-bg)', border: '1px solid var(--phx-warm-border)',
        borderRadius: 10, padding: '12px 14px', marginBottom: 12,
      }}>
        <span style={{ fontSize: 12, fontWeight: 700, color: '#c8a45c', border: '1px solid #c8a45c', borderRadius: 6, padding: '2px 8px' }}>主令牌</span>
        <code style={{ flex: 1, minWidth: 0, fontSize: 12, wordBreak: 'break-all', color: 'var(--phx-warm-text)' }}>{primaryToken}</code>
        <button className="btn btn-sm btn-outline" style={{ flexShrink: 0 }} onClick={() => copyText(primaryToken)}>复制</button>
        <button className="btn btn-sm btn-danger" style={{ flexShrink: 0 }} onClick={handleResetPrimary}>重置</button>
      </div>

      {/* 子令牌列表 */}
      {tokens.length === 0 ? (
        <div className="empty-state" style={{ padding: '12px' }}>暂无子令牌，点击右上角「+ 添加令牌」创建</div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {tokens.map(t => (
            <div key={t.id} style={{ border: '1px solid var(--phx-border)', borderRadius: 8, padding: '10px 12px', opacity: t.disabled ? 0.5 : 1 }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 6, marginBottom: 6 }}>
                <div>
                  <span style={{ fontWeight: 600 }}>#{t.id}</span>
                  {t.name && <span style={{ marginLeft: 6, color: 'var(--phx-text-secondary)' }}>{t.name}</span>}
                </div>
                <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
                  <button className="btn btn-sm btn-outline" onClick={() => copyText(t.token || '')}>复制</button>
                  <button className="btn btn-sm btn-outline" onClick={() => handleToggle(t)}>{t.disabled ? '启用' : '停用'}</button>
                  <button className="btn btn-sm btn-outline" onClick={() => handleReset(t)}>重置</button>
                  <button className="btn btn-sm btn-danger" onClick={() => handleDelete(t)}>删除</button>
                </div>
              </div>
              <div style={{ fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all', color: 'var(--phx-text-secondary)', marginBottom: 4 }}>{t.token}</div>
              <div style={{ fontSize: 12, color: 'var(--phx-text-secondary)' }}>
                调用上限 {fmtLimit(t.max_calls, '次')} · 已用 <strong>{t.call_count}</strong> 次
                <span style={{ margin: '0 6px' }}>|</span>
                积分上限 {fmtLimit(t.max_nuts, '个')} · 已耗 <strong>{t.nuts_consumed}</strong> 个
                {t.disabled && <span style={{ marginLeft: 8, color: 'var(--phx-red)' }}>已停用</span>}
              </div>
            </div>
          ))}
        </div>
      )}

      <div style={{ fontSize: 12, color: 'var(--phx-text-secondary)', marginTop: 10 }}>
        已调用总数：<strong>{totalCalls}</strong> 次
      </div>

      {/* 我的兑换码 */}
      <div style={{ marginTop: 10 }}>
        <button className="btn btn-sm btn-outline" onClick={handleShowCodes} style={{ fontSize: 12, padding: '2px 10px' }}>
          {showCodes ? '收起' : '展开'} 我的兑换码（{myCodes.length}）
        </button>
        {showCodes && myCodes.length > 0 && (
          <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
            {myCodes.map((c: any) => (
              <div key={c.id} style={{ display: 'flex', alignItems: 'center', gap: 8, border: '1px solid var(--phx-border)', borderRadius: 6, padding: '6px 10px' }}>
                <code style={{ flex: 1, fontSize: 12, wordBreak: 'break-all' }}>{c.code}</code>
                <span style={{ fontSize: 12, color: 'var(--phx-text-secondary)', flexShrink: 0 }}>{c.amount} 积分</span>
                <button className="btn btn-sm btn-outline" style={{ fontSize: 11, padding: '2px 8px', minWidth: 0 }} onClick={() => { navigator.clipboard.writeText(c.code); showToast('已复制', 'ok'); }}>复制</button>
              </div>
            ))}
          </div>
        )}
        {showCodes && myCodes.length === 0 && (
          <div className="empty-state" style={{ padding: '8px', fontSize: 12 }}>暂无兑换码</div>
        )}
      </div>

      {/* 我的红包 */}
      <div style={{ marginTop: 10 }}>
        <button className="btn btn-sm btn-outline" onClick={handleShowPackets} style={{ fontSize: 12, padding: '2px 10px' }}>
          {showPackets ? '收起' : '展开'} 我的红包（{myPackets.length}）
        </button>
        {showPackets && myPackets.length > 0 && (
          <div style={{ marginTop: 8, display: 'flex', flexDirection: 'column', gap: 6 }}>
            {myPackets.map((p: any) => (
              <div key={p.id} style={{ display: 'flex', alignItems: 'center', gap: 8, border: '1px solid var(--phx-border)', borderRadius: 6, padding: '6px 10px', flexWrap: 'wrap' }}>
                <code style={{ fontSize: 12, wordBreak: 'break-all' }}>{p.code}</code>
                <span style={{ fontSize: 12, color: 'var(--phx-text-secondary)' }}>池 {p.total} · 剩 {p.remaining} / {p.remaining_count} 份</span>
                <button className="btn btn-sm btn-outline" style={{ fontSize: 11, padding: '2px 8px', minWidth: 0 }} onClick={() => { navigator.clipboard.writeText(p.code); showToast('已复制', 'ok'); }}>复制</button>
              </div>
            ))}
          </div>
        )}
        {showPackets && myPackets.length === 0 && (
          <div className="empty-state" style={{ padding: '8px', fontSize: 12 }}>暂无红包</div>
        )}
      </div>

      {/* 添加令牌弹窗 */}
      <Modal open={showAdd} title="添加令牌" message="为子令牌命名并设置限制（留空表示不限）。"
        confirmLabel="创建" cancelLabel="取消"
        onConfirm={handleCreate}
        onCancel={() => { setShowAdd(false); setAddName(''); setAddMaxCalls(''); setAddMaxNuts(''); }}>
        <input className="input" value={addName} onChange={e => setAddName(e.target.value)} placeholder="令牌名称（如 工作机）" style={{ width: '100%', marginTop: 8 }} />
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <input className="input" type="number" min="0" value={addMaxCalls} onChange={e => setAddMaxCalls(e.target.value)} placeholder="调用上限" title="累计调用次数上限，留空为不限" style={{ flex: 1 }} />
          <input className="input" type="number" min="0" value={addMaxNuts} onChange={e => setAddMaxNuts(e.target.value)} placeholder="积分上限" title="累计消耗积分上限，留空为不限" style={{ flex: 1 }} />
        </div>
      </Modal>

      {/* 新令牌结果弹窗 */}
      <Modal open={!!result} title={result?.title || ''} message="令牌仅在此展示一次，请复制保存。"
        confirmLabel="完成" onConfirm={() => setResult(null)}>
        <div style={{ fontFamily: 'monospace', fontWeight: 700, fontSize: 13, wordBreak: 'break-all', background: 'var(--phx-warm-solid)', padding: 12, borderRadius: 8, marginTop: 8 }}>
          {result?.value}
        </div>
        <button className="btn btn-sm btn-primary" style={{ width: '100%', marginTop: 8 }} onClick={() => { if (result) copyText(result.value); }}>复制令牌</button>
      </Modal>

      {/* 积分转兑换码弹窗 */}
      <Modal open={showToCode} title="积分转兑换码" message="每 10 积分可生成 9 积分的兑换码（差额不返还）。"
        confirmLabel="转换" cancelLabel="关闭"
        onConfirm={handleToCode}
        onCancel={() => { setShowToCode(false); setToCodeResult(null); }}>
        <Captcha key={captchaKey} ref={captchaRef} />
        <input className="input" type="number" min="10" value={toCodeAmount} onChange={e => setToCodeAmount(e.target.value)} placeholder="输入要转换的积分数量" style={{ width: '100%', textAlign: 'center', marginTop: 8 }} />
        {toCodeResult && (
          <div className={'result-box ' + (toCodeResult.ok ? 'ok' : 'err')} style={{ marginTop: 8 }}>
            {toCodeResult.msg}
            {toCodeResult.code && (
              <div style={{ fontFamily: 'monospace', fontWeight: 700, marginTop: 4, wordBreak: 'break-all' }}>{toCodeResult.code}</div>
            )}
          </div>
        )}
      </Modal>

      {/* 创建答题码弹窗 */}
      <Modal open={showQCode} title="创建答题码" message="设置一个问题，答对的人才能兑换。每 10 积分可生成 9 积分的答题码（差额不返还）。"
        confirmLabel="创建" cancelLabel="关闭"
        onConfirm={handleCreateQuestionCode}
        onCancel={() => { setShowQCode(false); setQcResult(null); }}>
        <Captcha key={qcCaptchaKey} ref={qcCaptchaRef} />
        <input className="input" type="number" min="10" value={qcAmount} onChange={e => setQcAmount(e.target.value)} placeholder="消耗的积分数量" style={{ width: '100%', marginTop: 8 }} />
        <input className="input" value={qcQuestion} onChange={e => setQcQuestion(e.target.value)} placeholder="问题（答对才能兑换）" style={{ width: '100%', marginTop: 8 }} />
        <input className="input" value={qcAnswer} onChange={e => setQcAnswer(e.target.value)} placeholder="答案（仅作校验，不展示）" style={{ width: '100%', marginTop: 8 }} />
        {qcResult && (
          <div className={'result-box ' + (qcResult.ok ? 'ok' : 'err')} style={{ marginTop: 8 }}>
            {qcResult.msg}
            {qcResult.code && (
              <div style={{ fontFamily: 'monospace', fontWeight: 700, marginTop: 4, wordBreak: 'break-all' }}>{qcResult.code}</div>
            )}
          </div>
        )}
      </Modal>

      {/* 创建红包弹窗 */}
      <Modal open={showRP} title="发红包" message="扣除积分生成红包，大家随机瓜分，每人限抢一次。每 10 积分池约 9 积分。"
        confirmLabel="创建" cancelLabel="关闭"
        onConfirm={handleCreateRedPacket}
        onCancel={() => { setShowRP(false); setRpResult(null); }}>
        <Captcha key={rpCaptchaKey} ref={rpCaptchaRef} />
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <input className="input" type="number" min="1" value={rpAmount} onChange={e => setRpAmount(e.target.value)} placeholder="消耗积分" title="扣除的积分，红包池约为其 9/10" style={{ flex: 1 }} />
          <input className="input" type="number" min="1" value={rpCount} onChange={e => setRpCount(e.target.value)} placeholder="份数" title="最多可领取人数，需 ≤ 积分" style={{ flex: 1 }} />
        </div>
        {rpResult && (
          <div className={'result-box ' + (rpResult.ok ? 'ok' : 'err')} style={{ marginTop: 8 }}>
            {rpResult.msg}
            {rpResult.code && (
              <div style={{ fontFamily: 'monospace', fontWeight: 700, marginTop: 4, wordBreak: 'break-all' }}>{rpResult.code}</div>
            )}
          </div>
        )}
      </Modal>
    </section>
  );
}
