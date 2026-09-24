// Projects drill-down state at module scope: views remount on navigation,
// module state does not, so the selected agent/project survive the round
// trip through the session viewer (same bargain as viewer.svelte.ts).

export const projectsNav = $state({ agent: '', project: '', from: 'stats' as 'stats' | 'home' });

// openAgentProjects enters the page for one agent (Home card or Stats row
// click): there is no section root, the page is always one agent's projects.
// from records the entry point — the list's back button returns there.
export function openAgentProjects(agent: string, from: 'stats' | 'home') {
  projectsNav.agent = agent;
  projectsNav.project = '';
  projectsNav.from = from;
}

export function openProject(project: string) {
  projectsNav.project = project;
}

export function closeProject() {
  projectsNav.project = '';
}
