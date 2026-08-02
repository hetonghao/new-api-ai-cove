import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const notificationPopoverSource = readFileSync(
  join(__dirname, '../notification-popover.tsx'),
  'utf8'
)
const announcementDetailSource = readFileSync(
  join(
    __dirname,
    '../../features/dashboard/components/overview/announcement-detail-dialog.tsx'
  ),
  'utf8'
)

test('system notice and announcements use shared RichContent rendering', () => {
  assert.match(
    notificationPopoverSource,
    /import\('@\/components\/rich-content'\)[\s\S]*module\.RichContent/
  )
  assert.match(
    notificationPopoverSource,
    /<RichContent breaks content=\{notice\} \/>/
  )
  assert.match(
    notificationPopoverSource,
    /<RichContent breaks content=\{item\.content \|\| ''\} \/>/
  )
  assert.doesNotMatch(notificationPopoverSource, /marked\.parse/)
  assert.doesNotMatch(notificationPopoverSource, /dangerouslySetInnerHTML/)
})

test('announcement details use shared RichContent rendering', () => {
  assert.match(
    announcementDetailSource,
    /import \{ RichContent \} from '@\/components\/rich-content'/
  )
  assert.match(
    announcementDetailSource,
    /<RichContent breaks content=\{announcement\.content\} \/>/
  )
  assert.doesNotMatch(announcementDetailSource, /marked\.parse/)
  assert.doesNotMatch(announcementDetailSource, /dangerouslySetInnerHTML/)
})
