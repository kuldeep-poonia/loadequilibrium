const trimRight = value => String(value || '').replace(/\/+$/, '')

const httpOrigin = trimRight(import.meta.env.VITE_BACKEND_HTTP_ORIGIN)
const wsOrigin = trimRight(import.meta.env.VITE_BACKEND_WS_ORIGIN)
const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'

export const API_BASE = `${httpOrigin || location.origin}/api/v1`
export const WS_URL = `${wsOrigin || `${wsProtocol}//${location.host}`}/ws`
