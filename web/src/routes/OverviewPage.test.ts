import { describe, expect, it } from 'vitest';
import { engineStatusSummary, trafficStatusLabel } from './OverviewPage';

const text = (value: string) => value;

describe('OverviewPage traffic status labels', () => {
  it('shows sing-box unsupported as an informational realtime stats limitation', () => {
    expect(trafficStatusLabel('unsupported', text)).toBe('当前 sing-box 二进制不支持实时统计');
    expect(engineStatusSummary({ xray: 'ok', singbox: 'unsupported' }, text)).toBe('xray: 统计正常 · singbox: 当前 sing-box 二进制不支持实时统计');
  });

  it('shows not_configured without treating it as a core failure label', () => {
    expect(trafficStatusLabel('not_configured', text)).toBe('未配置对应核心入站');
    expect(engineStatusSummary({ singbox: 'not_configured' }, text)).toBe('singbox: 未配置对应核心入站');
  });

  it('distinguishes waiting and unavailable labels', () => {
    expect(trafficStatusLabel('waiting', text)).toBe('等待采样');
    expect(trafficStatusLabel('unavailable', text)).toBe('统计接口不可用');
  });
});
