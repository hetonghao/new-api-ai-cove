import * as z from 'zod'

export const ANNOUNCEMENT_CONTENT_LIMIT = 3000
export const ANNOUNCEMENT_CONTENT_DESCRIPTION =
  'Maximum 3000 characters (counted by Unicode characters). Supports Markdown and HTML.'
export const ANNOUNCEMENT_CONTENT_LIMIT_ERROR =
  'Content must be 3000 characters or fewer'

export function getAnnouncementContentLength(content: string) {
  return Array.from(content).length
}

export const announcementSchema = z.object({
  content: z
    .string()
    .min(1, 'Content is required')
    .superRefine((content, ctx) => {
      if (getAnnouncementContentLength(content) > ANNOUNCEMENT_CONTENT_LIMIT) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: ANNOUNCEMENT_CONTENT_LIMIT_ERROR,
        })
      }
    }),
  publishDate: z.string().min(1, 'Publish date is required'),
  type: z.enum(['default', 'ongoing', 'success', 'warning', 'error']),
  extra: z
    .string()
    .max(100, 'Extra must be less than 100 characters')
    .optional(),
})
