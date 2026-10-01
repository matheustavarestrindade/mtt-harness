import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Runes are selected by each component's $state/$props usage. Do not force runes
// onto dependencies that still expose Svelte-compatible legacy components.
export default { preprocess: vitePreprocess() };
