// Backend API Client Module
import { getAccessToken } from './auth.js';
import { CONFIG } from './config.js';

// Generic API call with authentication
export async function callApi(endpoint, options = {}) {
  const url = CONFIG.apiBaseUrl + endpoint;
  const token = getAccessToken();

  const headers = {
    'Content-Type': 'application/json',
    ...options.headers
  };

  if (token) {
    headers['Authorization'] = 'Bearer ' + token;
  }

  try {
    const response = await fetch(url, {
      ...options,
      headers
    });

    if (!response.ok) {
      const errorText = await response.text();
      const errorMsg = errorText || response.statusText;
      throw new Error('API Error ' + response.status + ': ' + errorMsg);
    }

    const contentType = response.headers.get('content-type');
    if (contentType && contentType.includes('application/json')) {
      return await response.json();
    }

    return await response.text();
  } catch (error) {
    console.error('API call failed: ' + endpoint, error);
    throw error;
  }
}

// Test public endpoint
export async function testPublic() {
  return await callApi('/api/public', { method: 'GET' });
}

// Test protected endpoint
export async function testProtected() {
  return await callApi('/api/protected', { method: 'GET' });
}

// Get user profile
export async function getUserProfile() {
  return await callApi('/api/me', { method: 'GET' });
}

// Get weather data
export async function getWeather() {
  return await callApi('/api/weather', { method: 'GET' });
}

// Format response for display
export function formatResponse(data) {
  try {
    return JSON.stringify(data, null, 2);
  } catch (error) {
    return String(data);
  }
}

// Check backend health
export async function checkBackendHealth() {
  try {
    const response = await fetch(CONFIG.apiBaseUrl + '/api/public');
    return response.ok;
  } catch (error) {
    console.error('Backend not available:', error);
    return false;
  }
}
