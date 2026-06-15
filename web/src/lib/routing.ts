import type { Inbound } from '../api/types';

export function generatedInboundTag(inbound: Pick<Inbound, 'id' | 'protocol'>): string {
  return `inbound-${inbound.id}-${String(inbound.protocol || '').trim().toLowerCase()}`;
}
