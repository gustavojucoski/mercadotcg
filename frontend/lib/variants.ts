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
 *  If the variant has an explicit label (e.g. "Poke Ball", "League") it takes
 *  priority; otherwise the finish ENUM is translated via FINISH_LABEL. */
export function finishLabel(finish: string, label?: string | null): string {
  if (label && label.length > 0) return label
  return FINISH_LABEL[finish] ?? finish
}
