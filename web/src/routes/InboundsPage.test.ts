import { describe, expect, it } from 'vitest';
import type { Inbound } from '../api/types';
import {
  applyInboundTemplate,
  buildClientPayload,
  buildFullInboundPayload,
  bytesToGB,
  clientFormValues,
  createDefaultInbound,
  gbToBytes,
  hasAttachableSettingCert,
  inboundFormValues,
  mergeInboundTraffic,
  sanitizeInboundFormValues,
} from './InboundsPage';

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

  it('preserves valid advanced fields and clears fields hidden for the current combination', () => {
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
      hy2_obfs_password: '',
      hy2_mport: '',
      tuic_zero_rtt: false,
      shadowtls_password: '',
      tls_alpn: '',
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
      tls_fingerprint: 'chrome',
      tls_alpn: '',
      ss_method: '',
      xhttp_mode: '',
      hy2_up_mbps: 0,
      hy2_down_mbps: 0,
      tuic_congestion_control: '',
      shadowtls_version: 0,
    });
  });

  it('applies recommended and compatibility templates without leaking unrelated advanced fields', () => {
    const base = inboundFormValues(createDefaultInbound());

    const recommended = applyInboundTemplate(base, 'recommended');
    const recommendedAgain = applyInboundTemplate(base, 'recommended');
    expect(recommended).toMatchObject({
      protocol: 'vless',
      network: 'tcp',
      security: 'reality',
      port: 0,
      reality_dest: 'www.cloudflare.com:443',
      reality_server_names: 'www.cloudflare.com',
      tls_fingerprint: 'chrome',
      tls_alpn: '',
    });
    expect(recommended.reality_short_id).toHaveLength(8);
    expect(recommendedAgain.reality_short_id).toHaveLength(8);
    expect(recommendedAgain.reality_short_id).not.toBe(recommended.reality_short_id);

    const compatible = applyInboundTemplate(recommended, 'compatible');
    expect(compatible).toMatchObject({
      protocol: 'vmess',
      network: 'ws',
      security: 'tls',
      ws_path: '/migate',
      ws_host: 'example.com',
      tls_sni: 'example.com',
    });
    expect(compatible.uuid).toBe(recommended.uuid);
    expect(compatible.reality_dest).toBe('');
    expect(compatible.reality_server_names).toBe('');
    expect(compatible.reality_short_id).toBe('');
  });

  it('applies performance and simple templates with generated secrets', () => {
    const base = inboundFormValues(createDefaultInbound());

    const performance = applyInboundTemplate(base, 'performance');
    const performanceAgain = applyInboundTemplate(base, 'performance');
    expect(performance).toMatchObject({
      protocol: 'hysteria2',
      network: 'udp',
      security: 'tls',
      port: 0,
      hy2_up_mbps: 100,
      hy2_down_mbps: 100,
      hy2_obfs: 'salamander',
      tls_sni: 'example.com',
      hy2_mport: '',
    });
    expect(performance.uuid).toHaveLength(24);
    expect(performance.hy2_obfs_password).toHaveLength(18);
    expect(performanceAgain.hy2_obfs_password).toHaveLength(18);
    expect(performanceAgain.hy2_obfs_password).not.toBe(performance.hy2_obfs_password);

    const simple = applyInboundTemplate(base, 'simple');
    expect(simple).toMatchObject({
      protocol: 'shadowsocks',
      network: 'tcp',
      security: 'none',
      port: 0,
      ss_method: '2022-blake3-aes-128-gcm',
    });
    expect(simple.uuid).toHaveLength(24);
    expect(simple.hy2_obfs).toBe('');
    expect(simple.hy2_obfs_password).toBe('');
    expect(simple.hy2_mport).toBe('');
    expect(simple.tls_sni).toBe('');
  });

  it('sanitizes protocol, transport, and security changes to supported combinations', () => {
    const hy2 = applyInboundTemplate(inboundFormValues(createDefaultInbound()), 'performance');
    const vless = sanitizeInboundFormValues(hy2, { protocol: 'vless' });
    expect(vless).toMatchObject({
      protocol: 'vless',
      network: 'tcp',
      security: 'reality',
      reality_dest: 'www.cloudflare.com:443',
      reality_server_names: 'www.cloudflare.com',
      hy2_obfs: '',
      hy2_obfs_password: '',
    });

    const reality = applyInboundTemplate(inboundFormValues(createDefaultInbound()), 'recommended');
    const vmess = sanitizeInboundFormValues(reality, { protocol: 'vmess' });
    expect(vmess).toMatchObject({
      protocol: 'vmess',
      network: 'tcp',
      security: 'tls',
      reality_dest: '',
      reality_server_names: '',
      reality_short_id: '',
      tls_fingerprint: 'chrome',
    });

    const ws = sanitizeInboundFormValues(reality, { network: 'ws' });
    expect(ws).toMatchObject({
      protocol: 'vless',
      network: 'ws',
      security: 'tls',
      reality_dest: '',
      reality_server_names: '',
      reality_short_id: '',
      ws_path: '',
      ws_host: '',
      tls_fingerprint: 'chrome',
    });
  });

  it('removes invalid advanced fields from the submitted payload after manual switches', () => {
    const values = inboundFormValues(existing);
    values.protocol = 'vmess';
    values.network = 'ws';
    values.security = 'reality';

    const payload = buildFullInboundPayload(existing, values);

    expect(payload).toMatchObject({
      protocol: 'vmess',
      network: 'ws',
      security: 'tls',
      ws_path: '/ws',
      ws_host: 'cdn.example.com',
      reality_dest: '',
      reality_server_names: '',
      reality_short_id: '',
      reality_private_key: '',
      reality_public_key: '',
      hy2_mport: '',
      shadowtls_password: '',
      tls_alpn: 'h2,http/1.1',
    });
  });

  it('keeps ShadowTLS handshake SNI but clears its unsupported inbound password', () => {
    const values = inboundFormValues({
      ...existing,
      protocol: 'shadowtls',
      network: 'tcp',
      security: 'none',
      tls_sni: 'handshake.example.com',
      shadowtls_version: 3,
      shadowtls_password: 'legacy-shadow-password',
    });

    const payload = buildFullInboundPayload(existing, values);

    expect(payload).toMatchObject({
      protocol: 'shadowtls',
      network: 'tcp',
      security: 'none',
      tls_sni: 'handshake.example.com',
      shadowtls_version: 3,
      shadowtls_password: '',
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

  it('keeps an empty inbound port as zero so the backend can auto-assign it', () => {
    const values = inboundFormValues(createDefaultInbound());
    values.port = 0;

    const payload = buildFullInboundPayload(null, values);

    expect(payload.port).toBe(0);
  });

  it('normalizes blank inbound ports to zero for backend auto-assignment', () => {
    const values = inboundFormValues(createDefaultInbound());
    values.port = '' as unknown as typeof values.port;

    const payload = buildFullInboundPayload(null, values);

    expect(payload.port).toBe(0);
  });

  it('allows attaching a settings certificate only after it is issued with both files', () => {
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: false, cert_path: '/etc/xray/certs/example.com.pem', key_path: '/etc/xray/certs/example.com.key' })).toBe(false);
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: true, cert_path: '/etc/xray/certs/example.com.pem', key_path: '' })).toBe(false);
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: true, cert_path: '   ', key_path: '/etc/xray/certs/example.com.key' })).toBe(false);
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: true, cert_path: '/etc/xray/certs/example.com.pem', key_path: '/etc/xray/certs/example.com.key' })).toBe(true);
  });

  it('merges lightweight traffic refresh without replacing full config fields', () => {
    const current: Inbound[] = [{
      id: 1,
      remark: 'edge',
      protocol: 'vless',
      port: 443,
      network: 'tcp',
      security: 'reality',
      enabled: true,
      uuid: '11111111-1111-4111-8111-111111111111',
      reality_private_key: 'private-key',
      clients: [{ id: 10, inbound_id: 1, email: 'sam@example.com', uuid: 'client-uuid', enabled: true, traffic_limit: 1000, expiry_at: 0 }],
    }];
    const traffic: Inbound[] = [{
      id: 1,
      remark: 'edge',
      protocol: 'vless',
      port: 443,
      network: 'tcp',
      security: 'reality',
      enabled: false,
      clients: [{ id: 10, inbound_id: 1, email: 'sam@example.com', uuid: 'client-uuid', enabled: false, up: 12, down: 34, traffic_limit: 2000, expiry_at: 99 }],
      traffic_up: 12,
      traffic_down: 34,
      traffic_total: 46,
      traffic_stats_source: 'xray',
      realtime_stats_source: 'xray',
      client_traffic: { '10': { up: 12, down: 34, source: 'xray', realtime_source: 'xray' } },
    }];

    const [merged] = mergeInboundTraffic(current, traffic);

    expect(merged.enabled).toBe(false);
    expect(merged.reality_private_key).toBe('private-key');
    expect(merged.traffic_total).toBe(46);
    expect(merged.clients?.[0]).toMatchObject({ email: 'sam@example.com', enabled: false, up: 12, down: 34, traffic_limit: 2000, expiry_at: 99 });
  });
});

