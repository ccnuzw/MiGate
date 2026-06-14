import { describe, expect, it, vi } from 'vitest';
import { api } from './endpoints';
import { appPath, basePath, apiFetch } from './client';

describe('api client', () => {
  it('detects base path from panel routes', () => {
    window.history.replaceState({}, '', '/panel/login');
    expect(basePath()).toBe('/panel');
    expect(appPath('/api/session')).toBe('/panel/api/session');
  });

  it('keeps the base path on nested SPA routes', () => {
    window.__MIGATE_BASE_PATH__ = undefined;
    window.history.replaceState({}, '', '/panel');
    expect(basePath()).toBe('/panel');
    expect(appPath('/api/session')).toBe('/panel/api/session');

    window.history.replaceState({}, '', '/panel/inbounds');
    expect(basePath()).toBe('/panel');
    expect(appPath('/api/inbounds')).toBe('/panel/api/inbounds');
    expect(appPath('/sub/client-token')).toBe('/panel/sub/client-token');

    window.history.replaceState({}, '', '/panel/settings');
    expect(basePath()).toBe('/panel');
    expect(appPath('/login')).toBe('/panel/login');

    window.history.replaceState({}, '', '/foo/panel/inbounds');
    expect(basePath()).toBe('/foo/panel');
    expect(appPath('/api/inbounds')).toBe('/foo/panel/api/inbounds');
  });

  it('uses the backend-injected base path when present', () => {
    window.__MIGATE_BASE_PATH__ = '/panel';
    window.history.replaceState({}, '', '/panel/routing');
    expect(basePath()).toBe('/panel');
    expect(appPath('/api/routing-rules')).toBe('/panel/api/routing-rules');
    window.__MIGATE_BASE_PATH__ = undefined;
  });

  it('throws ApiError for failed JSON responses', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ error: 'bad' }), { status: 400, headers: { 'content-type': 'application/json' } })),
    );
    await expect(apiFetch('/api/test')).rejects.toMatchObject({ status: 400, message: 'bad' });
    vi.unstubAllGlobals();
  });

  it('prefers backend message over error code', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(JSON.stringify({ error: 'duplicate_client', message: 'email exists' }), { status: 409, headers: { 'content-type': 'application/json' } })),
    );
    await expect(apiFetch('/api/test')).rejects.toMatchObject({ status: 409, message: 'email exists' });
    vi.unstubAllGlobals();
  });

  it('keeps outbound fields when toggling enabled state', async () => {
    window.history.replaceState({}, '', '/');
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      expect(init?.method).toBe('PUT');
      expect(JSON.parse(String(init?.body))).toMatchObject({
        id: 9,
        tag: 'proxy-socks',
        protocol: 'socks',
        address: '127.0.0.1',
        port: 1080,
        enabled: false,
      });
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'content-type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);
    await api.toggleOutbound({ id: 9, tag: 'proxy-socks', protocol: 'socks', address: '127.0.0.1', port: 1080, enabled: true }, false);
    expect(fetchMock).toHaveBeenCalledWith('/api/outbounds/9', expect.any(Object));
    vi.unstubAllGlobals();
  });

  it('sends explicit confirmation for core system actions', async () => {
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      expect(init?.method).toBe('POST');
      expect(JSON.parse(String(init?.body))).toMatchObject({ confirm: true, allow_system_changes: true });
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200, headers: { 'content-type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);
    await api.xrayApply();
    await api.xrayInstall();
    await api.xrayUninstall();
    await api.singboxApply();
    await api.singboxInstall();
    await api.singboxUninstall();
    expect(fetchMock).toHaveBeenCalledTimes(6);
    vi.unstubAllGlobals();
  });
});
