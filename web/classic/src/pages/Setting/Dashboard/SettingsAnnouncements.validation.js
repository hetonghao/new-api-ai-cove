export const ANNOUNCEMENT_CONTENT_LIMIT = 3000
export const ANNOUNCEMENT_CONTENT_LIMIT_ERROR = '公告内容不能超过3000个字符'

export function getAnnouncementContentLength(content) {
  return Array.from(content).length
}

export function validateAnnouncementContentBeforeSave(content) {
  if (getAnnouncementContentLength(content) > ANNOUNCEMENT_CONTENT_LIMIT) {
    return ANNOUNCEMENT_CONTENT_LIMIT_ERROR
  }
  return null
}
