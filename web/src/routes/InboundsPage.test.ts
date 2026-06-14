import { describe, expect, it } from 'vitest';
import type { Inbound } from '../api/types';
import { buildFullInboundPayload, createDefaultInbound, inboundFormValues } from './InboundsPage';

describe('inbound payload helpers', () => {
  const existing: Inbound = {
    id: 12,
    remark: 'edge',
    protocol: 'vless',
    port: 443,
    network: 'xhttp',
    security: 'reality',
    enabled: true,
    uuid: '11111111-1111-4111-8111-111111111111',
    clients: [],
    traffic_up: 100,
    traffic_down: 200,
    traffic_total: 300,
    traffic_stats_source: 'db',
    realtime_stats_source: 'xray',
    client_traffic: {},
    ws_path: '/ws',
    ws_host: 'cdn.example.com',
    grpc_service_name: 'grpc-edge',
    reality_dest: 'www.cloudflare.com:443',
    reality_server_names: 'www.cloudflare.com',
    reality_short_id: 'abcd',
    reality_private_key: 'private-key',
    reality_public_key: 'public-key',
    ss_method: '2022-blake3-aes-128-gcm',
    tls_cert_file: '/etc/xray/certs/example.pem',
    tls_key_file: '/etc/xray/certs/example.key',
    tls_sni: 'example.com',
    tls_fingerprint: 'chrome',
    tls_alpn: 'h2,http/1.1',
    xhttp_path: '/xhttp',
    xhttp_mode: 'stream-one',
    hy2_up_mbps: 100,
    hy2_down_mbps: 200,
    hy2_obfs: 'salamander',
    hy2_obfs_password: 'obfs-secret',
    hy2_mport: '40000-50000',
    tuic_congestion_control: 'bbr',
    tuic_zero_rtt: true,
    shadowtls_version: 3,
    shadowtls_password: 'shadow-secret',
  };

  it('preserves advanced fields when editing visible basic fields', () => {
    const values = inboundFormValues(existing);
    values.remark = 'edge-updated';
    values.port = 8443;

    const payload = buildFullInboundPayload(existing, values);

    expect(payload).toMatchObject({
      remark: 'edge-updated',
      port: 8443,
      reality_private_key: 'private-key',
      reality_public_key: 'public-key',
      xhttp_path: '/xhttp',
      xhttp_mode: 'stream-one',
      hy2_obfs_password: 'obfs-secret',
      hy2_mport: '40000-50000',
      tuic_zero_rtt: true,
      shadowtls_password: 'shadow-secret',
    });
    expect(payload).not.toHaveProperty('id');
    expect(payload).not.toHaveProperty('clients');
    expect(payload).not.toHaveProperty('traffic_total');
    expect(payload).not.toHaveProperty('client_traffic');
  });

  it('provides defaults for advanced fields on new inbound', () => {
    const inbound = createDefaultInbound();
    const payload = buildFullInboundPayload(null, inboundFormValues(inbound));

    expect(payload).toMatchObject({
      protocol: 'vless',
      security: 'reality',
      reality_dest: 'www.cloudflare.com:443',
      reality_server_names: 'www.cloudflare.com',
      ss_method: '2022-blake3-aes-128-gcm',
      xhttp_mode: 'stream-one',
      hy2_up_mbps: 100,
      hy2_down_mbps: 100,
      tuic_congestion_control: 'bbr',
      shadowtls_version: 3,
    });
  });

  it('normalizes missing numeric advanced fields when editing a basic inbound', () => {
    const basic: Inbound = {
      id: 13,
      remark: 'plain',
      protocol: 'vless',
      port: 8443,
      network: 'tcp',
      security: 'none',
      enabled: true,
      uuid: '22222222-2222-4222-8222-222222222222',
      clients: [],
    };

    const payload = buildFullInboundPayload(basic, inboundFormValues(basic));

    expect(payload.hy2_up_mbps).toBe(0);
    expect(payload.hy2_down_mbps).toBe(0);
    expect(payload.shadowtls_version).toBe(0);
    expect(typeof payload.hy2_up_mbps).toBe('number');
    expect(typeof payload.hy2_down_mbps).toBe('number');
    expect(typeof payload.shadowtls_version).toBe('number');
  });
});
