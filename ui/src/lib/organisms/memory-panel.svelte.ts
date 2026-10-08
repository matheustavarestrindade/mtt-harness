import type { MemoryConfiguration } from '../atoms/memory';
import { WorkspacePluginPanelState } from './plugin-panel.svelte';

export class MemoryPanelState extends WorkspacePluginPanelState<MemoryConfiguration> {
  constructor() {
    super('context', 'Memory');
  }
}
