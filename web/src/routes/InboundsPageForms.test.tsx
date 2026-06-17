import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConfirmProvider, ToastProvider } from '../components/ui';
import type { Client, Inbound } from '../api/types';
import { I18nProvider } from '../lib/i18n';
import { createDefaultInbound } from './InboundsPage';
import { ClientModal, InboundModal } from './InboundsPageForms';

vi.mock('../api/endpoints', () => ({
  api: {
    certStatus: vi.fn(async () => ({ issued: false, cert_path: '', key_path: '', domain: '' })),
    generateRealityKeypair: vi.fn(async () => ({ private_key: 'private', public_key: 'public' })),
    createInbound: vi.fn(async (body) => ({ inbound: { id: 99, clients: [], ...body } })),
    updateInbound: vi.fn(async (_id, body) => ({ inbound: body })),
    createClient: vi.fn(async (_inboundId, body) => ({ client: { id: 7, inbound_id: _inboundId, ...body } })),
    updateClient: vi.fn(async (_inboundId, id, body) => ({ client: { id, inbound_id: _inboundId, ...body } })),
  },
}));

let root: Root | null = null;
let container: HTMLDivElement | null = null;

afterEach(() => {
  if (root) {
    act(() => root?.unmount());
  }
  root = null;
  container?.remove();
  container = null;
});

describe('inbound and client modal credential behavior', () => {
  it('keeps client UUID stable across parent rerenders and inbound object reference changes', () => {
    const inbound = inboundWithClient();
    const client = inbound.clients![0];
    renderModal(<ClientModal inbound={inbound} client={client} onClose={() => undefined} onSaved={() => undefined} />);

    openCredentialSection();
    const initial = inputByLabel('UUID').value;

    const refreshedInbound = { ...inbound, traffic_total: 2048, clients: [{ ...client, up: 128 }] };
    renderModal(<ClientModal inbound={refreshedInbound} client={refreshedInbound.clients![0]} onClose={() => undefined} onSaved={() => undefined} />);

    expect(inputByLabel('UUID').value).toBe(initial);
  });

  it('regenerates client credentials only when the user clicks regenerate', () => {
    const inbound = inboundWithClient();
    const client = inbound.clients![0];
    renderModal(<ClientModal inbound={inbound} client={client} onClose={() => undefined} onSaved={() => undefined} />);

    openCredentialSection();
    const initial = inputByLabel('UUID').value;
    clickButtonByTitle('重新生成客户端凭据');

    expect(inputByLabel('UUID').value).not.toBe(initial);
  });

  it('regenerates the default client credential when a new node protocol changes', () => {
    renderModal(<InboundModal inbound={createDefaultInbound()} onClose={() => undefined} onSaved={() => undefined} />);

    openCredentialSection('客户端凭据');
    const initial = inputByLabel('UUID').value;
    clickButtonByText('高级设置');
    const protocol = inputByLabel('协议') as HTMLSelectElement;
    changeValue(protocol, 'tuic');

    expect(inputByLabel('TUIC UUID').value).not.toBe(initial);
    expect(inputByLabel('TUIC 密码').value).not.toBe('');
  });
});

function renderModal(node: React.ReactNode) {
  if (!container) {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  }
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  act(() => {
    root!.render(
      <I18nProvider>
        <QueryClientProvider client={queryClient}>
          <ToastProvider>
            <ConfirmProvider>{node}</ConfirmProvider>
          </ToastProvider>
        </QueryClientProvider>
      </I18nProvider>,
    );
  });
}

function inboundWithClient(): Inbound {
  const inbound = { ...createDefaultInbound(), id: 1, remark: 'edge', uuid: 'inbound-id' };
  const client: Client = {
    id: 10,
    inbound_id: 1,
    email: 'phone',
    uuid: '11111111-1111-4111-8111-111111111111',
    credential_id: '11111111-1111-4111-8111-111111111111',
    enabled: true,
    traffic_limit: 0,
    expiry_at: 0,
  };
  return { ...inbound, clients: [client] };
}

function openCredentialSection(label = '展开编辑') {
  clickButtonByText(label);
}

function inputByLabel(label: string) {
  const field = Array.from(document.querySelectorAll('label')).find((item) => item.textContent?.includes(label));
  const input = field?.querySelector('input, select') as HTMLInputElement | HTMLSelectElement | null;
  if (!input) throw new Error(`missing input: ${label}`);
  return input;
}

function clickButtonByText(text: string) {
  const button = Array.from(document.querySelectorAll('button')).find((item) => item.textContent?.includes(text));
  if (!button) throw new Error(`missing button text: ${text}`);
  act(() => button.dispatchEvent(new MouseEvent('click', { bubbles: true })));
}

function clickButtonByTitle(title: string) {
  const button = document.querySelector(`button[title="${title}"]`);
  if (!button) throw new Error(`missing button title: ${title}`);
  act(() => button.dispatchEvent(new MouseEvent('click', { bubbles: true })));
}

function changeValue(input: HTMLInputElement | HTMLSelectElement, value: string) {
  act(() => {
    input.value = value;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });
}
