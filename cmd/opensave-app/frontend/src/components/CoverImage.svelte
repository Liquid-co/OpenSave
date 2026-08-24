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

  // Blob URLs are held by the browser until released, and a scan grid builds
  // hundreds of these.
  const release = () => {
    if (objectUrl) URL.revokeObjectURL(objectUrl);
    objectUrl = '';
  };
  onDestroy(release);

  async function load(url) {
    if (!url) {
      release();
      failed = false;
      explicit = false;
      return;
    }
    if (loadedFor === url) return;
    loadedFor = url;
    try {
      const res = await fetch(url);
      if (!res.ok) {
        // 404 is ordinary: this game has no cover anywhere. The tile falls
        // back to whatever the parent draws underneath.
        release();
        failed = true;
        return;
      }
      explicit = res.headers.get('X-Cover-Explicit') === '1';
      const blob = await res.blob();
      release();
      objectUrl = URL.createObjectURL(blob);
      failed = false;
    } catch {
      release();
      failed = true;
    }
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
