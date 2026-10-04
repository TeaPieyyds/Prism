import { useState } from 'react';
import { submitSurvey } from '../api';
import ModalPortal from './ModalPortal';

interface Props {
  survey: any;
  questions: any[];
  onClose: () => void;
  onSubmitted: (reward: number) => void;
}

function parseOptions(opts: string): string[] {
  try { return JSON.parse(opts); } catch { return []; }
}

export default function SurveyModal({ survey, questions, onClose, onSubmitted }: Props) {
  const [answers, setAnswers] = useState<Record<number, string>>({});
  const [otherTexts, setOtherTexts] = useState<Record<number, string>>({});
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const handleSubmit = async () => {
    for (const q of questions) {
      const v = typeof answers[q.id] === 'string' ? answers[q.id].trim() : '';
      if (!v) {
        setError('请把所有问题都答完~');
        return;
      }
    }
    setSubmitting(true);
    setError('');
    const answerList = Object.entries(answers).map(([qid, text]) => ({
      question_id: Number(qid),
      answer_text: text,
      survey_id: survey.id,
      user_id: 0,
    }));
    const res: any = await submitSurvey(survey.id, answerList);
    if (res.ok) {
      onSubmitted(res.reward_nuts || survey.reward_nuts || 0);
    } else {
      setError(res.error || '哎呀,提交失败了,请稍后重试~');
      setSubmitting(false);
    }
  };

  return (
    <ModalPortal>
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal-content" onClick={e => e.stopPropagation()}
        style={{ maxWidth: 520, maxHeight: '85dvh', display: 'flex', flexDirection: 'column', padding: '24px' }}>
        <h3 style={{ margin: '0 0 4px', fontSize: 18, color: 'var(--phx-text)' }}>{survey.title}</h3>
        <p style={{ margin: '0 0 16px', fontSize: 13, color: 'var(--phx-text-secondary)' }}>
          完成问卷可获得 <strong style={{ color: 'var(--phx-success)' }}>{survey.reward_nuts} 板栗</strong>
        </p>
        <div style={{ flex: 1, overflowY: 'auto', marginBottom: 16, minHeight: 0 }}>
          {questions.map((q: any, idx: number) => (
            <div key={q.id} style={{ marginBottom: 20 }}>
              <p style={{ margin: '0 0 10px', fontWeight: 700, fontSize: 14 }}>
                {idx + 1}. {q.question_text}
              </p>
              {q.question_type === 'choice' ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {parseOptions(q.options).map((opt: string, oi: number) => {
                    const isOther = opt === '其他';
                    const selected = answers[q.id] === opt || (isOther && (answers[q.id]||'').startsWith('其他:'));
                    return (
                    <div key={oi}>
                    <label style={{
                      display: 'flex', alignItems: 'center', gap: 8,
                      padding: '10px 14px', borderRadius: 12, cursor: 'pointer',
                      background: selected ? 'rgba(25,200,185,0.1)' : 'var(--phx-bg)',
                      border: selected ? '2px solid var(--phx-primary)' : '1px solid var(--phx-border-light)',
                      fontSize: 14, transition: 'all .15s',
                    }}>
                      <input type="radio" name={`q_${q.id}`} value={opt}
                        checked={selected}
                        onChange={() => setAnswers(p => ({ ...p, [q.id]: opt }))}
                        style={{ accentColor: 'var(--phx-primary)' }} />
                      {opt}
                    </label>
                    {isOther && selected && (
                      <input className="input" placeholder="请填写具体内容..."
                        style={{margin:'6px 0 0 28px',width:'calc(100% - 50px)'}}
                        value={otherTexts[q.id] || ''}
                        onChange={e => {
                          setOtherTexts(p => ({ ...p, [q.id]: e.target.value }));
                          setAnswers(p => ({ ...p, [q.id]: '其他:' + e.target.value }));
                        }} />
                    )}
                    </div>
                  )})}
                </div>
              ) : q.question_type === 'multi_choice' ? (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                  {parseOptions(q.options).map((opt: string, oi: number) => {
                    const selected = ((answers[q.id] as string) || '').split(',').filter(Boolean);
                    const isOther = opt === '其他';
                    const checked = selected.includes(opt);
                    const otherChecked = selected.some(s => s.startsWith('其他:'));
                    const isChecked = checked || (isOther && otherChecked);
                    return (
                    <div key={oi}>
                      <label style={{
                        display: 'flex', alignItems: 'center', gap: 8,
                        padding: '10px 14px', borderRadius: 12, cursor: 'pointer',
                        background: isChecked ? 'rgba(25,200,185,0.1)' : 'var(--phx-bg)',
                        border: isChecked ? '2px solid var(--phx-primary)' : '1px solid var(--phx-border-light)',
                        fontSize: 14, transition: 'all .15s',
                      }}>
                        <input type="checkbox" value={opt} checked={isChecked}
                          onChange={() => {
                            if (isOther) {
                              if (otherChecked) {
                                const next = selected.filter(s => !s.startsWith('其他:'));
                                setAnswers(p => ({ ...p, [q.id]: next.join(',') }));
                              } else {
                                const next = [...selected, '其他:' + (otherTexts[q.id] || '')];
                                setAnswers(p => ({ ...p, [q.id]: next.join(',') }));
                              }
                            } else {
                              const next = checked ? selected.filter(s => s !== opt) : [...selected, opt];
                              setAnswers(p => ({ ...p, [q.id]: next.join(',') }));
                            }
                          }}
                          style={{ accentColor: 'var(--phx-primary)' }} />
                        {opt}
                      </label>
                      {isOther && isChecked && (
                        <input className="input" placeholder="请填写具体内容..."
                          style={{margin:'6px 0 0 28px',width:'calc(100% - 50px)'}}
                          value={otherTexts[q.id] || ''}
                          onChange={e => {
                            setOtherTexts(p => ({ ...p, [q.id]: e.target.value }));
                            const next = selected.filter(s => !s.startsWith('其他:'));
                            next.push('其他:' + e.target.value);
                            setAnswers(p => ({ ...p, [q.id]: next.join(',') }));
                          }} />
                      )}
                    </div>
                  )})}
                </div>
              ) : (
                <textarea className="input textarea"
                  style={{ minHeight: 80, margin: 0 }}
                  placeholder="请输入你的回答..."
                  value={answers[q.id] || ''}
                  onChange={e => setAnswers(p => ({ ...p, [q.id]: e.target.value }))} />
              )}
            </div>
          ))}
        </div>
        {error && <div className="result-box err" style={{ marginBottom: 10 }}>{error}</div>}
        <div style={{ display: 'flex', gap: 10, justifyContent: 'flex-end' }}>
          <button className="btn btn-outline btn-sm" onClick={onClose} style={{ width: 'auto' }}>取消</button>
          <button className="btn btn-primary btn-sm" onClick={handleSubmit}
            disabled={submitting} style={{ width: 'auto' }}>
            {submitting ? '提交中...' : '提交问卷'}
          </button>
        </div>
      </div>
    </div>
    </ModalPortal>
  );
}
