import { describe, expect, it } from 'vitest';
import { redirectTarget } from './LoginPage';

describe('login redirect helpers', () => {
  it('normalizes base-path login next targets before blocking login loops', () => {
    window.__MIGATE_BASE_PATH__ = '/panel';
    expect(redirectTarget({ search: '?next=/panel/login', state: null } as ReturnType<typeof import('react-router-dom').useLocation>)).toBe('/');
    expect(redirectTarget({ search: '?next=/panel/login/reset', state: null } as ReturnType<typeof import('react-router-dom').useLocation>)).toBe('/');
    window.__MIGATE_BASE_PATH__ = undefined;
  });

  it('does not treat login-prefixed non-login routes as the login page', () => {
    window.__MIGATE_BASE_PATH__ = '';
    expect(redirectTarget({ search: '?next=/login-help', state: null } as ReturnType<typeof import('react-router-dom').useLocation>)).toBe('/login-help');
    window.__MIGATE_BASE_PATH__ = undefined;
  });

  it('rejects next targets that become protocol-relative after base-path stripping', () => {
    window.__MIGATE_BASE_PATH__ = '/panel';
    expect(redirectTarget({ search: '?next=/panel//evil.com', state: null } as ReturnType<typeof import('react-router-dom').useLocation>)).toBe('/');
    window.__MIGATE_BASE_PATH__ = undefined;
  });
});
