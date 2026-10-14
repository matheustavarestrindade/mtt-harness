export interface DataNode {
  key: string;
  kind: 'object' | 'array' | 'text' | 'number' | 'boolean' | 'empty' | 'omitted';
  text: string;
  children: DataNode[];
  omitted: number;
}

export function dataLabel(key: string): string {
  return key
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/_/g, ' ')
    .replace(/^./, (letter) => letter.toUpperCase());
}

// Rendering limits bound the DOM independently from the size of retained data.
// Copy/download use the original value, including any omitted entries.
export function structuredData(value: unknown, entryLimit = 30): DataNode {
  let remaining = 400;
  function visit(value: unknown, key: string, depth: number): DataNode {
    const node: DataNode = { key, kind: 'empty', text: 'Not set', children: [], omitted: 0 };
    if (remaining-- <= 0 || depth > 8)
      return { ...node, kind: 'omitted', text: 'Nested data omitted' };
    if (value === null || value === undefined) return node;
    if (typeof value === 'string')
      return {
        ...node,
        kind: 'text',
        text: value.length > 32_000 ? value.slice(0, 32_000) : value,
        omitted: Math.max(0, value.length - 32_000),
      };
    if (typeof value === 'number') return { ...node, kind: 'number', text: String(value) };
    if (typeof value === 'boolean') return { ...node, kind: 'boolean', text: value ? 'Yes' : 'No' };
    const limit = Math.min(entryLimit, Math.max(0, remaining));
    const shown: [string, unknown][] = [];
    let total = 0;
    if (Array.isArray(value)) {
      total = value.length;
      for (let index = 0; index < Math.min(total, limit); index++)
        shown.push([String(index + 1), value[index]]);
    } else {
      for (const name in value) {
        if (!Object.hasOwn(value, name)) continue;
        total++;
        if (shown.length < limit) shown.push([name, (value as Record<string, unknown>)[name]]);
      }
    }
    return {
      ...node,
      kind: Array.isArray(value) ? 'array' : 'object',
      text: total ? '' : Array.isArray(value) ? 'No items' : 'No fields',
      children: shown.map(([name, item]) => visit(item, name, depth + 1)),
      omitted: total - shown.length,
    };
  }
  return visit(value, '', 0);
}

export function structuredJSON(text: string): { value: unknown } | null {
  if (text.length > 1_000_000 || !/^[\s]*[\[{]/.test(text)) return null;
  try {
    const value: unknown = JSON.parse(text);
    return value && typeof value === 'object' ? { value } : null;
  } catch {
    return null;
  }
}

export function tableColumns(node: DataNode): string[] {
  if (
    node.kind !== 'array' ||
    !node.children.length ||
    node.children.some((child) => child.kind !== 'object')
  )
    return [];
  const columns = [
    ...new Set(node.children.flatMap((child) => child.children.map((field) => field.key))),
  ];
  return columns.length <= 8 &&
    node.children.every((child) =>
      child.children.every((field) => !['object', 'array'].includes(field.kind)),
    )
    ? columns
    : [];
}
