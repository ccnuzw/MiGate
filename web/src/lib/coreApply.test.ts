import { describe, expect, it, vi } from 'vitest';
import { coreApplyWarning, coreApplyWarningTone, showCoreApplyWarning } from './coreApply';

describe('core apply warning helpers', () => {
  it('detects xray and sing-box apply failures', () => {
    expect(coreApplyWarning({ xray: { applied: false, detail: 'xray validation failed' } }, '已保存，但核心配置未生效')).toBe('已保存，但核心配置未生效：xray validation failed');
    expect(coreApplyWarning({ singbox: { applied: false, detail: 'sing-box restart failed' } }, '已保存，但核心配置未生效')).toBe('已保存，但核心配置未生效：sing-box restart failed');
    expect(coreApplyWarningTone({ xray: { applied: false, detail: 'bad config' } })).toBe('error');
  });

  it('does not treat sing-box not_needed as a failure', () => {
    const response = { xray: { applied: true }, singbox: { applied: false, reason: 'not_needed' } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('');
    expect(coreApplyWarningTone(response)).toBe('info');
  });

  it('reports xray listener warnings as info', () => {
    const showToast = vi.fn();
    const response = { xray: { applied: true, post_apply_warnings: ['配置已应用，但端口未监听：2443/tcp'] } };
    expect(showCoreApplyWarning(response, '已保存，但核心配置未生效', showToast)).toBe(true);
    expect(showToast).toHaveBeenCalledWith('配置已应用，但端口未监听：2443/tcp', 'info');
  });

  it('reports xray semantic warnings as info without failing the save', () => {
    const response = { xray: { applied: true, warnings: ['xray_ws_path_invalid'] } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('节点已保存，Xray WS/H2 path 配置需要检查');
    expect(coreApplyWarningTone(response)).toBe('info');
  });

  it('keeps apply failures higher priority than semantic warnings', () => {
    const response = { xray: { applied: false, detail: 'invalid config', warnings: ['xray_ws_path_invalid'] } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('已保存，但核心配置未生效：invalid config');
    expect(coreApplyWarningTone(response)).toBe('error');
  });
});
