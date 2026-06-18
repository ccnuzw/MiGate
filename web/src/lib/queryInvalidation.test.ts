import { describe, expect, it, vi } from 'vitest';
import { refreshTopologyDependencies } from './queryInvalidation';

describe('query invalidation helpers', () => {
  it('refreshes every topology dependency after topology-affecting writes', () => {
    const queryClient = { invalidateQueries: vi.fn() };

    refreshTopologyDependencies(queryClient as never);

    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['inbounds'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['inbounds', 'traffic'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['outbounds'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['routing-rules'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['dashboard-summary'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['traffic-summary'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['traffic-inbounds'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['traffic-clients'] });
    expect(queryClient.invalidateQueries).toHaveBeenCalledWith({ queryKey: ['traffic-series'] });
  });
});
