export const FINISH_LABEL: Record<string, string> = {
  normal: 'Normal',
  holo: 'Holo',
  reverse_holo: 'Reverse Holo',
  cosmos_holo: 'Cosmos Holo',
  galaxy_holo: 'Galaxy Holo',
  textured: 'Textured',
  gold_etched: 'Gold Etched',
  master_ball_mirror: 'Master Ball Mirror',
  poke_ball_mirror: 'Poke Ball Mirror',
  first_edition: '1ª Edição',
  shadowless: 'Shadowless',
  unlimited: 'Unlimited',
}

/** Returns a human-readable label for a card variant.
 *  Priority: explicit label > promo > finish ENUM translation. */
export function finishLabel(finish: string, label?: string | null, isPromo?: boolean): string {
  if (label && label.length > 0) return label
  if (isPromo) return 'Promo'
  return FINISH_LABEL[finish] ?? finish
}
