import type { ApiResponse } from '../types';

function safe(v: unknown, fallback = '-'): string {
  if (v === undefined || v === null || v === '') return fallback;
  return String(v);
}

function statusBadge(status?: string): string {
  const map: Record<string, string> = {
    normal: '正常',
    banned: '封禁',
    offline: '离线',
    unknown: '未知',
  };
  const colors: Record<string, string> = {
    normal: 'var(--phx-success)',
    banned: 'var(--phx-error)',
    offline: 'var(--phx-warning)',
    unknown: 'var(--phx-text-disabled)',
  };
  const s = status || 'unknown';
  return `<span style="color:${colors[s] || colors.unknown};font-weight:700">${map[s] || s}</span>`;
}

function accountInfoHtml(d: ApiResponse): string {
  let h = `<b>昵称:</b> ${safe(d.display_name || d.name)}<br>`;
  h += `<b>UID:</b> ${safe(d.uid)}<br>`;
  h += `<b>等级:</b> ${safe(d.growth_level, '0')}　<b>积分:</b> ${safe(d.score, '0')}<br>`;
  h += `<b>皮肤:</b> ${safe(d.skin_number, '0')}　<b>披风:</b> ${safe(d.cape_number, '0')}<br>`;
  h += `<b>VIP:</b> ${d.is_vip ? '是' : '否'}　<b>实名:</b> ${safe(d.realname_status, '0')}<br>`;
  h += `<b>防沉迷:</b> ${safe(d.anti_addition_status, '0')}　<b>准入:</b> ${safe(d.access_game_flag, '0')}<br>`;
  h += `<b>来源:</b> ${safe(d.source)}<br>`;
  h += `<b>状态:</b> ${statusBadge(d.status)}`;
  if (d.signature) h += `<br><b>签名:</b> ${d.signature}`;
  return h;
}

interface Props {
  data: ApiResponse;
  showCookieInfo?: boolean;
}

export default function ResultBox({ data, showCookieInfo }: Props) {
  if (!data) return null;

  const isOk = data.ok;
  const isVerify = !isOk && data.need_verify;

  let cls = 'result-box ';
  if (isOk) cls += 'ok';
  else if (isVerify) cls += 'info';
  else cls += 'err';

  return (
    <div className={cls}>
      {isOk ? (
        <div>
          <div className="result-icon">✓</div>
          <div className="result-text">
            {showCookieInfo ? (
              <div dangerouslySetInnerHTML={{ __html: accountInfoHtml(data) }} />
            ) : (
              <span>{data.message || '成功'}</span>
            )}
          </div>
        </div>
      ) : isVerify ? (
        <div>
          <div className="result-icon">⚠</div>
          <div className="result-text">
            <b>需要安全验证</b>
            {data.verify_url && (
              <a
                href={data.verify_url}
                target="_blank"
                rel="noopener"
                className="verify-link"
              >
                打开验证页面
              </a>
            )}
          </div>
        </div>
      ) : (
        <div>
          <div className="result-icon">✕</div>
          <div className="result-text">
            {data.error || data.message || '失败'}
          </div>
        </div>
      )}
    </div>
  );
}
