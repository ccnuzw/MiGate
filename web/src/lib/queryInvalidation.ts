import type { QueryClient } from '@tanstack/react-query';

export function refreshTopologyDependencies(queryClient: QueryClient) {
  queryClient.invalidateQueries({ queryKey: ['inbounds'] });
  queryClient.invalidateQueries({ queryKey: ['inbounds', 'traffic'] });
  queryClient.invalidateQueries({ queryKey: ['outbounds'] });
  queryClient.invalidateQueries({ queryKey: ['routing-rules'] });
  queryClient.invalidateQueries({ queryKey: ['dashboard-summary'] });
  queryClient.invalidateQueries({ queryKey: ['traffic-summary'] });
  queryClient.invalidateQueries({ queryKey: ['traffic-inbounds'] });
  queryClient.invalidateQueries({ queryKey: ['traffic-clients'] });
  queryClient.invalidateQueries({ queryKey: ['traffic-series'] });
}

export function refetchTopologyDependencies(queryClient: QueryClient) {
  return Promise.all([
    queryClient.refetchQueries({ queryKey: ['inbounds'] }),
    queryClient.refetchQueries({ queryKey: ['outbounds'] }),
    queryClient.refetchQueries({ queryKey: ['routing-rules'] }),
  ]);
}
