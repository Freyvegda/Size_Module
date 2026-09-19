// Deterministic part colours: the same part code always gets the same hue.
export function partColor(code: string, priority = 100): string {
  let hash = 2166136261
  for (let i = 0; i < code.length; i++) {
    hash ^= code.charCodeAt(i)
    hash = Math.imul(hash, 16777619)
  }
  const hue = Math.abs(hash) % 360
  const saturation = priority <= 1 ? 72 : 52
  const lightness = 58
  // NOTE: comma syntax keeps three.js' CSS colour parser happy.
  return `hsl(${hue}, ${saturation}%, ${lightness}%)`
}
