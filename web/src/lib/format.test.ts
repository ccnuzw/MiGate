import { describe, expect, it } from 'vitest';
import { serviceLabel, versionLabel } from './format';

describe('display format helpers', () => {
  it('localizes service status enum values', () => {
    expect(serviceLabel('running')).toBe('运行中');
    expect(serviceLabel('not_managed')).toBe('未托管');
    expect(serviceLabel('not_installed')).toBe('未安装');
    expect(serviceLabel('unknown')).toBe('未知');
    expect(serviceLabel(undefined)).toBe('未知');
  });

  it('localizes version sentinel values', () => {
    expect(versionLabel('not_installed')).toBe('未安装');
    expect(versionLabel('unknown')).toBe('未知');
    expect(versionLabel('Xray 25.6.8')).toBe('Xray 25.6.8');
    expect(versionLabel(undefined)).toBe('-');
  });
});
