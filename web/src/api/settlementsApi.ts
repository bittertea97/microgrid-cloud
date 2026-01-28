import { http } from './http'
import type { SettlementRecord, SettlementsQueryParams } from '../types/api'

const extractSettlements = (payload: unknown): SettlementRecord[] => {
  if (Array.isArray(payload)) {
    return payload as SettlementRecord[]
  }

  if (payload && typeof payload === 'object') {
    const data = payload as { data?: unknown; items?: unknown }
    if (Array.isArray(data.data)) {
      return data.data as SettlementRecord[]
    }
    if (Array.isArray(data.items)) {
      return data.items as SettlementRecord[]
    }
  }

  return []
}

export const fetchSettlements = async (params: SettlementsQueryParams) => {
  const response = await http.get('/v1/settlements', { params })
  return extractSettlements(response.data)
}
