import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Client, Inbound, InboundCapability, TrafficV2Snapshot } from '../api/types';
import { ConfirmProvider, ToastProvider } from '../components/ui';
import { I18nProvider } from '../lib/i18n';
import {
  allowedInboundNetworks,
  allowedInboundSecurities,
  applyInboundCapabilitiesFromAPI,
  applyInboundTemplate,
  buildClientPayload,
  buildFullInboundPayload,
  bytesToGB,
  clientFormValues,
  clientUsageSummary,
  createDefaultInbound,
  enabledInboundAdvancedFields,
  gbToBytes,
  hasAttachableSettingCert,
  inboundClientMatchesQuery,
  inboundMatchesQuery,
  inboundCredentialType,
  inboundFormValues,
  inboundProtocolOptions,
  nextClientName,
  protocolBadgeClasses,
  rateLabel,
  resetInboundCapabilitiesForTest,
  sanitizeInboundFormValues,
  shouldSyncInboundWSHost,
  supportsInboundShareLink,
} from './InboundsPage';
import InboundsPage from './InboundsPage';

const apiMock = vi.hoisted(() => ({
  inbounds: vi.fn<() => Promise<Inbound[]>>(async () => []),
  trafficV2Snapshot: vi.fn<() => Promise<TrafficV2Snapshot>>(async () => emptyTrafficSnapshot()),
  inboundCapabilities: vi.fn(async () => []),
  toggleInbound: vi.fn(async () => ({})),
  deleteInbound: vi.fn(async () => ({})),
  toggleClient: vi.fn(async () => ({})),
  deleteClient: vi.fn(async () => ({})),
  resetClientTraffic: vi.fn(async () => ({})),
  subscriptionLink: vi.fn(async () => 'vless://example'),
}));

vi.mock('../api/endpoints', () => ({ api: apiMock }));
vi.mock('./InboundsPageForms', () => ({
  savedClientLinkActions: (protocol: string) => (['shadowtls'].includes(protocol) ? [] : ['share']),
  InboundModal: () => null,
  ClientModal: ({ client }: { client?: Client }) => client ? createElement('div', { role: 'dialog', 'data-testid': 'client-edit-modal' }, client.email) : null,
}));

let root: Root | null = null;
let container: HTMLDivElement | null = null;

afterEach(() => {
  resetInboundCapabilitiesForTest();
  if (root) {
    act(() => root?.unmount());
  }
  root = null;
  container?.remove();
  container = null;
  localStorage.clear();
  vi.clearAllMocks();
});

describe('inbounds page i18n coverage', () => {
  it('keeps primary page Chinese copy behind explicit text calls', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/routes/InboundsPage.tsx'), 'utf8');
    const forbidden = [
      /title="[^"]*[\u4e00-\u9fff]/,
      /description="[^"]*[\u4e00-\u9fff]/,
      /placeholder="[^"]*[\u4e00-\u9fff]/,
      /showToast\('[^']*[\u4e00-\u9fff]/,
      /<option[^>]*>[^<{]*[\u4e00-\u9fff]/,
      /<EmptyState\s+title="[^"]*[\u4e00-\u9fff]/,
    ];
    for (const pattern of forbidden) {
      expect(source).not.toMatch(pattern);
    }
  });
});

