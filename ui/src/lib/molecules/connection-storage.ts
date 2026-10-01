export interface ConnectionPreferences {
  base: string;
  token: string;
}
const key = 'mtt-console-connection';
export function loadConnection(): ConnectionPreferences {
  try {
    const value: unknown = JSON.parse(sessionStorage.getItem(key) || 'null');
    if (
      value &&
      typeof value === 'object' &&
      'base' in value &&
      'token' in value &&
      typeof value.base === 'string' &&
      typeof value.token === 'string'
    )
      return { base: value.base, token: value.token };
  } catch {
    /* Storage may be disabled; the connection form still works. */
  }
  return { base: '/api', token: '' };
}
export function saveConnection(value: ConnectionPreferences) {
  try {
    sessionStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* Tab storage is optional. */
  }
}
export function forgetConnection() {
  try {
    sessionStorage.removeItem(key);
  } catch {
    /* Tab storage is optional. */
  }
}
