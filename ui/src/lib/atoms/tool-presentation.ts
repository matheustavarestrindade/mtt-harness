export function toolDuration(value: number): string {
  if (value === 0) return '∞';
  return value >= 1000 ? `${Number((value / 1000).toFixed(3))} s` : `${value} ms`;
}

export function toolObject(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function fileActionRange(action: Record<string, unknown>): string {
  const start = action.start_line,
    end = action.end_line;
  if (typeof start !== 'number' && typeof end !== 'number') return '';
  return `L${typeof start === 'number' ? start : 1}${end === start ? '' : `–${typeof end === 'number' ? end : 'EOF'}`}`;
}
