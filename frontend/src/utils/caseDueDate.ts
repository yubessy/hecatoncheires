import { calendarDate } from './calendarDate'

// The built-in case due date travels as a full ISO timestamp at midnight UTC of
// the chosen day (the same convention as an Action's due date), and is shown
// and edited as a calendar day (YYYY-MM-DD).

export type CaseDueDateInput = { dueDate: string } | { clearDueDate: true }

// toDueDateTimestamp turns a "YYYY-MM-DD" day into the timestamp the GraphQL
// Time scalar accepts. Anything else (already a timestamp) passes through.
export function toDueDateTimestamp(day: string): string {
  return /^\d{4}-\d{2}-\d{2}$/.test(day) ? `${day}T00:00:00Z` : day
}

// caseDueDatePatch builds the updateCase / submitDraft input for a due date
// picked in the UI: a day sets it, an empty value clears it.
export function caseDueDatePatch(day: string | null | undefined): CaseDueDateInput {
  return day ? { dueDate: toDueDateTimestamp(day) } : { clearDueDate: true }
}

// caseDueDateCreate builds the createCase / createDraft input: a day sets it,
// an empty value sends nothing (a new case has no due date to clear).
export function caseDueDateCreate(day: string | null | undefined): { dueDate?: string } {
  return day ? { dueDate: toDueDateTimestamp(day) } : {}
}

// dueDateDay reads a stored due date back as the day an <input type="date">
// takes, '' when unset.
export function dueDateDay(value: string | null | undefined): string {
  return calendarDate(value)
}
