const DEFAULT_API_BASE_URL = 'http://localhost:8080';

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, '');
}

function toWebSocketUrl(apiBaseUrl: string) {
  if (apiBaseUrl.startsWith('https://')) {
    return `wss://${apiBaseUrl.slice('https://'.length)}/ws`;
  }
  if (apiBaseUrl.startsWith('http://')) {
    return `ws://${apiBaseUrl.slice('http://'.length)}/ws`;
  }
  return `ws://${apiBaseUrl}/ws`;
}

export const API_BASE_URL = trimTrailingSlash(
  process.env.NEXT_PUBLIC_API_BASE_URL || DEFAULT_API_BASE_URL
);

export const WS_URL = trimTrailingSlash(
  process.env.NEXT_PUBLIC_WS_URL || toWebSocketUrl(API_BASE_URL)
);
