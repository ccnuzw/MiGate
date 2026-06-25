import { describe, expect, it } from 'vitest';
import { formatBytes, serviceLabel, versionLabel } from './format';

describe('display format helpers', () => {
  it('formats sub-byte chart ticks without leaking missing units', () => {
    expect(formatBytes(0.2)).toBe('0 B');
    expect(formatBytes(819.2)).toBe('819 B');
    expect(formatBytes(1024)).toBe('1.0 KB');
  });

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

  it('keeps only the core name and version for verbose core version output', () => {
    expect(versionLabel('Xray 25.6.8 (Xray, Penetrates Everything.) Custom (go1.24 linux/amd64)')).toBe('Xray 25.6.8');
    expect(versionLabel('sing-box version 1.13.13\nEnvironment: go1.25 linux/amd64\nTags: with_quic')).toBe('sing-box 1.13.13');
  });
});
