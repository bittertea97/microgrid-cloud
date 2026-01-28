export type SettlementRecord = {
  day_start?: string
  time_key?: string
  energy_kwh?: number
  amount?: number
  currency?: string
  version?: number | string
  updated_at?: string
  [key: string]: unknown
}

export type SettlementsQueryParams = {
  station_id?: string
  from?: string
  to?: string
}
