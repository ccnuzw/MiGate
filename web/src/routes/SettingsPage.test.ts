import { describe, expect, it } from 'vitest';
import { certIssuePayload, settingsPayload } from './SettingsPage';

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
});
