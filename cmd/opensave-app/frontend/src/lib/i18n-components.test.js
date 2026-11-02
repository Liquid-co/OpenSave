import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { createServer } from 'vite';

let server;
let AppearanceOptions;
let locale;

beforeAll(async () => {
  server = await createServer({ server: { middlewareMode: true }, appType: 'custom' });
  AppearanceOptions = (await server.ssrLoadModule('/src/views/settings/AppearanceOptions.svelte')).default;
  locale = (await server.ssrLoadModule('/src/lib/i18n.js')).locale;
});
afterAll(async () => server?.close());

describe('language picker rendering', () => {
  it('keeps language names distinct from theme names in every locale', () => {
    for (const language of ['en', 'zh-Hans', 'zh-Hant']) {
      locale.set(language);
      const { html } = AppearanceOptions.render();
      expect(html).toMatch(/value="en"[^>]*>English<\/option>/);
      expect(html).toMatch(/value="zh-Hans"[^>]*>简体中文<\/option>/);
      expect(html).toMatch(/value="zh-Hant"[^>]*>繁體中文<\/option>/);
      expect(html).not.toContain('appearance.theme.');
    }
  });
});
