// Optimize navigation state at module scope: views remount on navigation,
// module state does not, so the report's scope survives the round trip
// (same bargain as timeline.svelte.ts).

import type { ViewName } from './stores.svelte';

export const optimizeNav = $state({ agent: '', project: '', from: 'home' as ViewName });

// openOptimize enters the cross-session waste report for a scope (an agent's
// project from its page, or everything from Home); from records the entry
// point — the back button returns there. Callers navigate with
// go('optimize') themselves.
export function openOptimize(agent: string, project: string, from: ViewName) {
  optimizeNav.agent = agent;
  optimizeNav.project = project;
  optimizeNav.from = from;
}
