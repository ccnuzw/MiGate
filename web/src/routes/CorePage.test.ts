import { describe, expect, it } from 'vitest';
import { configSyncReasonLabel, configSyncState, coreActionResult, coreStatusMetrics, coreStatusRefetchInterval, diagnosticWarningLabel, isCoreInstalled, singboxDiagnosticChecks, singboxDiagnosticsSummary, singboxListeningDiagnostics } from './CorePage';

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

  it('surfaces post-apply listener warnings without marking apply as failed', () => {
    expect(coreActionResult({ singbox: { applied: true, post_apply_warnings: ['配置已应用，但端口未监听：21000/udp'] } }, 'sing-box 配置已应用')).toEqual({
      ok: true,
      message: '配置已应用，但端口未监听：21000/udp',
      tone: 'info',
    });
    expect(coreActionResult({ applied: true, post_apply_warnings: ['配置已应用，但端口未监听：21000/udp'] }, 'sing-box 配置已应用')).toEqual({
      ok: true,
      message: '配置已应用，但端口未监听：21000/udp',
      tone: 'info',
    });
    expect(coreActionResult({ applied: true, warnings: ['singbox_stats_unsupported'], singbox: { applied: true, post_apply_warnings: ['配置已应用，但端口未监听：21001/udp'] } }, 'sing-box 配置已应用')).toEqual({
      ok: true,
      message: '配置已应用，但端口未监听：21001/udp',
      tone: 'info',
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

  it('formats sing-box port listening diagnostics', () => {
    expect(singboxListeningDiagnostics({
      service: 'sing-box',
      status: 'running',
      listening_ports: [
        { inbound_id: 9, protocol: 'hysteria2', port: 21001, network: 'udp', listening: false },
      ],
    })).toEqual([
      { inboundId: 9, protocol: 'hysteria2', port: 21001, transport: 'udp', listening: false },
    ]);
  });

  it('summarizes sing-box generated and disk config sync state', () => {
    expect(configSyncState({ config_path: '/etc/sing-box/config.json', in_sync: true, disk: { config_path: '', hash: 'abc' }, generated: { config_path: '', hash: 'abc' } })).toMatchObject({
      ok: true,
      label: '磁盘配置与数据库生成配置一致',
    });
    expect(configSyncState({ config_path: '/etc/sing-box/config.json', in_sync: false, reason: 'hash_mismatch', disk: { config_path: '', hash: 'old' }, generated: { config_path: '', hash: 'new' } })).toMatchObject({
      ok: false,
      label: '磁盘配置与数据库生成配置不一致',
      detail: '原因：配置 hash 不一致 · disk: old · generated: new',
    });
    expect(configSyncReasonLabel('disk_parse_failed')).toBe('磁盘配置解析失败');
    expect(configSyncState(undefined, false, new Error('preview failed'))).toEqual({
      ok: false,
      label: '生成配置预览失败',
      detail: 'preview failed',
    });
  });

  it('summarizes sing-box diagnostics and critical checks', () => {
    const diagnostics = {
      installed: true,
      managed: true,
      service: 'sing-box',
      service_status: 'running',
      config_path: '/etc/sing-box/config.json',
      config_exists: true,
      config_valid: true,
      disk_generated_in_sync: false,
      sync_reason: 'hash_mismatch',
      expected_listeners: [{ inbound_id: 9, protocol: 'hysteria2', port: 21001, transport: 'udp', listening: false }],
      missing_listeners: [{ inbound_id: 9, protocol: 'hysteria2', port: 21001, transport: 'udp', listening: false }],
      recent_logs: ['line 1', 'line 2'],
      warnings: ['singbox_missing_listeners'],
      suggestions: ['查看日志'],
    };
    expect(singboxDiagnosticsSummary(diagnostics)).toEqual({
      tone: 'warning',
      label: '警告',
      detail: 'singbox_missing_listeners',
    });
    expect(singboxDiagnosticChecks(diagnostics)).toEqual(expect.arrayContaining([
      { label: '配置同步', value: '配置 hash 不一致', ok: false },
      { label: '端口监听', value: '缺失 1 个', ok: false },
    ]));
    expect(diagnosticWarningLabel('singbox_missing_listeners')).toBe('服务运行但入站端口未监听');
    expect(diagnosticWarningLabel('unknown_warning')).toBe('unknown_warning');
  });
});
