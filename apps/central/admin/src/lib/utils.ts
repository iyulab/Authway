import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import { intlLocale } from "@/i18n"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

// A value that is no date shows as a dash rather than failing the page.
const formatWith = (date: string | Date, options?: Intl.DateTimeFormatOptions) => {
  const value = new Date(date)
  return Number.isNaN(value.getTime()) ? '-' : new Intl.DateTimeFormat(intlLocale(), options).format(value)
}

/** A date and time in the console's current language. */
export function formatDate(date: string | Date) {
  return formatWith(date, {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  })
}

/** A date (no time) in the console's current language. */
export function formatDay(date: string | Date) {
  return formatWith(date)
}

/** A date and time, short, in the console's current language. */
export function formatDateTime(date: string | Date) {
  return formatWith(date, { dateStyle: "short", timeStyle: "medium" })
}

/** A number in the console's current language. */
export function formatNumber(value: number) {
  return new Intl.NumberFormat(intlLocale()).format(value)
}
