<script>
  // Feedback toasts, as the ToastsV4 board draws them (lib/toasts.js keeps
  // the rules).
  //
  // Where: bottom centre of the page, just above the status bar, never over
  // the sidebar. With a dialog open they move to the top of the window,
  // clear of the dialog's buttons.
  //
  // What: about a game, its cover, with a small badge for what happened;
  // otherwise a small tinted icon. The colour of the kind is only there — no
  // coloured borders. A title, a second line, and the one thing to do next.
  // Undo counts down on its own button. Something under way has a bar, and
  // ends as done in the same place.
  //
  // More than three fold into a stack: the newest in front, the rest behind
  // it, all of them shown while the pointer is on the stack or focus is in it.
  //
  // Screen readers hear each toast once, when it arrives (errors at once);
  // a toast whose bar moves is not read again for it.
  import { toasts, games } from '../lib/stores.js';
  import { MAX_SHOWN } from '../lib/toasts.js';
  import { openDialogs } from '../lib/dialogs.js';
  import { createHold } from '../lib/toasthold.js';
  import { get } from 'svelte/store';
  import { gameCover, isDaemonURL } from '../lib/api.js';
  import { gated } from '../lib/motion.js';
  // `t` is a toast in the loop below, so the store comes in under its own name.
  import { t as i18n } from '../lib/i18n.js';
  import { onDestroy } from 'svelte';
  import { flip } from 'svelte/animate';
  import { cubicOut } from 'svelte/easing';
  import CoverImage from './CoverImage.svelte';
  import X from 'lucide-svelte/icons/x';
  import Check from 'lucide-svelte/icons/check';
  import Info from 'lucide-svelte/icons/info';
  import CircleAlert from 'lucide-svelte/icons/circle-alert';
  import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
  import Clock from 'lucide-svelte/icons/clock';
  import Download from 'lucide-svelte/icons/download';
  import RotateCw from 'lucide-svelte/icons/rotate-cw';

  /** Before the app has a page to sit in: over the whole window. */
  export let floating = false;

  const TILE = { success: Check, received: Download, info: Info, warning: TriangleAlert, error: CircleAlert, progress: Clock };

  $: top = $openDialogs > 0;
  $: list = $toasts;
  $: folded = list.length > MAX_SHOWN;
  let open = false;
  // Newest nearest the edge they sit on.
  $: shown = folded && !open ? list.slice(-1) : list;
  $: if (!folded) open = false;

  const gameOf = (t) => (t.game == null ? null : typeof t.game === 'string' ? $games[t.game] ?? null : t.game);
  // A game's cover for what happened to it; a tile for what went wrong or is
  // still going, where the colour of the icon says more than the art.
  const coverFor = (t) => (t.kind === 'success' || t.kind === 'received' || t.kind === 'info' ? gameOf(t) : null);
  const closable = (t) => t.kind === 'error' || t.kind === 'warning' || (!!t.action && !t.undo && t.kind !== 'progress');
  // Errors with something to try list their buttons under the words.
  const below = (t) => t.kind === 'error' && (!!t.retry || !!t.action);
  const pct = (p) => (p?.total ? Math.min(100, Math.round((p.done / p.total) * 100)) : 0);
  // How far an Undo's countdown has gone, so a toast drawn again (the stack
  // opening) carries on from there.
  const spent = (t) => Math.min(t.life, t.life - t.left + (t.held ? 0 : Date.now() - t.woundAt));

  function act(t) {
    toasts.remove(t.id);
    t.action.run();
  }
  function retry(t) {
    toasts.remove(t.id);
    t.retry();
  }

  // ── Screen readers ──
  const heard = new Set();
  let polite = '';
  let urgent = '';
  $: for (const t of list) {
    if (heard.has(t.id)) continue;
    heard.add(t.id);
    const said = t.detail ? `${t.title}. ${t.detail}` : t.title;
    if (t.kind === 'error') urgent = said;
    else polite = said;
  }

  // While the pointer or focus is on the toasts, none of them runs out —
  // not only the one pointed at: an opened stack is being read as a whole,
  // and one arriving under the pointer is in front of someone reading. The
  // stack opens meanwhile.
  //
  // The pointer's leaving is not left to mouseleave alone. The toasts' box
  // takes no pointer events itself (the page shows through between them), a
  // toast that runs out goes from under a still pointer, and a window can
  // lose the pointer without saying so — any of which left them held for
  // good. So while it is in, every move is checked against the box, and
  // leaving the window counts as leaving.
  let box;
  let holding = false;
  const hold = createHold({
    onChange(on) {
      holding = on;
      const all = get(toasts);
      if (on) {
        for (const t of all) toasts.pause(t.id);
        if (all.length > MAX_SHOWN) open = true;
      } else {
        open = false;
        for (const t of all) toasts.resume(t.id);
        unwatch();
      }
    }
  });
  // One arriving while they are held is held too, and opens the stack it
  // makes.
  $: if (holding) for (const t of list) if (!t.held) toasts.pause(t.id);
  $: if (holding && folded) open = true;

  function pointerEnter() {
    hold.pointerIn();
    document.addEventListener('pointermove', watch, true);
    document.documentElement.addEventListener('pointerleave', hold.pointerOut);
    window.addEventListener('blur', hold.pointerOut);
  }
  function watch(e) {
    if (box?.contains(e.target)) hold.pointerIn();
    else hold.pointerOut();
  }
  function unwatch() {
    document.removeEventListener('pointermove', watch, true);
    document.documentElement.removeEventListener('pointerleave', hold.pointerOut);
    window.removeEventListener('blur', hold.pointerOut);
  }
  function focusOut(e) {
    if (!box?.contains(e.relatedTarget)) hold.focusOut();
  }
  onDestroy(() => {
    hold.stop();
    unwatch();
  });

  const settle = gated(flip);
  // Out: a short drop toward the edge it came from, fading as it goes.
  const away = gated((node, { dir = 1 } = {}) => ({
    duration: 160,
    easing: cubicOut,
    css: (u) => `transform: translateY(${(1 - u) * 10 * dir}px) scale(${0.97 + 0.03 * u}); opacity: ${u}`
  }));
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="toasts" class:top class:floating class:folded class:open bind:this={box} data-holding={holding} on:pointerenter={pointerEnter} on:pointerleave={hold.pointerOut} on:focusin={hold.focusIn} on:focusout={focusOut}>
  {#each shown as t (t.id)}
    {@const g = coverFor(t)}
    <div class="slot" out:away={{ dir: top ? -1 : 1 }} animate:settle={{ duration: 220 }}>
      {#if folded && !open}
        <span class="layer one" aria-hidden="true"></span>
        <span class="layer two" aria-hidden="true"></span>
      {/if}
      <div class="toast k-{t.kind}" class:below={below(t)} class:bar={!!t.progress}>
        {#if g}
          {@const src = gameCover(g, true)}
          <span class="cv" class:dim={t.undo}>
            <span class="initial" aria-hidden="true">{g.name?.charAt(0) ?? ''}</span>
            {#if src && isDaemonURL(src)}<CoverImage {src} alt="" />{:else if src}<img {src} alt="" />{/if}
            {#if !t.undo && (t.kind === 'success' || t.kind === 'received')}
              <span class="badge"><svelte:component this={t.icon ?? TILE[t.kind]} size={12} strokeWidth={3} /></span>
            {/if}
          </span>
        {:else}
          <span class="tile"><svelte:component this={t.icon ?? TILE[t.kind] ?? Info} size={16} /></span>
        {/if}

        <div class="words">
          <b>{t.title}</b>
          {#if t.detail}<span>{t.detail}</span>{/if}
          {#if below(t)}
            <span class="acts">
              {#if t.retry}<button class="btn link" on:click={() => retry(t)}><RotateCw size={13} />{$i18n('toast.retry')}</button>{/if}
              {#if t.action}<button class="btn link quiet" on:click={() => act(t)}>{t.action.label}</button>{/if}
            </span>
          {/if}
        </div>

        {#if folded && !open}
          <button class="more" on:click={() => (open = true)}>{$i18n('toast.more', { n: list.length - 1 })}</button>
        {:else if !below(t) && t.action}
          {#if t.undo}
            <button class="btn undo" class:held={t.held} on:click={() => act(t)}>
              <svg viewBox="0 0 20 20" aria-hidden="true">
                <circle class="track" cx="10" cy="10" r="8" />
                <circle class="left" cx="10" cy="10" r="8" style="animation-duration: {t.life}ms; animation-delay: -{spent(t)}ms" />
              </svg>{t.action.label}
            </button>
          {:else}
            <button class="btn link" on:click={() => act(t)}>{t.action.label}</button>
          {/if}
        {/if}
        {#if closable(t) && !(folded && !open)}
          <button class="btn x" aria-label={$i18n('common.dismiss')} title={$i18n('common.dismiss')} on:click={() => toasts.close(t.id)}><X size={14} /></button>
        {/if}

        {#if t.progress}
          <span class="track-bar" role="progressbar" aria-label={t.title} aria-valuemin="0" aria-valuemax="100" aria-valuenow={pct(t.progress)}><span style="width: {pct(t.progress)}%"></span></span>
        {/if}
      </div>
    </div>
  {/each}
</div>
<div class="sr" aria-live="polite">{polite}</div>
<div class="sr" aria-live="assertive">{urgent}</div>

<style>
  .toasts {
    position: absolute;
    left: 50%;
    bottom: 16px;
    z-index: 100;
    width: min(420px, calc(100% - 32px));
    display: flex;
    flex-direction: column;
    gap: 8px;
    transform: translateX(-50%);
    pointer-events: none;
  }
  .toasts.floating {
    position: fixed;
    bottom: calc(var(--statusbar-h) + 16px);
  }
  /* A dialog is open: the top of the window, newest nearest the edge. */
  .toasts.top {
    position: fixed;
    top: 14px;
    bottom: auto;
    z-index: 120;
    flex-direction: column-reverse;
  }
  .toasts.open {
    max-height: calc(100vh - 160px);
    overflow-y: auto;
    padding: 14px 0 2px;
    margin-bottom: -2px;
  }
  .slot {
    position: relative;
    pointer-events: auto;
  }
  .toast {
    position: relative;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 10px;
    border-radius: 14px;
    background: var(--bg-hover);
    border: 1px solid var(--border-strong);
    box-shadow: 0 18px 44px rgba(0, 0, 0, 0.55);
    font-size: var(--text-base);
    pointer-events: auto;
    animation: toast-in 0.32s cubic-bezier(0.3, 1.25, 0.5, 1);
  }
  .top .toast {
    animation-name: toast-in-top;
  }
  :global([data-theme='light']) .toast {
    background: var(--bg-raised);
    box-shadow: 0 14px 36px rgba(20, 20, 40, 0.18);
  }
  /* In: up from the edge, a little small, settling. Movement only — it reads
     from its first frame. */
  @keyframes toast-in {
    from {
      transform: translateY(16px) scale(0.96);
    }
  }
  @keyframes toast-in-top {
    from {
      transform: translateY(-16px) scale(0.96);
    }
  }
  .toast.below {
    align-items: flex-start;
  }
  .toast.bar {
    padding-bottom: 15px;
  }

  /* The stack's edges, peeking out behind the front toast. */
  .layer {
    position: absolute;
    height: 100%;
    border-radius: 14px;
    border: 1px solid var(--border-strong);
    pointer-events: none;
  }
  .layer.one {
    left: 10px;
    right: 10px;
    top: -7px;
    background: color-mix(in srgb, var(--bg-hover) 88%, black);
  }
  .layer.two {
    left: 20px;
    right: 20px;
    top: -14px;
    background: color-mix(in srgb, var(--bg-hover) 76%, black);
  }
  .top .layer.one {
    top: 7px;
  }
  .top .layer.two {
    top: 14px;
  }
  .folded:not(.open) {
    padding-top: 14px;
  }
  .top.folded:not(.open) {
    padding: 0 0 14px;
  }

  .tile {
    width: 34px;
    height: 34px;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: 10px;
    background: var(--bg-active);
    color: var(--text-dim);
  }
  .k-success .tile {
    background: rgba(var(--success-rgb), 0.14);
    color: var(--success);
  }
  .k-received .tile {
    background: rgba(94, 234, 212, 0.14);
    color: #5eead4;
  }
  .k-progress .tile {
    background: rgba(var(--info-rgb), 0.14);
    color: var(--info);
  }
  .k-warning .tile {
    background: rgba(var(--warn-rgb), 0.14);
    color: var(--warn);
  }
  .k-error .tile {
    background: rgba(var(--danger-rgb), 0.14);
    color: var(--danger);
  }

  .cv {
    position: relative;
    width: 32px;
    height: 44px;
    flex: none;
    margin-right: 4px;
    border-radius: 5px;
    background: var(--bg-active);
    animation: cv-in 0.35s cubic-bezier(0.3, 1.3, 0.5, 1);
  }
  @keyframes cv-in {
    from {
      transform: scale(0.85);
    }
  }
  .cv :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    border-radius: 5px;
    object-fit: cover;
  }
  .cv.dim :global(img),
  .cv.dim .initial {
    opacity: 0.55;
  }
  .initial {
    position: absolute;
    inset: 0;
    display: grid;
    place-items: center;
    font-weight: 700;
    color: var(--text-faint);
  }
  .badge {
    position: absolute;
    right: -6px;
    bottom: -6px;
    width: 20px;
    height: 20px;
    display: grid;
    place-items: center;
    border-radius: 50%;
    background: #22c55e;
    color: #052e16;
    box-shadow: 0 0 0 2px var(--bg-hover);
    animation: badge-in 0.4s cubic-bezier(0.3, 1.6, 0.5, 1) 0.08s backwards;
  }
  :global([data-theme='light']) .badge {
    box-shadow: 0 0 0 2px var(--bg-raised);
  }
  .k-received .badge {
    background: #5eead4;
    color: #042f2e;
  }
  @keyframes badge-in {
    from {
      transform: scale(0.3);
    }
  }

  .words {
    flex: 1;
    min-width: 0;
  }
  .words b {
    display: block;
    font-weight: 600;
    overflow-wrap: anywhere;
  }
  .words > span {
    display: block;
    font-size: var(--text-md);
    color: var(--text-dim);
    overflow-wrap: anywhere;
  }
  .acts {
    display: flex !important;
    gap: 4px;
    margin: 6px 0 0 -10px;
  }

  .btn {
    flex: none;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 6px;
    height: 30px;
    padding: 0 10px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    font: inherit;
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    cursor: pointer;
  }
  .link {
    color: var(--accent-text);
  }
  .link:hover {
    background: rgba(var(--accent-rgb), 0.12);
  }
  .link.quiet {
    color: var(--text-dim);
  }
  .link.quiet:hover {
    background: var(--bg-active);
    color: var(--text);
  }
  .x {
    width: 30px;
    padding: 0;
    color: var(--text-faint);
  }
  .x:hover {
    background: var(--bg-active);
    color: var(--text);
  }
  .more {
    flex: none;
    padding: 4px 8px;
    border: 0;
    border-radius: 6px;
    background: transparent;
    color: var(--text-dim);
    font: inherit;
    font-size: var(--text-sm);
    font-weight: 600;
    cursor: pointer;
  }
  .more:hover {
    background: var(--bg-active);
    color: var(--text);
  }

  /* Undo: the ring on the button empties as its time runs out, and stops
     while the toast is held. */
  .undo {
    gap: 8px;
    padding: 0 10px 0 6px;
    background: var(--bg-active);
    border: 1px solid var(--border-strong);
    color: var(--text);
  }
  .undo:hover {
    border-color: var(--accent);
  }
  .undo svg {
    width: 20px;
    height: 20px;
    transform: rotate(-90deg);
  }
  .undo circle {
    fill: none;
    stroke-width: 2.5;
  }
  .undo .track {
    stroke: var(--border-strong);
  }
  .undo .left {
    stroke: var(--accent-text);
    stroke-linecap: round;
    stroke-dasharray: 50.27;
    animation-name: drain;
    animation-timing-function: linear;
    animation-fill-mode: both;
  }
  .undo.held .left {
    animation-play-state: paused;
  }
  @keyframes drain {
    to {
      stroke-dashoffset: 50.27;
    }
  }

  /* Under way: a thin bar along the foot, under the words. */
  .track-bar {
    position: absolute;
    left: 56px;
    right: 14px;
    bottom: 8px;
    height: 3px;
    border-radius: 2px;
    background: var(--bg-active);
    overflow: hidden;
  }
  .track-bar > span {
    display: block;
    height: 100%;
    border-radius: 2px;
    background: var(--info);
    transition: width 0.3s ease;
  }

  .sr {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0 0 0 0);
    white-space: nowrap;
  }
</style>
