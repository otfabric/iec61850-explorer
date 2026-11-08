import type { Quality, TimeValue, Value } from '../service/types';

export interface Summary {
  text: string;
  quality?: Quality;
  time?: TimeValue;
}

const PRIMARY = ['stVal', 'mag', 'f', 'i', 'cVal', 'instMag', 'setVal', 'general'];

/**
 * The value an engineer reads first: the attribute itself, or for a data
 * object its stVal or mag.f, with the quality and time next to it.
 */
export function summarize(value: Value): Summary {
  if (!value.children || value.children.length === 0) {
    return { text: value.text, quality: value.quality, time: value.time };
  }
  const byName = new Map(value.children.map((c) => [c.name ?? '', c]));
  const quality = byName.get('q')?.quality;
  const time = byName.get('t')?.time;
  for (const name of PRIMARY) {
    const child = byName.get(name);
    if (child) {
      const inner = summarize(child);
      return { text: inner.text, quality: quality ?? inner.quality, time: time ?? inner.time };
    }
  }
  return { text: value.text, quality, time };
}
