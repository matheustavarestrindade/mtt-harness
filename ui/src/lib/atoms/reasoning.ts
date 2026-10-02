import type { Model } from './types';

export function reasoningEffortLabel(effort: string): string {
  if (!effort) return 'Default';
  if (effort === 'none') return 'Off';
  if (effort === 'xhigh') return 'Extra high';
  return effort[0].toUpperCase() + effort.slice(1);
}

export function findSessionModel(models: Model[], identifier: string): Model | undefined {
  const exact = models.find((model) => model.ID === identifier);
  if (exact || !identifier || identifier.includes('/')) return exact;
  const matches = models.filter((model) => model.ID.endsWith(`/${identifier}`));
  return matches.length === 1 ? matches[0] : undefined;
}
