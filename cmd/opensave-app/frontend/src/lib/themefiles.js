// A theme to and from a file (lib/customthemes.js says what is in one):
// written through the system's save dialog in the app, downloaded in a
// browser; read from a file someone picks, checked before anything uses it.
import { exportTheme, importTheme, MAX_FILE_BYTES } from './customthemes.js';
import { native } from './api.js';
import { toast } from './stores.js';

const fileName = (name) => `${String(name || 'theme').replace(/[\\/:*?"<>|]+/g, '').trim() || 'theme'}.opensave-theme`;

/** Writes a theme to a file, and says where. `t` translates. */
export async function exportToFile(theme, t) {
  const file = fileName(theme?.name);
  const text = exportTheme(theme);
  try {
    const saving = native.saveThemeFile(file, text);
    if (saving) {
      const path = await saving;
      if (path) toast(t('themeEditor.exported', { path }), 'success');
      return;
    }
    const url = URL.createObjectURL(new Blob([text], { type: 'application/json' }));
    const link = Object.assign(document.createElement('a'), { href: url, download: file });
    document.body.appendChild(link);
    link.click();
    link.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    toast(t('themeEditor.downloaded', { file }), 'success');
  } catch (e) {
    toast(e?.message ?? String(e), 'error');
  }
}

/** A theme from a picked file: {theme} or {error}, a catalog key. */
export async function readThemeFile(file) {
  if (!file) return { error: 'themeEditor.import.notATheme' };
  if (file.size > MAX_FILE_BYTES) return { error: 'themeEditor.import.tooBig' };
  return importTheme(await file.text());
}
