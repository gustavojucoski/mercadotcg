'use client'

import { useState, useEffect } from 'react'
import { PokemonSet } from './types'

let cache: PokemonSet[] | null = null

export function useSets() {
  const [sets, setSets] = useState<PokemonSet[]>(cache ?? [])
  const [loading, setLoading] = useState(cache === null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (cache !== null) return
    fetch('https://api.pokemontcg.io/v2/sets?orderBy=-releaseDate&select=id,name,ptcgoCode,releaseDate,series')
      .then(r => r.json())
      .then(data => {
        const list: PokemonSet[] = (data.data ?? []).filter((s: PokemonSet) => s.ptcgoCode)
        cache = list
        setSets(list)
        setLoading(false)
      })
      .catch((err: Error) => {
        setError(err.message)
        setLoading(false)
      })
  }, [])

  return { sets, loading, error }
}