describe('client form helpers', () => {
  it('converts traffic limits between GB and bytes', () => {
    expect(gbToBytes(1)).toBe(1073741824);
    expect(gbToBytes(1.5)).toBe(1610612736);
    expect(gbToBytes(0)).toBe(0);
    expect(bytesToGB(1073741824)).toBe(1);
    expect(bytesToGB(1610612736)).toBe(1.5);
  });

  it('reflects existing client traffic and custom expiry values', () => {
    const inbound = createDefaultInbound();
    const expiryAt = Math.floor(new Date('2030-01-01T23:59:59').getTime() / 1000);
    const values = clientFormValues(inbound, {
      id: 1,
      inbound_id: inbound.id,
      email: 'sam',
      uuid: '11111111-1111-4111-8111-111111111111',
      enabled: true,
      traffic_limit: 5 * 1024 ** 3,
      expiry_at: expiryAt,
    });

    expect(values.traffic_limit_gb).toBe(5);
    expect(values.expiry_mode).toBe('custom');
    expect(values.expiry_date).toBe('2030-01-01');
  });

  it('builds the API payload with bytes and the current expiry contract', () => {
    const payload = buildClientPayload({
      email: 'sam',
      uuid: 'secret',
      enabled: true,
      traffic_limit_gb: 2,
      expiry_mode: 'custom',
      expiry_date: '2030-01-01',
    });

    const expectedExpiry = Math.floor(new Date('2030-01-01T23:59:59').getTime() / 1000);

    expect(payload).toMatchObject({
      email: 'sam',
      uuid: 'secret',
      enabled: true,
      traffic_limit: 2 * 1024 ** 3,
      expiry_at: expectedExpiry,
    });
  });
});
