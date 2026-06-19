import { get, post, put } from './client';
import type { CertStatus, CertificateApplyResponse, CertificateOperation, CertificatePreflight, Inbound, ManagedCertificate, Settings, UpdateCheck, UpdateStatus, VersionInfo } from './types';

const confirmedSystemAction = { confirm: true, allow_system_changes: true };

export const settingsAPI = {
  settings: () => get<Settings>('/api/settings'),
  saveSettings: (body: Settings) => put<{ status: string }>('/api/settings', body),
  certStatus: () => get<CertStatus>('/api/cert/status'),
  issueCert: (domain: string, email: string) => post<{ status: string; domain: string; cert_path: string; key_path: string; preflight?: CertificatePreflight }>('/api/cert/issue', { domain, email, ...confirmedSystemAction }),
  certificates: () => get<{ certificates: ManagedCertificate[] }>('/api/certificates'),
  certificatePreflight: (domains: string[], email: string) => post<{ preflight: CertificatePreflight }>('/api/certificates/preflight', { domains, email }),
  createCertificate: (domains: string[], email: string) => post<{ certificate: ManagedCertificate; preflight: CertificatePreflight }>('/api/certificates', { domains, email, ...confirmedSystemAction }),
  importCertificate: (body: { name?: string; fullchain: string; private_key: string }) => post<{ certificate: ManagedCertificate }>('/api/certificates/import', { ...body, ...confirmedSystemAction }),
  renewDueCertificates: (days = 30) => post<{ status: string; renewal: { checked: ManagedCertificate[]; renewed: ManagedCertificate[]; failed: ManagedCertificate[] } }>('/api/certificates/renew-due', { days, ...confirmedSystemAction }),
  deleteCertificate: (id: number) => post<{ status: string }>(`/api/certificates/${id}/delete`, confirmedSystemAction),
  certificateOperations: (id: number) => get<{ operations: CertificateOperation[] }>(`/api/certificates/${id}/operations?limit=20`),
  certificateInboundTargets: () => get<{ inbounds: Inbound[] }>('/api/certificates/inbounds'),
  applyCertificate: (id: number, inboundIds: number[]) => post<CertificateApplyResponse>(`/api/certificates/${id}/apply`, { inbound_ids: inboundIds, ...confirmedSystemAction }),
  restart: () => post<{ status: string }>('/api/restart', confirmedSystemAction),
  serviceStatus: () => get<{ service: string; status: string; detail?: string }>('/api/service/status'),
  version: () => get<VersionInfo>('/api/version'),
  updateCheck: () => get<UpdateCheck>('/api/update/check'),
  updateStatus: () => get<UpdateStatus>('/api/update/status'),
  updateLogs: () => get<{ logs?: string; lines?: string[]; path?: string }>('/api/update/logs?lines=160'),
  update: () => post<{ status: string; command: string; message?: string }>('/api/update', confirmedSystemAction),
};
