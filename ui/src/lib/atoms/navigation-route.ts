export interface NavigationRoute {
  workspace: string;
  session: string;
  agent: string;
}
export type NavigationHistory = 'push' | 'replace' | false;
export interface WorkspaceNavigation {
  session?: string;
  agent?: string;
  history?: NavigationHistory;
}

export function parseNavigationRoute(url: URL): NavigationRoute {
  return {
    workspace: url.searchParams.get('workspace') ?? '',
    session: url.searchParams.get('session') ?? '',
    agent: url.searchParams.get('agent') ?? '',
  };
}
export function navigationURL(url: URL, route: NavigationRoute): URL {
  const result = new URL(url);
  for (const key of ['workspace', 'session', 'agent'] as const) {
    if (route[key]) result.searchParams.set(key, route[key]);
    else result.searchParams.delete(key);
  }
  return result;
}
