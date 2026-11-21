<script>
  // Feedback toasts (lib/toasts.js keeps the rules).
  //
  // Where: bottom centre of the page, just above the status bar, never over
  // the sidebar. With a dialog open they move to the top of the window,
  // clear of the dialog's buttons.
  //
  // What: about a game, its art, with a small badge for what happened;
  // otherwise a tinted icon. The colour of the kind is only there and in the
  // thin bar along the foot, which shrinks as the toast's time runs out (or
  // fills, for something under way). A title, a second line, the one thing
  // to do next, and a close button on every one.
  //
  // Away: the close button, Escape while it has focus, or a swipe sideways.
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
  import LoaderCircle from 'lucide-svelte/icons/loader-circle';
  import ArrowDownToLine from 'lucide-svelte/icons/arrow-down-to-line';
  import RotateCw from 'lucide-svelte/icons/rotate-cw';
  import Undo2 from 'lucide-svelte/icons/undo-2';

  /** Before the app has a page to sit in: over the whole window. */
  export let floating = false;

  const ICON = { success: Check, received: ArrowDownToLine, info: Info, warning: TriangleAlert, error: CircleAlert, progress: LoaderCircle };

  $: top = $openDialogs > 0;
  $: list = $toasts;
  $: folded = list.length > MAX_SHOWN;
  let open = false;
  // Newest nearest the edge they sit on.
  $: shown = folded && !open ? list.slice(-1) : list;
  $: if (!folded) open = false;

  const gameOf = (t) => (t.game == null ? null : typeof t.game === 'string' ? ($games[t.game] ?? null) : t.game);
  // A game's art for what happened to it; an icon for what went wrong or is
  // still going, where the colour says more than the art.
  const artFor = (t) => (t.kind === 'success' || t.kind === 'received' || t.kind === 'info' ? gameOf(t) : null);
  // Errors with something to try list their buttons under the words.
  const below = (t) => t.kind === 'error' && (!!t.retry || !!t.action);
  const pct = (p) => (p?.total ? Math.min(100, Math.round((p.done / p.total) * 100)) : 0);
  // How much of a toast's time has gone: the bar of one drawn again (a stack
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

  // The bar that shrinks as a toast's time runs out. Its timing is set once,
  // when it is drawn and when its time starts again from the top (`round`),
  // and only paused and resumed in between: changing the delay of a running
  // animation moves it.
  function countdown(node, t) {
    let round = t.round;
    const start = (t) => {
      node.style.animation = 'none';
      void node.offsetWidth;
      node.style.animation = '';
      node.style.animationDuration = `${t.life}ms`;
      node.style.animationDelay = `-${spent(t)}ms`;
    };
    start(t);
    node.style.animationPlayState = t.held ? 'paused' : 'running';
    return {
      update(next) {
        if (next.round !== round) {
          round = next.round;
          start(next);
        }
        node.style.animationPlayState = next.held ? 'paused' : 'running';
      }
    };
  }

  // ── Swiping one away ──
  // Sideways, from anywhere on it but its buttons; let go past a short way,
  // or flicked, and it goes in that direction. Otherwise it springs back.
  const swiped = new Map(); // id -> how far it had gone, for its way out
  let drag = null;
  function grab(e, t) {
    if (e.button !== 0 || e.target.closest('button')) return;
    drag = { id: t.id, el: e.currentTarget, x0: e.clientX, dx: 0, at: performance.now() };
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {}
  }
  function pull(e) {
    if (!drag) return;
    drag.dx = e.clientX - drag.x0;
    drag.el.style.transform = `translateX(${drag.dx}px)`;
    drag.el.style.opacity = String(Math.max(0.2, 1 - Math.abs(drag.dx) / 260));
  }
  function letGo() {
    if (!drag) return;
    const { id, el, dx, at } = drag;
    drag = null;
    const speed = Math.abs(dx) / Math.max(performance.now() - at, 1);
    if (Math.abs(dx) > 96 || (Math.abs(dx) > 24 && speed > 0.55)) {
      swiped.set(id, dx);
      toasts.close(id);
      return;
    }
    el.style.transition = 'transform 0.3s cubic-bezier(0.3, 1.4, 0.5, 1), opacity 0.2s';
    el.style.transform = '';
    el.style.opacity = '';
    setTimeout(() => (el.style.transition = ''), 320);
  }
  function keys(e, t) {
    if (e.key === 'Escape') {
      e.stopPropagation();
      toasts.close(t.id);
    }
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
  // Out: swiped ones carry on sideways; the rest sink a little toward the
  // edge they came from, shrinking as they fade.
  const away = gated((node, { id }) => {
    const dx = swiped.get(id);
    swiped.delete(id);
    if (dx) {
      const dir = Math.sign(dx);
      return { duration: 200, easing: cubicOut, css: (u) => `transform: translateX(${dir * (1 - u) * 180}px); opacity: ${u}` };
    }
    const dir = top ? -1 : 1;
    return {
      duration: 200,
      easing: cubicOut,
      css: (u) => `transform: translateY(${(1 - u) * 10 * dir}px) scale(${0.94 + 0.06 * u}); opacity: ${u}`
    };
  });
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
  class="toasts"
  class:top
  class:floating
  class:folded
  class:open
  bind:this={box}
  data-holding={holding}
  on:pointerenter={pointerEnter}
  on:pointerleave={hold.pointerOut}
  on:focusin={hold.focusIn}
  on:focusout={focusOut}
>
  {#each shown as t (t.id)}
    {@const g = artFor(t)}
    {@const stacked = folded && !open}
    <div class="slot" out:away={{ id: t.id }} animate:settle={{ duration: 240 }}>
      {#if stacked}
        <span class="layer one" aria-hidden="true"></span>
        <span class="layer two" aria-hidden="true"></span>
      {/if}
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div
        class="toast k-{t.kind}"
        class:below={below(t)}
        on:pointerdown={(e) => grab(e, t)}
        on:pointermove={pull}
        on:pointerup={letGo}
        on:pointercancel={letGo}
        on:keydown={(e) => keys(e, t)}
      >
        <span class="lead">
          {#if g}
            {@const src = gameCover(g, true)}
            <span class="art" class:dim={t.undo}>
              {#if src && isDaemonURL(src)}<CoverImage {src} alt="" />{:else if src}<img {src} alt="" />{/if}
            </span>
            {#if !t.undo && (t.kind === 'success' || t.kind === 'received')}
              <span class="mark"><svelte:component this={t.icon ?? ICON[t.kind]} size={11} strokeWidth={3.2} /></span>
            {/if}
          {:else}
            <span class="glyph" class:spin={t.kind === 'progress' && !t.icon}><svelte:component this={t.icon ?? ICON[t.kind] ?? Info} size={18} /></span>
          {/if}
        </span>

        <div class="words">
          <b>{t.title}</b>
          {#if t.detail}<span class="detail">{t.detail}</span>{/if}
          {#if below(t)}
            <span class="acts">
              {#if t.retry}<button class="pill" on:click={() => retry(t)}><RotateCw size={13} />{$i18n('toast.retry')}</button>{/if}
              {#if t.action}<button class="quiet" on:click={() => act(t)}>{t.action.label}</button>{/if}
            </span>
          {/if}
        </div>

        <!-- The one thing to do stays in reach in a stack too: an Undo
             waits for nobody. -->
        {#if !below(t) && t.action}
          <button class="pill" on:click={() => act(t)}>{#if t.undo}<Undo2 size={14} />{/if}{t.action.label}</button>
        {/if}
        {#if stacked}
          <button class="count" aria-label={$i18n('toast.more', { n: list.length - 1 })} title={$i18n('toast.more', { n: list.length - 1 })} on:click={() => (open = true)}
            >+{list.length - 1}</button
          >
        {:else}
          <button class="close" aria-label={$i18n('common.dismiss')} title={$i18n('common.dismiss')} on:click={() => toasts.close(t.id)}><X size={15} /></button>
        {/if}

        {#if t.progress}
          <span class="bar fill" role="progressbar" aria-label={t.title} aria-valuemin="0" aria-valuemax="100" aria-valuenow={pct(t.progress)}><span style="width: {pct(t.progress)}%"></span></span>
        {:else if Number.isFinite(t.life)}
          <span class="bar timer" aria-hidden="true" use:countdown={t}></span>
        {/if}
      </div>
    </div>
  {/each}
</div>
<div class="sr" aria-live="polite">{polite}</div>
<div class="sr" aria-live="assertive">{urgent}</div>

<style>
  .toasts {
    /* The surface, a step up from the page, lit faintly from above. */
    --toast-bg: var(--bg-hover);
    --toast-edge: rgba(255, 255, 255, 0.075);
    --toast-lift: rgba(255, 255, 255, 0.04);
    --toast-shade: rgba(0, 0, 0, 0.5);
    --pill: rgba(255, 255, 255, 0.07);
    --pill-hover: rgba(255, 255, 255, 0.13);
    --layer-dark: rgba(0, 0, 0, 0.22);
    position: absolute;
    left: 50%;
    bottom: 16px;
    z-index: 100;
    width: min(390px, calc(100% - 32px));
    display: flex;
    flex-direction: column;
    gap: 8px;
    transform: translateX(-50%);
    pointer-events: none;
  }
  :global([data-theme='light']) .toasts {
    --toast-bg: #fff;
    --toast-edge: rgba(20, 20, 40, 0.09);
    --toast-lift: rgba(255, 255, 255, 0);
    --toast-shade: rgba(20, 20, 40, 0.16);
    --pill: rgba(20, 20, 40, 0.055);
    --pill-hover: rgba(20, 20, 40, 0.1);
    --layer-dark: rgba(20, 20, 40, 0.04);
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
  .folded:not(.open) {
    padding-top: 14px;
  }
  .top.folded:not(.open) {
    padding: 0 0 14px;
  }

  .slot {
    position: relative;
    pointer-events: auto;
  }
  .toast {
    --kind: var(--accent-text);
    position: relative;
    z-index: 1;
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 60px;
    padding: 10px 8px 10px 10px;
    border-radius: 16px;
    background: linear-gradient(180deg, var(--toast-lift), transparent 70%), var(--toast-bg);
    background: linear-gradient(180deg, var(--toast-lift), transparent 70%), color-mix(in srgb, var(--toast-bg) 88%, transparent);
    /* Small, so cheap: the page shows through, softened, and the toast
       reads as a layer above it. */
    -webkit-backdrop-filter: blur(16px) saturate(1.4);
    backdrop-filter: blur(16px) saturate(1.4);
    border: 1px solid var(--toast-edge);
    box-shadow:
      0 1px 2px var(--toast-shade),
      0 12px 32px -10px var(--toast-shade),
      0 28px 56px -24px var(--toast-shade);
    overflow: hidden;
    font-size: var(--text-base);
    touch-action: pan-y;
    user-select: none;
    animation: toast-in 0.42s cubic-bezier(0.2, 1.25, 0.4, 1);
  }
  .top .toast {
    animation-name: toast-in-top;
  }
  /* In: up from the edge, a little small, settling. Movement only — it reads
     from its first frame. */
  @keyframes toast-in {
    from {
      transform: translateY(18px) scale(0.94);
    }
  }
  @keyframes toast-in-top {
    from {
      transform: translateY(-18px) scale(0.94);
    }
  }
  .toast.below {
    align-items: flex-start;
  }
  .k-success,
  .k-received {
    --kind: var(--success);
  }
  .k-received {
    --kind: #2dd4bf;
  }
  .k-progress {
    --kind: var(--info);
  }
  .k-warning {
    --kind: var(--warn);
  }
  .k-error {
    --kind: var(--danger);
  }

  /* The stack's edges, peeking out behind the front toast. */
  .layer {
    position: absolute;
    height: 100%;
    border-radius: 16px;
    background: var(--toast-bg);
    border: 1px solid var(--toast-edge);
    box-shadow: inset 0 0 0 999px var(--layer-dark);
    pointer-events: none;
  }
  .layer.one {
    left: 10px;
    right: 10px;
    top: -7px;
  }
  .layer.two {
    left: 20px;
    right: 20px;
    top: -14px;
    box-shadow: inset 0 0 0 999px var(--layer-dark), inset 0 0 0 999px var(--layer-dark);
  }
  .top .layer.one {
    top: 7px;
  }
  .top .layer.two {
    top: 14px;
  }

  /* What it is about: the game's art, or the kind's icon. */
  .lead {
    position: relative;
    flex: none;
    width: 40px;
    height: 40px;
  }
  .art {
    position: absolute;
    inset: 0;
    overflow: hidden;
    border-radius: 10px;
    background: linear-gradient(140deg, var(--bg-active), var(--bg-raised));
    box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
    animation: art-in 0.4s cubic-bezier(0.3, 1.3, 0.5, 1);
  }
  @keyframes art-in {
    from {
      transform: scale(0.86);
    }
  }
  .art :global(img) {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: 50% 30%;
  }
  .art.dim :global(img) {
    opacity: 0.45;
    filter: grayscale(0.6);
  }
  .mark {
    position: absolute;
    right: -5px;
    bottom: -5px;
    width: 19px;
    height: 19px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--kind);
    color: #04120a;
    box-shadow: 0 0 0 2.5px var(--toast-bg);
    animation: mark-in 0.45s cubic-bezier(0.3, 1.6, 0.5, 1) 0.12s backwards;
  }
  :global([data-theme='light']) .mark {
    color: #fff;
  }
  .mark :global(svg),
  .glyph :global(svg) {
    display: block;
  }
  @keyframes mark-in {
    from {
      transform: scale(0.2);
    }
  }
  .glyph {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 10px;
    background: var(--pill);
    background: color-mix(in srgb, var(--kind) 15%, transparent);
    color: var(--kind);
  }
  .k-info .glyph {
    background: var(--pill);
    color: var(--text-dim);
  }
  .glyph.spin :global(svg) {
    animation: turn 1s linear infinite;
  }
  @keyframes turn {
    to {
      transform: rotate(360deg);
    }
  }

  .words {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 1px;
  }
  .words b {
    font-weight: 600;
    line-height: 1.3;
    letter-spacing: -0.005em;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
  }
  .detail {
    font-size: var(--text-md);
    line-height: 1.35;
    color: var(--text-dim);
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    overflow: hidden;
    overflow-wrap: anywhere;
  }
  .below .words {
    padding-top: 2px;
  }
  .acts {
    display: flex;
    gap: 6px;
    margin-top: 8px;
  }

  button {
    font: inherit;
    cursor: pointer;
  }
  .pill {
    flex: none;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 30px;
    padding: 0 12px;
    border: 0;
    border-radius: 9px;
    background: var(--pill);
    color: var(--text);
    font-size: var(--text-md);
    font-weight: 600;
    white-space: nowrap;
    transition:
      background 0.15s,
      transform 0.15s cubic-bezier(0.2, 0.7, 0.2, 1);
  }
  .pill:hover {
    background: var(--pill-hover);
  }
  .pill:active {
    transform: scale(0.96);
  }
  .count {
    flex: none;
    min-width: 30px;
    height: 24px;
    padding: 0 8px;
    border: 0;
    border-radius: 12px;
    background: var(--pill);
    color: var(--text-dim);
    font-size: var(--text-sm);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }
  .count:hover {
    background: var(--pill-hover);
    color: var(--text);
  }
  .quiet {
    height: 30px;
    padding: 0 10px;
    border: 0;
    border-radius: 9px;
    background: transparent;
    color: var(--text-dim);
    font-size: var(--text-md);
    font-weight: 600;
  }
  .quiet:hover {
    background: var(--pill);
    color: var(--text);
  }
  .close {
    flex: none;
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-left: -4px;
    padding: 0;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--text-faint);
    transition:
      background 0.15s,
      color 0.15s;
  }
  .close:hover {
    background: var(--pill);
    color: var(--text);
  }
  .close :global(svg) {
    display: block;
  }
  .below .close {
    margin-top: -2px;
  }

  /* Along the foot: how much time it has left, shrinking toward its start —
     or, for something under way, how far it has come. */
  .bar {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 2px;
    pointer-events: none;
  }
  .bar.timer {
    background: var(--kind);
    opacity: 0.75;
    transform-origin: left center;
    animation-name: shrink;
    animation-timing-function: linear;
    animation-fill-mode: both;
  }
  .k-info .bar.timer {
    background: var(--accent-text);
  }
  :global([data-motion='off']) .bar.timer {
    display: none;
  }
  @keyframes shrink {
    from {
      transform: scaleX(1);
    }
    to {
      transform: scaleX(0);
    }
  }
  .bar.fill {
    background: var(--pill);
    background: color-mix(in srgb, var(--kind) 18%, transparent);
  }
  .bar.fill > span {
    display: block;
    height: 100%;
    background: var(--kind);
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
