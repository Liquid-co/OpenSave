import { describe, expect, it } from 'vitest';
import { nextInDirection } from './spatial.js';

const box = (left, top, w = 100, h = 60) => ({ left, top, right: left + w, bottom: top + h });

describe('nextInDirection', () => {
  // A sidebar entry at the left, and a row of three tiles with a second row.
  const sidebar = box(0, 200, 200, 30);
  const t1 = box(300, 100);
  const t2 = box(420, 100);
  const t3 = box(540, 100);
  const t4 = box(300, 200);
  const rects = [sidebar, t1, t2, t3, t4];

  it('goes along the row, and down to the one below', () => {
    expect(nextInDirection(rects, t1, 'right')).toBe(2);
    expect(nextInDirection(rects, t2, 'right')).toBe(3);
    expect(nextInDirection(rects, t1, 'down')).toBe(4);
    expect(nextInDirection(rects, t4, 'up')).toBe(1);
  });

  it('prefers straight ahead over nearer but off to the side', () => {
    // From the tile below, left goes to the sidebar entry level with it.
    expect(nextInDirection(rects, t4, 'left')).toBe(0);
    // From the first tile, left also reaches the sidebar: nothing else is that way.
    expect(nextInDirection(rects, t1, 'left')).toBe(0);
  });

  it('says when nothing lies that way', () => {
    expect(nextInDirection(rects, t3, 'right')).toBe(-1);
    expect(nextInDirection(rects, t1, 'up')).toBe(-1);
  });

  it('does not reach round a corner at the end of a row', () => {
    // A toolbar button above the row, over the last tile's right half: right
    // from the last tile is not there.
    const toolbar = box(600, 40, 60, 30);
    const withBar = [...rects, toolbar];
    expect(nextInDirection(withBar, t3, 'right')).toBe(-1);
    // Nor one well above and off to the right, further than it is along.
    const far = box(680, 0, 40, 20);
    expect(nextInDirection([...rects, far], t3, 'right')).toBe(-1);
    // But one ahead and a little higher is still that way.
    const near = box(700, 60, 40, 20);
    expect(nextInDirection([...rects, near], t3, 'right')).toBe(5);
    // And up from the last tile does reach the toolbar button.
    expect(nextInDirection(withBar, t3, 'up')).toBe(5);
  });

  it('goes down from a tab to the wide box below it, not across to the sidebar', () => {
    // A game page: a tab at the left of the page, a comment box the width of
    // it below, and a sidebar button off to the left a little further down.
    const tab = box(280, 164, 110, 37);
    const comment = box(295, 236, 800, 32);
    const sidebarAdd = box(209, 346, 24, 24);
    expect(nextInDirection([tab, comment, sidebarAdd], tab, 'down')).toBe(1);
  });

  it('among controls level with it, takes the one most in line', () => {
    const wide = box(1100, 236, 135, 34);
    const restore = box(1110, 340, 63, 30);
    const del = box(1180, 340, 45, 30);
    expect(nextInDirection([wide, restore, del], wide, 'down')).toBe(1);
  });
});
