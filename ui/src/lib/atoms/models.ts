import type { Model } from './types';
import { modelLabel } from './format';

export interface ModelOption {
  value: string;
  label: string;
  provider: string;
  providerLabel: string;
  disabled?: boolean;
}

export interface ModelGroup {
  provider: string;
  label: string;
  options: ModelOption[];
}

const providerLabels = new Map([
  ['openai', 'OpenAI'],
  ['openai-codex', 'OpenAI coding plan'],
  ['deepseek', 'DeepSeek'],
  ['test', 'Test provider'],
]);
const modelOrder = new Intl.Collator('en', { sensitivity: 'base', numeric: true });

export function modelOption(model: Pick<Model, 'ID' | 'Name'>): ModelOption {
  const separator = model.ID.indexOf('/');
  const provider = separator < 0 ? '' : model.ID.slice(0, separator);
  return {
    value: model.ID,
    label: modelLabel(model),
    provider,
    providerLabel: providerLabels.get(provider) || provider || 'Other models',
  };
}

// Search accepts names, provider names and hidden IDs without putting API
// identifiers into the option text. Ignore separators in names such as GPT-6.
function searchText(value: string): string {
  return value
    .normalize('NFKD')
    .toLocaleLowerCase('en')
    .replace(/[^\p{L}\p{N}]/gu, '');
}

export function modelOptionMatches(option: ModelOption, query: string): boolean {
  const searchable = searchText(`${option.label} ${option.providerLabel} ${option.value}`);
  return query
    .trim()
    .split(/\s+/)
    .every((word) => searchable.includes(searchText(word)));
}

export function groupModelOptions(options: ModelOption[], query = ''): ModelGroup[] {
  const providers = new Map<string, ModelGroup>();
  for (const option of options) {
    if (!modelOptionMatches(option, query)) continue;
    let group = providers.get(option.provider);
    if (!group) {
      group = { provider: option.provider, label: option.providerLabel, options: [] };
      providers.set(option.provider, group);
    }
    group.options.push(option);
  }
  return [...providers.values()]
    .sort((first, second) => modelOrder.compare(first.label, second.label))
    .map((group) => ({
      ...group,
      options: group.options.sort(
        (first, second) =>
          modelOrder.compare(first.label, second.label) ||
          modelOrder.compare(first.value, second.value),
      ),
    }));
}
