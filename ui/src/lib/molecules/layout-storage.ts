export interface LayoutPreferences {
  navigationCollapsed: boolean;
  memoryCollapsed: boolean;
}

const key = 'mtt-console-layout';
export function loadLayout(): LayoutPreferences {
  try {
    const value = JSON.parse(localStorage.getItem(key) || 'null');
    return {
      navigationCollapsed: value?.navigationCollapsed === true,
      memoryCollapsed: value?.memoryCollapsed === true,
    };
  } catch {
    return { navigationCollapsed: false, memoryCollapsed: false };
  }
}

export function saveLayout(value: LayoutPreferences) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Layout preferences are optional; no connection credentials are stored here.
  }
}
