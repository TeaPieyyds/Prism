import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from 'react';
import { api } from './api';

export interface AuthUser {
  id: number;
  username: string;
  email: string;
  role: 'user' | 'admin';
  token: string;
  challenge_override?: boolean;
  growth_override?: boolean;
  growth_override_value?: number;
  nuts_balance?: number;
  subscription_start?: string;
  subscription_until?: string;
}

interface AuthContextType {
  user: AuthUser | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<{ ok: boolean; error?: string }>;
  logout: () => void;
  checkAuth: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType>({
  user: null,
  loading: true,
  login: async () => ({ ok: false }),
  logout: () => {},
  checkAuth: async () => {},
});

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);

  const checkAuth = useCallback(async () => {
    const token = localStorage.getItem('session_token');
    if (!token) {
      setLoading(false);
      return;
    }
    const res = await api('GET', '/api/auth/me');
    if (res.ok && res.user) {
      setUser(res.user as AuthUser);
    } else if (!res.ok) {
      localStorage.removeItem('session_token');
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    checkAuth();
  }, [checkAuth]);

  const login = async (email: string, password: string) => {
    const res = await api('POST', '/api/auth/login', { email, password });
    if (res.ok && res.user) {
      const u = res.user as AuthUser;
      setUser(u);
      localStorage.setItem('session_token', u.token);
      return { ok: true };
    }
    if ((res as any).require_activation) {
      return { ok: false, require_activation: true, error: (res.error || '哎呀,你还没有激活账号,请先激活~') as string };
    }
    return { ok: false, error: (res.error || '糟糕,登录失败了,请核对账号密码后再试~') as string };
  };

  const logout = () => {
    setUser(null);
    localStorage.removeItem('session_token');
  };

  return (
    <AuthContext.Provider value={{ user, loading, login, logout, checkAuth }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
