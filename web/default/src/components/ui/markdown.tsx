/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import ReactMarkdown from 'react-markdown'
import rehypeRaw from 'rehype-raw'
import remarkGfm from 'remark-gfm'

import { cn } from '@/lib/utils'

interface MarkdownProps {
  breaks?: boolean
  children: string
  className?: string
}

export function Markdown(props: MarkdownProps) {
  return (
    <div
      className={cn(
        'max-w-none text-sm',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        '[overflow-wrap:anywhere] break-words',
        props.className
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        rehypePlugins={[rehypeRaw]}
        components={{
          a: ({ node, className: linkClassName, ...anchorProps }) => (
            <a
              {...anchorProps}
              target='_blank'
              rel='noopener noreferrer'
              className={cn(
                'text-primary no-underline hover:underline',
                linkClassName
              )}
            />
          ),
          h1: ({ node, className: headingClassName, ...headingProps }) => (
            <h1
              {...headingProps}
              className={cn(
                'mt-6 mb-3 text-2xl font-semibold tracking-tight',
                headingClassName
              )}
            />
          ),
          h2: ({ node, className: headingClassName, ...headingProps }) => (
            <h2
              {...headingProps}
              className={cn(
                'mt-5 mb-3 text-xl font-semibold tracking-tight',
                headingClassName
              )}
            />
          ),
          h3: ({ node, className: headingClassName, ...headingProps }) => (
            <h3
              {...headingProps}
              className={cn(
                'mt-4 mb-2 text-lg font-semibold tracking-tight',
                headingClassName
              )}
            />
          ),
          p: ({ node, className: paragraphClassName, ...paragraphProps }) => (
            <p
              {...paragraphProps}
              className={cn('my-2 leading-relaxed', paragraphClassName)}
            />
          ),
          ul: ({ node, className: listClassName, ...listProps }) => (
            <ul
              {...listProps}
              className={cn('my-2 ml-5 list-disc space-y-1', listClassName)}
            />
          ),
          ol: ({ node, className: listClassName, ...listProps }) => (
            <ol
              {...listProps}
              className={cn(
                'my-2 ml-5 list-decimal space-y-1',
                listClassName
              )}
            />
          ),
          li: ({ node, className: itemClassName, ...itemProps }) => (
            <li {...itemProps} className={cn('my-1', itemClassName)} />
          ),
          blockquote: ({
            node,
            className: blockquoteClassName,
            ...blockquoteProps
          }) => (
            <blockquote
              {...blockquoteProps}
              className={cn(
                'border-l-primary bg-muted/50 my-3 border-l-2 py-1 pl-4',
                blockquoteClassName
              )}
            />
          ),
          code: ({ node, className: codeClassName, ...codeProps }) => (
            <code
              {...codeProps}
              className={cn(
                'bg-muted rounded px-1 py-0.5 font-mono text-[0.925em]',
                codeClassName
              )}
            />
          ),
          pre: ({ node, className: preClassName, ...preProps }) => (
            <pre
              {...preProps}
              className={cn(
                'bg-muted my-3 overflow-x-auto rounded border p-3',
                preClassName
              )}
            />
          ),
          table: ({ node, className: tableClassName, ...tableProps }) => (
            <div className='my-3 overflow-x-auto'>
              <table
                {...tableProps}
                className={cn(
                  'w-full border-collapse text-sm',
                  tableClassName
                )}
              />
            </div>
          ),
          thead: ({ node, className: headClassName, ...headProps }) => (
            <thead {...headProps} className={cn('bg-muted', headClassName)} />
          ),
          th: ({ node, className: cellClassName, ...cellProps }) => (
            <th
              {...cellProps}
              className={cn(
                'border px-3 py-2 text-left font-semibold',
                cellClassName
              )}
            />
          ),
          td: ({ node, className: cellClassName, ...cellProps }) => (
            <td
              {...cellProps}
              className={cn('border px-3 py-2 align-top', cellClassName)}
            />
          ),
        }}
      >
        {props.breaks ? props.children.replaceAll('\n', '  \n') : props.children}
      </ReactMarkdown>
    </div>
  )
}
