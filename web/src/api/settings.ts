import { get, post, put } from './client';
import type { CertStatus, Settings, UpdateCheck, UpdateStatus } from './types';

const confirmedSystemAction = { confirm: true, allow_system_changes: true };

export const settingsAPI = {
  settings: () => get<Settings>('/api/settings'),
  saveSettings: (body: Settings) => put<{ status: string }>('/api/settings', body),
  certStatus: () => get<CertStatus>('/api/cert/status'),
  issueCert: (domain: string, email: string) => post<{ status: string; domain: string; cert_path: string; key_path: string }>('/api/cert/issue', { domain, email, ...confirmedSystemAction }),
  restart: () => post<{ status: string }>('/api/restart', confirmedSystemAction),
  serviceStatus: () => get<{ service: string; status: string; detail?: string }>('/api/service/status'),
  updateCheck: () => get<UpdateCheck>('/api/update/check'),
  updateStatus: () => get<UpdateStatus>('/api/update/status'),
  updateLogs: () => get<{ logs?: string; lines?: string[]; path?: string }>('/api/update/logs?lines=160'),
  update: () => post<{ status: string; command: string; message?: string }>('/api/update', confirmedSystemAction),
};
