// Projects drill-down state at module scope: views remount on navigation,
// module state does not, so the selected agent/project survive the round
// trip through the session viewer (same bargain as viewer.svelte.ts).

export const projectsNav = $state({ agent: '', project: '' });

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
