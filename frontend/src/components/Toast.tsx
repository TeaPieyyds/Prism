import { useState, useEffect, useCallback } from 'react';
import { successSound } from '../sounds';

interface ToastItem { id: number; message: string; type: 'ok' | 'err' | 'info'; leaving: boolean; }

let toastId = 0;
let addToastFn: ((msg: string, type: ToastItem['type']) => void) | null = null;

export function showToast(msg: string, type: ToastItem['type'] = 'info') {
  if (type === 'ok') successSound();
  addToastFn?.(msg, type);
}

export default function Toast() {
  const [toasts, setToasts] = useState<ToastItem[]>([]);

  const add = useCallback((message: string, type: ToastItem['type']) => {
    const id = ++toastId;
    setToasts(prev => [...prev, { id, message, type, leaving: false }]);
    setTimeout(() => {
      setToasts(prev => prev.map(t => t.id === id ? { ...t, leaving: true } : t));
      setTimeout(() => setToasts(prev => prev.filter(t => t.id !== id)), 250);
    }, 2500);
  }, []);

  useEffect(() => { addToastFn = add; return () => { addToastFn = null; }; }, [add]);

  if (toasts.length === 0) return null;
  return (
    <div style={{position:'fixed',top:16,right:16,zIndex:9999,display:'flex',flexDirection:'column',gap:8}}>
      {toasts.map(t => (
        <div key={t.id} className={`toast-item toast-${t.type}${t.leaving ? ' leaving' : ''}`}>
          {t.type === 'ok' ? '✅' : t.type === 'err' ? '❌' : 'ℹ️'} {t.message}
        </div>
      ))}
    </div>
  );
}
