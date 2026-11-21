<script>
  // One setting, as the settings menu lays them out: what it is and a line
  // on what it does on the left, its control on the right. A control too wide
  // for that (a list, a row of choices) goes `under` the words instead.
  //
  // `id` is the setting's anchor (`set-<id>`, lib/settingsmenu.js): search
  // and links bring it into view, and it is lit for a moment when they do.
  export let id = '';
  export let label = '';
  export let hint = '';
  /** The id of the control the label names, when it is one. */
  export let control = '';
  /** The control goes below the words, full width. */
  export let under = false;
  export let danger = false;
</script>

<div class="srow" class:under class:danger id={id ? `set-${id}` : undefined}>
  <div class="words">
    {#if control}<label class="name" for={control}>{label}</label>{:else}<span class="name">{label}</span>{/if}
    {#if hint || $$slots.hint}<p class="hint"><slot name="hint">{hint}</slot></p>{/if}
  </div>
  {#if $$slots.default}<div class="ctl"><slot /></div>{/if}
</div>

<style>
  .srow {
    display: flex;
    align-items: center;
    gap: 12px 24px;
    padding: 14px 18px;
    border-top: 1px solid var(--border);
    border-radius: 0;
    transition: background 0.6s;
  }
  .srow:first-child {
    border-top: 0;
  }
  .srow.under {
    flex-direction: column;
    align-items: stretch;
  }
  .words {
    flex: 1;
    min-width: 0;
  }
  .name {
    display: block;
    font-weight: 600;
  }
  .danger .name {
    color: var(--danger-text);
  }
  .hint {
    margin-top: 2px;
    font-size: var(--text-md);
    color: var(--text-dim);
  }
  .ctl {
    flex: none;
    display: flex;
    align-items: center;
    gap: 8px;
    max-width: 55%;
  }
  .under .ctl {
    max-width: none;
    flex-direction: column;
    align-items: stretch;
  }
  /* Brought here by search or a link: lit, then let go. */
  .srow:global(.found) {
    background: rgba(var(--accent-rgb), 0.1);
    transition: none;
  }
</style>
