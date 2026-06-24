import { apiText, del, get, patch, post, put } from './client';
import type { Client, CreateClientResponse, CreateInboundResponse, Inbound, InboundCapability, SingboxWriteResponse } from './types';

export const inboundsAPI = {
  inbounds: async () => {
    const response = await get<Inbound[] | { inbounds?: Inbound[] }>('/api/inbounds');
    return Array.isArray(response) ? response : response.inbounds || [];
  },
  inboundCapabilities: async () => {
    const response = await get<{ capabilities?: InboundCapability[] }>('/api/inbound-capabilities');
    return response.capabilities || [];
  },
  generateRealityKeypair: () => post<{ private_key: string; public_key: string }>('/api/reality/keypair', {}),
  createInbound: (body: Record<string, unknown>) => post<CreateInboundResponse>('/api/inbounds', body),
  updateInbound: (id: number, body: Record<string, unknown>) => put<(Inbound | { inbound: Inbound }) & SingboxWriteResponse>(`/api/inbounds/${id}`, body),
  deleteInbound: (id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/inbounds/${id}`),
  toggleInbound: (id: number, enabled: boolean) => patch<(Inbound | { inbound: Inbound }) & SingboxWriteResponse>(`/api/inbounds/${id}/enabled`, { enabled }),
  createClient: (inboundId: number, body: Record<string, unknown>) => post<CreateClientResponse>(`/api/inbounds/${inboundId}/clients`, body),
  updateClient: (inboundId: number, id: number, body: Record<string, unknown>) => put<({ client?: Client } | Client) & SingboxWriteResponse>(`/api/inbounds/${inboundId}/clients/${id}`, body),
  deleteClient: (inboundId: number, id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/inbounds/${inboundId}/clients/${id}`),
  toggleClient: (inboundId: number, id: number, enabled: boolean) => patch<({ client?: Client } | Client) & SingboxWriteResponse>(`/api/inbounds/${inboundId}/clients/${id}/enabled`, { enabled }),
  resetClientTraffic: (inboundId: number, id: number) => post(`/api/inbounds/${inboundId}/clients/${id}/reset-traffic`),
  subscriptionLink: (token: string) => apiText(`/sub/${encodeURIComponent(token)}`, {}, 'share_link_unavailable').then((body) => body.trim()),
};
