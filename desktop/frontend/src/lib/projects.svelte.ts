// Projects drill-down state at module scope: views remount on navigation,
// module state does not, so the selected agent/project survive the round
// trip through the session viewer (same bargain as viewer.svelte.ts).

export const projectsNav = $state({ agent: '', project: '' });

// openProjects is the sidebar/section entry: it always lands on the list.
// A previously opened project detail is kept only for the viewer's back trip,
// which navigates without going through here.
export function openProjects() {
  projectsNav.project = '';
}

// openAgentProjects enters the list view for one agent (Home card click).
export function openAgentProjects(agent: string) {
  projectsNav.agent = agent;
  projectsNav.project = '';
}

export function openProject(project: string) {
  projectsNav.project = project;
}

export function closeProject() {
  projectsNav.project = '';
}
