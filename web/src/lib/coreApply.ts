import type { CreateClientResponse, CreateInboundResponse, SingboxApplySummary, XrayApplySummary } from '../api/types';

type CoreWriteResponse = {
  applied?: boolean;
  config_changed?: boolean;
  pending_apply?: boolean;
  pending_cores?: string[];
  auto_apply?: Record<string, { status?: string; error?: string; detail?: string }>;
  auto_apply_error?: Record<string, { error?: string; detail?: string }>;
  detail?: string;
  error?: string;
  warnings?: string[];
  post_apply_warnings?: string[];
  non_fatal_warnings?: string[];
  singbox?: SingboxApplySummary;
  xray?: XrayApplySummary;
};

export function coreApplyWarning(response: CreateInboundResponse | CreateClientResponse | CoreWriteResponse | unknown, prefix: string): string {
  if (!response || typeof response !== 'object') return '';
  const data = response as CoreWriteResponse;
  const autoApplyFailure = autoApplyFailureDetail(data);
  if (autoApplyFailure) {
    return `${savedPrefix(prefix)}，但核心配置自动同步失败：${autoApplyFailure}`;
  }
  if (data.config_changed === true && autoApplyQueuedOrRunning(data)) {
    return `${savedPrefix(prefix)}，正在同步核心配置`;
  }
  if (data.config_changed === false) {
    return '';
  }
  if (data.pending_apply) {
    return pendingApplyMessage(data, prefix);
  }
  const failedCore = failedCoreResult(data);
  if (failedCore) {
    const detail = failedCore.detail || failedCore.error || xrayFailureOutput(failedCore) || data.detail || data.error || '未知错误';
    return `${prefix}：${detail}`;
  }
  const postApplyWarnings = firstNonEmptyWarnings(data.post_apply_warnings, data.xray?.post_apply_warnings, data.singbox?.post_apply_warnings);
  const listenerWarning = firstListenerWarning(postApplyWarnings) || firstListenerWarning(firstNonEmptyWarnings(data.warnings, data.xray?.warnings, data.singbox?.warnings));
  if (listenerWarning) return listenerWarning;
  const semanticWarning = firstXraySemanticWarning(firstNonEmptyWarnings(data.xray?.warnings, data.warnings));
  if (semanticWarning) return xraySemanticWarningLabel(semanticWarning);
  return '';
}

export function coreApplyWarningTone(response: CreateInboundResponse | CreateClientResponse | CoreWriteResponse | unknown): 'error' | 'info' {
  if (!response || typeof response !== 'object') return 'error';
  const data = response as CoreWriteResponse;
  if (autoApplyFailureDetail(data)) return 'error';
  if (data.config_changed === true && autoApplyQueuedOrRunning(data)) return 'info';
  if (data.config_changed === false) return 'info';
  if (data.pending_apply) return 'info';
  return failedCoreResult(data) ? 'error' : 'info';
}

function savedPrefix(prefix: string): string {
  const marker = '，但';
  const index = prefix.indexOf(marker);
  if (index > 0) return prefix.slice(0, index);
  return prefix || '已保存';
}

function autoApplyQueuedOrRunning(data: CoreWriteResponse): boolean {
  return Object.values(data.auto_apply || {}).some((job) => ['queued', 'running'].includes(String(job?.status || '').toLowerCase()));
}

function autoApplyFailureDetail(data: CoreWriteResponse): string {
  const explicit = Object.values(data.auto_apply_error || {}).find(Boolean);
  if (explicit) return explicit.detail || explicit.error || '未知错误';
  const failed = Object.values(data.auto_apply || {}).find((job) => String(job?.status || '').toLowerCase() === 'failed' || job?.error);
  return failed?.detail || failed?.error || '';
}

function pendingApplyMessage(data: CoreWriteResponse, prefix: string): string {
  const cores = Array.isArray(data.pending_cores) ? data.pending_cores.map(coreLabel).filter(Boolean) : [];
  const suffix = cores.length ? `：${cores.join('、')} 有更改，需点击核心页“应用配置”后生效` : '：有更改，需点击核心页“应用配置”后生效';
  return `${prefix}${suffix}`;
}

function coreLabel(core: string): string {
  const normalized = String(core || '').trim().toLowerCase();
  if (normalized === 'xray') return 'Xray';
  if (normalized === 'sing-box' || normalized === 'singbox') return 'sing-box';
  return core;
}

function failedCoreResult(data: CoreWriteResponse): (CoreWriteResponse & { error_output?: string; status?: string }) | XrayApplySummary | SingboxApplySummary | null {
  if (data.xray?.applied === false) return data.xray;
  if (data.singbox?.applied === false && data.singbox.reason !== 'not_needed') return data.singbox;
  if (data.applied === false) return data;
  return null;
}

function firstNonEmptyWarnings(...groups: Array<string[] | undefined>): string[] {
  return groups.find((warnings) => Array.isArray(warnings) && warnings.length > 0) || [];
}

function firstListenerWarning(warnings: string[]): string {
  return warnings.find((warning) => warning.startsWith('配置已应用，但端口未监听')) || '';
}

function firstXraySemanticWarning(warnings: string[]): string {
  return warnings.find((warning) => isXraySemanticWarning(warning)) || '';
}

function isXraySemanticWarning(warning: string): boolean {
  return [
    'xray_ws_path_invalid',
    'xray_grpc_service_name_invalid',
    'xray_xhttp_path_invalid',
    'xray_reality_settings_incomplete',
    'xray_tls_certificate_missing',
    'xray_shadowsocks_credentials_missing',
  ].includes(warning);
}

function xraySemanticWarningLabel(warning: string): string {
  const labels: Record<string, string> = {
    xray_ws_path_invalid: '节点已保存，Xray WS/H2 path 配置需要检查',
    xray_grpc_service_name_invalid: '节点已保存，Xray gRPC serviceName 配置需要检查',
    xray_xhttp_path_invalid: '节点已保存，Xray XHTTP path 配置需要检查',
    xray_reality_settings_incomplete: '节点已保存，Xray REALITY 配置不完整',
    xray_tls_certificate_missing: '节点已保存，Xray TLS 证书配置缺失',
    xray_shadowsocks_credentials_missing: '节点已保存，Xray Shadowsocks 2022 缺少可用凭据',
  };
  return labels[warning] || warning;
}

export function coreApplyFailureWarning(response: CreateInboundResponse | CreateClientResponse | CoreWriteResponse | unknown, prefix: string): string {
  if (!response || typeof response !== 'object') return '';
  const data = response as CoreWriteResponse;
  const failedCore = failedCoreResult(data);
  if (!failedCore) return '';
  const detail = failedCore.detail || failedCore.error || xrayFailureOutput(failedCore) || '未知错误';
  return `${prefix}：${detail}`;
}

function xrayFailureOutput(result: XrayApplySummary | SingboxApplySummary | (CoreWriteResponse & { error_output?: string; status?: string })): string {
  const xray = result as XrayApplySummary & { error_output?: string; status?: string };
  return xray.error_output || xray.status || '';
}

export function showCoreApplyWarning(
  response: unknown,
  prefix: string,
  showToast: (title: string, tone?: 'success' | 'error' | 'info') => void,
  text: (value: string) => string = (value) => value,
): boolean {
  const warning = coreApplyWarning(response, prefix);
  if (!warning) return false;
  showToast(text(warning), coreApplyWarningTone(response));
  return true;
}
