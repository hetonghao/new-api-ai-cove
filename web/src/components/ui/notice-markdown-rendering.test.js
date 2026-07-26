import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const noticeModalSource = readFileSync(join(__dirname, 'NoticeModal.jsx'), 'utf8');
const announcementsPanelSource = readFileSync(
  join(__dirname, '../dashboard/AnnouncementsPanel.jsx'),
  'utf8',
);

test('classic system notice uses shared MarkdownRenderer instead of marked HTML injection', () => {
  assert.match(
    noticeModalSource,
    /import MarkdownRenderer from '\.\.\/common\/markdown\/MarkdownRenderer';/,
  );
  assert.match(noticeModalSource, /<MarkdownRenderer[\s\S]*content=\{noticeContent\}/);
  assert.doesNotMatch(noticeModalSource, /marked\.parse/);
  assert.doesNotMatch(noticeModalSource, /dangerouslySetInnerHTML/);
});

test('classic announcements panel uses shared MarkdownRenderer instead of marked HTML injection', () => {
  assert.match(
    announcementsPanelSource,
    /import MarkdownRenderer from '\.\.\/common\/markdown\/MarkdownRenderer';/,
  );
  assert.match(
    announcementsPanelSource,
    /<MarkdownRenderer[\s\S]*content=\{item\.content \|\| ''\}/,
  );
  assert.doesNotMatch(announcementsPanelSource, /marked\.parse/);
  assert.doesNotMatch(announcementsPanelSource, /dangerouslySetInnerHTML/);
});
