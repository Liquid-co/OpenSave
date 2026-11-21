// Settings is a menu over the app, not a page (the SettingsV3 board): opened
// on top of wherever you are, and closed back to it. Its sections, which one
// is open, and finding a setting by what it is called.
import { writable } from 'svelte/store';

/** The sections, in the order the menu lists them. */
export const SECTIONS = ['general', 'appearance', 'sync', 'devices', 'cloud', 'notifications', 'storage', 'controller', 'advanced', 'about'];

// The old page's tabs, which links around the app (and a notification's
// saved destination) still name.
const FROM_TAB = { general: 'general', sync: 'sync', storage: 'storage', advanced: 'advanced', support: 'about' };

/** Which section a request means: a section, an old tab, or the first. */
export function sectionFor(params = {}) {
  const asked = params?.section ?? params?.tab;
  if (SECTIONS.includes(asked)) return asked;
  return FROM_TAB[asked] ?? 'general';
}

/** null while closed; {section, focus} while open. `focus` is a setting's
 *  id to bring into view (a search result, a link to one setting). */
export const settingsMenu = writable(null);

export function openSettings(params = {}) {
  settingsMenu.set({ section: sectionFor(params), focus: params?.focus ?? null });
}
export function closeSettings() {
  settingsMenu.set(null);
}

/**
 * Every setting, by section, as the catalog names it: what search looks
 * through. Each id is also the row's anchor (`set-<id>`), so a result can
 * take you to it.
 */
export const SETTINGS_INDEX = [
  ['general', 'language', 'settings.language'],
  ['general', 'deviceName', 'settings.general.deviceName'],
  ['general', 'deviceType', 'settings.general.deviceType', 'settings.general.typeHint'],
  ['general', 'deviceId', 'settings.general.deviceId', 'settings.general.deviceIdHint'],
  ['general', 'startOnBoot', 'settings.general.startOnBoot', 'settings.general.startOnBootHint'],
  ['general', 'betas', 'settings.general.offerBetas', 'settings.general.betasStable'],
  ['general', 'setupGuide', 'settings.general.setupGuide', 'settings.general.setupGuideHint'],

  ['appearance', 'theme', 'settings.appearance.themes', 'settings.appearance.themesHint'],
  ['appearance', 'lightDark', 'settings.appearance.lightDark', 'settings.appearance.lightDarkHint'],
  ['appearance', 'accent', 'settings.appearance.accent'],
  ['appearance', 'coverColours', 'settings.appearance.coverColours', 'settings.appearance.coverColoursHint'],
  ['appearance', 'backdrop', 'settings.appearance.backdrop'],
  ['appearance', 'backdropCycle', 'settings.appearance.backdropCycle', 'settings.appearance.backdropCycleHint'],
  ['appearance', 'sidebar', 'settings.appearance.sidebar'],
  ['appearance', 'size', 'settings.appearance.size'],
  ['appearance', 'animations', 'settings.appearance.animations', 'settings.appearance.animationsHint'],
  ['appearance', 'library', 'settings.general.library', 'settings.general.libraryHint'],

  ['sync', 'autoSyncOnTrack', 'settings.sync.autoSyncOnTrack'],
  ['sync', 'detectNewGames', 'settings.sync.detectNewGames', 'settings.sync.detectNewGamesHint'],
  ['sync', 'matchByAppId', 'settings.sync.matchByAppId', 'settings.sync.matchByAppIdHint'],
  ['sync', 'unknownGame', 'settings.sync.unknownGameLabel', 'settings.sync.unknownGameHint'],
  ['sync', 'speedLimit', 'settings.sync.bandwidthLimit', 'settings.sync.speedHint'],

  ['devices', 'relayUrl', 'settings.sync.relayUrl'],
  ['devices', 'hostRelay', 'settings.sync.hostRelay', 'settings.sync.hostRelayHint'],

  ['cloud', 'cloudBackup', 'home.cloudBackup', 'settings.sync.cloudBackupHint'],

  ['notifications', 'notifications', 'settings.general.notifications', 'settings.notifications.bellHint'],
  ['notifications', 'desktopNotes', 'settings.notifications.desktopCheckbox', 'settings.notifications.desktopHint'],
  ['notifications', 'quietWhilePlaying', 'settings.notifications.quietCheckbox', 'settings.notifications.quietHint'],

  ['storage', 'spaceUsed', 'settings.storage.spaceUsed'],
  ['storage', 'canRestore', 'settings.storage.canRestore'],
  ['storage', 'snapshotsFolder', 'settings.storage.snapshotsFolder', 'settings.storage.snapshotsFolderHint'],
  ['storage', 'safetyFolder', 'settings.storage.safetyFolder', 'settings.storage.safetyFolderHint'],
  ['storage', 'autoDelete', 'settings.storage.autoDelete', 'settings.storage.autoDeleteHint'],
  ['storage', 'maxAuto', 'settings.storage.maxAuto', 'settings.storage.maxAutoHint'],
  ['storage', 'maxManual', 'settings.storage.maxManual'],
  ['storage', 'applyLimits', 'settings.storage.applyLimits', 'settings.storage.applyLimitsHint'],

  ['controller', 'controller', 'settings.general.controller', 'settings.controller.hint'],

  ['advanced', 'port', 'settings.advanced.daemonPort', 'settings.advanced.daemonPortHint'],
  ['advanced', 'pathTranslation', 'settings.advanced.pathTranslation', 'settings.advanced.pathTranslationHint'],
  ['advanced', 'scanPaths', 'settings.storage.scanPaths', 'settings.storage.scanPathsHint'],
  ['advanced', 'excludePaths', 'settings.storage.excludePaths', 'settings.storage.excludePathsHint'],
  ['advanced', 'reset', 'settings.reset.title', 'settings.reset.hint'],

  ['about', 'version', 'settings.about.version'],
  ['about', 'support', 'settings.support.title', 'settings.support.lede'],
  ['about', 'discord', 'settings.about.discord']
].map(([section, id, label, hint = '']) => ({ section, id, label, hint }));

// Whether `text` has a word starting with `word`: "port" finds "Daemon
// port", not "Support". Writing without spaces between words (Chinese,
// Japanese) is matched anywhere.
const SPACELESS = /[぀-ヿ㐀-鿿가-힯]/;
const escape = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
function hasWord(text, word) {
  if (SPACELESS.test(word)) return text.includes(word);
  return new RegExp(String.raw`(^|[^\p{L}\p{N}])` + escape(word), 'u').test(text);
}

/**
 * The settings whose name, explanation or section has every word of `query`
 * — at the start of one of its words — those matching by name first, then
 * in the menu's order. `t` translates a catalog key: search reads what is
 * on the screen, in the language it is in.
 */
export function searchSettings(query, t, index = SETTINGS_INDEX) {
  const words = String(query ?? '')
    .toLocaleLowerCase()
    .split(/\s+/)
    .filter(Boolean);
  if (!words.length) return [];
  const found = [];
  index.forEach((entry, order) => {
    const name = t(entry.label).toLocaleLowerCase();
    const rest = `${entry.hint ? t(entry.hint) : ''} ${t(`settings.section.${entry.section}`)}`.toLocaleLowerCase();
    if (!words.every((w) => hasWord(name, w) || hasWord(rest, w))) return;
    const byName = words.every((w) => hasWord(name, w));
    found.push({ ...entry, byName, order });
  });
  return found.sort((a, b) => Number(b.byName) - Number(a.byName) || a.order - b.order).map(({ byName, order, ...e }) => e);
}
