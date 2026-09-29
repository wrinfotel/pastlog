// Timeline navigation state at module scope: views remount on navigation,
// module state does not, so the timeline scope survives the round trip
// through the session viewer (same bargain as projects.svelte.ts).

import type { ViewName } from './stores.svelte';

export const timelineNav = $state({ agent: '', project: '', from: 'home' as ViewName });

// openTimeline enters the merged message stream, globally (agent/project "")
// or scoped to one project; from records the entry point — the back button
// returns there. Callers navigate with go('timeline') themselves.
export function openTimeline(agent: string, project: string, from: ViewName) {
  timelineNav.agent = agent;
  timelineNav.project = project;
  timelineNav.from = from;
}
