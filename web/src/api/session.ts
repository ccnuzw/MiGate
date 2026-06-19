import { del, get, post } from './client';
import type { Session, SessionInfo, VersionResponse } from './types';

export const sessionAPI = {
  login: (username: string, password: string) => post<{ status: string }>('/api/login', { username, password }),
  logout: () => post<{ status: string }>('/api/logout'),
  session: () => get<Session>('/api/session'),
  sessions: () => get<SessionInfo[]>('/api/sessions'),
  revokeSession: (id: number) => del<{ status: string }>(`/api/sessions/${id}`),
  revokeOtherSessions: () => del<{ status: string; revoked: number }>('/api/sessions/others'),
  health: () => get<{ status: string; mode: string }>('/api/health'),
  version: () => get<VersionResponse>('/api/version'),
};
