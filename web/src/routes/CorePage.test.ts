import { describe, expect, it } from 'vitest';
import { coreActionResult } from './CorePage';

describe('core action result', () => {
  it('treats xray apply validation failures as business errors', () => {
    expect(coreActionResult({ status: 'failed', error: 'validation' }, 'Xray 配置已应用')).toEqual({
      ok: false,
      message: 'validation',
    });
    expect(coreActionResult({ xray: { status: 'failed: validation' } }, 'Xray 配置已应用')).toEqual({
      ok: false,
      message: 'failed: validation',
    });
  });

  it('detects nested xray and sing-box failures from HTTP 200 bodies', () => {
    expect(coreActionResult({ xray: { status: 'not_managed', error_output: 'service is not managed' } }, 'Xray 配置已应用')).toEqual({
      ok: false,
      message: 'service is not managed',
    });
    expect(coreActionResult({ singbox: { applied: false, reason: 'invalid config' } }, 'sing-box 配置已应用')).toEqual({
      ok: false,
      message: 'invalid config',
    });
  });
});
