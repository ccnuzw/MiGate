import { del, get, post, put } from './client';
import type { RoutingRule, SingboxWriteResponse } from './types';

export type RoutingRuleResponse = (RoutingRule | { rule: RoutingRule }) & SingboxWriteResponse;

function unwrapRoutingRule(response: RoutingRuleResponse): RoutingRule & SingboxWriteResponse {
  if ('rule' in response && response.rule) {
    return {
      ...(response.rule as RoutingRule),
      applied: response.applied,
      error: response.error,
      detail: response.detail,
      warnings: response.warnings,
      post_apply_warnings: response.post_apply_warnings,
      non_fatal_warnings: response.non_fatal_warnings,
      singbox: response.singbox,
      xray: response.xray,
    };
  }
  return response as RoutingRule & SingboxWriteResponse;
}

export const routingAPI = {
  routingRules: () => get<RoutingRule[]>('/api/routing-rules'),
  createRoutingRule: async (body: Record<string, unknown>) => unwrapRoutingRule(await post<RoutingRuleResponse>('/api/routing-rules', body)),
  updateRoutingRule: async (id: number, body: Record<string, unknown>) => unwrapRoutingRule(await put<RoutingRuleResponse>(`/api/routing-rules/${id}`, body)),
  deleteRoutingRule: (id: number) => del<{ status: string } & SingboxWriteResponse>(`/api/routing-rules/${id}`),
  reorderRoutingRules: (ids: number[]) => post<{ status: string } & SingboxWriteResponse>('/api/routing-rules/reorder', { ids }),
};
