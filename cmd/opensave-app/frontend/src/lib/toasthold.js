// Whether the toasts are being read: the pointer is on them, focus is in
// one, or the stack was tapped open (a touch screen has no pointer to rest
// on them). While they are, none of them runs out and the stack stays open
// (components/Toasts.svelte).
//
// The pointer counts as gone a short moment after it leaves, so crossing the
// gap between two toasts lets nothing go; coming back within that moment
// keeps the hold. Focus counts until it leaves the toasts.

export function createHold({ onChange, setTimer = setTimeout, clearTimer = clearTimeout, grace = 250 }) {
  let pointer = false;
  let focus = false;
  let pinned = false;
  let holding = false;
  let timer = null;

  const settle = () => {
    const now = pointer || focus || pinned;
    if (now === holding) return;
    holding = now;
    onChange(now);
  };
  const cancel = () => {
    if (timer) clearTimer(timer);
    timer = null;
  };

  return {
    /** The pointer is on a toast (entered, or moved while on one). */
    pointerIn() {
      cancel();
      pointer = true;
      settle();
    },
    /** The pointer is somewhere else: off the toasts, or out of the window. */
    pointerOut() {
      if (!pointer || timer) return;
      timer = setTimer(() => {
        timer = null;
        pointer = false;
        settle();
      }, grace);
    },
    focusIn() {
      focus = true;
      settle();
    },
    focusOut() {
      focus = false;
      settle();
    },
    /** Tapped open, or closed again by a tap anywhere else. */
    pin(on) {
      pinned = on;
      settle();
    },
    get holding() {
      return holding;
    },
    get pointer() {
      return pointer;
    },
    stop: cancel
  };
}
