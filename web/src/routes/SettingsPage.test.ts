import { describe, expect, it } from 'vitest';
import { certIssuePayload, certSettingsPayload, isUpdateInProgress, isUpdateTerminal, settingsPayload, updateStatusRefetchInterval } from './SettingsPage';

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
    expect(isUpdateInProgress('idle')).toBe(false);
    expect(updateStatusRefetchInterval('updating')).toBe(5000);
    expect(updateStatusRefetchInterval('idle', true)).toBe(5000);
    expect(updateStatusRefetchInterval('completed')).toBe(false);
    expect(updateStatusRefetchInterval(undefined)).toBe(false);
    expect(isUpdateTerminal('failed')).toBe(true);
    expect(isUpdateTerminal('updating')).toBe(false);
  });
});
