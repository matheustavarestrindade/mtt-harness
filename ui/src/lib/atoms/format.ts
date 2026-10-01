import type { Message, Model, Statistics } from './types';

export function modelLabel(model: Model): string {
  return model.Name ? `${model.Name} (${model.ID})` : model.ID;
}

export function workspaceName(path: string): string {
  return (
    path
      .replace(/[\\/]+$/, '')
      .split(/[\\/]/)
      .pop() || path
  );
}
export function shortID(identifier: string): string {
  return identifier.slice(0, 8);
}
export function messageText(message: Message): string {
  return (message.Content ?? [])
    .filter((content) => content.Type === 'text')
    .map((content) => content.Text)
    .join('\n');
}
export function count(value: number): string {
  return new Intl.NumberFormat(undefined, {
    notation: value >= 10_000 ? 'compact' : 'standard',
    maximumFractionDigits: 1,
  }).format(value);
}
export function time(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.valueOf())
    ? ''
    : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
export function costLabel(statistics: Statistics | null): string {
  if (!statistics) return '—';
  const costs = statistics.Costs?.length
    ? statistics.Costs
    : statistics.Cost
      ? [statistics.Cost]
      : [];
  if (!costs.length) return 'Unavailable';
  return costs
    .map((cost) => {
      try {
        return new Intl.NumberFormat(undefined, {
          style: 'currency',
          currency: cost.Currency,
          maximumFractionDigits: 4,
        }).format(cost.Value);
      } catch {
        return `${cost.Value.toFixed(4)} ${cost.Currency}`;
      }
    })
    .join(' + ');
}
