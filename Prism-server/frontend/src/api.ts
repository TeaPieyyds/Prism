import type { ApiResponse } from './types';

const BASE = '';

export async function api<T = ApiResponse>(
  method: string,
  path: string,
  data?: unknown
): Promise<T> {
  try {
    const opts: RequestInit = { method, headers: {} };
    const token = localStorage.getItem('session_token');
    if (token) {
      (opts.headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
    }
    if (data) {
      if (data instanceof FormData) {
        opts.body = data;
      } else {
        (opts.headers as Record<string, string>)['Content-Type'] = 'application/json';
        opts.body = JSON.stringify(data);
      }
    }
    const res = await fetch(BASE + path, opts);
    return await res.json();
  } catch (e) {
    console.error('API error', method, path, e);
    return { ok: false, error: '糟糕,网络开小差了,请检查网络连接后重试~' } as T;
  }
}

// Auth
export const register = (email: string, username: string, password: string, inviteCode?: string, captchaId?: string, captchaAnswer?: string) =>
  api('POST', '/api/auth/register', { email, username, password, invite_code: inviteCode || '', c: captchaId || '', a: captchaAnswer || '' });

// Checkin
export const checkin = () => api('POST', '/api/checkin');
export const checkinStatus = () => api('GET', '/api/checkin/status');

// Invite
export const myInviteCode = () => api('GET', '/api/invite/my-code');
export const redeemInvite = (code: string) => api('POST', '/api/invite/redeem', { code });
export const login = (email: string, password: string) =>
  api('POST', '/api/auth/login', { email, password });

// Accounts
export const listAccounts = () => api('GET', '/api/accounts');
export const addAccount = (cookie: string, shared = false) =>
  api('POST', '/api/accounts/add', { cookie, shared });
export const switchAccount = (id: number, tokenId?: number) =>
  api('POST', `/api/accounts/${id}/switch`, tokenId ? { token_id: tokenId } : undefined);
export const deleteAccount = (id: number) =>
  api('DELETE', `/api/accounts/${id}`);
export const refreshAccount = (id: number) =>
  api('POST', `/api/accounts/${id}/refresh`);
export const activeAccount = (tokenId?: number) =>
  api('GET', `/api/accounts/active${tokenId ? `?token_id=${tokenId}` : ''}`);
export const updateAccount = (id: number, data: { display_name?: string; shared?: boolean; auto_refresh_enabled?: boolean }) =>
  api('POST', `/api/accounts/${id}/update`, data);
export const updateProfile = (username: string) =>
  api('POST', '/api/auth/profile', { username });
export const updatePassword = (old_password: string, new_password: string) =>
  api('POST', '/api/auth/password', { old_password, new_password });
export const resetToken = () => api('POST', '/api/auth/reset-token');
// Multi-token management (secondary API tokens)
export const listTokens = () => api('GET', '/api/auth/tokens');
export const createToken = (data: { name?: string; max_calls?: number; max_nuts?: number }) =>
  api('POST', '/api/auth/tokens', data);
export const resetSubToken = (id: number) =>
  api('POST', `/api/auth/tokens/${id}/reset`);
export const updateToken = (id: number, data: { name?: string; max_calls?: number; max_nuts?: number; disabled?: boolean }) =>
  api('POST', `/api/auth/tokens/${id}/update`, data);
export const deleteToken = (id: number) =>
  api('DELETE', `/api/auth/tokens/${id}/delete`);
export const nutsToCode = (amount: number, captchaId?: string, captchaAnswer?: string) =>
  api('POST', '/api/auth/nuts/to-code', { amount, c: captchaId || '', a: captchaAnswer || '' });
export const nutsToQuestionCode = (amount: number, question: string, answer: string, captchaId?: string, captchaAnswer?: string) =>
  api('POST', '/api/auth/nuts/to-question-code', { amount, question, answer, c: captchaId || '', a: captchaAnswer || '' });
export const createRedPacket = (amount: number, count: number, captchaId?: string, captchaAnswer?: string) =>
  api('POST', '/api/auth/nuts/redpacket', { amount, count, c: captchaId || '', a: captchaAnswer || '' });
export const listRedPackets = () => api('GET', '/api/auth/redpackets');
export const listMyCodes = () => api('GET', '/api/auth/codes');
export const challengeOverride = (enabled: boolean) =>
  api('POST', '/api/auth/challenge-override', { enabled });
export const growthOverride = (enabled: boolean, value: number) =>
  api('POST', '/api/auth/growth-override', { enabled, value });
export const getNuts = () => api('GET', '/api/auth/nuts');
export const myLogs = (limit = 80, offset = 0, keyword?: string, action?: string) => {
  let path = `/api/auth/logs?limit=${limit}&offset=${offset}`;
  if (keyword) path += `&keyword=${encodeURIComponent(keyword)}`;
  if (action) path += `&action=${encodeURIComponent(action)}`;
  return api('GET', path);
};
export const updateAccountNickname = (id: number, nickname: string) =>
  api('POST', `/api/accounts/${id}/nickname`, { nickname });
export const getRealnamePresets = () => api('GET', '/api/realname-presets');
export const forgotPassword = (email: string, captchaId?: string, captchaAnswer?: string) =>
  api('POST', '/api/auth/forgot-password', { email, c: captchaId || '', a: captchaAnswer || '' });
export const resetPassword = (email: string, code: string, password: string) =>
  api('POST', '/api/auth/reset-password', { email, code, password });
export const changeEmail = (email: string, captchaId?: string, captchaAnswer?: string) =>
  api('POST', '/api/auth/change-email', { email, c: captchaId || '', a: captchaAnswer || '' });
export const confirmEmail = (email: string, code: string) =>
  api('POST', '/api/auth/confirm-email', { email, code });

// Backward-compatible wrappers for older components
export const guestLogin = () => api('POST', '/api/mpay/guest');
export const guestSendVerifySms = (flow_id: string) =>
  api('POST', '/api/mpay/guest/send-verify-sms', { flow_id });
export const guestBatch = (flow_id: string, real_name?: string, id_num?: string, max_batch?: number) =>
  api('POST', '/api/mpay/guest/batch', { flow_id, real_name, id_num, max_batch });
export const guestFinishSms = (flow_id: string) =>
  api('POST', '/api/mpay/guest/finish-sms', { flow_id });
export const guestRealname = (flow_id: string, name: string, id_num: string) =>
  api('POST', '/api/mpay/guest/realname', { flow_id, name, id_num });
export const guestRealnameAbandon = (flow_id: string) =>
  api('POST', '/api/mpay/guest/realname-abandon', { flow_id });
export const guestRename = (flow_id: string, name: string) =>
  api('POST', '/api/mpay/guest/rename', { flow_id, name });
export const guestRenameAbandon = (flow_id: string) =>
  api('POST', '/api/mpay/guest/rename-abandon', { flow_id });
export const emailLogin = (email: string, password: string) =>
  api('POST', '/api/mpay/login', { email, password });
export const sendSms = (phone: string) =>
  api('POST', '/api/mpay/phone/sms', { phone });
export const verifyPhone = (phone: string, code: string) =>
  api('POST', '/api/mpay/phone/verify', { phone, code });
export const addCookie = (cookie: string) => addAccount(cookie, false);
export const listCookies = () => listAccounts();
export const switchCookie = (id: string | number) => switchAccount(Number(id));
export const deleteCookie = (id: string | number) => deleteAccount(Number(id));
export const refreshCookie = (id: string | number) => refreshAccount(Number(id));
export const activeCookie = () => activeAccount();

// Admin
export const adminUsers = () => api('GET', '/api/admin/users');
export const adminAccounts = () => api('GET', '/api/admin/accounts');
export const adminUpdateUser = (id: number, data: { role?: string; disabled?: boolean; email?: string; username?: string; password?: string }) =>
  api('POST', `/api/admin/users/${id}`, data);
export const adminDeleteUser = (id: number) => api('DELETE', `/api/admin/users/${id}`);
export const adminAddAccount = (data: { cookie?: string; owner_id?: number; shared?: boolean; display_name?: string; uid?: string }) =>
  api('POST', '/api/admin/accounts/add', data);
export const adminAddUser = (email: string, username: string, password: string) =>
  api('POST', '/api/admin/users/add', { email, username, password });
export const adminUpdateAccount = (id: number, data: { display_name?: string; shared?: boolean; owner_id?: number; disabled?: boolean }) =>
  api('POST', `/api/admin/accounts/${id}`, data);
export const adminAuditLogs = (limit = 50, offset = 0, keyword?: string, action?: string, dateFrom?: string, dateTo?: string) => {
  let path = `/api/admin/logs/audit?limit=${limit}&offset=${offset}`;
  if (keyword) path += `&keyword=${encodeURIComponent(keyword)}`;
  if (action) path += `&action=${encodeURIComponent(action)}`;
  if (dateFrom) path += `&date_from=${encodeURIComponent(dateFrom)}`;
  if (dateTo) path += `&date_to=${encodeURIComponent(dateTo)}`;
  return api('GET', path);
};
export const adminSystemLogs = (limit = 50, offset = 0, keyword?: string, level?: string, dateFrom?: string, dateTo?: string) => {
  let path = `/api/admin/logs/system?limit=${limit}&offset=${offset}`;
  if (keyword) path += `&keyword=${encodeURIComponent(keyword)}`;
  if (level) path += `&action=${encodeURIComponent(level)}`;
  if (dateFrom) path += `&date_from=${encodeURIComponent(dateFrom)}`;
  if (dateTo) path += `&date_to=${encodeURIComponent(dateTo)}`;
  return api('GET', path);
};
export const adminAdjustNuts = (user_id: number, amount: number, reason: string) =>
  api('POST', '/api/admin/nuts/adjust', { user_id, amount, reason });
export const adminGrantSubscription = (user_id: number, days: number) =>
  api('POST', '/api/admin/subscription', { user_id, days });
export const adminRevokeSubscription = (user_id: number) =>
  api('POST', '/api/admin/subscription/revoke', { user_id });
export const adminCreateRedPacket = (total: number, count: number) =>
  api('POST', '/api/admin/redpackets', { total, count });
export const addRealnamePreset = (name: string, idNumber: string) =>
  api('POST', '/api/admin/realname-presets/add', { name, id_number: idNumber });
export const deleteRealnamePreset = (id: number) =>
  api('DELETE', `/api/admin/realname-presets/${id}`);
export const toggleRealnamePreset = (id: number, enabled: boolean) =>
  api('POST', '/api/admin/realname-preset-toggle', { id, enabled });
export const getAllRealnamePresets = () =>
  api('GET', '/api/admin/realname-presets-all');
export const getSkinPresets = () =>
  api('GET', '/api/phoenix/skin/presets');
export const addSkinPreset = (name: string, item_id: string) =>
  api('POST', `/api/admin/skin-presets?name=${encodeURIComponent(name)}&item_id=${encodeURIComponent(item_id)}`);
export const deleteSkinPreset = (id: number) =>
  api('DELETE', `/api/admin/skin-presets?id=${id}`);
export const updateSkinPreset = (id: number, name: string, item_id: string) =>
  api('PUT', '/api/admin/skin-presets', { id, name, item_id });
export const changeSkin = (item_id: string, account_id?: number | null) =>
  api('POST', '/api/phoenix/skin/change', account_id ? { item_id, account_id } : { item_id });
export const getDownloadInfo = (item_id: string) =>
  api('GET', `/api/download?item_id=${encodeURIComponent(item_id)}`);
export const purchaseItem = (item_id: string) =>
  api('POST', '/api/purchase', { item_id });

// Stats
export const getUserStatsSummary = (hours = 168) =>
  api('GET', `/api/auth/stats/summary?hours=${hours}`);
export const getUserEndpointStats = (hours = 168, granularity = 'hour') =>
  api('GET', `/api/auth/stats/endpoints?hours=${hours}&granularity=${granularity}`);
export const getUserServerStats = (hours = 720) =>
  api('GET', `/api/auth/stats/servers?hours=${hours}`);
export const getUserNutsTimeline = (hours = 720) =>
  api('GET', `/api/auth/stats/nuts?hours=${hours}`);
export const getUserActiveEndpoints = () =>
  api('GET', '/api/auth/stats/active-endpoints');

// Payment
export const getPaymentProducts = () =>
  api('GET', '/api/payment/products');
export const createPaymentOrder = (amount: number, pay_type: string) =>
  api('POST', '/api/payment/order', { amount, pay_type });
export const getPaymentOrders = () =>
  api('GET', '/api/payment/orders');
export const getPaymentOrderStatus = (order_no: string) =>
  api('GET', `/api/payment/order/status?order_no=${encodeURIComponent(order_no)}`);
export const adminPaymentOrders = (limit?: number, offset?: number) =>
  api('GET', `/api/admin/payment/orders?limit=${limit || 50}&offset=${offset || 0}`);
export const adminRetryPayment = (order_no: string) =>
  api('POST', '/api/admin/payment/retry', { order_no });

// Toolbox
export const getVersionConfig = () => api('GET', '/api/admin/toolbox/version');
export const updateVersionConfig = (cfg: any) => api('POST', '/api/admin/toolbox/version', cfg);
export const getAnnouncements = () => api('GET', '/api/admin/toolbox/announcements');
export const createAnnouncement = (a: any) => api('POST', '/api/admin/toolbox/announcements', a);
export const updateAnnouncement = (id: string, a: any) => api('PUT', `/api/admin/toolbox/announcement?id=${encodeURIComponent(id)}`, a);
export const deleteAnnouncement = (id: string) => api('DELETE', `/api/admin/toolbox/announcement?id=${encodeURIComponent(id)}`);
export const toggleAnnouncement = (id: string) => api('POST', `/api/admin/toolbox/announcement/toggle?id=${encodeURIComponent(id)}`);
export const getSignatures = () => api('GET', '/api/admin/toolbox/signatures');
export const addSignature = (pkg: string, hash: string, label: string) => api('POST', '/api/admin/toolbox/signatures', { package_name: pkg, signature_hash: hash, label });
export const deleteSignature = (id: number) => api('DELETE', `/api/admin/toolbox/signatures?id=${id}`);
export const getAppVersions = () => api('GET', '/api/admin/toolbox/versions');
export const switchAppVersion = (id: number) => api('POST', '/api/admin/toolbox/versions/switch', { id });
export const deleteAppVersion = (id: number) => api('DELETE', `/api/admin/toolbox/versions?id=${id}`);
export const scanReleases = () => api('POST', '/api/admin/toolbox/versions/scan');
// Toolbox user-facing
export const getMyToolboxInfo = () => api('GET', '/api/toolbox/my-info');
export const updateMyToolbox = (data: any) => api('POST', '/api/toolbox/my-info/update', data);
export const startToolboxTrial = () => api('POST', '/api/toolbox/start-trial');
export const purchaseToolboxFeature = (feature: string) => api('POST', `/api/toolbox/purchase/${feature}`);
export const extendToolboxAccess = (days: number) => api('POST', `/api/toolbox/purchase/extension?days=${days}`);
export const toolboxDownloadUrl = () => '/api/toolbox/download';
// Toolbox admin user management
export const getAdminToolboxUsers = () => api('GET', '/api/admin/toolbox/users');
export const getAdminToolboxUserConfig = (userId: number) => api('GET', `/api/admin/toolbox/user-config?user_id=${userId}`);
export const updateAdminToolboxUserConfig = (data: any) => api('POST', '/api/admin/toolbox/user-config', data);
export const getAdminToolboxDefaults = () => api('GET', '/api/admin/toolbox/defaults');
export const updateAdminToolboxDefaults = (data: any) => api('POST', '/api/admin/toolbox/defaults', data);
// Push notification management
export const getAdminPushMessages = (limit = 50, offset = 0) => api('GET', `/api/admin/push/messages?limit=${limit}&offset=${offset}`);
export const sendAdminPush = (data: any) => api('POST', '/api/admin/push/send', data);
export const deleteAdminPushMessage = (id: string) => api('DELETE', `/api/admin/push/message?id=${encodeURIComponent(id)}`);
export const getAdminPushStats = () => api('GET', '/api/admin/push/stats');
export const getAdminOnlineUsers = () => api('GET', '/api/admin/push/online-users');
export const getAdminUserDetail = (userId: number) => api('GET', `/api/admin/push/user-detail?user_id=${userId}`);
export const getAdminStatsOverview = (hours = 168, excludeTest = true) =>
  api('GET', `/api/admin/stats/overview?hours=${hours}&exclude_test=${excludeTest ? 1 : 0}`);
export const getAdminEndpointStats = (hours = 168, granularity = 'hour', excludeTest = true) =>
  api('GET', `/api/admin/stats/endpoints?hours=${hours}&granularity=${granularity}&exclude_test=${excludeTest ? 1 : 0}`);
export const getAdminPerUserStats = (hours = 168, excludeTest = true) =>
  api('GET', `/api/admin/stats/users?hours=${hours}&exclude_test=${excludeTest ? 1 : 0}`);
export const getAdminRegistrationStats = (hours = 720, granularity = 'day', excludeTest = true) =>
  api('GET', `/api/admin/stats/registrations?hours=${hours}&granularity=${granularity}&exclude_test=${excludeTest ? 1 : 0}`);
export const getAdminUserDetailStats = (userId: number) =>
  api('GET', `/api/admin/stats/user-detail?id=${userId}`);
export const getAdminErrorStats = (hours = 168, excludeTest = true) =>
  api('GET', `/api/admin/stats/errors?hours=${hours}&exclude_test=${excludeTest ? 1 : 0}`);
export const adminGetMessages = () => api('GET', '/api/admin/messages');
export const adminSetMessages = (messages: Record<string, string>) =>
  api('POST', '/api/admin/messages/set', { messages });
export const getAdminBusyHours = (excludeTest = true) =>
  api('GET', `/api/admin/stats/busy-hours?exclude_test=${excludeTest ? 1 : 0}`);
export const getAdminMethodStats = (excludeTest = true) =>
  api('GET', `/api/admin/stats/methods?exclude_test=${excludeTest ? 1 : 0}`);

// ── Survey ──
export const getActiveSurvey = () => api('GET', '/api/survey/active');
export const submitSurvey = (surveyId: number, answers: { question_id: number; answer_text: string }[]) =>
  api('POST', '/api/survey/submit', { survey_id: surveyId, answers });

export const adminListSurveys = () => api('GET', '/api/admin/surveys');
export const adminCreateSurvey = (data: {
  title: string; reward_nuts: number;
  questions: { question_text: string; question_type: string; sort_order: number; options: string }[];
}) => api('POST', '/api/admin/surveys/create', data);
export const adminUpdateSurvey = (id: number, title: string, rewardNuts: number) =>
  api('POST', '/api/admin/surveys/update', { id, title, reward_nuts: rewardNuts });
export const adminUpdateSurveyQuestions = (id: number, questions: any[]) =>
  api('POST', '/api/admin/surveys/questions', { id, questions });
export const adminToggleSurvey = (id: number) =>
  api('POST', '/api/admin/surveys/toggle', { id });
export const adminDeleteSurvey = (id: number) =>
  api('POST', '/api/admin/surveys/delete', { id });
export const adminClearSurveyAnswers = (id: number, userId?: number) =>
  api('POST', '/api/admin/surveys/clear-answers', { id, user_id: userId ?? null });
export const adminGetSurveyQuestions = (id: number) =>
  api('GET', `/api/admin/surveys/questions/list?id=${id}`);
export const adminGetSurveyStats = (id: number) =>
  api('GET', `/api/admin/surveys/stats?id=${id}`);
export const adminGetUserSurveyAnswers = (id: number, userId: number) =>
  api('GET', `/api/admin/surveys/user-answers?id=${id}&user_id=${userId}`);

// Server Owner Accounts
export const setServerOwner = (id: number, enabled: boolean) =>
  api('POST', `/api/accounts/${id}/server-owner`, { enabled });
export const listServerOwners = () => api('GET', '/api/accounts/server-owners');
export const getOwnerServers = () => api('GET', '/api/owner/servers');
export const refreshOwnerServers = () => api('POST', '/api/owner/servers/refresh');

// ── MC 建筑文件市场 ──
export const adminMarketFiles = (page = 1, pageSize = 20, q = '') =>
  api('GET', `/api/admin/market/files?page=${page}&page_size=${pageSize}&q=${encodeURIComponent(q)}`);
export const adminMarketFlagged = () => api('GET', '/api/admin/market/flagged');
export const adminMarketFileStatus = (id: number, status: number) =>
  api('POST', `/api/admin/market/files/${id}/status`, { status });
export const adminMarketFileUnflag = (id: number) =>
  api('POST', `/api/admin/market/files/${id}/unflag`);
export const adminMarketFileDelete = (id: number) =>
  api('POST', `/api/admin/market/files/${id}/delete`);
export const adminMarketConfigGet = () => api('GET', '/api/admin/market/config');
export const adminMarketConfigUpdate = (c: { upload_reward_nuts: number; report_auto_takedown: number }) =>
  api('POST', '/api/admin/market/config', c);
export const adminMarketCategoriesGet = () => api('GET', '/api/admin/market/categories');
export const adminMarketCategoryCreate = (name: string) => api('POST', '/api/admin/market/categories', { name });
export const adminMarketCategoryRename = (id: number, name: string) => api('PUT', `/api/admin/market/categories/${id}`, { name });
export const adminMarketCategoryDelete = (id: number) => api('DELETE', `/api/admin/market/categories/${id}`);
export const adminMarketFileUpdate = (id: number, data: { price_pts: number; allow_anonymous: boolean; tags: string[]; description: string; categories: string[] }) =>
  api('POST', `/api/admin/market/files/${id}/update`, data);
