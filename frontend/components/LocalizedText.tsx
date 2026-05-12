'use client'

import { useLang } from '@/lib/locale'

interface LocalizedTextProps {
  en: string
  pt?: string | null
  /** HTML element to render. Defaults to span. */
  as?: 'span' | 'p' | 'h1' | 'h2' | 'h3'
  className?: string
}

/**
 * Renders a string that reacts to the user's language preference.
 * Use this in RSC pages to avoid prop-drilling display name logic
 * when the page layout itself is a Server Component.
 *
 * SEO note: `generateMetadata` in the RSC page should always use the
 * EN name directly, not this component. This component only affects
 * the visible UI rendering.
 */
export function LocalizedText({ en, pt, as: Tag = 'span', className }: LocalizedTextProps) {
  const { t } = useLang()
  return <Tag className={className}>{t(en, pt)}</Tag>
}
