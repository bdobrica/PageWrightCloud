// Environment configuration
export const config = {
  apiUrl: import.meta.env.VITE_PAGEWRIGHT_API_URL || 'http://localhost:8085',
  wsUrl: import.meta.env.VITE_PAGEWRIGHT_WS_URL || 'ws://localhost:8085/ws',
  defaultDomain: import.meta.env.VITE_PAGEWRIGHT_DEFAULT_DOMAIN || 'pagewright.io',
};
