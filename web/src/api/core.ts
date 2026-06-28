import { get, post } from './client';
import type { ConfigValidation, CoreActionResponse, CoreApplyJobStatus, CoreStatus, SingboxConfigPreview, SingboxDiagnostics, VersionResponse, XrayConfigPreview, XrayDiagnostics } from './types';

const confirmedSystemAction = { confirm: true, allow_system_changes: true };

export const coreAPI = {
  xrayStatus: () => get<CoreStatus>('/api/xray/status'),
  coreApplyJob: (id: string) => get<CoreApplyJobStatus>(`/api/core/apply-jobs/${encodeURIComponent(id)}`),
  xrayDiagnostics: () => get<XrayDiagnostics>('/api/xray/diagnostics'),
  xrayVersion: () => get<VersionResponse>('/api/xray/version'),
  xrayConfig: () => get<unknown>('/api/xray/config'),
  xrayConfigPreview: () => get<XrayConfigPreview>('/api/xray/config/preview'),
  xrayValidate: () => get<ConfigValidation>('/api/xray/validate'),
  xrayApply: () => post<CoreActionResponse>('/api/xray/apply', confirmedSystemAction),
  xrayInstall: () => post<CoreActionResponse>('/api/xray/install', confirmedSystemAction),
  xrayUninstall: () => post<CoreActionResponse>('/api/xray/uninstall', confirmedSystemAction),
  xrayDelete: () => post<CoreActionResponse>('/api/xray/delete', confirmedSystemAction),
  xrayRestart: () => post<CoreActionResponse>('/api/xray/restart', confirmedSystemAction),
  xrayStop: () => post<CoreActionResponse>('/api/xray/stop', confirmedSystemAction),
  xrayLogs: () => get<{ logs?: string; lines?: string[] }>('/api/xray/logs'),
  singboxStatus: () => get<CoreStatus>('/api/singbox/status'),
  singboxDiagnostics: () => get<SingboxDiagnostics>('/api/singbox/diagnostics'),
  singboxVersion: () => get<VersionResponse>('/api/singbox/version'),
  singboxConfig: () => get<unknown>('/api/singbox/config'),
  singboxConfigPreview: () => get<SingboxConfigPreview>('/api/singbox/config/preview'),
  singboxValidate: () => get<ConfigValidation>('/api/singbox/validate'),
  singboxApply: () => post<CoreActionResponse>('/api/singbox/apply', confirmedSystemAction),
  singboxInstall: () => post<CoreActionResponse>('/api/singbox/install', confirmedSystemAction),
  singboxUninstall: () => post<CoreActionResponse>('/api/singbox/uninstall', confirmedSystemAction),
  singboxDelete: () => post<CoreActionResponse>('/api/singbox/delete', confirmedSystemAction),
  singboxRestart: () => post<CoreActionResponse>('/api/singbox/restart', confirmedSystemAction),
  singboxStop: () => post<CoreActionResponse>('/api/singbox/stop', confirmedSystemAction),
  singboxLogs: () => get<{ logs?: string; lines?: string[] }>('/api/singbox/logs'),
};
