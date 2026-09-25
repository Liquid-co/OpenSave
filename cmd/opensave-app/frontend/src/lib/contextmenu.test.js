import { afterEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { closeMenu, contextMenu, openMenu } from './contextmenu.js';

// Just enough of an element: where it is, and its data attributes.
const element = () => ({ dataset: {}, getBoundingClientRect: () => ({ left: 100, top: 40 }) });
const rightClick = (target, x = 5, y = 6) => ({ preventDefault() {}, stopPropagation() {}, currentTarget: target, clientX: x, clientY: y });

describe('the element a menu was opened on', () => {
  it('is marked while its menu is open, and only then', () => {
    const tile = element();
    openMenu(rightClick(tile), []);
    expect(get(contextMenu).anchor).toBe(tile);
    expect('menuOpen' in tile.dataset).toBe(true);
    closeMenu();
    expect('menuOpen' in tile.dataset).toBe(false);
  });

  it('passes the mark on when a menu opens on something else', () => {
    const a = element();
    const b = element();
    openMenu(rightClick(a), []);
    openMenu(rightClick(b), []);
    expect('menuOpen' in a.dataset).toBe(false);
    expect('menuOpen' in b.dataset).toBe(true);
    closeMenu();
  });

  it('opens from the keyboard at the element, which has no pointer position', () => {
    openMenu(rightClick(element(), 0, 0), []);
    expect(get(contextMenu)).toMatchObject({ x: 112, y: 52 });
    closeMenu();
  });
});

describe('focus when the menu closes', () => {
  // A stand-in document: what has focus, and the page itself.
  const body = { closest: () => null };
  const saved = globalThis.document;
  afterEach(() => {
    globalThis.document = saved;
  });
  const focusable = (inMenu = false) => {
    const el = {
      ...element(),
      isConnected: true,
      closest: (sel) => (inMenu && sel === '.menu' ? {} : null),
      focus() {
        globalThis.document.activeElement = el;
      }
    };
    return el;
  };

  it('goes back to what it was opened on, from the menu or from nowhere', () => {
    globalThis.document = { body, activeElement: body };
    const tile = focusable();
    openMenu(rightClick(tile), []);
    globalThis.document.activeElement = focusable(true); // an item in the menu
    closeMenu();
    expect(globalThis.document.activeElement).toBe(tile);

    openMenu(rightClick(tile), []);
    globalThis.document.activeElement = body; // the item went with the menu
    closeMenu();
    expect(globalThis.document.activeElement).toBe(tile);
  });

  it('stays where it is when something else took it, or the element is gone', () => {
    const search = focusable();
    globalThis.document = { body, activeElement: search };
    const tile = focusable();
    openMenu(rightClick(tile), []);
    closeMenu(); // clicked into the search box, say
    expect(globalThis.document.activeElement).toBe(search);

    const gone = { ...focusable(), isConnected: false };
    openMenu(rightClick(gone), []);
    globalThis.document.activeElement = body;
    closeMenu();
    expect(globalThis.document.activeElement).toBe(body);
  });
});
