import { describe, it, expect } from 'vitest'
import { caseDueDateCreate, caseDueDatePatch, dueDateDay, toDueDateTimestamp } from './caseDueDate'

describe('caseDueDate', () => {
  it('promotes a calendar day to midnight UTC and passes a timestamp through', () => {
    expect(toDueDateTimestamp('2030-06-30')).toBe('2030-06-30T00:00:00Z')
    expect(toDueDateTimestamp('2030-06-30T12:00:00Z')).toBe('2030-06-30T12:00:00Z')
  })

  it('builds an update patch that sets or clears', () => {
    expect(caseDueDatePatch('2030-06-30')).toEqual({ dueDate: '2030-06-30T00:00:00Z' })
    expect(caseDueDatePatch('')).toEqual({ clearDueDate: true })
    expect(caseDueDatePatch(null)).toEqual({ clearDueDate: true })
    expect(caseDueDatePatch(undefined)).toEqual({ clearDueDate: true })
  })

  it('builds a create input that sets or omits', () => {
    expect(caseDueDateCreate('2030-06-30')).toEqual({ dueDate: '2030-06-30T00:00:00Z' })
    expect(caseDueDateCreate('')).toEqual({})
    expect(caseDueDateCreate(null)).toEqual({})
  })

  it('reads a stored timestamp back as a calendar day', () => {
    expect(dueDateDay('2030-06-30T00:00:00Z')).toBe('2030-06-30')
    expect(dueDateDay(null)).toBe('')
    expect(dueDateDay(undefined)).toBe('')
  })
})
