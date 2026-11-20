<script>
  // The four-digit pairing check, as tiles (lib/pairing.js).
  import { checkDigits } from '../../lib/pairing.js';
  import { t } from '../../lib/i18n.js';

  export let code = '';
  /** 'sm' in a row, 'md' on its own. */
  export let size = 'sm';
  $: digits = checkDigits(code);
</script>

{#if digits.length}
  <span class="digits {size}" role="img" aria-label={$t('pairing.checkAria', { code: digits.join(' ') })}>
    {#each digits as d}<span>{d}</span>{/each}
  </span>
{/if}

<style>
  .digits {
    display: inline-flex;
    gap: 4px;
    flex-shrink: 0;
  }
  .digits span {
    display: grid;
    place-items: center;
    border-radius: 7px;
    background: var(--bg-active);
    border: 1px solid rgba(var(--accent-rgb), 0.45);
    color: var(--text);
    font-family: 'Cascadia Mono', Consolas, monospace;
    font-weight: 700;
    font-variant-numeric: tabular-nums;
  }
  .sm span {
    width: 24px;
    height: 30px;
    font-size: var(--text-md);
  }
  .md span {
    width: 34px;
    height: 42px;
    font-size: var(--text-xl);
    border-radius: 9px;
  }
</style>
