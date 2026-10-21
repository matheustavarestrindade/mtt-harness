<script lang="ts">
  import { Cable, Eye, EyeOff, LoaderCircle } from 'lucide-svelte';
  import { Button } from '../atoms/ui/button';
  import { Input } from '../atoms/ui/input';
  import { Label } from '../atoms/ui/label';
  import SettingsPageTitle from './SettingsPageTitle.svelte';
  let {
    initialBase = '/api',
    initialToken = '',
    busy = false,
    error = '',
    connected = false,
    onConnect,
    onDisconnect,
  }: {
    initialBase?: string;
    initialToken?: string;
    busy?: boolean;
    error?: string;
    connected?: boolean;
    onConnect: (base: string, token: string) => Promise<void>;
    onDisconnect: () => void;
  } = $props();
  let base = $state('/api');
  let token = $state('');
  let reveal = $state(false);
  $effect(() => {
    base = initialBase;
    token = initialToken;
    reveal = false;
  });
</script>

<section aria-label="API connection" class="min-w-0 w-full space-y-5">
  <SettingsPageTitle
    icon={Cable}
    title="Connection"
    description="Use your harness API token. This console connects through the UI-side proxy."
  />
  <form
    class="space-y-5"
    onsubmit={(event) => {
      event.preventDefault();
      void onConnect(base, token);
    }}
  >
    <div class="space-y-2">
      <Label for="api-base">API address</Label><Input
        id="api-base"
        bind:value={base}
        class="h-11 font-mono text-sm"
        placeholder="/api"
        required
      />
      <p class="text-xs leading-5 text-muted-foreground">
        Use <code class="text-foreground">/api</code> locally. Set the proxy target in
        <code>ui/.env</code>.
      </p>
    </div>
    <div class="space-y-2">
      <Label for="api-token">API token</Label>
      <div class="relative">
        <Input
          id="api-token"
          bind:value={token}
          type={reveal ? 'text' : 'password'}
          class="h-11 pr-12 font-mono text-sm"
          autocomplete="off"
          spellcheck={false}
          placeholder="Paste the token from the startup log"
        /><Button
          type="button"
          variant="ghost"
          size="icon"
          class="icon-button absolute top-0 right-0"
          aria-label={reveal ? 'Hide token' : 'Show token'}
          onclick={() => (reveal = !reveal)}
          >{#if reveal}<EyeOff class="size-4" />{:else}<Eye class="size-4" />{/if}</Button
        >
      </div>
      <p class="text-xs text-muted-foreground">The token stays in this browser tab.</p>
    </div>
    {#if error}<p
        role="alert"
        class="rounded-lg border border-destructive/25 bg-destructive/10 p-3 text-sm leading-6 text-destructive"
      >
        {error}
      </p>{/if}
    <div class="flex flex-wrap justify-end gap-2">
      {#if connected}<Button variant="outline" class="h-11" type="button" onclick={onDisconnect}
          >Disconnect</Button
        >{/if}<Button type="submit" class="h-11" disabled={busy || !base.trim()}
        >{#if busy}<LoaderCircle class="size-4 animate-spin" />{:else}<Cable
            class="size-4"
          />{/if}{busy ? 'Connecting…' : 'Connect to API'}</Button
      >
    </div>
  </form>
</section>
