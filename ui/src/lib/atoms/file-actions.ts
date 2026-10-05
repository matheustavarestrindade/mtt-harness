import type { ToolCall } from './types';

function fileActionInput(call: ToolCall) {
  if (call.Name !== 'file_actions' || !call.Input || typeof call.Input !== 'object') return null;
  const input = call.Input as Record<string, unknown>;
  if (!Array.isArray(input.actions)) return null;
  const operations = input.actions
    .slice(0, 32)
    .flatMap((action: unknown) =>
      action && typeof action === 'object' && 'op' in action && typeof action.op === 'string'
        ? [action.op]
        : [],
    );
  return { input, operations };
}

export function toolActivityLabel(call: ToolCall): string {
  const details = fileActionInput(call);
  if (!details?.operations.length) return call.Name;
  const path = typeof details.input.path === 'string' ? ` · ${details.input.path}` : '';
  return `${details.operations.join(' → ')}${path}`;
}

// Only a single read without extra output has a plain file body. Mutation
// summaries, mixed chains and error diagnostics must remain literal output.
export function plainFileReadPath(call: ToolCall): string | null {
  if (
    call.Name === 'read' &&
    call.Input &&
    typeof call.Input === 'object' &&
    'path' in call.Input &&
    typeof call.Input.path === 'string'
  )
    return call.Input.path;
  const details = fileActionInput(call);
  if (
    !details ||
    details.operations.length !== 1 ||
    details.operations[0] !== 'read' ||
    details.input.return ||
    details.input.on_error
  )
    return null;
  return typeof details.input.path === 'string' ? details.input.path : null;
}
