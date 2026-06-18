import { describe, expect, it, vi } from 'vitest';
import { showSingboxApplyWarning, singboxApplyWarning } from './singboxApply';

describe('sing-box apply warning helpers', () => {
  it('builds warning text from top-level or nested apply failures', () => {
    expect(singboxApplyWarning({ applied: false, detail: 'invalid config' }, '已保存，但 sing-box 配置未生效')).toBe('已保存，但 sing-box 配置未生效：invalid config');
    expect(singboxApplyWarning({ singbox: { applied: false, detail: 'restart failed' } }, '已删除，但 sing-box 配置未生效')).toBe('已删除，但 sing-box 配置未生效：restart failed');
    expect(singboxApplyWarning({ singbox: { applied: true, warnings: ['配置已应用，但端口未监听：21000/udp'] } }, '已保存，但 sing-box 配置未生效')).toBe('配置已应用，但端口未监听：21000/udp');
    expect(singboxApplyWarning({ singbox: { applied: true, post_apply_warnings: ['配置已应用，但端口未监听：21000/udp'] } }, '已保存，但 sing-box 配置未生效')).toBe('配置已应用，但端口未监听：21000/udp');
    expect(singboxApplyWarning({ singbox: { applied: true } }, '已保存，但 sing-box 配置未生效')).toBe('');
  });

  it('does not interrupt users for non-fatal stats capability warnings', () => {
    expect(singboxApplyWarning({ singbox: { applied: true, warnings: ['singbox_stats_unsupported'], non_fatal_warnings: ['singbox_stats_unsupported'] } }, '已保存，但 sing-box 配置未生效')).toBe('');
    expect(singboxApplyWarning({ warnings: ['singbox_stats_capability_check_failed'] }, '已保存，但 sing-box 配置未生效')).toBe('');
  });

  it('shows toast for inbound, client, outbound, and routing write responses', () => {
    const showToast = vi.fn();
    for (const prefix of [
      '已保存，但 sing-box 配置未生效',
      '已删除，但 sing-box 配置未生效',
      '规则已保存，但 sing-box 配置未生效',
    ]) {
      expect(showSingboxApplyWarning({ singbox: { applied: false, detail: 'not applied' } }, prefix, showToast)).toBe(true);
    }
    expect(showToast).toHaveBeenCalledTimes(3);
    expect(showToast).toHaveBeenCalledWith('规则已保存，但 sing-box 配置未生效：not applied', 'error');
  });

  it('shows a non-error toast for post-apply listener warnings', () => {
    const showToast = vi.fn();
    expect(showSingboxApplyWarning({ singbox: { applied: true, post_apply_warnings: ['配置已应用，但端口未监听：21000/udp'] } }, '已保存，但 sing-box 配置未生效', showToast)).toBe(true);
    expect(showToast).toHaveBeenCalledWith('配置已应用，但端口未监听：21000/udp', 'info');
  });
});
