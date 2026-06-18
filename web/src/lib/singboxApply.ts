import type { CreateClientResponse, CreateInboundResponse, SingboxApplySummary } from '../api/types';

type SingboxWriteResponse = {
  applied?: boolean;
  detail?: string;
  error?: string;
  warnings?: string[];
  post_apply_warnings?: string[];
  non_fatal_warnings?: string[];
  singbox?: SingboxApplySummary;
};

export function singboxApplyWarning(response: CreateInboundResponse | CreateClientResponse | SingboxWriteResponse | unknown, prefix: string): string {
  if (!response || typeof response !== 'object') return '';
  const data = response as SingboxWriteResponse;
  if (data.applied === false || data.singbox?.applied === false) {
    const detail = data.detail || data.singbox?.detail || data.error || data.singbox?.error || '未知错误';
    return `${prefix}：${detail}`;
  }
  const postApplyWarnings = firstNonEmptyWarnings(data.post_apply_warnings, data.singbox?.post_apply_warnings);
  const listenerWarning = firstListenerWarning(postApplyWarnings) || firstListenerWarning(firstNonEmptyWarnings(data.warnings, data.singbox?.warnings));
  if (listenerWarning) return listenerWarning;
  return '';
}

export function singboxApplyWarningTone(response: CreateInboundResponse | CreateClientResponse | SingboxWriteResponse | unknown): 'error' | 'info' {
  if (!response || typeof response !== 'object') return 'error';
  const data = response as SingboxWriteResponse;
  if (data.applied === false || data.singbox?.applied === false) return 'error';
  return 'info';
}

function firstNonEmptyWarnings(...groups: Array<string[] | undefined>): string[] {
  return groups.find((warnings) => Array.isArray(warnings) && warnings.length > 0) || [];
}

function firstListenerWarning(warnings: string[]): string {
  return warnings.find((warning) => warning.startsWith('配置已应用，但端口未监听')) || '';
}

export function singboxApplyFailureWarning(response: CreateInboundResponse | CreateClientResponse | SingboxWriteResponse | unknown, prefix: string): string {
  if (!response || typeof response !== 'object') return '';
  const data = response as SingboxWriteResponse;
  if (data.applied !== false && data.singbox?.applied !== false) return '';
  const detail = data.detail || data.singbox?.detail || data.error || data.singbox?.error || '未知错误';
  return `${prefix}：${detail}`;
}

export function showSingboxApplyWarning(
  response: unknown,
  prefix: string,
  showToast: (title: string, tone?: 'success' | 'error' | 'info') => void,
  text: (value: string) => string = (value) => value,
): boolean {
  const warning = singboxApplyWarning(response, prefix);
  if (!warning) return false;
  showToast(text(warning), singboxApplyWarningTone(response));
  return true;
}
