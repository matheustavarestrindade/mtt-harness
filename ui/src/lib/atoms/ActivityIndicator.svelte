<script lang="ts">
  import { cn } from './utils';

  let { active = true, class: className = '' }: { active?: boolean; class?: string } = $props();
  const phases = [0, 1, 2, 7, 8, 3, 6, 5, 4];
</script>

<span
  aria-hidden="true"
  data-slot="activity-indicator"
  data-active={active}
  class={cn('bit-loader inline-grid size-4 shrink-0 grid-cols-3 gap-[2px]', className)}
>
  {#each phases as phase}
    <span class="bit" style:--bit-delay={`${phase * -0.12}s`}></span>
  {/each}
</span>

<style>
  .bit {
    min-width: 0;
    aspect-ratio: 1;
    background: currentColor;
    border-radius: 1px;
    opacity: 0.3;
  }

  .bit-loader[data-active='true'] .bit {
    animation: bit-flip 1.4s ease-in-out infinite;
    animation-delay: var(--bit-delay);
  }

  @keyframes bit-flip {
    0%,
    100% {
      opacity: 0.25;
      transform: perspective(12px) rotateY(0deg) scale(0.75);
    }
    40%,
    60% {
      opacity: 1;
      transform: perspective(12px) rotateY(180deg) scale(1);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .bit-loader[data-active='true'] .bit {
      animation: none;
    }
    .bit:nth-child(2n + 1) {
      opacity: 0.9;
    }
  }
</style>
