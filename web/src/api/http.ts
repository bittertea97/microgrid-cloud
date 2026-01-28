import axios from 'axios'

export const http = axios.create({
  baseURL: '/api',
  timeout: 15000
})

// BEGIN AGENT-1 AUTH INTERCEPTORS
// END AGENT-1 AUTH INTERCEPTORS
