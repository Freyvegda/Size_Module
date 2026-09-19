// Formatting helpers. The API speaks micrometers; humans read millimeters.

export const MICRON_PER_MM = 1000
export const MICRON_PER_M = 1_000_000

export function micronToMm(value: number, digits = 0): string {
  return (value / MICRON_PER_MM).toFixed(digits)
}

export function micronToM(value: number, digits = 2): string {
  return (value / MICRON_PER_M).toFixed(digits)
}

export function areaM2(value: number, digits = 2): string {
  return `${value.toFixed(digits)} m²`
}

export function percent(value: number, digits = 1): string {
  return `${value.toFixed(digits)}%`
}

export function mmToMicron(mm: number): number {
  return Math.round(mm * MICRON_PER_MM)
}
