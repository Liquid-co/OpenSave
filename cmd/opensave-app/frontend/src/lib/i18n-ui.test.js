import { describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { locale, t, tr } from './i18n.js';
import { gameStatus, librarySummary } from './gamestatus.js';
import { describeSummary } from './restore.js';
import { timeAgo } from './timeago.js';
import { plural } from './format.js';
import { contentsLabel } from './scan.js';

describe('localized presentation helpers', () => {
  it('switches live between Chinese scripts and English without changing state ids', () => {
    for (const [language, label] of [['zh-Hans', '存档文件夹不存在'], ['zh-Hant', '存檔資料夾不存在'], ['en', 'Save folder missing']]) {
      locale.set(language);
      expect(gameStatus({ savePathMissing: true })).toMatchObject({ state: 'missing', label });
    }
  });

  it('keeps English plural behavior and uses Chinese count units', () => {
    expect(plural(1, 'file')).toBe('1 file');
    expect(plural(3, 'file')).toBe('3 files');
    locale.set('zh-Hans');
    expect(plural(1, 'file')).toBe('1 个文件');
    expect(plural(3, 'file')).toBe('3 个文件');
    expect(contentsLabel({ measured: true, fileCount: 2, totalBytes: 10, latestMtime: 100 }, 100000)).toBe('2 个文件 · 10 B · 刚刚');
  });

  it('translates restore consequences without leaking English grammar', () => {
    locale.set('zh-Hans');
    expect(describeSummary({ changed: 2, restored: 1, removed: 3 })).toBe('2 个文件将改变, 1 个文件将恢复, 3 个文件将移除');
    expect(timeAgo(null)).toBe('从未');
    expect(librarySummary([{ game: {}, status: { state: 'empty' } }]).headline).toBe('1 款游戏尚无快照');
  });

  it('keeps names, paths and error details as interpolation data', () => {
    locale.set('zh-Hans');
    const name = 'My {game} <save>';
    const text = tr('gameactions.stoppedTrackingItsSnapshotsStayOn', { p0: name });
    expect(text).toContain(name);
    expect(text).toContain('已停止跟踪');
    expect(get(t)('missing.translation')).toBe('missing.translation');
  });
});
