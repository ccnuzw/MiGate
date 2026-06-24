import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { createRoot } from 'react-dom/client';
import { act } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { TrafficV2Patch, TrafficV2Snapshot } from '../api/types';
import { engineStatusSummary, mergeTrafficV2Snapshot, realtimeRateLabel, realtimeTotalLabel, trafficHint, trafficRateSummary, trafficStatusLabel, useTrafficStream, validationStatusLabel, validationSummary } from './OverviewPage';

const text = (value: string) => value;

let root: ReturnType<typeof createRoot> | undefined;
let container: HTMLDivElement | undefined;

afterEach(() => {
  if (root) {
    act(() => root?.unmount());
  }
  root = undefined;
  container?.remove();
  container = undefined;
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('OverviewPage traffic status labels', () => {
  it('shows sing-box unsupported as an informational realtime stats limitation', () => {
    expect(trafficStatusLabel('unsupported', text)).toBe('当前 sing-box 二进制不支持实时统计');
    expect(engineStatusSummary({ xray: 'ok', singbox: 'unsupported' }, text)).toBe('xray: 统计正常 · singbox: 当前 sing-box 二进制不支持实时统计');
  });

  it('shows not_configured without treating it as a core failure label', () => {
    expect(trafficStatusLabel('not_configured', text)).toBe('未配置对应核心入站');
    expect(engineStatusSummary({ singbox: 'not_configured' }, text)).toBe('singbox: 未配置对应核心入站');
  });

  it('distinguishes waiting and unavailable labels', () => {
    expect(trafficStatusLabel('waiting', text)).toBe('等待采样');
    expect(trafficStatusLabel('unavailable', text)).toBe('统计接口不可用');
  });

  it('folds traffic status into the current rate summary', () => {
    expect(trafficRateSummary(1024, 2048, 'ok', { xray: 'ok' }, text)).toBe('1.0 KB/s ↑ / 2.0 KB/s ↓ · 统计正常');
    expect(trafficRateSummary(0, 0, 'unavailable', { xray: 'unavailable', singbox: 'not_configured' }, text)).toBe('统计接口不可用 · xray: 统计接口不可用 · singbox: 未配置对应核心入站');
  });

  it('formats realtime rate and sample hint explicitly', () => {
    expect(realtimeRateLabel(0, 0)).toBe('0 B/s ↑ / 0 B/s ↓');
    expect(realtimeTotalLabel(3072, 'ok', text)).toBe('3.0 KB/s');
    expect(realtimeTotalLabel(3072, 'stale', text)).toBe('统计状态过期');
    expect(trafficHint('2026-06-24T00:00:00Z', 5, 'inbound', 'ok', 'sampled', text)).toBe('采样时间: 2026-06-24T00:00:00Z · 采样窗口: 5.0s · 统计源: inbound · 状态: 统计正常 · 说明: sampled');
  });
});

describe('OverviewPage traffic stream', () => {
  it('merges patch payload by id without changing unchanged rows', () => {
    const base: TrafficV2Snapshot = {
      generated_at: '2026-06-24T00:00:00Z',
      observed_at: '2026-06-24T00:00:00Z',
      window_seconds: 5,
      total: {
        cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' },
        realtime: { delta_up: 1, delta_down: 2, delta_total: 3, rate_up: 1, rate_down: 2, rate_total: 3, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'inbound', message: '' },
      },
      inbounds: [
        { id: 1, remark: 'edge-a', protocol: 'vless', port: 443, enabled: true, cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' }, realtime: { delta_up: 1, delta_down: 2, delta_total: 3, rate_up: 1, rate_down: 2, rate_total: 3, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'inbound', message: '' } },
        { id: 2, remark: 'edge-b', protocol: 'trojan', port: 8443, enabled: true, cumulative: { up: 30, down: 40, total: 70, status: 'ok', source: 'inbound', message: '' }, realtime: { delta_up: 0, delta_down: 0, delta_total: 0, rate_up: 0, rate_down: 0, rate_total: 0, observed_at: '', window_seconds: 0, status: 'waiting', source: 'inbound', message: '' } },
      ],
      clients: [
        { id: 10, inbound_id: 1, email: 'a@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: { up: 1, down: 2, total: 3, status: 'ok', source: 'client', message: '' }, realtime: { delta_up: 1, delta_down: 1, delta_total: 2, rate_up: 0.1, rate_down: 0.1, rate_total: 0.2, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'client', message: '' } },
      ],
      coverage: { overall: 'ok', engines: { xray: 'ok' }, ok: 1, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 0 },
    };
    const patch: TrafficV2Patch = {
      generated_at: '2026-06-24T00:00:05Z',
      observed_at: '2026-06-24T00:00:05Z',
      window_seconds: 5,
      total: {
        cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' },
        realtime: { delta_up: 9, delta_down: 10, delta_total: 19, rate_up: 9, rate_down: 10, rate_total: 19, observed_at: '2026-06-24T00:00:05Z', window_seconds: 5, status: 'stale', source: 'inbound', message: 'traffic sample is stale' },
      },
      inbounds: [
        { id: 2, remark: 'edge-b', protocol: 'trojan', port: 8443, enabled: true, cumulative: { up: 30, down: 40, total: 70, status: 'ok', source: 'inbound', message: '' }, realtime: { delta_up: 0, delta_down: 0, delta_total: 0, rate_up: 0, rate_down: 0, rate_total: 0, observed_at: '2026-06-24T00:00:05Z', window_seconds: 0, status: 'unsupported', source: 'inbound', message: 'unsupported' } },
      ],
      removed_inbound_ids: [1],
      clients: [
        { id: 10, inbound_id: 1, email: 'a@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: { up: 1, down: 2, total: 3, status: 'ok', source: 'client', message: '' }, realtime: { delta_up: 3, delta_down: 4, delta_total: 7, rate_up: 0.3, rate_down: 0.4, rate_total: 0.7, observed_at: '2026-06-24T00:00:05Z', window_seconds: 5, status: 'unsupported', source: 'client', message: 'unsupported' } },
      ],
      removed_client_ids: [999],
      coverage: { overall: 'partial', engines: { xray: 'partial' }, ok: 0, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 1 },
    };

    const merged = mergeTrafficV2Snapshot(base, patch);

    expect(merged.total.realtime.status).toBe('stale');
    expect(merged.inbounds).toHaveLength(1);
    expect(merged.inbounds[0].id).toBe(2);
    expect(merged.inbounds[0].realtime.status).toBe('unsupported');
    expect(merged.clients[0].realtime.status).toBe('unsupported');
    expect(merged.coverage.overall).toBe('partial');
  });

  it('applies removal-only patch payloads without throwing away the whole merge', () => {
    const base: TrafficV2Snapshot = {
      generated_at: '2026-06-24T00:00:00Z',
      observed_at: '2026-06-24T00:00:00Z',
      window_seconds: 5,
      total: {
        cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' },
        realtime: { delta_up: 1, delta_down: 2, delta_total: 3, rate_up: 1, rate_down: 2, rate_total: 3, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'inbound', message: '' },
      },
      inbounds: [
        { id: 1, remark: 'edge-a', protocol: 'vless', port: 443, enabled: true, cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' }, realtime: { delta_up: 1, delta_down: 2, delta_total: 3, rate_up: 1, rate_down: 2, rate_total: 3, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'inbound', message: '' } },
      ],
      clients: [
        { id: 10, inbound_id: 1, email: 'a@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: { up: 1, down: 2, total: 3, status: 'ok', source: 'client', message: '' }, realtime: { delta_up: 1, delta_down: 1, delta_total: 2, rate_up: 0.1, rate_down: 0.1, rate_total: 0.2, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'client', message: '' } },
      ],
      coverage: { overall: 'ok', engines: { xray: 'ok' }, ok: 1, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 0 },
    };

    const merged = mergeTrafficV2Snapshot(base, {
      generated_at: '2026-06-24T00:00:05Z',
      observed_at: '2026-06-24T00:00:05Z',
      window_seconds: 5,
      removed_client_ids: [10],
    });

    expect(merged.clients).toHaveLength(0);
    expect(merged.inbounds).toHaveLength(1);
  });

  it('keeps EventSource open on transient errors so the browser can reconnect and throttles REST fallback invalidation', () => {
    const instances: FakeEventSource[] = [];
    class FakeEventSource {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSED = 2;
      close = vi.fn();
      onerror: ((event: Event) => void) | null = null;
      listeners = new Map<string, Array<(event: Event) => void>>();
      constructor(public url: string) {
        instances.push(this);
      }
      addEventListener = vi.fn((type: string, listener: (event: Event) => void) => {
        const current = this.listeners.get(type) || [];
        current.push(listener);
        this.listeners.set(type, current);
      });
      removeEventListener = vi.fn((type: string, listener: (event: Event) => void) => {
        const current = this.listeners.get(type) || [];
        this.listeners.set(type, current.filter((item) => item !== listener));
      });
      dispatchEvent = vi.fn();
    }
    vi.stubGlobal('EventSource', FakeEventSource);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const isFetchingSpy = vi.spyOn(queryClient, 'isFetching').mockReturnValue(0);
    const nowSpy = vi.spyOn(Date, 'now');
    nowSpy.mockReturnValueOnce(1000).mockReturnValueOnce(2000).mockReturnValueOnce(7000).mockReturnValueOnce(7000);

    function Probe() {
      useTrafficStream(true);
      return null;
    }

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => {
      root!.render(createElement(QueryClientProvider, { client: queryClient }, createElement(Probe)));
    });

    expect(instances).toHaveLength(1);
    expect(instances[0].onerror).toBeNull();
    expect(instances[0].close).not.toHaveBeenCalled();
    expect(instances[0].listeners.get('stream-error')).toHaveLength(1);
    expect(instances[0].listeners.get('error')).toHaveLength(1);

    act(() => {
      instances[0].listeners.get('stream-error')?.[0](new Event('stream-error'));
      instances[0].listeners.get('stream-error')?.[0](new Event('stream-error'));
    });
    expect(isFetchingSpy).toHaveBeenCalledWith({ queryKey: ['traffic-v2-snapshot'] });
    expect(invalidateSpy).toHaveBeenCalledTimes(1);
    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['traffic-v2-snapshot'] });

    act(() => {
      instances[0].listeners.get('stream-error')?.[0](new Event('stream-error'));
    });
    expect(invalidateSpy).toHaveBeenCalledTimes(2);

    act(() => root?.unmount());
    expect(instances[0].removeEventListener).toHaveBeenCalledWith('snapshot', expect.any(Function));
    expect(instances[0].removeEventListener).toHaveBeenCalledWith('error', expect.any(Function));
    expect(instances[0].removeEventListener).toHaveBeenCalledWith('stream-error', expect.any(Function));
    expect(instances[0].close).toHaveBeenCalledTimes(1);
    root = undefined;
  });

  it('skips fallback invalidation while traffic snapshot is already fetching', () => {
    const instances: FakeEventSource[] = [];
    class FakeEventSource {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSED = 2;
      close = vi.fn();
      listeners = new Map<string, Array<(event: Event) => void>>();
      constructor(public url: string) {
        instances.push(this);
      }
      addEventListener = vi.fn((type: string, listener: (event: Event) => void) => {
        const current = this.listeners.get(type) || [];
        current.push(listener);
        this.listeners.set(type, current);
      });
      removeEventListener = vi.fn();
      dispatchEvent = vi.fn();
    }
    vi.stubGlobal('EventSource', FakeEventSource);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    vi.spyOn(queryClient, 'isFetching').mockReturnValue(1);

    function Probe() {
      useTrafficStream(true);
      return null;
    }

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => {
      root!.render(createElement(QueryClientProvider, { client: queryClient }, createElement(Probe)));
    });

    act(() => {
      instances[0].listeners.get('stream-error')?.[0](new Event('stream-error'));
    });
    expect(invalidateSpy).not.toHaveBeenCalled();

    act(() => root?.unmount());
    root = undefined;
  });

  it('applies patch and delta events into the traffic snapshot cache', () => {
    const instances: FakeEventSource[] = [];
    class FakeEventSource {
      static CONNECTING = 0;
      static OPEN = 1;
      static CLOSED = 2;
      close = vi.fn();
      listeners = new Map<string, Array<(event: Event) => void>>();
      constructor(public url: string) {
        instances.push(this);
      }
      addEventListener = vi.fn((type: string, listener: (event: Event) => void) => {
        const current = this.listeners.get(type) || [];
        current.push(listener);
        this.listeners.set(type, current);
      });
      removeEventListener = vi.fn();
      dispatchEvent = vi.fn();
    }
    vi.stubGlobal('EventSource', FakeEventSource);
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    queryClient.setQueryData(['traffic-v2-snapshot'], {
      generated_at: '2026-06-24T00:00:00Z',
      observed_at: '2026-06-24T00:00:00Z',
      window_seconds: 5,
      total: {
        cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' },
        realtime: { delta_up: 1, delta_down: 2, delta_total: 3, rate_up: 1, rate_down: 2, rate_total: 3, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'inbound', message: '' },
      },
      inbounds: [
        { id: 1, remark: 'edge', protocol: 'vless', port: 443, enabled: true, cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' }, realtime: { delta_up: 1, delta_down: 2, delta_total: 3, rate_up: 1, rate_down: 2, rate_total: 3, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'inbound', message: '' } },
      ],
      clients: [
        { id: 10, inbound_id: 1, email: 'user@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: { up: 1, down: 2, total: 3, status: 'ok', source: 'client', message: '' }, realtime: { delta_up: 1, delta_down: 1, delta_total: 2, rate_up: 0.1, rate_down: 0.1, rate_total: 0.2, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status: 'ok', source: 'client', message: '' } },
      ],
      coverage: { overall: 'ok', engines: { xray: 'ok' }, ok: 1, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 0 },
    } satisfies TrafficV2Snapshot);

    function Probe() {
      useTrafficStream(true);
      return null;
    }

    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => {
      root!.render(createElement(QueryClientProvider, { client: queryClient }, createElement(Probe)));
    });

    act(() => {
      instances[0].listeners.get('patch')?.[0]({
        data: JSON.stringify({
          generated_at: '2026-06-24T00:00:05Z',
          observed_at: '2026-06-24T00:00:05Z',
          window_seconds: 5,
          inbounds: [{ id: 1, remark: 'edge', protocol: 'vless', port: 443, enabled: true, cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' }, realtime: { delta_up: 0, delta_down: 0, delta_total: 0, rate_up: 0, rate_down: 0, rate_total: 0, observed_at: '2026-06-24T00:00:05Z', window_seconds: 0, status: 'waiting', source: 'inbound', message: 'waiting' } }],
          clients: [{ id: 10, inbound_id: 1, email: 'user@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: { up: 1, down: 2, total: 3, status: 'ok', source: 'client', message: '' }, realtime: { delta_up: 7, delta_down: 8, delta_total: 15, rate_up: 0.7, rate_down: 0.8, rate_total: 1.5, observed_at: '2026-06-24T00:00:05Z', window_seconds: 5, status: 'unsupported', source: 'client', message: 'unsupported' } }],
          removed_client_ids: [],
          coverage: { overall: 'partial', engines: { xray: 'partial' }, ok: 0, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 1 },
        }),
      } as MessageEvent);
      instances[0].listeners.get('delta')?.[0]({
        data: JSON.stringify({
          generated_at: '2026-06-24T00:00:10Z',
          observed_at: '2026-06-24T00:00:10Z',
          window_seconds: 5,
          total: {
            cumulative: { up: 10, down: 20, total: 30, status: 'ok', source: 'inbound', message: '' },
            realtime: { delta_up: 11, delta_down: 12, delta_total: 23, rate_up: 1.1, rate_down: 1.2, rate_total: 2.3, observed_at: '2026-06-24T00:00:10Z', window_seconds: 5, status: 'unsupported', source: 'inbound', message: 'unsupported' },
          },
        }),
      } as MessageEvent);
    });

    const snapshot = queryClient.getQueryData<TrafficV2Snapshot>(['traffic-v2-snapshot']);
    expect(snapshot?.inbounds[0].realtime.status).toBe('waiting');
    expect(snapshot?.clients[0].realtime.status).toBe('unsupported');
    expect(snapshot?.total.realtime.status).toBe('unsupported');
    expect(snapshot?.coverage.overall).toBe('partial');

    act(() => root?.unmount());
    root = undefined;
  });
});

describe('OverviewPage validation labels', () => {
  const enText = (value: string) => ({
    生成中: 'Generating',
    不可用: 'Unavailable',
    失败: 'Failed',
    通过: 'Passed',
    未知: 'Unknown',
    等待校验结果: 'Waiting for validation result',
  })[value] || value;

  it('uses explicit translations for validation status labels', () => {
    expect(validationStatusLabel({ loading: true }, enText)).toBe('Generating');
    expect(validationStatusLabel({ loading: false, error: new Error('boom') }, enText)).toBe('Unavailable');
    expect(validationStatusLabel({ loading: false, valid: false }, enText)).toBe('Failed');
    expect(validationStatusLabel({ loading: false, valid: true }, enText)).toBe('Passed');
    expect(validationStatusLabel({ loading: false }, enText)).toBe('Unknown');
  });

  it('uses explicit translations for empty validation summary', () => {
    expect(validationSummary(undefined, enText)).toBe('Waiting for validation result');
  });
});
