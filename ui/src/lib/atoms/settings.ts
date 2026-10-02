export type SettingsSection = 'general' | 'usage' | 'providers' | 'connection';
export type RuntimeSettingKey = 'agent_depth_limit' | 'process_limit';
export type RuntimeSettings = Partial<Record<RuntimeSettingKey, string>>;

export const runtimeSettingFields: {
  key: RuntimeSettingKey;
  label: string;
  description: string;
}[] = [
  {
    key: 'agent_depth_limit',
    label: 'Agent depth limit',
    description: 'Maximum child-agent depth. Zero disables child agents.',
  },
  {
    key: 'process_limit',
    label: 'Process limit',
    description: 'Maximum concurrent processes per workspace. Zero prevents new processes.',
  },
];
