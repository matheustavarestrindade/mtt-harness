import type { ToolActivity } from './transcript';
import { backgroundProcessReceipt } from './process-notices';

// A completion notice is only a notification summary, and can contain stderr
// alone. It must never replace a foreground result containing the actual output.
export function bashOutputKind(
  activity: ToolActivity,
): 'result' | 'started' | 'inactivity' | 'missing' {
  if (!activity.result) return 'missing';
  if (activity.call.Name !== 'bash') return 'result';
  if (backgroundProcessReceipt(activity.call, activity.result)) return 'started';
  const processID = activity.processRecord?.ID ?? activity.processNotice?.processID;
  const text = (activity.result.Content ?? [])
    .filter((part) => part.Type === 'text')
    .map((part) => part.Text)
    .join('\n');
  if (
    processID &&
    text.startsWith(`process ${processID}: state=running; no output for `) &&
    text.includes('\nstdout (last 20 lines / 4096 bytes):\n') &&
    text.includes('\nstderr (last 20 lines / 4096 bytes):\n')
  )
    return 'inactivity';
  return 'result';
}
