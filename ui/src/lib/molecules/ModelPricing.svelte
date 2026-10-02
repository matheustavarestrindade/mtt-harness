<script lang="ts">
  import type { Model } from '../atoms/types';
  let { model }: { model: Model } = $props();
  function rate(value: number, currency: string): string {
    try {
      return new Intl.NumberFormat(undefined, {
        style: 'currency',
        currency,
        minimumFractionDigits: 0,
        maximumFractionDigits: 8,
      }).format(value);
    } catch {
      return `${currency} ${value}`;
    }
  }
  const prices = $derived(model.Prices);
  const source = $derived.by(() => {
    if (!prices?.Source) return '';
    try {
      const url = new URL(prices.Source);
      return ['https:', 'http:'].includes(url.protocol) && !url.username && !url.password
        ? url.href
        : '';
    } catch {
      return '';
    }
  });
</script>

<section aria-label="Model pricing" class="min-w-0 space-y-2 text-xs text-muted-foreground">
  {#if model.Billing === 'subscription'}
    <p>Subscription access · no per-token charge estimate. Plan limits still apply.</p>
  {:else if prices}
    <p class="font-medium">Rates per 1 million tokens · {prices.Currency}</p>
    <dl class="grid grid-cols-2 gap-x-4 gap-y-1.5">
      <dt>Input</dt>
      <dd class="text-right font-mono">{rate(prices.Input, prices.Currency)}</dd>
      <dt>Output</dt>
      <dd class="text-right font-mono">{rate(prices.Output, prices.Currency)}</dd>
      <dt>Cache read</dt>
      <dd class="text-right font-mono">
        {prices.CacheReadUnknown ? 'Unavailable' : rate(prices.CacheRead, prices.Currency)}
      </dd>
      <dt>Cache write</dt>
      <dd class="text-right font-mono">
        {prices.CacheWriteUnknown ? 'Unavailable' : rate(prices.CacheWrite, prices.Currency)}
      </dd>
      {#if prices.Reasoning !== undefined && prices.Reasoning !== prices.Output}
        <dt>Reasoning output</dt>
        <dd class="text-right font-mono">{rate(prices.Reasoning, prices.Currency)}</dd>
      {/if}
    </dl>
    {#each prices.Tiers ?? [] as tier}
      <p class="leading-5">
        Above {tier.AboveInputTokens.toLocaleString()} input tokens: {rate(
          tier.Input,
          prices.Currency,
        )} input / {rate(tier.Output, prices.Currency)} output per million.
      </p>
    {/each}
    {#if source}<a
        class="inline-block min-h-11 content-center underline underline-offset-4"
        href={source}
        target="_blank"
        rel="noopener noreferrer">Price source</a
      >{/if}
    <p class="leading-5">
      Usage costs are estimates from these rates. Provider discounts and time-based pricing can
      change the billed amount.
    </p>
  {:else}<p>Pricing unavailable. Token usage is still recorded.</p>{/if}
</section>