describe('inbound client panel behavior', () => {
  it('keeps client lists collapsed by default and expands only the selected node', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['phone-b'])]);
    renderPage();

    await waitForText('edge-a');
    expect(pageText()).not.toContain('phone-a');
    expect(pageText()).not.toContain('phone-b');

    clickButtonByExactText('展开', cardByTitle('edge-a'));

    expect(pageText()).toContain('phone-a');
    expect(pageText()).not.toContain('phone-b');
  });

  it('expands and collapses all visible client lists from the toolbar', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['phone-b'])]);
    renderPage();

    await waitForText('展开全部客户端');
    clickButtonByText('展开全部客户端');

    expect(pageText()).toContain('phone-a');
    expect(pageText()).toContain('phone-b');
    expect(pageText()).toContain('收起全部客户端');

    clickButtonByText('收起全部客户端');
    expect(pageText()).not.toContain('phone-a');
    expect(pageText()).not.toContain('phone-b');
  });

  it('closes a single expanded client list from outside clicks and Escape', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['phone-b'])]);
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    expect(pageText()).toContain('phone-a');

    act(() => document.body.dispatchEvent(new MouseEvent('click', { bubbles: true })));
    expect(pageText()).not.toContain('phone-a');

    clickButtonByExactText('展开', cardByTitle('edge-a'));
    expect(pageText()).toContain('phone-a');

    act(() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(pageText()).not.toContain('phone-a');
  });

  it('switches single-node expansion without leaving the previous node open', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['phone-b'])]);
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    clickButtonByExactText('展开', cardByTitle('edge-b'));

    expect(pageText()).not.toContain('phone-a');
    expect(pageText()).toContain('phone-b');
  });

  it('does not collapse bulk-expanded clients from one outside click', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['phone-b'])]);
    renderPage();

    await waitForText('展开全部客户端');
    clickButtonByText('展开全部客户端');
    act(() => document.body.dispatchEvent(new MouseEvent('click', { bubbles: true })));

    expect(pageText()).toContain('phone-a');
    expect(pageText()).toContain('phone-b');
  });

  it('closes more menus from outside clicks even when client panels are not singly expanded', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a'])]);
    renderPage();

    await waitForText('edge-a');
    const menu = moreActionsByCard('edge-a');
    openDetails(menu);
    expect(menu.open).toBe(true);

    act(() => document.body.dispatchEvent(new MouseEvent('click', { bubbles: true })));

    expect(menu.open).toBe(false);
  });

  it('closes the previous more menu when another more menu is opened', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['phone-b'])]);
    renderPage();

    await waitForText('edge-a');
    const firstMenu = moreActionsByCard('edge-a');
    const secondMenu = moreActionsByCard('edge-b');
    openDetails(firstMenu);
    expect(firstMenu.open).toBe(true);

    act(() => {
      secondMenu.querySelector('summary')?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(firstMenu.open).toBe(false);
    expect(secondMenu.open).toBe(true);
  });

  it('lets Escape close an open more menu before collapsing the client list', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a'])]);
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    expect(pageText()).toContain('phone-a');
    const menu = moreActionsByCard('edge-a');
    openDetails(menu);
    menu.querySelector('button')?.focus();

    act(() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));

    expect(menu.open).toBe(false);
    expect(pageText()).toContain('phone-a');
  });

  it('keeps client action clicks from closing the expanded list', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a'])]);
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    const clientRow = rowByClientName('phone-a');
    const copyButton = clientRow.querySelector('button[title="复制节点链接"]');
    if (!copyButton) throw new Error('missing client copy action');

    act(() => copyButton.dispatchEvent(new MouseEvent('click', { bubbles: true })));

    expect(pageText()).toContain('phone-a');
  });

  it('keeps client edit inside the more actions menu', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a'])]);
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    const clientRow = rowByClientName('phone-a');

    expect(clientRow.querySelector('.client-actions > button[title="编辑"]')).toBeNull();
    const menu = clientMoreActionsByName('phone-a');
    openDetails(menu);

    const editMenuItem = Array.from(menu.querySelectorAll('.more-actions-menu button')).find((item) => item.textContent?.trim() === '编辑');
    if (!editMenuItem) throw new Error('missing client edit menu item');
    await act(async () => {
      editMenuItem.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      await Promise.resolve();
    });

    await vi.waitFor(() => {
      expect(pageRoot().querySelector('[data-testid="client-edit-modal"]')?.textContent).toContain('phone-a');
    });
  });

  it('auto-expands nodes whose clients match the search query', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['tablet-b'])]);
    renderPage();

    await waitForText('edge-a');
    changeInput(pageRoot().querySelector('input[placeholder="搜索节点、客户端、协议、端口..."]') as HTMLInputElement, 'tablet');

    expect(pageText()).toContain('edge-b');
    expect(pageText()).toContain('tablet-b');
    expect(pageText()).not.toContain('phone-a');
    expect(buttonByText('收起全部客户端')).toBeDisabled();
    expect(buttonByText('搜索命中')).toBeDisabled();
  });

  it('does not disable the bulk toggle for client matches hidden by filters', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a']), sampleInbound(2, 'edge-b', ['tablet-b'])]);
    renderPage();

    await waitForText('edge-a');
    changeSelect(selectByValue('all'), 'vless');
    changeInput(pageRoot().querySelector('input[placeholder="搜索节点、客户端、协议、端口..."]') as HTMLInputElement, 'tablet');

    expect(pageText()).not.toContain('edge-b');
    expect(buttonByText('展开全部客户端')).not.toBeDisabled();
  });
});

