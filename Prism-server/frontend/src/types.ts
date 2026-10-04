export interface StoredCookie {
  id: string;
  name: string;
  uid: string;
  cookie: string;
  status: string;
  display_name?: string;
  growth_level?: string;
  score?: string;
  skin_number?: string;
  cape_number?: string;
  is_vip?: boolean;
  realname_status?: string;
  anti_addition_status?: string;
  access_game_flag?: string;
  source?: string;
  signature?: string;
  is_active?: boolean;
  last_used?: number;
}

export interface ApiResponse {
  ok: boolean;
  error?: string;
  message?: string;
  stage?: string;
  need_verify?: boolean;
  verify_url?: string;
  code?: string;
  flow_id?: string;
  phone_hint?: string;

  // Cookie info
  id?: string;
  name?: string;
  uid?: string;
  display_name?: string;
  growth_level?: string;
  score?: string;
  skin_number?: string;
  cape_number?: string;
  is_vip?: boolean;
  realname_status?: string;
  anti_addition_status?: string;
  access_game_flag?: string;
  source?: string;
  signature?: string;
  status?: string;
  cookie?: string;
  count?: number;
  results?: Array<{ uid: string; cookie: string }>;

  // Cookie list
  cookies?: StoredCookie[];
  accounts?: StoredCookie[];
  user?: {
    id: number;
    username: string;
    email: string;
    role: 'user' | 'admin';
    token: string;
  };
  users?: Array<{
    id: number;
    username: string;
    email: string;
    role: string;
    verified: boolean;
    token: string;
  }>;

  // Stats
  summary?: UserStatsSummary;
  endpoints?: EndpointBreakdown[];
  servers?: ServerJoinStat[];
  points?: NutsTimelinePoint[];
  overview?: AdminOverview;
  registrations?: RegistrationStat[];
}

export interface UserStatsSummary {
  user_id: number;
  username: string;
  total_calls: number;
  success: number;
  errors: number;
  success_rate: number;
  join_count: number;
  nuts_used: number;
}

export interface EndpointStat {
  time: string;
  total: number;
  errors: number;
}

export interface EndpointBreakdown {
  endpoint: string;
  data: EndpointStat[];
}

export interface ServerJoinStat {
  server_code: string;
  count: number;
}

export interface NutsTimelinePoint {
  time: string;
  balance: number;
}

export interface ApiToken {
  id: number;
  user_id: number;
  name: string;
  token?: string;
  max_calls: number; // -1 = unlimited
  max_nuts: number;  // -1 = unlimited
  call_count: number;
  nuts_consumed: number;
  disabled: boolean;
  active_account_id?: number | null;
  active_account_name?: string;
  created_at: string;
}

export interface AdminOverview {
  total_calls: number;
  success_rate: number;
  active_users: number;
  total_users: number;
  today_regs: number;
}

export interface RegistrationStat {
  time: string;
  count: number;
}
