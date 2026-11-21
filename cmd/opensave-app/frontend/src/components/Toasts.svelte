<script>
  // Feedback toasts (lib/toasts.js keeps the rules).
  //
  // Where: bottom centre of the page, just above the status bar, never over
  // the sidebar. With a dialog open they move to the top of the window,
  // clear of the dialog's buttons.
  //
  // A stack: the newest in front, each older one a step behind it — smaller,
  // further back, darker, only its edge showing (lib/toaststack.js). A new
  // one pushes the rest back; one that goes lets the rest come forward.
  // Pointing at the stack, focusing into it, or tapping it on a touch screen
  // opens it into a column, and holds every toast in it.
  //
  // What: about a game, its art, with a small badge for what happened;
  // otherwise a tinted icon. The colour of the kind is only there and in the
  // thin bar along the foot, which shrinks as the toast's time runs out (or
  // fills, for something under way). A title, a second line, the one thing
  // to do next, and a close button on every one.
  //
  // Every toast is the same size, whatever it says: two lines of words — a
  // title and a second line, or a title given both — each cut short with the
  // whole of it on hover (and read in full to a screen reader), and one
  // button beside them, never under.
  //
  // Away: the close button, Escape while it has focus, or a throw to either
  // side — it tilts as it is dragged, and goes if let go far enough or fast
  // enough; otherwise it springs back.
  //
  // Screen readers hear each toast once, when it arrives (errors at once);
  // a toast whose bar moves is not read again for it.
  import { toasts, games } from '../lib/stores.js';
  import { stackPlaces, STACK } from '../lib/toaststack.js';
  import { openDialogs } from '../lib/dialogs.js';
  import { createHold } from '../lib/toasthold.js';
  import { get } from 'svelte/store';
  import { gameCover, isDaemonURL } from '../lib/api.js';
  import { gated } from '../lib/motion.js';
  // `t` is a toast in the loop below, so the store comes in under its own name.
  import { t as i18n } from '../lib/i18n.js';
  import { onDestroy } from 'svelte';
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
  // Newest first: the front of the stack.
  $: order = list.slice().reverse();

  let holding = false;
  $: open = holding && order.length > 1;
  $: stack = stackPlaces(order, {}, { open });
  // Away from the edge they sit on: up from the bottom, down from the top.
  $: away = top ? 1 : -1;

  const gameOf = (t) => (t.game == null ? null : typeof t.game === 'string' ? ($games[t.game] ?? null) : t.game);
  // A game's art for what happened to it; an icon for what went wrong or is
  // still going, where the colour says more than the art.
  const artFor = (t) => (t.kind === 'success' || t.kind === 'received' || t.kind === 'info' ? gameOf(t) : null);
  // The one button beside the words: Try again, when there is that to do,
  // over anything else.
  const button = (t) => (t.retry ? 'retry' : t.action ? 'action' : null);
  const pct = (p) => (p?.total ? Math.min(100, Math.round((p.done / p.total) * 100)) : 0);
  // How much of a toast's time has gone: the bar of one drawn again carries
  // on from there.
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

  // ── Throwing one away ──
  // The front one, or any of them while the stack is open. It follows the
  // pointer, tilting the way it goes and fading with distance — and the one
  // behind it comes forward as it goes, its words showing through, so it is
  // already arriving when this one leaves. Let go past a third of its width,
  // or thrown, it flies off that side; otherwise both spring back. A tap on a
  // closed stack (on a touch screen, where nothing hovers) opens it instead.
  const thrown = new Set();
  let drag = null;
  function grab(e, t, i) {
    if (e.button !== 0 || e.target.closest('button')) return;
    if (!open && i > 0) return;
    const next = !open && i === 0 ? e.currentTarget.parentElement.nextElementSibling?.querySelector('.toast') : null;
    drag = { id: t.id, el: e.currentTarget, next, x0: e.clientX, dx: 0, vx: 0, at: performance.now(), moved: false, touch: e.pointerType !== 'mouse' };
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {}
  }
  function pull(e) {
    if (!drag) return;
    const dx = e.clientX - drag.x0;
    if (!drag.moved && Math.abs(dx) < 5) return;
    const now = performance.now();
    if (!drag.moved) {
      drag.moved = true;
      drag.el.style.transition = 'none';
      drag.el.style.animation = 'none';
      if (drag.next) drag.next.style.transition = 'none';
    }
    drag.vx = (dx - drag.dx) / Math.max(now - drag.at, 1);
    drag.at = now;
    drag.dx = dx;
    const w = drag.el.offsetWidth || 1;
    drag.el.style.transform = `translateX(${dx}px) rotate(${(dx / w) * 9}deg)`;
    drag.el.style.opacity = String(1 - Math.min(Math.abs(dx) / (w * 1.3), 0.65));
    if (drag.next) bringForward(drag.next, Math.min(Math.abs(dx) / (w / 3), 1));
  }
  // The card behind, `k` of the way to the front: moved and grown within its
  // place (which the stack still says is one back), and lit, its words
  // showing.
  function bringForward(el, k) {
    el.style.setProperty('--lift', String(k));
    el.style.transform = k ? `translateY(${-away * STACK.peek * k}px) scale(${1 + (1 / (1 - STACK.shrink) - 1) * k})` : '';
  }
  function settleBack(el) {
    if (!el) return;
    el.style.transition = 'transform 0.5s var(--settle)';
    bringForward(el, 0);
    el.style.removeProperty('--lift');
    setTimeout(() => (el.style.transition = ''), 520);
  }
  function letGo() {
    if (!drag) return;
    const { id, el, next, dx, vx, at, moved, touch } = drag;
    drag = null;
    if (!moved) {
      if (touch && !open && list.length > 1) pinOpen();
      return;
    }
    const w = el.offsetWidth || 1;
    // A throw is still moving when it is let go.
    const flung = performance.now() - at < 90 && Math.abs(vx) > 0.6 && Math.sign(vx) === Math.sign(dx);
    if (Math.abs(dx) > w / 3 || flung) {
      const side = Math.sign(dx) || 1;
      el.style.transition = 'transform 0.4s cubic-bezier(0.2, 0.7, 0.3, 1), opacity 0.4s ease-out';
      el.style.transform = `translateX(${side * (w + 140)}px) rotate(${side * 18}deg)`;
      el.style.opacity = '0';
      thrown.add(id);
      // Gone from the stack while still in flight, so the one behind takes
      // the front as it leaves, from where the drag had brought it.
      setTimeout(() => {
        toasts.close(id);
        settleBack(next);
      }, 80);
      return;
    }
    el.style.transition = 'transform 0.55s cubic-bezier(0.25, 1.6, 0.4, 1), opacity 0.25s';
    el.style.transform = '';
    el.style.opacity = '';
    setTimeout(() => (el.style.transition = ''), 560);
    settleBack(next);
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

  // While the pointer or focus is on the toasts, none of them runs out and
  // the stack stands open.
  //
  // The pointer's leaving is not left to pointerleave alone. The toasts' box
  // takes no pointer events itself (the page shows through between them), a
  // toast that runs out goes from under a still pointer, and a window can
  // lose the pointer without saying so — any of which left them held for
  // good. So while it is in, every move is checked against the box, and
  // leaving the window counts as leaving.
  let box;
  const hold = createHold({
    onChange(on) {
      holding = on;
      const all = get(toasts);
      if (on) for (const t of all) toasts.pause(t.id);
      else {
        for (const t of all) toasts.resume(t.id);
        unwatch();
      }
    }
  });
  // One arriving while they are held is held too.
  $: if (holding) for (const t of list) if (!t.held) toasts.pause(t.id);
  // Nothing left to hold open.
  $: if (!list.length) unpin();

  function pointerEnter(e) {
    if (e.pointerType !== 'mouse') return;
    hold.pointerIn();
    document.addEventListener('pointermove', watch, true);
    document.documentElement.addEventListener('pointerleave', hold.pointerOut);
    window.addEventListener('blur', hold.pointerOut);
  }
  function watch(e) {
    if (e.pointerType !== 'mouse') return;
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
  // Tapped open: shut again by a tap anywhere else.
  function pinOpen() {
    hold.pin(true);
    document.addEventListener('pointerdown', tapElsewhere, true);
  }
  function tapElsewhere(e) {
    if (!box?.contains(e.target)) unpin();
  }
  function unpin() {
    document.removeEventListener('pointerdown', tapElsewhere, true);
    hold.pin(false);
  }
  onDestroy(() => {
    hold.stop();
    unwatch();
    unpin();
  });

  // Out: one thrown has already gone on its way and only needs the time to
  // finish; the rest sink a little toward the edge they came from, shrinking
  // as they fade.
  const leave = gated((node, { id, dir }) => {
    if (thrown.delete(id)) return { duration: 340 };
    return {
      duration: 220,
      easing: cubicOut,
      css: (u) => `transform: translateY(${(1 - u) * 14 * -dir}px) scale(${0.9 + 0.1 * u}); opacity: ${u}`
    };
  });
</script>

<!-- svelte-ignore a11y-no-static-element-interactions -->
<div
  class="toasts"
  class:top
  class:floating
  class:open
  style="height: {stack.height}px"
  bind:this={box}
  data-holding={holding}
  on:pointerenter={pointerEnter}
  on:pointerleave={(e) => e.pointerType === 'mouse' && hold.pointerOut()}
  on:focusin={hold.focusIn}
  on:focusout={focusOut}
>
  {#each order as t, i (t.id)}
    {@const p = stack.places[t.id]}
    {@const g = artFor(t)}
    <div
      class="slot"
      class:back={p.depth > 0}
      class:hidden={!p.shown}
      style="transform: translateY({away * p.y}px) scale({p.scale}); z-index: {300 - i}; --depth: {p.depth}"
    >
      <!-- svelte-ignore a11y-no-static-element-interactions -->
      <div
        class="toast k-{t.kind}"
        class:one-line={!t.detail}
        out:leave={{ id: t.id, dir: away }}
        on:pointerdown={(e) => grab(e, t, i)}
        on:pointermove={pull}
        on:pointerup={letGo}
        on:pointercancel={letGo}
        on:keydown={(e) => keys(e, t)}
      >
        <div class="row">
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

          <div class="words" title={t.detail ? `${t.title}\n${t.detail}` : t.title}>
            <b>{t.title}</b>
            {#if t.detail}<span class="detail">{t.detail}</span>{/if}
          </div>

          {#if button(t) === 'retry'}
            <button class="pill" on:click={() => retry(t)}><RotateCw size={13} />{$i18n('toast.retry')}</button>
          {:else if button(t) === 'action'}
            <button class="pill" on:click={() => act(t)}>{#if t.undo}<Undo2 size={14} />{/if}{t.action.label}</button>
          {/if}
          <button class="close" aria-label={$i18n('common.dismiss')} title={$i18n('common.dismiss')} on:click={() => toasts.close(t.id)}><X size={15} /></button>
        </div>

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
    --toast-rim: rgba(255, 255, 255, 0.13);
    /* How much darker each card further back is. */
    --dusk: rgba(4, 4, 8, 0.34);
    /* Every toast's height (lib/toaststack.js stacks them by it). */
    --toast-h: 60px;
    /* One curve for the stack moving: quick, settling with a touch of give. */
    --settle: cubic-bezier(0.2, 1.12, 0.32, 1);
    position: absolute;
    left: 50%;
    bottom: 16px;
    z-index: 100;
    width: min(400px, calc(100% - 32px));
    transform: translateX(-50%);
    pointer-events: none;
    transition: height 0.45s var(--settle);
  }
  :global([data-theme='light']) .toasts {
    --toast-bg: #fff;
    --toast-edge: rgba(20, 20, 40, 0.09);
    --toast-lift: rgba(255, 255, 255, 0);
    --toast-shade: rgba(20, 20, 40, 0.16);
    --pill: rgba(20, 20, 40, 0.055);
    --pill-hover: rgba(20, 20, 40, 0.1);
    --toast-rim: rgba(20, 20, 40, 0.14);
    --dusk: rgba(60, 60, 80, 0.16);
  }
  .toasts.floating {
    position: fixed;
    bottom: calc(var(--statusbar-h) + 16px);
  }
  /* A dialog is open: the top of the window, the stack hanging down. */
  .toasts.top {
    position: fixed;
    top: 14px;
    bottom: auto;
    z-index: 120;
  }

  /* Each card's place in the stack, from lib/toaststack.js. */
  .slot {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    transform-origin: 50% 100%;
    pointer-events: auto;
    transition:
      transform 0.5s var(--settle),
      opacity 0.3s ease;
  }
  .top .slot {
    top: 0;
    bottom: auto;
    transform-origin: 50% 0;
  }
  .slot.hidden {
    opacity: 0;
    pointer-events: none;
  }

  .toast {
    --kind: var(--accent-text);
    position: relative;
    box-sizing: border-box;
    height: var(--toast-h);
    border-radius: 16px;
    background: linear-gradient(180deg, var(--toast-lift), transparent 70%), var(--toast-bg);
    background: linear-gradient(180deg, var(--toast-lift), transparent 70%), color-mix(in srgb, var(--toast-bg) 90%, transparent);
    /* Small, so cheap: the page shows through, softened, and the toast
       reads as a layer above it. */
    -webkit-backdrop-filter: blur(16px) saturate(1.4);
    backdrop-filter: blur(16px) saturate(1.4);
    border: 1px solid var(--toast-edge);
    box-shadow:
      0 -6px 18px -10px var(--toast-shade),
      0 1px 2px var(--toast-shade),
      0 12px 32px -10px var(--toast-shade),
      0 28px 56px -24px var(--toast-shade);
    overflow: hidden;
    font-size: var(--text-base);
    touch-action: pan-y;
    user-select: none;
    /* Grown and tilted from where it stands on the edge, as the stack is. */
    transform-origin: 50% 100%;
    transition: height 0.45s var(--settle);
    animation: toast-in 0.55s cubic-bezier(0.2, 1.25, 0.35, 1);
  }
  .top .toast {
    transform-origin: 50% 0;
    animation-name: toast-in-top;
  }
  /* In: up from the edge, a little small, settling in front of the rest.
     Movement only — it reads from its first frame. */
  @keyframes toast-in {
    from {
      transform: translateY(30px) scale(0.9);
    }
  }
  @keyframes toast-in-top {
    from {
      transform: translateY(-30px) scale(0.9);
    }
  }
  /* Further back, darker: a veil over the card, deeper per step. */
  .toast::after {
    content: '';
    position: absolute;
    inset: 0;
    border-radius: inherit;
    background: var(--dusk);
    opacity: calc((var(--depth) - var(--lift, 0)) * 0.42);
    pointer-events: none;
    transition: opacity 0.4s ease;
  }
  .k-success {
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

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 100%;
    box-sizing: border-box;
    padding: 0 8px 0 9px;
    transition: opacity 0.25s ease;
  }
  /* Behind the front: only the edge shows, so nothing on it is read — and
     the edge is drawn a little brighter, so the stack reads as one. */
  .back .row,
  .back .bar {
    opacity: var(--lift, 0);
  }
  .back .toast {
    border-color: var(--toast-rim);
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
    animation: art-in 0.45s cubic-bezier(0.3, 1.3, 0.5, 1);
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
    animation: mark-in 0.45s cubic-bezier(0.3, 1.6, 0.5, 1) 0.14s backwards;
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
  /* Two lines, always: a title and a second line, each cut short — or a
     title alone, given both. */
  .words b {
    font-weight: 600;
    line-height: 1.3;
    letter-spacing: -0.005em;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }
  .one-line .words b {
    white-space: normal;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow-wrap: anywhere;
  }
  .detail {
    font-size: var(--text-md);
    line-height: 1.35;
    color: var(--text-dim);
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
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

  /* Along the foot: how much time it has left, shrinking toward its start —
     or, for something under way, how far it has come. */
  .bar {
    position: absolute;
    left: 0;
    right: 0;
    bottom: 0;
    height: 2px;
    pointer-events: none;
    transition: opacity 0.25s ease;
  }
  .bar.timer {
    background: var(--kind);
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
