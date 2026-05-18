import type { MetadataRoute } from 'next'
import { fetchSets } from '@/lib/catalog'

const BASE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? 'https://mercadotcg.com.br'

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const entries: MetadataRoute.Sitemap = [
    {
      url: BASE_URL,
      lastModified: new Date(),
      changeFrequency: 'daily',
      priority: 1,
    },
    {
      url: `${BASE_URL}/sets`,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 0.9,
    },
    {
      url: `${BASE_URL}/sets/pokemon`,
      lastModified: new Date(),
      changeFrequency: 'weekly',
      priority: 0.8,
    },
  ]

  try {
    const data = await fetchSets('pokemon', { page: 1, limit: 200 })
    for (const set of data.sets) {
      entries.push({
        url: `${BASE_URL}/sets/pokemon/${set.code}`,
        lastModified: set.release_date ? new Date(set.release_date) : new Date(),
        changeFrequency: 'monthly',
        priority: 0.6,
      })
    }
  } catch {
    // sitemap degrades gracefully when API is unavailable
  }

  return entries
}
