// Overwritten at container start by docker-entrypoint.sh so one image can point
// at any backend. Empty here means "fall back to VITE_API_URL, then origin".
window.__ARBI_CONFIG__ = { apiUrl: '' }
