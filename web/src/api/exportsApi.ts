import { http } from './http'
import type { SettlementsQueryParams } from '../types/api'

export const downloadSettlementsCsv = async (params: SettlementsQueryParams) => {
  const response = await http.get('/v1/exports/settlements.csv', {
    params,
    responseType: 'blob'
  })
  return response.data as Blob
}
