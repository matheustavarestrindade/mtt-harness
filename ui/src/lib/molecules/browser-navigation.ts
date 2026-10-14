import {
  navigationURL,
  parseNavigationRoute,
  type NavigationHistory,
  type NavigationRoute,
} from '../atoms/navigation-route';

export function readNavigationRoute(): NavigationRoute {
  return parseNavigationRoute(new URL(window.location.href));
}
export function writeNavigationRoute(route: NavigationRoute, mode: NavigationHistory = 'push') {
  if (!mode) return;
  const url = navigationURL(new URL(window.location.href), route);
  if (url.href === window.location.href) return;
  if (mode === 'replace') window.history.replaceState(window.history.state, '', url);
  else window.history.pushState(null, '', url);
}
export function observeNavigationRoute(restore: (route: NavigationRoute) => void): () => void {
  const listener = () => restore(readNavigationRoute());
  window.addEventListener('popstate', listener);
  return () => window.removeEventListener('popstate', listener);
}
