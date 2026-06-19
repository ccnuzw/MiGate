import { describe, expect, it } from 'vitest';
import { certIssuePayload, certSettingsPayload, formatUpdateLogs, isUpdateInProgress, isUpdateTerminal, settingsPayload, updateDependencyRefetchInterval, updateStatusRefetchInterval, updateStatusSummaryKey } from './SettingsPage';

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
});
