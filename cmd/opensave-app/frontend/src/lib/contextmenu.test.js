import { describe, expect, it } from 'vitest';
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
