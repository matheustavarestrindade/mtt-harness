import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

const root = fileURLToPath(new URL('../src/', import.meta.url));
const failures = [];
const primitiveDirectory = path.join(root, 'lib/atoms/ui');
const settingsPages = new Set([
  'ConnectionForm.svelte',
  'RuntimeSettingsPanel.svelte',
  'RepetitionPanel.svelte',
  'SidekickPanel.svelte',
  'WatcherPanel.svelte',
  'UsagePanel.svelte',
  'ProvidersPanel.svelte',
]);
const pluginPages = new Set([
  'SidekickPanel.svelte',
  'RepetitionPanel.svelte',
  'WatcherPanel.svelte',
]);
const sharedPatterns = new Map([
  ['Workbench.svelte', 'ConfigurationMenu'],
  ['Navigation.svelte', 'ConfigurationMenu'],
  ['Composer.svelte', 'ConfigurationMenu'],
  ['SettingsDialog.svelte', 'DialogTitleBar'],
  ['SessionSettingsDialog.svelte', 'DialogTitleBar'],
]);

async function inspect(directory) {
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const filename = path.join(directory, entry.name);
    if (filename === primitiveDirectory) continue;
    if (entry.isDirectory()) {
      await inspect(filename);
      continue;
    }
    if (!entry.isFile() || !entry.name.endsWith('.svelte')) continue;
    const source = await readFile(filename, 'utf8');
    const required = pluginPages.has(entry.name)
      ? 'PluginHeaderControls'
      : sharedPatterns.get(entry.name);
    if (required && !source.includes(`<${required}`))
      failures.push(`${path.relative(root, filename)}: use the shared ${required} component`);
    if (settingsPages.has(entry.name) && source.includes('· Workspace settings'))
      failures.push(
        `${path.relative(root, filename)}: workspace scope belongs to Configuration navigation, not repeated page labels`,
      );
    if (
      settingsPages.has(entry.name) &&
      (/<h2\b/.test(source) || !source.includes('<SettingsPageTitle'))
    )
      failures.push(
        `${path.relative(root, filename)}: the page title belongs to SettingsPageTitle`,
      );
    for (const match of source.matchAll(/<(button|input|select|textarea|details|summary)\b/g)) {
      const line = source.slice(0, match.index).split('\n').length;
      failures.push(
        `${path.relative(root, filename)}:${line}: use the shared UI primitive instead of <${match[1]}>`,
      );
    }
  }
}

await inspect(root);
if (failures.length) {
  console.error(failures.join('\n'));
  process.exitCode = 1;
} else console.log('UI controls: native controls stay inside the shared primitives.');
