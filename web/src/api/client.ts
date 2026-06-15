export class ApiError extends Error {
  status: number;
  payload: unknown;

  constructor(status: number, message: string, payload: unknown) {
    super(message);
    this.status = status;
    this.payload = payload;
  }
}

export function basePath(): string {
  if (window.__MIGATE_BASE_PATH__) return window.__MIGATE_BASE_PATH__;
  const path = window.location.pathname;
  const known = ['/login', '/assets/', '/api/', '/sub/'];
  for (const marker of known) {
    const index = path.indexOf(marker);
    if (index > 0) return path.slice(0, index);
  }
  if (path === '/' || path === '/login') return '';
  if (path === '/panel') return '/panel';
  if (path.endsWith('/login')) return path.slice(0, -'/login'.length);
  const spaRoutes = ['/inbounds', '/outbounds', '/routing', '/topology', '/xray', '/singbox', '/settings'];
  for (const route of spaRoutes) {
    if (path === route) return '';
    if (path.endsWith(route)) return path.slice(0, -route.length);
    const nestedIndex = path.indexOf(`${route}/`);
    if (nestedIndex > 0) return path.slice(0, nestedIndex);
  }
  const firstSegment = path.match(/^\/[^/]+/)?.[0] || '';
  return firstSegment === path ? '' : firstSegment;
}

export function appPath(path: string): string {
  const base = basePath();
  const clean = path.startsWith('/') ? path : `/${path}`;
  return `${base}${clean}` || '/';
}

async function parseResponse(response: Response) {
  const contentType = response.headers.get('content-type') || '';
  if (contentType.includes('application/json')) return response.json();
  return response.text();
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const hasBody = init.body !== undefined && init.body !== null;
  if (hasBody && !headers.has('Content-Type') && !(init.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }
  headers.set('Accept', 'application/json');
  const response = await fetch(appPath(path), {
    credentials: 'same-origin',
    ...init,
    headers,
  });
  const payload = await parseResponse(response).catch(() => null);
  if (response.status === 401 && shouldRedirectOnUnauthorized(path)) {
    const current = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    window.location.assign(`${appPath('/login')}?next=${encodeURIComponent(current)}`);
  }
  if (!response.ok) {
    const message =
      payload && typeof payload === 'object' && 'message' in payload
        ? String((payload as { message: unknown }).message)
        : payload && typeof payload === 'object' && 'error' in payload
          ? String((payload as { error: unknown }).error)
          : response.statusText || 'request_failed';
    throw new ApiError(response.status, message, payload);
  }
  return payload as T;
}

function shouldRedirectOnUnauthorized(path: string): boolean {
  const endpoint = normalizeAPIPath(path);
  return endpoint !== '/api/session' && endpoint !== '/api/login';
}

function normalizeAPIPath(path: string): string {
  const withoutHash = path.split('#', 1)[0];
  const withoutQuery = withoutHash.split('?', 1)[0];
  if (withoutQuery.length > 1) return withoutQuery.replace(/\/+$/, '');
  return withoutQuery || '/';
}

export function get<T>(path: string) {
  return apiFetch<T>(path);
}

export function post<T>(path: string, body?: unknown) {
  return apiFetch<T>(path, {
    method: 'POST',
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function patch<T>(path: string, body: unknown) {
  return apiFetch<T>(path, { method: 'PATCH', body: JSON.stringify(body) });
}

export function put<T>(path: string, body: unknown) {
  return apiFetch<T>(path, { method: 'PUT', body: JSON.stringify(body) });
}

export function del<T>(path: string) {
  return apiFetch<T>(path, { method: 'DELETE' });
}
