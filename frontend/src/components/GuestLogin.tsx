import { useEffect, useRef, useState } from 'react';
import { guestLogin, guestFinishSms, guestBatch, guestRealname, guestRealnameAbandon, guestRename, guestRenameAbandon } from '../api';

interface Props {
  onSuccess?: () => void;
}

type Step = 'idle' | 'sms' | 'ready' | 'registering' | 'realname' | 'rename' | 'done';

interface SmsInfo {
  code: string;
  phone: string;
  phone_bak: string;
  verify_url: string;
  flow_id: string;
}

// 批量注册上限：越大越好，后端循环到设备不再产出(如短信验证过期/风控)时自然停止。
const DEFAULT_BATCH = 5000;
// 改名步骤超时：每个账号分配时间内未完成改名 → 自动放弃入未发送。
const RENAME_TIMEOUT_MS = 5 * 60 * 1000;

function PickaxeSVG() {
  return (
    <svg width="56" height="56" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M14 2L18 6L10 14L8 16L6 18L2 22L6 18L8 16L10 14L14 2Z" fill="#8B8B8B" stroke="#5A5A5A" strokeWidth="0.5"/>
      <path d="M18 6L22 8L14 16L12 14L18 6Z" fill="#A0A0A0" stroke="#6B6B6B" strokeWidth="0.5"/>
      <rect x="15" y="5" width="2" height="3" rx="0.5" fill="#5C3D2E"/>
      <rect x="12" y="8" width="2" height="2" rx="0.5" fill="#5C3D2E"/>
      <rect x="9" y="11" width="2" height="2" rx="0.5" fill="#5C3D2E"/>
    </svg>
  );
}

