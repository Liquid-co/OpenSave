<script>
  // A choice of one from a list, drawn as the app draws its own menus rather
  // than handed to the system, whose list is a different object on every
  // platform and matches none of them.
  //
  // The list hangs from the box, as wide as it at least, opening upward when
  // there is no room below. It is drawn at the top of the page (portal), so
  // a scrolling panel it sits in does not cut it off.
  //
  // Keys: Enter, Space or the arrows open it; the arrows move, Home and End
  // go to either end, a letter goes to the next choice starting with it,
  // Enter chooses, Escape and Tab close.
  //
  // Fires `change` with the value chosen — and a DOM change event from the
  // box, as a native select does, so a form saving on change saves.
  import { createEventDispatcher, tick, onDestroy } from 'svelte';
  import ChevronDown from 'lucide-svelte/icons/chevron-down';
  import Check from 'lucide-svelte/icons/check';
  import { unfold } from '../../lib/motion.js';

  export let value;
  /** [{value, label}] */
  export let options = [];
  export let id = undefined;
  export let label = undefined;
  export let disabled = false;
  /** A width for the box; otherwise as wide as its longest choice needs. */
  export let width = undefined;

  const dispatch = createEventDispatcher();
  let open = false;
  let box;
  let list;
  let place = '';

  $: current = options.find((o) => o.value === value);

  function measure() {
    const r = box.getBoundingClientRect();
    const below = window.innerHeight - r.bottom - 12;
    const above = r.top - 12;
    const wants = Math.min(options.length * 36 + 12, 320);
    const up = below < wants && above > below;
    const room = Math.max(Math.min(up ? above : below, 320), 120);
    place = `left: ${Math.round(r.left)}px; min-width: ${Math.round(r.width)}px; max-height: ${Math.round(room)}px; ${
      up ? `bottom: ${Math.round(window.innerHeight - r.top + 6)}px` : `top: ${Math.round(r.bottom + 6)}px`
    }`;
  }
  async function show() {
    if (disabled || open) return;
    measure();
    open = true;
    await tick();
    (list?.querySelector('[aria-selected="true"]') ?? list?.querySelector('button'))?.focus({ preventScroll: false });
  }
  function hide(focusBox = true) {
    open = false;
    if (focusBox) box?.focus();
  }
  function choose(o) {
    const changed = o.value !== value;
    value = o.value;
    hide();
    if (!changed) return;
    dispatch('change', o.value);
    box?.dispatchEvent(new Event('change', { bubbles: true }));
  }

  function boxKeys(e) {
    if (['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) {
      e.preventDefault();
      show();
    }
  }
  function listKeys(e) {
    const all = [...list.querySelectorAll('button')];
    const i = all.indexOf(document.activeElement);
    const go = (j) => {
      e.preventDefault();
      all[Math.max(0, Math.min(all.length - 1, j))]?.focus();
    };
    if (e.key === 'ArrowDown') go(i + 1);
    else if (e.key === 'ArrowUp') go(i - 1);
    else if (e.key === 'Home') go(0);
    else if (e.key === 'End') go(all.length - 1);
    else if (e.key === 'Escape') {
      e.preventDefault();
      e.stopPropagation();
      hide();
    } else if (e.key === 'Tab') hide(false);
    else if (e.key.length === 1 && /\S/.test(e.key)) {
      // The next choice starting with the letter, round from the one after.
      const k = e.key.toLocaleLowerCase();
      for (let n = 1; n <= all.length; n++) {
        const b = all[(i + n) % all.length];
        if (b.textContent.trim().toLocaleLowerCase().startsWith(k)) {
          e.preventDefault();
          b.focus();
          break;
        }
      }
    }
  }

  // Closed by a press anywhere else, and by anything moving the box from
  // under it (a scroll, a resize).
  function outside(e) {
    if (open && !box?.contains(e.target) && !list?.contains(e.target)) hide(false);
  }
  function moved(e) {
    if (open && !list?.contains(e.target)) hide(false);
  }

  // The list lives on the page itself, out of any panel that would clip it.
  function portal(node) {
    document.body.appendChild(node);
    return { destroy: () => node.remove() };
  }
  onDestroy(() => (open = false));
</script>

<svelte:window on:pointerdown|capture={outside} on:resize={() => open && hide(false)} on:scroll|capture={moved} />

<button
  bind:this={box}
  type="button"
  class="select"
  class:open
  {id}
  {disabled}
  aria-haspopup="listbox"
  aria-expanded={open}
  aria-label={label}
  style:width
  on:click={() => (open ? hide() : show())}
  on:keydown={boxKeys}
>
  <span class="cur">{current?.label ?? ''}</span>
  <span class="chev"><ChevronDown size={15} /></span>
</button>

{#if open}
  <div class="list" role="listbox" aria-label={label} bind:this={list} style={place} use:portal on:keydown={listKeys} transition:unfold={{ origin: 'top left', duration: 140, y: -4 }}>
    {#each options as o (o.value)}
      <button type="button" role="option" aria-selected={o.value === value} class:on={o.value === value} on:click={() => choose(o)}>
        <span class="label">{o.label}</span>
        <span class="tick">{#if o.value === value}<Check size={15} />{/if}</span>
      </button>
    {/each}
  </div>
{/if}

<style>
  /* The box: the same as a text field, with a chevron. */
  .select {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    max-width: 100%;
    height: 36px;
    padding: 0 10px 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: 9px;
    background: var(--bg);
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
    text-align: left;
    cursor: pointer;
    transition:
      border-color 0.12s,
      box-shadow 0.12s,
      background-color 0.12s;
  }
  .select:hover:not(:disabled) {
    background-color: var(--bg-hover);
  }
  .select.open,
  .select:focus-visible {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px rgba(var(--accent-rgb), 0.16);
    outline: none;
  }
  .select:disabled {
    opacity: 0.5;
    cursor: default;
  }
  .cur {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .chev {
    display: grid;
    color: var(--text-faint);
    transition: transform 0.2s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .open .chev {
    transform: rotate(180deg);
  }

  /* The list: as the app's menus are drawn (ui/Dropdown). */
  .list {
    position: fixed;
    z-index: 140;
    display: flex;
    flex-direction: column;
    gap: 2px;
    overflow-y: auto;
    padding: 5px;
    border-radius: 12px;
    background: var(--bg-raised);
    border: 1px solid var(--border-strong);
    box-shadow: var(--shadow);
    outline: none;
  }
  .list button {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 34px;
    padding: 6px 10px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: var(--text-md);
    text-align: left;
    white-space: nowrap;
    cursor: pointer;
  }
  .list button:hover,
  .list button:focus-visible {
    background: var(--bg-hover);
    outline: none;
  }
  .list button.on {
    font-weight: 600;
  }
  .label {
    flex: 1;
  }
  .tick {
    width: 15px;
    display: inline-flex;
    color: var(--accent-text);
  }
</style>
