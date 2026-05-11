'use client'

import { useEffect } from 'react'
import { useParams, useRouter } from 'next/navigation'

export default function StoreIndexPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()

  useEffect(() => {
    router.replace(`/lojas/${id}/perfil`)
  }, [id, router])

  return null
}
