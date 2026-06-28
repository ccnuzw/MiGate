import { afterEach, describe, expect, it, vi } from 'vitest';
import { __resetCoreApplyJobTrackingForTests, configureCoreApplyJobTracking, coreApplyWarning, coreApplyWarningTone, showCoreApplyWarning, trackCoreApplyJobsFromResponse } from './coreApply';

afterEach(() => {
  vi.useRealTimers();
  __resetCoreApplyJobTrackingForTests();
});

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

  it('reports pending apply as an informational save result', () => {
    const response = { pending_apply: true, pending_cores: ['xray', 'sing-box'], xray: { pending_apply: true }, singbox: { pending_apply: true } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('已保存，但核心配置未生效：Xray、sing-box 有更改，需点击核心页“应用配置”后生效');
    expect(coreApplyWarningTone(response)).toBe('info');
  });

  it('does not show historical pending as a save warning when this save did not change core config', () => {
    const response = { config_changed: false, pending_apply: true, pending_cores: ['xray'], xray: { pending_apply: true, pending_reason: 'validation_failed' } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('');
    expect(coreApplyWarningTone(response)).toBe('info');
  });

  it('reports queued automatic core sync for config-changing saves', () => {
    const response = { config_changed: true, changed_cores: ['xray'], auto_apply: { xray: { status: 'queued' } } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('已保存，正在同步核心配置');
    expect(coreApplyWarningTone(response)).toBe('info');
  });

  it('tracks queued jobs and reports final success', async () => {
    vi.useFakeTimers();
    const showToast = vi.fn();
    configureCoreApplyJobTracking(vi.fn().mockResolvedValue({ id: 'job-1', core: 'xray', status: 'succeeded' }));
    const response = { config_changed: true, changed_cores: ['xray'], auto_apply: { xray: { id: 'job-1', core: 'xray', status: 'queued' } } };
    expect(showCoreApplyWarning(response, '已保存，但核心配置未生效', showToast)).toBe(true);
    expect(showToast).toHaveBeenCalledWith('已保存，正在同步核心配置', 'info');
    await vi.advanceTimersByTimeAsync(700);
    expect(showToast).toHaveBeenCalledWith('Xray 配置已同步', 'success');
  });

  it('tracks queued jobs and reports final failure', async () => {
    vi.useFakeTimers();
    const showToast = vi.fn();
    configureCoreApplyJobTracking(vi.fn().mockResolvedValue({ id: 'job-1', core: 'sing-box', status: 'failed', detail: 'check failed' }));
    trackCoreApplyJobsFromResponse({ config_changed: true, auto_apply: { singbox: { id: 'job-1', core: 'sing-box', status: 'running' } } }, showToast);
    await vi.advanceTimersByTimeAsync(700);
    expect(showToast).toHaveBeenCalledWith(
      'sing-box 自动同步失败：check failed',
      'error',
      expect.objectContaining({ label: '查看核心页', onClick: expect.any(Function) }),
    );
  });

  it('merges successful jobs for multiple cores', async () => {
    vi.useFakeTimers();
    const showToast = vi.fn();
    configureCoreApplyJobTracking(vi.fn()
      .mockResolvedValueOnce({ id: 'job-x', core: 'xray', status: 'succeeded' })
      .mockResolvedValueOnce({ id: 'job-s', core: 'sing-box', status: 'succeeded' }));
    trackCoreApplyJobsFromResponse({
      config_changed: true,
      auto_apply: {
        xray: { id: 'job-x', core: 'xray', status: 'queued' },
        singbox: { id: 'job-s', core: 'sing-box', status: 'queued' },
      },
    }, showToast);
    await vi.advanceTimersByTimeAsync(700);
    expect(showToast).toHaveBeenCalledWith('核心配置已同步', 'success');
  });

  it('does not track jobs when config did not change', async () => {
    vi.useFakeTimers();
    const showToast = vi.fn();
    const fetcher = vi.fn().mockResolvedValue({ id: 'job-1', core: 'xray', status: 'succeeded' });
    configureCoreApplyJobTracking(fetcher);
    trackCoreApplyJobsFromResponse({ config_changed: false, auto_apply: { xray: { id: 'job-1', core: 'xray', status: 'queued' } } }, showToast);
    await vi.advanceTimersByTimeAsync(1000);
    expect(fetcher).not.toHaveBeenCalled();
    expect(showToast).not.toHaveBeenCalled();
  });

  it('reports automatic core sync failures', () => {
    const response = { config_changed: true, auto_apply_error: { xray: { error: 'apply_locked', detail: 'lock busy' } } };
    expect(coreApplyWarning(response, '已保存，但核心配置未生效')).toBe('已保存，但核心配置自动同步失败：lock busy');
    expect(coreApplyWarningTone(response)).toBe('error');
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
