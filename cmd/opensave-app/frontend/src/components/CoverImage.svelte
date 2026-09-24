<script>
  // One cover image, blurred when the daemon says it is explicit.
  //
  // An <img src> cannot read a response header, and whether a cover is explicit
  // is only known once the daemon has answered — it comes back as
  // X-Cover-Explicit. So this fetches the bytes itself, reads the header, and
  // shows the result from a blob.
  //
  // Both screens that show covers use this, rather than each deciding for
  // itself. The blur previously existed only in the sidebar, so the same
  // picture was covered in one place and bare in the other.
  import { onDestroy } from 'svelte';

  /** Cover URL on the local daemon, or '' for no cover. */
  export let src = '';
  export let alt = '';
  /** Reveal an explicit cover — the parent decides, on hover or focus. */
  export let revealed = false;

  let objectUrl = '';
  let explicit = false;
  let failed = false;
  let loadedFor = '';
  let attempt = 0;
  let retryTimer = null;

  // Blob URLs are held by the browser until released, and a scan grid builds
  // hundreds of these.
  const release = () => {
    if (objectUrl) URL.revokeObjectURL(objectUrl);
    objectUrl = '';
  };
  const cancelRetry = () => {
    clearTimeout(retryTimer);
    retryTimer = null;
  };
  onDestroy(() => {
    release();
    cancelRetry();
  });

  // Retry delays for a cover that could not be fetched this time. The first
  // two cover the usual cause, which is over in a second; the last two reach
  // past a daemon that is restarting. Finite, because a sidebar of fifty
  // games should not keep asking forever.
  const retryDelays = [1500, 5000, 15000, 45000];

  async function load(url, isRetry = false) {
    if (!url) {
      cancelRetry();
      release();
      failed = false;
      explicit = false;
      loadedFor = '';
      attempt = 0;
      return;
    }
    // Already showing this one, and not the retry timer calling: leave
    // everything alone — including any retry still pending. This reactive
    // statement re-runs on every render, and a render happens whenever the
    // games store updates, which is often. Cancelling here (as the first
    // version of this did) meant the pending retry was thrown away by the
    // next unrelated re-render and the tile never recovered after all.
    if (loadedFor === url && !isRetry) return;
    cancelRetry();
    loadedFor = url;
    try {
      // A retry skips the browser's cache. The copy in it can be the reason
      // the first try failed: one cached from an <img> of the same URL carries
      // no CORS headers, and a fetch handed it fails exactly as a dropped
      // connection does — every time, for as long as it is cached (a week).
      // The daemon now marks covers as varying by origin, so no new copy like
      // that is made; this heals the ones made before it did.
      const res = await fetch(url, isRetry ? { cache: 'reload' } : undefined);
      if (!res.ok) {
        // 404 is ordinary: this game has no cover anywhere. The tile falls
        // back to whatever the parent draws underneath. Not retried — the
        // daemon remembers a miss for the next hour and would only answer
        // 404 again; a cover appearing later changes the App ID or the name,
        // which changes the url, which starts this over anyway.
        giveUp();
        return;
      }
      explicit = res.headers.get('X-Cover-Explicit') === '1';
      const blob = await res.blob();
      release();
      objectUrl = URL.createObjectURL(blob);
      failed = false;
      attempt = 0;
    } catch {
      // Anything that is not an answer: the daemon still starting, a dropped
      // connection, a first fetch that had to reach the network. These pass.
      //
      // Retried, because without it they did not. loadedFor is set before the
      // request, and the reactive statement below only re-runs when src
      // changes — which for a tracked game it never does. So one unlucky
      // moment left a game showing its initials for the rest of the session,
      // with the daemon serving that very cover from its disk cache the whole
      // time. Every tile fails in the same moment, so it looked like a
      // feature nobody had built.
      release();
      failed = true;
      if (attempt < retryDelays.length) {
        const delay = retryDelays[attempt++];
        retryTimer = setTimeout(() => load(url, true), delay);
      }
    }
  }

  function giveUp() {
    release();
    failed = true;
    attempt = 0;
  }

  $: load(src);
  $: hidden = failed || !objectUrl;
</script>

{#if !hidden}
  <img
    src={objectUrl}
    {alt}
    class:explicit={explicit && !revealed}
    loading="lazy"
    on:load
  />
{/if}

<style>
  /* Explicit art is blurred, not removed: the tile keeps the game's shape and
     colour so a shelf stays recognisable, while nothing in it is legible. The
     slight scale hides the soft edge a blur leaves at the border; the parent
     clips it. */
  img.explicit {
    filter: blur(14px) saturate(0.7);
    transform: scale(1.15);
  }
  img {
    transition: filter 120ms ease, transform 120ms ease;
  }
</style>