export default function GuestLogin({ onSuccess }: Props) {
  const [step, setStep] = useState<Step>('idle');
  const [sms, setSms] = useState<SmsInfo | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [given, setGiven] = useState(0);
  const [renameTotal, setRenameTotal] = useState(0);
  const [renameDone, setRenameDone] = useState(0);
  const [name, setName] = useState('');
  const [realName, setRealName] = useState('');
  const [realIdNum, setRealIdNum] = useState('');

  const renameDoneRef = useRef(0);
  renameDoneRef.current = renameDone;

  // 改名步骤超时自动放弃（进入 rename 或每成功改一个号后重新计时）
  useEffect(() => {
    if (step !== 'rename' || !sms) return;
    const t = setTimeout(async () => {
      try { await guestRenameAbandon(sms.flow_id); } catch { /* ignore */ }
      setGiven(renameDoneRef.current);
      setStep('done');
      onSuccess?.();
    }, RENAME_TIMEOUT_MS);
    return () => clearTimeout(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, sms, renameDone]);

  const handleStart = async () => {
    setLoading(true);
    setError('');
    try {
      const r: any = await guestLogin();
      if (r?.ok) {
        setGiven(1);
        setStep('done');
        onSuccess?.();
        return;
      }
      if (r?.need_verify && r?.flow_id) {
        setSms({
          code: r.code || '367550',
          phone: r.phone || '1069016373035',
          phone_bak: r.phone_bak || '10698163016373035',
          verify_url: r.verify_url || '',
          flow_id: r.flow_id,
        });
        setStep('sms');
        return;
      }
      setError(r?.error || '糟糕,生成设备失败了,请稍后重试~');
    } catch (e: any) {
      setError(e?.message || '糟糕,生成设备失败了,请稍后重试~');
    } finally {
      setLoading(false);
    }
  };

  const handleSent = async () => {
    if (!sms) return;
    setLoading(true);
    setError('');
    try {
      const r: any = await guestFinishSms(sms.flow_id);
      if (r?.ok) {
        setStep('ready');
      } else {
        setError(r?.error || '哎呀,短信验证还没完成,请先完成验证再试~');
      }
    } catch (e: any) {
      setError(e?.message || '哎呀,短信验证还没完成,请先完成验证再试~');
    } finally {
      setLoading(false);
    }
  };

  const handleBatch = async () => {
    if (!sms) return;
    setLoading(true);
    setError('');
    setStep('registering');
    try {
      const r: any = await guestBatch(sms.flow_id, undefined, undefined, DEFAULT_BATCH);
      if (r?.ok) {
        setRenameTotal(r.to_rename || 0);
        setRenameDone(0);
        if ((r.to_realname || 0) > 0) {
          setStep('realname');
        } else if ((r.to_rename || 0) > 0) {
          setStep('rename');
        } else {
          setGiven(0);
          setStep('done');
          onSuccess?.();
        }
      } else {
        setError(r?.error || '糟糕,批量注册失败了,请稍后重试~');
        setStep('ready');
      }
    } catch (e: any) {
      setError(e?.message || '糟糕,批量注册失败了,请稍后重试~');
      setStep('ready');
    } finally {
      setLoading(false);
    }
  };

  const handleRename = async () => {
    if (!sms || !name.trim()) {
      setError('请输入昵称');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const r: any = await guestRename(sms.flow_id, name.trim());
      if (r?.ok) {
        setName('');
        const nd = renameDone + 1;
        setRenameDone(nd);
        if ((r.remaining || 0) === 0) {
          setGiven(nd);
          setStep('done');
          onSuccess?.();
        }
      } else {
        setError(r?.error || '改名失败，请换一个名字试试');
      }
    } catch (e: any) {
      setError(e?.message || '改名失败，请换一个名字试试');
    } finally {
      setLoading(false);
    }
  };

  const handleAbandon = async () => {
    if (!sms) return;
    setLoading(true);
    setError('');
    try {
      await guestRenameAbandon(sms.flow_id);
      setGiven(renameDone);
      setStep('done');
      onSuccess?.();
    } catch (e: any) {
      setError(e?.message || '操作失败，请重试');
    } finally {
      setLoading(false);
    }
  };

  const handleRealname = async () => {
    if (!sms || !realName.trim() || !realIdNum.trim()) {
      setError('请填写姓名和身份证号');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const r: any = await guestRealname(sms.flow_id, realName.trim(), realIdNum.trim());
      if (r?.ok) {
        setRealName('');
        setRealIdNum('');
        setRenameTotal(r.to_rename || 0);
        setRenameDone(0);
        if ((r.to_rename || 0) > 0) {
          setStep('rename');
        } else {
          setGiven(r.count || 0);
          setStep('done');
          onSuccess?.();
        }
      } else {
        setError(r?.error || '该身份证实名验证失败，请换一个');
      }
    } catch (e: any) {
      setError(e?.message || '该身份证实名验证失败，请换一个');
    } finally {
      setLoading(false);
    }
  };

  const handleRealnameAbandon = async () => {
    if (!sms) return;
    setLoading(true);
    setError('');
    try {
      await guestRealnameAbandon(sms.flow_id);
      setGiven(0);
      setStep('done');
      onSuccess?.();
    } catch (e: any) {
      setError(e?.message || '操作失败，请重试');
    } finally {
      setLoading(false);
    }
  };

  const reset = () => {
    setStep('idle');
    setSms(null);
    setError('');
    setGiven(0);
    setRenameTotal(0);
    setRenameDone(0);
    setName('');
    setRealName('');
    setRealIdNum('');
  };

  return (
    <div className="card">
      <div className="card-header" style={{ gap: 10 }}>
        <PickaxeSVG />
        <h2>游客注册</h2>
      </div>

      {step === 'idle' && (
        <>
          <p className="card-desc">点击下方按钮开始注册</p>
          <button className="btn btn-primary" style={{ width: '100%' }} disabled={loading}
            onClick={handleStart}>
            {loading ? '生成设备中…' : '开始注册'}
          </button>
          {error && <p style={{ color: 'var(--danger)', marginTop: 10, fontSize: 13 }}>{error}</p>}
        </>
      )}

      {step === 'sms' && sms && (
        <>
          <p className="card-desc">
            请使用<b>任意手机号</b>发送验证码
          </p>
          <div style={{ background: 'var(--bg)', borderRadius: 8, padding: 14, margin: '12px 0',
            textAlign: 'center', lineHeight: 1.9 }}>
            <div style={{ fontSize: 24, fontWeight: 800, color: 'var(--ink)', letterSpacing: 3 }}>
              {sms.code}
            </div>
            <div style={{ fontSize: 15, color: 'var(--ink)' }}>发送至 <b>{sms.phone}</b></div>
            <div style={{ fontSize: 12, color: 'var(--muted)' }}>备用：{sms.phone_bak}</div>
          </div>
          <a className="btn btn-outline"
            href={`sms:${sms.phone}?body=${encodeURIComponent(sms.code)}`}
            style={{ width: '100%', textAlign: 'center', marginBottom: 10 }}>
            打开短信应用发送
          </a>
          {sms.verify_url && (
            <a className="btn btn-outline" href={sms.verify_url} target="_blank" rel="noopener"
              style={{ width: '100%', textAlign: 'center', marginBottom: 10 }}>
              打开网页发送页面
            </a>
          )}
          <button className="btn btn-primary" style={{ width: '100%' }} disabled={loading}
            onClick={handleSent}>
            {loading ? '验证中…' : '我已发送，继续'}
          </button>
          {error && <p style={{ color: 'var(--danger)', marginTop: 10, fontSize: 13 }}>{error}</p>}
          <button className="btn btn-outline" style={{ width: '100%', marginTop: 10 }} onClick={reset}>
            取消
          </button>
        </>
      )}

      {step === 'ready' && (
        <>
          <p className="card-desc">点击下方按钮开始注册</p>
          <button className="btn btn-primary" style={{ width: '100%', marginTop: 10 }}
            disabled={loading} onClick={handleBatch}>
            {loading ? '批量注册中…' : '开始注册'}
          </button>
          {error && <p style={{ color: 'var(--danger)', marginTop: 10, fontSize: 13 }}>{error}</p>}
        </>
      )}

      {step === 'registering' && (
        <p className="card-desc" style={{ textAlign: 'center', padding: '20px 0' }}>
          正在批量注册，请稍候…
        </p>
      )}

      {step === 'realname' && (
        <>
          <p className="card-desc">请填写身份证完成实名</p>
          <input className="input" placeholder="姓名" value={realName}
            onChange={e => setRealName(e.target.value)} style={{ marginBottom: 8 }} />
          <input className="input" placeholder="身份证号" value={realIdNum}
            onChange={e => setRealIdNum(e.target.value)} />
          <button className="btn btn-primary" style={{ width: '100%', marginTop: 10 }}
            disabled={loading || !realName.trim() || !realIdNum.trim()} onClick={handleRealname}>
            {loading ? '实名中…' : '提交实名'}
          </button>
          {error && <p style={{ color: 'var(--danger)', marginTop: 10, fontSize: 13 }}>{error}</p>}
          <button className="btn btn-outline" style={{ width: '100%', marginTop: 10 }}
            disabled={loading} onClick={handleRealnameAbandon}>
            放弃实名
          </button>
        </>
      )}

      {step === 'rename' && (
        <>
          <p className="card-desc">
            请为第 <b>{renameDone + 1}</b> / {renameTotal} 个账号改名
          </p>
          <input className="input" placeholder="输入新昵称" value={name}
            onChange={e => setName(e.target.value)} />
          <button className="btn btn-primary" style={{ width: '100%', marginTop: 10 }}
            disabled={loading || !name.trim()} onClick={handleRename}>
            {loading ? '改名中…' : '提交改名'}
          </button>
          {error && <p style={{ color: 'var(--danger)', marginTop: 10, fontSize: 13 }}>{error}</p>}
          <button className="btn btn-outline" style={{ width: '100%', marginTop: 10 }}
            disabled={loading} onClick={handleAbandon}>
            放弃改名
          </button>
        </>
      )}

      {step === 'done' && (
        <>
          <p className="card-desc">注册完成，共 <b>{given}</b> 个账号。</p>
          <button className="btn btn-primary" style={{ width: '100%' }} onClick={reset}>
            再来一轮
          </button>
        </>
      )}
    </div>
  );
}