describe('inbound management display helpers', () => {
  const text = (value: string) => value;

  it('matches client names in search without relying on hidden credentials', () => {
    const inbound = sampleInbound(1, 'edge', ['phone']);
    inbound.clients![0].uuid = '11111111-1111-4111-8111-111111111111';

    expect(inboundMatchesQuery(inbound, 'phone')).toBe(true);
    expect(inboundClientMatchesQuery(inbound, 'phone')).toBe(true);
    expect(inboundClientMatchesQuery(inbound, '11111111')).toBe(false);
  });

  it('does not expose UUID, credential id, or normal stats copy in the default client row source', () => {
    const source = readFileSync(resolve(process.cwd(), 'src/routes/InboundsPage.tsx'), 'utf8');
    const clientRowSource = source.slice(source.indexOf('function ClientRow'), source.indexOf('function MetricTile'));

    expect(clientRowSource).not.toContain('client.uuid');
    expect(clientRowSource).not.toContain('credential_id');
    expect(clientRowSource).not.toContain("text('统计状态')");
    expect(clientRowSource).not.toContain("text('不限制')");
  });

  it('uses standardized inbound metadata and count badge copy', async () => {
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', ['phone-a', 'phone-b'])]);
    renderPage();

    await waitForText('edge-a');
    const card = cardByTitle('edge-a');
    const meta = card.querySelector('.inbound-meta-line');
    const heading = card.querySelector('.client-section-heading');

    expect(meta?.textContent).toContain('端口 441');
    expect(meta?.textContent).toContain('tcp');
    expect(meta?.textContent).toContain('reality');
    expect(meta?.textContent).not.toMatch(/:441\s*·/);
    expect(meta?.textContent).not.toContain('2 个客户端');
    expect(heading?.textContent).toBe('客户端2');
    expect(card.querySelector('.client-count-badge')?.textContent).toBe('2');
  });

  it('keeps long client names constrained without shrinking the enabled badge', async () => {
    const longName = 'very-long-client-name-that-should-stay-on-one-line-and-not-compress-the-status-badge@example.com';
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge-a', [longName])]);
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    const row = rowByClientName(longName);
    const name = row.querySelector('.client-name') as HTMLElement | null;
    const badge = row.querySelector('.client-name-row .status-badge') as HTMLElement | null;

    expect(name).toHaveAttribute('title', longName);
    expect(badge?.textContent).toBe('启用');
    const css = readFileSync(resolve(process.cwd(), 'src/styles/index.css'), 'utf8');
    expect(css).toMatch(/\.client-name\s*\{[\s\S]*min-width:\s*0;/);
    expect(css).toMatch(/\.client-name\s*\{[\s\S]*text-overflow:\s*ellipsis;/);
    expect(css).toMatch(/\.client-name-row \.status-badge\s*\{[\s\S]*white-space:\s*nowrap;/);
    expect(css).toMatch(/\.status-badge\s*\{[\s\S]*flex:\s*0 0 auto;/);
  });

  it('renders limited usage progress without showing percentage for unlimited clients', async () => {
    const inbound = sampleInbound(1, 'edge-a', ['unlimited', 'limited']);
    apiMock.inbounds.mockResolvedValueOnce([inbound]);
    apiMock.trafficV2Snapshot.mockResolvedValueOnce({
      ...emptyTrafficSnapshot(),
      clients: [
        { id: 10, inbound_id: 1, email: 'unlimited', enabled: true, traffic_limit: 0, expiry_at: 0, cumulative: v2Metric(0, 0), realtime: v2Realtime(0, 0, 'waiting') },
        { id: 11, inbound_id: 1, email: 'limited', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: v2Metric(95, 0), realtime: v2Realtime(0, 0, 'waiting') },
      ],
    });
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));

    expect(rowByClientName('unlimited').textContent).not.toContain('%');
    expect(rowByClientName('unlimited').querySelector('.usage-line')).toBeNull();
    expect(rowByClientName('limited').textContent).toContain('95%');
    expect(rowByClientName('limited').querySelector('.usage-bar')).not.toBeNull();
  });

  it('renders client usage text and progress from v2 client cumulative before legacy counters', async () => {
    const inbound = sampleInbound(1, 'edge-a', ['limited']);
    Object.assign(inbound.clients![0], {
      up: 999,
      down: 999,
      traffic_limit: 100,
    });
    apiMock.inbounds.mockResolvedValueOnce([inbound]);
    apiMock.trafficV2Snapshot.mockResolvedValueOnce({
      ...emptyTrafficSnapshot(),
      clients: [{ id: 10, inbound_id: 1, email: 'limited', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: v2Metric(30, 40), realtime: v2Realtime(0, 0, 'waiting') }],
    });
    renderPage();

    await waitForText('edge-a');
    clickButtonByExactText('展开', cardByTitle('edge-a'));
    const row = rowByClientName('limited');

    expect(row.textContent).toContain('已用 70 B / 100 B');
    expect(row.textContent).toContain('70%');
    expect(row.querySelector('.usage-bar')).not.toBeNull();
  });

  it('summarizes unlimited and limited client usage with the correct progress semantics', () => {
    expect(clientUsageSummary(0, v2Metric(0, 0), text)).toMatchObject({ label: '暂无流量', percentLabel: '' });
    expect(clientUsageSummary(0, v2Metric(1024, 0), text)).toMatchObject({ label: '已用 1.0 KB', percentLabel: '' });
    expect(clientUsageSummary(100, v2Metric(70, 0), text)).toMatchObject({ percent: 70, percentLabel: '70%', tone: 'warning' });
    expect(clientUsageSummary(100, v2Metric(95, 0), text)).toMatchObject({ percent: 95, percentLabel: '95%', tone: 'danger' });
    expect(clientUsageSummary(100, v2Metric(120, 0), text)).toMatchObject({ percent: 100, percentLabel: '已超额', tone: 'over' });
    expect(clientUsageSummary(100, v2Metric(1, 2, 70), text)).toMatchObject({ label: '已用 70 B / 100 B', percent: 70, tone: 'warning' });
  });

  it('uses explicit zero realtime fallback and complete protocol color classes', () => {
    expect(rateLabel(0, 0, text)).toBe('0 B/s');
    expect(rateLabel(1024, 2048, text)).toBe('1.0 KB/s ↑ / 2.0 KB/s ↓');
    expect(rateLabel(1024, 2048, text, 'waiting')).toBe('等待采样');
    expect(rateLabel(1024, 2048, text, 'stale')).toBe('统计已过期');
    expect(rateLabel(1024, 2048, text, 'unavailable')).toBe('统计不可用');
    expect(rateLabel(1024, 2048, text, 'unsupported')).toBe('实时统计不可用');
    expect(rateLabel(1024, 2048, text, 'partial')).toBe('1.0 KB/s ↑ / 2.0 KB/s ↓');
    expect(protocolBadgeClasses).toMatchObject({
      vless: 'protocol-vless',
      vmess: 'protocol-vmess',
      trojan: 'protocol-trojan',
      shadowsocks: 'protocol-shadowsocks',
      hysteria2: 'protocol-hysteria2',
      socks: 'protocol-socks',
      http: 'protocol-http',
    });
  });

  it('uses v2 traffic objects over conflicting legacy flat fields', async () => {
    const inbound: Inbound = {
      id: 1,
      remark: 'edge',
      protocol: 'vless',
      port: 443,
      network: 'tcp',
      security: 'none',
      enabled: true,
      clients: [],
    };
    const client = {
      id: 10,
      inbound_id: 1,
      email: 'sam@example.com',
      uuid: 'client-uuid',
      enabled: true,
      traffic_limit: 1000,
      expiry_at: 0,
    } satisfies Client;
    inbound.clients = [client];
    apiMock.inbounds.mockResolvedValueOnce([inbound]);
    apiMock.trafficV2Snapshot.mockResolvedValueOnce({
      ...emptyTrafficSnapshot(),
      inbounds: [{ id: 1, remark: 'edge', protocol: 'vless', port: 443, enabled: true, cumulative: v2Metric(10, 20), realtime: v2Realtime(4, 5) }],
      clients: [{ id: 10, inbound_id: 1, email: 'sam@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: v2Metric(30, 40), realtime: v2Realtime(6, 7) }],
    });
    renderPage();

    await waitForText('edge');
    const card = cardByTitle('edge');
    expect(card.textContent).toContain('30 B');
    expect(card.textContent).toContain('4 B/s ↑ / 5 B/s ↓');
    clickButtonByExactText('展开', card);
    const row = rowByClientName('sam@example.com');
    expect(row.textContent).toContain('已用 70 B / 1000 B');
    expect(row.textContent).not.toContain('6 B/s ↑ / 7 B/s ↓');
    expect(clientUsageIndicator(row)).toHaveAttribute('title', expect.stringContaining('6 B/s ↑ / 7 B/s ↓'));
    expect(row.textContent).not.toContain('999 B');
  });

  it('updates inbound and client traffic from stream patch events', async () => {
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
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge', ['sam@example.com'])]);
    apiMock.trafficV2Snapshot.mockResolvedValueOnce({
      ...emptyTrafficSnapshot(),
      inbounds: [{ id: 1, remark: 'edge', protocol: 'vless', port: 441, enabled: true, cumulative: v2Metric(10, 20), realtime: v2Realtime(4, 5) }],
      clients: [{ id: 10, inbound_id: 1, email: 'sam@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: v2Metric(30, 40), realtime: v2Realtime(6, 7) }],
    });
    renderPage();

    await waitForText('edge');
    let card = cardByTitle('edge');
    expect(card.textContent).toContain('4 B/s ↑ / 5 B/s ↓');
    clickButtonByExactText('展开', card);
    let row = rowByClientName('sam@example.com');
    expect(clientUsageIndicator(row)).toHaveAttribute('title', expect.stringContaining('6 B/s ↑ / 7 B/s ↓'));

    act(() => {
      instances[0].listeners.get('patch')?.[0]({
        data: JSON.stringify({
          generated_at: '2026-06-24T00:00:05Z',
          observed_at: '2026-06-24T00:00:05Z',
          window_seconds: 5,
          inbounds: [{ id: 1, remark: 'edge', protocol: 'vless', port: 441, enabled: true, cumulative: v2Metric(10, 20), realtime: v2Realtime(8, 9, 'unsupported', 'inbound') }],
          clients: [{ id: 10, inbound_id: 1, email: 'sam@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: v2Metric(30, 40), realtime: v2Realtime(1, 2, 'waiting', 'client') }],
          coverage: { overall: 'partial', engines: { xray: 'partial', singbox: 'not_configured' }, ok: 0, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 1 },
        }),
      } as MessageEvent);
    });

    await vi.waitFor(() => {
      card = cardByTitle('edge');
      expect(card.textContent).toContain('实时统计不可用');
      row = rowByClientName('sam@example.com');
      expect(row.textContent).not.toContain('等待采样');
      expect(clientUsageIndicator(row)).toHaveAttribute('title', expect.stringContaining('等待采样'));
      expect(clientUsageIndicator(row)).not.toHaveAttribute('title', expect.stringContaining('1 B/s ↑ / 2 B/s ↓'));
    });
  });

  it('drops deleted client traffic overlays from stream patch merges', async () => {
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
    apiMock.inbounds.mockResolvedValueOnce([sampleInbound(1, 'edge', ['sam@example.com'])]);
    apiMock.trafficV2Snapshot.mockResolvedValueOnce({
      ...emptyTrafficSnapshot(),
      inbounds: [{ id: 1, remark: 'edge', protocol: 'vless', port: 441, enabled: true, cumulative: v2Metric(10, 20), realtime: v2Realtime(4, 5) }],
      clients: [{ id: 10, inbound_id: 1, email: 'sam@example.com', enabled: true, traffic_limit: 100, expiry_at: 0, cumulative: v2Metric(30, 40), realtime: v2Realtime(6, 7) }],
    });
    renderPage();

    await waitForText('edge');
    clickButtonByExactText('展开', cardByTitle('edge'));
    expect(pageText()).toContain('sam@example.com');
    expect(clientUsageIndicator(rowByClientName('sam@example.com'))).toHaveAttribute('title', expect.stringContaining('6 B/s ↑ / 7 B/s ↓'));

    await act(async () => {
      instances[0].listeners.get('patch')?.[0]({
        data: JSON.stringify({
          generated_at: '2026-06-24T00:00:05Z',
          observed_at: '2026-06-24T00:00:05Z',
          window_seconds: 5,
          removed_client_ids: [10],
        }),
      } as MessageEvent);
      await Promise.resolve();
    });

    await vi.waitFor(() => {
      expect(pageText()).toContain('sam@example.com');
      expect(rowByClientName('sam@example.com').querySelector('.usage-bar, .usage-line')).toBeNull();
      expect(pageText()).not.toContain('6 B/s ↑ / 7 B/s ↓');
    });
  });
});

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
    ws_path: '/ws',
    ws_host: 'cdn.example.com',
    grpc_service_name: 'grpc-edge',
    reality_dest: 'www.cloudflare.com:443',
    reality_server_names: 'www.cloudflare.com',
    reality_short_id: 'abcd',
    reality_private_key: 'private-key',
    reality_public_key: 'public-key',
    ss_method: '2022-blake3-aes-128-gcm',
    tls_cert_file: '/etc/migate/certs/example/fullchain.pem',
    tls_key_file: '/etc/migate/certs/example/privkey.pem',
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

  it('applies UDP fast and light templates with generated secrets', () => {
    const base = inboundFormValues(createDefaultInbound());

    const udpFast = applyInboundTemplate(base, 'udp-fast');
    const udpFastAgain = applyInboundTemplate(base, 'udp-fast');
    expect(udpFast).toMatchObject({
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
    expect(udpFast.uuid).toMatch(/^[0-9a-f]{32}$/);
    expect(udpFast.hy2_obfs_password).toHaveLength(18);
    expect(udpFastAgain.hy2_obfs_password).toHaveLength(18);
    expect(udpFastAgain.hy2_obfs_password).not.toBe(udpFast.hy2_obfs_password);

    const light = applyInboundTemplate(base, 'light');
    expect(light).toMatchObject({
      protocol: 'shadowsocks',
      network: 'tcp',
      security: 'none',
      port: 0,
      ss_method: '2022-blake3-aes-128-gcm',
    });
    expect(light.uuid).toMatch(/^[0-9a-f]{24}$/);
    expect(light.hy2_obfs).toBe('');
    expect(light.hy2_obfs_password).toBe('');
    expect(light.hy2_mport).toBe('');
    expect(light.tls_sni).toBe('');
  });

  it('keeps generated inbound internal ID format aligned with regenerate behavior', () => {
    const base = inboundFormValues(createDefaultInbound());
    expect(base.uuid).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);

    expect(applyInboundTemplate(base, 'recommended').uuid).toMatch(/^[0-9a-f-]{36}$/i);
    expect(applyInboundTemplate(base, 'compatible').uuid).toMatch(/^[0-9a-f-]{36}$/i);
    expect(applyInboundTemplate(base, 'password').uuid).toMatch(/^[0-9a-f-]{36}$/i);
    expect(applyInboundTemplate(base, 'light').uuid).toMatch(/^[0-9a-f]{24}$/);
    expect(applyInboundTemplate(base, 'local-socks').uuid).toMatch(/^[0-9a-f]{24}$/);
    expect(applyInboundTemplate(base, 'local-http').uuid).toMatch(/^[0-9a-f]{24}$/);
    expect(applyInboundTemplate(base, 'udp-fast').uuid).toMatch(/^[0-9a-f]{32}$/);
    expect(applyInboundTemplate(base, 'low-latency').uuid).toMatch(/^[0-9a-f]{32}$/);
    expect(applyInboundTemplate(base, 'handshake-mask').uuid).toMatch(/^[0-9a-f]{32}$/);

    expect(sanitizeInboundFormValues(base, { protocol: 'socks' }).uuid).toMatch(/^[0-9a-f]{24}$/);
    expect(sanitizeInboundFormValues(base, { protocol: 'tuic' }).uuid).toMatch(/^[0-9a-f]{32}$/);
    expect(sanitizeInboundFormValues(applyInboundTemplate(base, 'udp-fast'), { protocol: 'vless' }).uuid).toMatch(/^[0-9a-f-]{36}$/i);
  });

  it('sanitizes protocol, transport, and security changes to supported combinations', () => {
    const hy2 = applyInboundTemplate(inboundFormValues(createDefaultInbound()), 'udp-fast');
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
      network: 'ws',
      security: 'tls',
      reality_dest: '',
      reality_server_names: '',
      reality_short_id: '',
      ws_path: '',
      ws_host: '',
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

  it('keeps socks/http as local proxy inbounds and drops unsupported transports', () => {
    const socks = sanitizeInboundFormValues(inboundFormValues(createDefaultInbound()), { protocol: 'socks' });
    expect(socks).toMatchObject({ protocol: 'socks', network: 'tcp', security: 'none' });
    expect(supportsInboundShareLink('socks')).toBe(true);
    expect(supportsInboundShareLink('http')).toBe(true);
    expect(supportsInboundShareLink('vless')).toBe(true);

    const invalid = sanitizeInboundFormValues(inboundFormValues(createDefaultInbound()), { network: 'quic' });
    expect(invalid.network).toBe('tcp');
  });

  it('uses API inbound capabilities as the active matrix with fallback reset', () => {
    applyInboundCapabilitiesFromAPI([
      {
        protocol: 'mystery',
        core: 'sing-box',
        networks: ['udp'],
        securities: ['tls'],
        default_network: 'udp',
        default_security: 'tls',
        security_by_network: { default: ['tls'] },
        advanced_fields: [],
        credential_type: 'password',
        subscription: 'none',
      },
      {
        protocol: 'tuic',
        core: 'sing-box',
        networks: ['udp'],
        securities: ['tls'],
        default_network: 'udp',
        default_security: 'tls',
        security_by_network: { default: ['tls'] },
        advanced_fields: ['tls_cert_file', 'tls_key_file', 'tls_sni', 'tuic_zero_rtt'],
        credential_type: 'credential_id_password',
        subscription: 'none',
        share_link: false,
        local_proxy_inbound: false,
      },
      {
        protocol: 'vless',
        core: 'xray',
        networks: ['grpc'],
        securities: ['none', 'reality'],
        default_network: 'grpc',
        default_security: 'reality',
        security_by_network: { default: ['none'], grpc: ['none', 'reality'] },
        advanced_fields: ['grpc_service_name', 'reality_dest', 'reality_server_names', 'reality_private_key', 'reality_public_key'],
        credential_type: 'uuid',
        subscription: 'full',
        share_link: true,
      },
    ]);

    expect(inboundProtocolOptions()).toEqual(['tuic', 'vless']);
    expect(allowedInboundNetworks('vless')).toEqual(['grpc']);
    expect(allowedInboundSecurities('vless', 'grpc')).toEqual(['none', 'reality']);
    expect(inboundCredentialType('tuic')).toBe('credential_id_password');
    expect(supportsInboundShareLink('tuic')).toBe(false);

    const normalized = sanitizeInboundFormValues(inboundFormValues(createDefaultInbound()), { protocol: 'vless' });
    expect(normalized).toMatchObject({ protocol: 'vless', network: 'grpc', security: 'reality' });

    resetInboundCapabilitiesForTest();
    expect(inboundProtocolOptions()).toContain('shadowtls');
    expect(supportsInboundShareLink('tuic')).toBe(true);
  });

  it('uses share_link as the authoritative frontend share capability', () => {
    applyInboundCapabilitiesFromAPI([
      {
        protocol: 'vless',
        core: 'xray',
        networks: ['tcp'],
        securities: ['none'],
        default_network: 'tcp',
        default_security: 'none',
        security_by_network: { default: ['none'] },
        advanced_fields: [],
        credential_type: 'uuid',
        subscription: 'full',
        share_link: false,
      },
      {
        protocol: 'shadowtls',
        core: 'sing-box',
        networks: ['tcp'],
        securities: ['none'],
        default_network: 'tcp',
        default_security: 'none',
        security_by_network: { default: ['none'] },
        advanced_fields: ['tls_sni', 'shadowtls_version'],
        credential_type: 'password',
        subscription: 'none',
        share_link: true,
      },
    ]);

    expect(supportsInboundShareLink('vless')).toBe(false);
    expect(supportsInboundShareLink('shadowtls')).toBe(true);
  });

  it('falls back safely when API capability fields are incomplete', () => {
    expect(() => applyInboundCapabilitiesFromAPI([
      {
        protocol: 'vless',
        core: 'xray',
        networks: undefined,
        securities: undefined,
        default_network: '',
        default_security: '',
        security_by_network: undefined,
        advanced_fields: undefined,
        credential_type: '',
        subscription: '',
      } as unknown as InboundCapability,
    ])).not.toThrow();

    expect(inboundProtocolOptions()).toEqual(['vless']);
    expect(allowedInboundNetworks('vless')).toContain('tcp');
    expect(allowedInboundSecurities('vless', 'tcp')).toContain('reality');
    expect(supportsInboundShareLink('vless')).toBe(true);
  });

  it('keeps ShadowTLS tls_sni as a protocol handshake field from API capabilities', () => {
    applyInboundCapabilitiesFromAPI([
      {
        protocol: 'shadowtls',
        core: 'sing-box',
        networks: ['tcp'],
        securities: ['none'],
        default_network: 'tcp',
        default_security: 'none',
        security_by_network: { default: ['none'] },
        advanced_fields: ['tls_sni', 'shadowtls_version'],
        credential_type: 'password',
        subscription: 'none',
      },
    ]);

    expect(enabledInboundAdvancedFields({ protocol: 'shadowtls', network: 'tcp', security: 'none' }).has('tls_sni')).toBe(true);
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

  it('shows saved-client link actions for user-password proxy protocols', () => {
    expect(supportsInboundShareLink('socks')).toBe(true);
    expect(supportsInboundShareLink('http')).toBe(true);
    expect(supportsInboundShareLink('shadowtls')).toBe(false);
    expect(supportsInboundShareLink('vless')).toBe(true);
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

  it('attaches an initial client only for new node payloads', () => {
    const values = inboundFormValues(createDefaultInbound());
    const client = buildClientPayload(clientFormValues(createDefaultInbound()), values.protocol);
    const created = buildFullInboundPayload(null, values, client);
    const updated = buildFullInboundPayload(existing, values, client);

    expect(created.initial_client).toMatchObject({ email: '首个客户端', enabled: true });
    expect(updated).not.toHaveProperty('initial_client');
  });

  it('keeps VMess template as WS + TLS by default', () => {
    const values = applyInboundTemplate(inboundFormValues(createDefaultInbound()), 'compatible');

    expect(values).toMatchObject({
      protocol: 'vmess',
      network: 'ws',
      security: 'tls',
      ws_path: '/migate',
      tls_sni: 'example.com',
    });
  });

  it('creates a non-empty default client name for new node payloads', () => {
    const inbound = createDefaultInbound();
    const values = clientFormValues(inbound);

    expect(values.email).toBe('首个客户端');
    expect(buildClientPayload(values, inbound.protocol).email).toBe('首个客户端');
  });

  it('uses numbered names when adding more clients to an existing node', () => {
    const inbound = {
      ...createDefaultInbound(),
      remark: 'edge',
      clients: [
        { id: 1, inbound_id: 1, email: 'edge 首个客户端', uuid: 'client-1', enabled: true },
      ],
    };

    expect(clientFormValues(inbound).email).toBe('客户端 2');
    expect(nextClientName({ ...inbound, clients: [...inbound.clients, { id: 2, inbound_id: 1, email: '客户端 2', uuid: 'client-2', enabled: true }] })).toBe('客户端 3');
    expect(nextClientName({ ...inbound, clients: [{ id: 2, inbound_id: 1, email: '客户端 2', uuid: 'client-2', enabled: true }] })).toBe('客户端 3');
    expect(nextClientName({ ...inbound, clients: [{ id: 3, inbound_id: 1, email: '客户端 3', uuid: 'client-3', enabled: true }] })).toBe('客户端 2');
  });

  it('normalizes blank inbound ports to zero for backend auto-assignment', () => {
    const values = inboundFormValues(createDefaultInbound());
    values.port = '' as unknown as typeof values.port;

    const payload = buildFullInboundPayload(null, values);

    expect(payload.port).toBe(0);
  });

  it('allows attaching a settings certificate only after it is issued with both files', () => {
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: false, cert_path: '/etc/migate/certs/example/fullchain.pem', key_path: '/etc/migate/certs/example/privkey.pem' })).toBe(false);
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: true, cert_path: '/etc/migate/certs/example/fullchain.pem', key_path: '' })).toBe(false);
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: true, cert_path: '   ', key_path: '/etc/migate/certs/example/privkey.pem' })).toBe(false);
    expect(hasAttachableSettingCert({ domain: 'example.com', email: 'admin@example.com', issued: true, cert_path: '/etc/migate/certs/example/fullchain.pem', key_path: '/etc/migate/certs/example/privkey.pem' })).toBe(true);
  });

  it('syncs WS/H2 host only for unset, template, or old-SNI values', () => {
    expect(shouldSyncInboundWSHost('ws', '', 'old.example.com')).toBe(true);
    expect(shouldSyncInboundWSHost('h2', 'example.com', 'old.example.com')).toBe(true);
    expect(shouldSyncInboundWSHost('ws', 'old.example.com', 'old.example.com')).toBe(true);
    expect(shouldSyncInboundWSHost('ws', 'cdn.example.com', 'old.example.com')).toBe(false);
    expect(shouldSyncInboundWSHost('tcp', 'example.com', 'old.example.com')).toBe(false);
  });

});

function renderPage() {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  act(() => {
    root!.render(createElement(
      I18nProvider,
      null,
      createElement(
        QueryClientProvider,
        { client: queryClient },
        createElement(
          ToastProvider,
          null,
          createElement(ConfirmProvider, null, createElement(InboundsPage)),
        ),
      ),
    ));
  });
}

function emptyTrafficSnapshot(): TrafficV2Snapshot {
  return {
    generated_at: '',
    observed_at: '',
    window_seconds: 0,
    total: { cumulative: v2Metric(0, 0, 0, 'waiting', 'migate'), realtime: v2Realtime(0, 0, 'waiting', 'inbound') },
    inbounds: [],
    clients: [],
    coverage: { overall: 'waiting', engines: { xray: 'not_configured', singbox: 'not_configured' }, ok: 0, waiting: 0, stale: 0, unavailable: 0, unsupported: 0, partial: 0 },
  };
}

function v2Metric(up: number, down: number, total = up + down, status = 'ok', source = 'client') {
  return { up, down, total, status, source, message: '' };
}

function v2Realtime(rateUp: number, rateDown: number, status = 'ok', source = 'client') {
  return { delta_up: 0, delta_down: 0, delta_total: 0, rate_up: rateUp, rate_down: rateDown, rate_total: rateUp + rateDown, observed_at: '2026-06-24T00:00:00Z', window_seconds: 5, status, source, message: '' };
}

function sampleInbound(id: number, remark: string, clientNames: string[]): Inbound {
  return {
    id,
    remark,
    protocol: id === 2 ? 'trojan' : 'vless',
    port: 440 + id,
    network: 'tcp',
    security: 'reality',
    enabled: true,
    uuid: `node-${id}`,
    clients: clientNames.map((name, index): Client => ({
      id: id * 10 + index,
      inbound_id: id,
      email: name,
      uuid: `uuid-${id}-${index}`,
      credential_id: `credential-${id}-${index}`,
      enabled: true,
      traffic_limit: index ? 100 : 0,
      expiry_at: 0,
    })),
  };
}

async function waitForText(value: string) {
  await vi.waitFor(() => {
    expect(pageText()).toContain(value);
  });
}

function clickButtonByText(value: string) {
  const button = buttonByText(value);
  act(() => button.dispatchEvent(new MouseEvent('click', { bubbles: true })));
}

function buttonByText(value: string) {
  const button = Array.from(pageRoot().querySelectorAll('button')).find((item) => item.textContent?.includes(value));
  if (!button) throw new Error(`missing button text: ${value}`);
  return button as HTMLButtonElement;
}

function clickButtonByExactText(value: string, scope: ParentNode = pageRoot()) {
  const button = Array.from(scope.querySelectorAll('button')).find((item) => item.textContent?.trim() === value);
  if (!button) throw new Error(`missing button text: ${value}`);
  act(() => button.dispatchEvent(new MouseEvent('click', { bubbles: true })));
}

function cardByTitle(title: string) {
  const heading = Array.from(pageRoot().querySelectorAll('h2')).find((item) => item.textContent?.trim() === title);
  const card = heading?.closest('.resource-card');
  if (!card) throw new Error(`missing card: ${title}`);
  return card;
}

function rowByClientName(name: string) {
  const label = Array.from(pageRoot().querySelectorAll('.client-name')).find((item) => item.textContent?.trim() === name);
  const row = label?.closest('.client-row');
  if (!row) throw new Error(`missing client row: ${name}`);
  return row;
}

function clientUsageIndicator(row: Element) {
  const indicator = row.querySelector('.usage-bar, .usage-line');
  if (!indicator) throw new Error('missing client usage indicator');
  return indicator;
}

function clientMoreActionsByName(name: string) {
  const menu = rowByClientName(name).querySelector('details[data-more-actions]') as HTMLDetailsElement | null;
  if (!menu) throw new Error(`missing client more actions: ${name}`);
  return menu;
}

function moreActionsByCard(title: string) {
  const menu = cardByTitle(title).querySelector('details[data-more-actions]') as HTMLDetailsElement | null;
  if (!menu) throw new Error(`missing more actions: ${title}`);
  return menu;
}

function openDetails(details: HTMLDetailsElement) {
  act(() => {
    details.open = true;
  });
}

function pageRoot() {
  if (!container) throw new Error('missing test container');
  return container;
}

function pageText() {
  return pageRoot().textContent || '';
}

function changeInput(input: HTMLInputElement | null, value: string) {
  if (!input) throw new Error('missing input');
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event('input', { bubbles: true }));
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

function changeSelect(select: HTMLSelectElement, value: string) {
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, 'value')?.set;
    setter?.call(select, value);
    select.dispatchEvent(new Event('input', { bubbles: true }));
    select.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

function selectByValue(value: string) {
  const select = Array.from(pageRoot().querySelectorAll('select')).find((item) => item.value === value);
  if (!select) throw new Error(`missing select value: ${value}`);
  return select as HTMLSelectElement;
}

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

  it('builds protocol-specific client credential payloads', () => {
    const tuicValues = clientFormValues({ ...createDefaultInbound(), protocol: 'tuic', network: 'udp', security: 'tls' });
    tuicValues.credential_id = 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa';
    tuicValues.password = 'tuic-secret';
    expect(buildClientPayload(tuicValues, 'tuic')).toMatchObject({
      uuid: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
      credential_id: 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
      password: 'tuic-secret',
    });

    const socksValues = clientFormValues({ ...createDefaultInbound(), protocol: 'socks', network: 'tcp', security: 'none' });
    socksValues.credential_id = 'sam';
    socksValues.password = 'secret';
    expect(buildClientPayload(socksValues, 'socks')).toMatchObject({ uuid: 'sam', credential_id: 'sam', password: 'secret' });

    const trojanValues = clientFormValues({ ...createDefaultInbound(), protocol: 'trojan', network: 'tcp', security: 'tls' });
    trojanValues.uuid = 'old-internal-id';
    trojanValues.credential_id = '';
    trojanValues.password = 'trojan-secret';
    expect(buildClientPayload(trojanValues, 'trojan')).toMatchObject({ uuid: 'trojan-secret', credential_id: '', password: 'trojan-secret' });

    const editingTrojan = clientFormValues({ ...createDefaultInbound(), protocol: 'trojan', network: 'tcp', security: 'tls' }, {
      id: 7,
      inbound_id: 1,
      email: 'trojan-user',
      uuid: 'stored-password',
      enabled: true,
    });
    expect(editingTrojan.password).toBe('stored-password');
    expect(editingTrojan.credential_id).toBe('');
    expect(buildClientPayload(editingTrojan, 'trojan')).toMatchObject({ uuid: 'stored-password', credential_id: '', password: 'stored-password' });
  });
});
