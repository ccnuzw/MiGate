import { describe, expect, it } from 'vitest';
import { ApiError } from '../api/client';
import { certificateStatusLabel, certIssuePayload, certSettingsPayload, formatUpdateLogs, isUpdateInProgress, isUpdateTerminal, parseDomains, preflightFromAPIError, settingsPayload, toggleID, updateDependencyRefetchInterval, updateStatusRefetchInterval, updateStatusSummaryKey } from './SettingsPage';

describe('settings helpers', () => {
  it('sends an empty password to preserve the existing backend password', () => {
    expect(settingsPayload({ panel_username: 'admin', panel_password: undefined }, { panel_port: 9999, panel_password: '' })).toMatchObject({
      panel_username: 'admin',
      panel_port: 9999,
      panel_password: '',
    });
  });

  it('uses current form domain and email when issuing certificates', () => {
    expect(certIssuePayload({ cert_domain: 'new.example.com', cert_email: 'ops@example.com' }, { domain: 'old.example.com', email: 'old@example.com' })).toEqual({
      domain: 'new.example.com',
      email: 'ops@example.com',
    });
  });

  it('saves only certificate fields from the certificate card', () => {
    expect(certSettingsPayload(
      { panel_port: 9999, web_base_path: '/panel', cert_domain: 'old.example.com', cert_email: 'old@example.com' },
      { panel_port: 7777, web_base_path: '/draft', cert_domain: 'new.example.com', cert_email: 'ops@example.com', panel_password: 'draft' },
    )).toMatchObject({
      panel_port: 9999,
      web_base_path: '/panel',
      cert_domain: 'new.example.com',
      cert_email: 'ops@example.com',
      panel_password: '',
    });
  });

  it('polls update status only while an update is active', () => {
    expect(isUpdateInProgress('updating')).toBe(true);
    expect(isUpdateInProgress('installing')).toBe(true);
    expect(isUpdateInProgress('restarting')).toBe(true);
    expect(isUpdateInProgress('idle')).toBe(false);
    expect(updateStatusRefetchInterval('updating')).toBe(5000);
    expect(updateStatusRefetchInterval('idle', true)).toBe(5000);
    expect(updateDependencyRefetchInterval(true)).toBe(5000);
    expect(updateDependencyRefetchInterval(false)).toBe(false);
    expect(updateStatusRefetchInterval('completed')).toBe(false);
    expect(updateStatusRefetchInterval(undefined)).toBe(false);
    expect(isUpdateTerminal('failed')).toBe(true);
    expect(isUpdateTerminal('restarting')).toBe(false);
    expect(isUpdateTerminal('updating')).toBe(false);
  });

  it('summarizes update completion and rollback states', () => {
    expect(updateStatusSummaryKey({ status: 'completed' })).toBe('升级成功，服务已可用');
    expect(updateStatusSummaryKey({ status: 'failed', rolled_back: true, rollback_status: 'restored' })).toBe('升级失败，已回滚，服务已恢复');
    expect(updateStatusSummaryKey({ status: 'failed', rolled_back: true, rollback_status: 'failed' })).toBe('');
  });

  it('formats update logs from API responses', () => {
    expect(formatUpdateLogs(undefined, 'empty')).toBe('empty');
    expect(formatUpdateLogs({ lines: ['a', 'b'] }, 'empty')).toBe('a\nb');
    expect(formatUpdateLogs({ logs: 'raw log' }, 'empty')).toBe('raw log');
  });

  it('normalizes certificate domains and statuses', () => {
    expect(parseDomains('Example.com, www.example.com  example.com')).toEqual(['example.com', 'www.example.com']);
    expect(certificateStatusLabel('expiring_soon')).toBe('即将到期');
    expect(toggleID([1, 2], 2)).toEqual([1]);
    expect(toggleID([1], 2)).toEqual([1, 2]);
  });

  it('extracts preflight checks from standard API error fields', () => {
    const preflight = { ok: false, checks: [{ code: 'http_01_port_unavailable', status: 'failed', detail: 'bind failed' }] };
    const error = new ApiError(409, 'preflight_failed', { error: { code: 'preflight_failed', fields: { preflight } } }, { code: 'preflight_failed', fields: { preflight } });
    expect(preflightFromAPIError(error)).toEqual(preflight);
    expect(preflightFromAPIError(new Error('legacy'))).toBeNull();
  });
});
