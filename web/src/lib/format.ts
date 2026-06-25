export function formatBytes(value: number | undefined | null): string {
  const n = Number(value || 0);
  if (!Number.isFinite(n) || n <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.max(0, Math.min(Math.floor(Math.log(n) / Math.log(1024)), units.length - 1));
  return `${(n / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function formatPercent(value: number | undefined | null): string {
  const n = Math.max(0, Math.min(100, Number(value || 0)));
  return `${n.toFixed(0)}%`;
}

export function formatDuration(seconds: number | undefined | null): string {
  const total = Math.max(0, Math.floor(Number(seconds || 0)));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

export function compactNumber(value: number | undefined | null): string {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 1, notation: 'compact' }).format(Number(value || 0));
}

export function serviceLabel(status?: string): string {
  const normalized = String(status || '').trim().toLowerCase();
  if (normalized === 'running' || normalized === 'active') return '运行中';
  if (normalized === 'stopped' || normalized === 'inactive') return '已停止';
  if (normalized === 'failed') return '失败';
  if (normalized === 'not_managed' || normalized === 'unmanaged') return '未托管';
  if (normalized === 'not_installed') return '未安装';
  if (normalized === 'unknown' || normalized === '') return '未知';
  return status || '未知';
}

export function versionLabel(version?: string): string {
  const raw = String(version || '').trim();
  const firstLine = raw.split('\n').map((line) => line.trim()).find((line) => line && !line.startsWith('Tags:')) || raw;
  const normalized = firstLine.toLowerCase();
  if (normalized === 'not_installed') return '未安装';
  if (normalized === 'unknown') return '未知';
  const xray = firstLine.match(/^xray\s+([^\s]+)/i);
  if (xray) return `Xray ${xray[1]}`;
  const singbox = firstLine.match(/^sing-box(?:\s+version)?\s+([^\s]+)/i);
  if (singbox) return `sing-box ${singbox[1]}`;
  return firstLine || '-';
}

export function randomUUID(): string {
  if (crypto.randomUUID) return crypto.randomUUID();
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === 'x' ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}
