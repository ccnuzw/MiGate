import { describe, expect, it } from 'vitest';
import { coreActionResult, coreStatusMetrics, coreStatusRefetchInterval, isCoreInstalled } from './CorePage';

describe('core action result', () => {
  it('treats xray apply validation failures as business errors', () => {
    expect(coreActionResult({ status: 'failed', error: 'validation' }, 'Xray 配置已应用')).toMatchObject({
      ok: false,
      message: 'validation',
    });
    expect(coreActionResult({ status: 'failed', error: 'validation' }, 'Xray 配置已应用')).not.toHaveProperty('detail');
    expect(coreActionResult({ xray: { status: 'failed: validation' } }, 'Xray 配置已应用')).toEqual({
      ok: false,
      message: 'failed: validation',
    });
  });

  it('detects nested xray and sing-box failures from HTTP 200 bodies', () => {
    expect(coreActionResult({ xray: { status: 'not_managed', error_output: 'service is not managed' } }, 'Xray 配置已应用')).toMatchObject({
      ok: false,
      message: 'service is not managed',
    });
    expect(coreActionResult({ singbox: { applied: false, reason: 'invalid config' } }, 'sing-box 配置已应用')).toMatchObject({
      ok: false,
      message: 'invalid config',
    });
  });

  it('keeps command and error details for failed operations', () => {
    expect(coreActionResult({ xray: { status: 'failed', error_output: 'invalid json', commands_executed: ['xray -test'] } }, 'Xray 配置已应用')).toEqual({
      ok: false,
      message: 'invalid json',
      detail: 'commands:\nxray -test',
    });
  });

  it('keeps command details for successful applied operations', () => {
    expect(coreActionResult({ applied: true, commands_executed: ['sing-box check'], output: 'ok' }, 'sing-box 配置已应用')).toEqual({
      ok: true,
      message: 'sing-box 配置已应用',
      detail: 'commands:\nsing-box check\n\ndetail:\nok',
    });
    expect(coreActionResult({ singbox: { applied: true, commands_executed: ['sing-box check -c /etc/sing-box/config.json'], output: 'ok' } }, 'sing-box 配置已应用')).toEqual({
      ok: true,
      message: 'sing-box 配置已应用',
      detail: 'commands:\nsing-box check -c /etc/sing-box/config.json\n\ndetail:\nok',
    });
    expect(coreActionResult({ commands_executed: [], singbox: { applied: true, commands_executed: ['sing-box reload'], output: 'ok' } }, 'sing-box 配置已应用')).toEqual({
      ok: true,
      message: 'sing-box 配置已应用',
      detail: 'commands:\nsing-box reload\n\ndetail:\nok',
    });
  });

  it('pauses core status polling while the page is hidden', () => {
    expect(coreStatusRefetchInterval(true)).toBe(12000);
    expect(coreStatusRefetchInterval(false)).toBe(false);
  });

  it('derives installed state from explicit status first, then version/status fallbacks', () => {
    expect(isCoreInstalled({ installed: false, version: 'Xray 25.6.8', status: 'running' })).toBe(false);
    expect(isCoreInstalled({ version: 'sing-box version 1.13.13' })).toBe(true);
    expect(isCoreInstalled({ version: 'not_installed', status: 'not_installed' })).toBe(false);
    expect(isCoreInstalled(undefined)).toBe(false);
  });

  it('formats sing-box managed status and config path for core metrics', () => {
    const metrics = coreStatusMetrics({
      service: 'sing-box',
      installed: true,
      managed: true,
      status: 'running',
      version: 'sing-box version 1.13.13',
      config_path: '/etc/sing-box/config.json',
      active_connections: 0,
    });
    expect(metrics).toEqual(expect.arrayContaining([
      { label: '托管', value: '已托管' },
      { label: '配置路径', value: '/etc/sing-box/config.json' },
    ]));
  });
});
