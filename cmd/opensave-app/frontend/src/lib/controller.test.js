import { describe, expect, it } from 'vitest';
import { controllerOn, pageStep } from './controller.js';

describe('controllerOn', () => {
  it('is on when set, off when set, and otherwise on by itself where it is wanted', () => {
    expect(controllerOn('on')).toBe(true);
    expect(controllerOn('off', { deviceType: 'deck', used: true })).toBe(false);
    expect(controllerOn('auto')).toBe(false);
    expect(controllerOn('auto', { used: true })).toBe(true);
    expect(controllerOn('auto', { deviceType: 'deck' })).toBe(true);
    expect(controllerOn('auto', { deviceType: 'handheld' })).toBe(true);
    expect(controllerOn('auto', { deviceType: 'desktop' })).toBe(false);
  });
});

describe('pageStep', () => {
  it('goes round the sidebar either way', () => {
    expect(pageStep('home', 1)).toBe('devices');
    expect(pageStep('home', -1)).toBe('changelog');
    expect(pageStep('changelog', 1)).toBe('home');
    // From a page not in the sidebar (a game's), as if from Home.
    expect(pageStep('game', 1)).toBe('devices');
  });
});
