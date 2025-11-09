// Authway SPA Sample - Main Application (Vanilla JS)
import './style.css';
import {
  initializeAuth,
  login,
  loginWithPopup,
  logout,
  handleCallback,
  isAuthenticated,
  getUser,
  getTokens,
  getTokenExpiry,
  getDynamicClaims,
  updateDynamicClaim,
  deleteDynamicClaim
} from './auth.js';
import {
  testPublic,
  testProtected,
  getUserProfile,
  getWeather,
  formatResponse,
  checkBackendHealth
} from './api.js';

// DOM elements - Login
const loginView = document.getElementById('login-view');
const userView = document.getElementById('user-view');
const popupLoginBtn = document.getElementById('popup-login-btn');
const redirectLoginBtn = document.getElementById('redirect-login-btn');
const logoutBtn = document.getElementById('logout-btn');

// DOM elements - User header
const userBasicInfo = document.getElementById('user-basic-info');

// DOM elements - Tabs
const tabBtns = document.querySelectorAll('.tab-btn');
const tabContents = document.querySelectorAll('.tab-content');

// DOM elements - Profile tab
const profileDetails = document.getElementById('profile-details');

// DOM elements - Claims tab
const loadClaimsBtn = document.getElementById('load-claims-btn');
const addClaimBtn = document.getElementById('add-claim-btn');
const addClaimForm = document.getElementById('add-claim-form');
const claimKeyInput = document.getElementById('claim-key-input');
const claimValueInput = document.getElementById('claim-value-input');
const saveClaimBtn = document.getElementById('save-claim-btn');
const cancelClaimBtn = document.getElementById('cancel-claim-btn');
const claimsList = document.getElementById('claims-list');

// DOM elements - Tokens tab
const tokenDetails = document.getElementById('token-details');

// DOM elements - API test buttons
const testPublicBtn = document.getElementById('test-public-btn');
const testProtectedBtn = document.getElementById('test-protected-btn');
const testMeBtn = document.getElementById('test-me-btn');
const testWeatherBtn = document.getElementById('test-weather-btn');

// DOM elements - API result containers
const publicResult = document.getElementById('public-result');
const protectedResult = document.getElementById('protected-result');
const meResult = document.getElementById('me-result');
const weatherResult = document.getElementById('weather-result');

// Initialize application
async function initApp() {
  console.log('🚀 Initializing Authway SPA Sample (Vanilla JS)...');

  try {
    // Initialize OAuth2 configuration
    await initializeAuth();

    // Check if this is an OAuth callback
    if (window.location.search.includes('code=')) {
      console.log('🔄 Processing OAuth callback...');
      try {
        await handleCallback();
        console.log('✅ Authentication successful!');
      } catch (error) {
        console.error('❌ Callback handling failed:', error);
        displayError('Authentication failed: ' + error.message);
      }
    }

    // Update UI based on authentication state
    updateUI();

    // Check backend health
    const backendHealthy = await checkBackendHealth();
    if (!backendHealthy) {
      console.warn('⚠️  Backend API is not responding. Make sure backend is running on http://localhost:5222');
    }

  } catch (error) {
    console.error('❌ App initialization failed:', error);
    displayError('Failed to initialize: ' + error.message);
  }
}

// Update UI based on authentication state
function updateUI() {
  if (isAuthenticated()) {
    // Show authenticated view
    loginView.style.display = 'none';
    userView.style.display = 'block';

    // Display user basic info in header
    const user = getUser();
    const expiry = getTokenExpiry();

    let expiryText = '';
    if (expiry && !expiry.isExpired) {
      const minutes = Math.floor(expiry.expiresIn / 60);
      expiryText = `<span class="token-expiry">Token expires in ${minutes}m</span>`;
    }

    userBasicInfo.innerHTML = `
      <div class="user-info-compact">
        <span class="user-name">👤 ${user.name || user.email || user.sub}</span>
        ${expiryText}
      </div>
    `;

    // Update profile tab
    updateProfileTab();

    // Update token tab
    updateTokenTab();

    console.log('✅ User authenticated:', user.sub);
  } else {
    // Show login view
    loginView.style.display = 'block';
    userView.style.display = 'none';
    console.log('ℹ️  User not authenticated');
  }
}

// Update profile tab
function updateProfileTab() {
  const user = getUser();
  if (!user) return;

  const profileHtml = Object.entries(user)
    .map(([key, value]) => {
      const displayValue = typeof value === 'object' ? JSON.stringify(value, null, 2) : value;
      return `
        <div class="profile-item">
          <strong>${key}:</strong>
          <span>${displayValue}</span>
        </div>
      `;
    })
    .join('');

  profileDetails.innerHTML = profileHtml;
}

// Update token tab
function updateTokenTab() {
  const tokens = getTokens();
  const expiry = getTokenExpiry();

  if (!tokens) {
    tokenDetails.innerHTML = '<p>No tokens available</p>';
    return;
  }

  const expiryInfo = expiry
    ? `
      <div class="token-item">
        <strong>Expires At:</strong>
        <span>${expiry.expiresAt.toLocaleString()}</span>
      </div>
      <div class="token-item">
        <strong>Expires In:</strong>
        <span>${Math.floor(expiry.expiresIn / 60)} minutes ${expiry.expiresIn % 60} seconds</span>
      </div>
      <div class="token-item">
        <strong>Status:</strong>
        <span class="${expiry.isExpired ? 'expired' : 'valid'}">${expiry.isExpired ? '❌ Expired' : '✅ Valid'}</span>
      </div>
    `
    : '';

  tokenDetails.innerHTML = `
    <div class="token-info">
      <div class="token-item">
        <strong>Access Token:</strong>
        <textarea readonly>${tokens.access_token || 'N/A'}</textarea>
      </div>
      <div class="token-item">
        <strong>ID Token:</strong>
        <textarea readonly>${tokens.id_token || 'N/A'}</textarea>
      </div>
      <div class="token-item">
        <strong>Refresh Token:</strong>
        <textarea readonly>${tokens.refresh_token || 'N/A'}</textarea>
      </div>
      <div class="token-item">
        <strong>Token Type:</strong>
        <span>${tokens.token_type || 'N/A'}</span>
      </div>
      ${expiryInfo}
    </div>
  `;
}

// Load and display dynamic claims
async function loadDynamicClaims() {
  try {
    loadClaimsBtn.disabled = true;
    loadClaimsBtn.textContent = 'Loading...';

    const claims = await getDynamicClaims();

    if (!claims || Object.keys(claims).length === 0) {
      claimsList.innerHTML = '<p class="no-claims">No dynamic claims found</p>';
      return;
    }

    const claimsHtml = Object.entries(claims)
      .map(([key, value]) => `
        <div class="claim-item">
          <div class="claim-key">${key}</div>
          <div class="claim-value">${value}</div>
          <button class="delete-claim-btn" data-key="${key}">Delete</button>
        </div>
      `)
      .join('');

    claimsList.innerHTML = claimsHtml;

    // Add delete event listeners
    document.querySelectorAll('.delete-claim-btn').forEach(btn => {
      btn.addEventListener('click', async (e) => {
        const key = e.target.dataset.key;
        if (confirm(`Delete claim "${key}"?`)) {
          await handleDeleteClaim(key);
        }
      });
    });

    displaySuccess('Claims loaded successfully');
  } catch (error) {
    console.error('Failed to load claims:', error);
    displayError('Failed to load claims: ' + error.message);
    claimsList.innerHTML = '<p class="error-text">Failed to load claims</p>';
  } finally {
    loadClaimsBtn.disabled = false;
    loadClaimsBtn.textContent = 'Load Claims';
  }
}

// Handle add claim button
function showAddClaimForm() {
  addClaimForm.style.display = 'block';
  claimKeyInput.value = '';
  claimValueInput.value = '';
  claimKeyInput.focus();
}

function hideAddClaimForm() {
  addClaimForm.style.display = 'none';
}

// Handle save claim
async function handleSaveClaim() {
  const key = claimKeyInput.value.trim();
  const value = claimValueInput.value.trim();

  if (!key || !value) {
    displayError('Both key and value are required');
    return;
  }

  try {
    saveClaimBtn.disabled = true;
    await updateDynamicClaim(key, value);
    hideAddClaimForm();
    await loadDynamicClaims();
    displaySuccess(`Claim "${key}" saved successfully`);
  } catch (error) {
    console.error('Failed to save claim:', error);
    displayError('Failed to save claim: ' + error.message);
  } finally {
    saveClaimBtn.disabled = false;
  }
}

// Handle delete claim
async function handleDeleteClaim(key) {
  try {
    await deleteDynamicClaim(key);
    await loadDynamicClaims();
    displaySuccess(`Claim "${key}" deleted successfully`);
  } catch (error) {
    console.error('Failed to delete claim:', error);
    displayError('Failed to delete claim: ' + error.message);
  }
}

// Display error message
function displayError(message) {
  const errorDiv = document.createElement('div');
  errorDiv.className = 'error';
  errorDiv.textContent = message;
  document.getElementById('auth-section').appendChild(errorDiv);

  // Auto-remove after 5 seconds
  setTimeout(() => errorDiv.remove(), 5000);
}

// Display success message
function displaySuccess(message) {
  const successDiv = document.createElement('div');
  successDiv.className = 'success';
  successDiv.textContent = message;
  document.getElementById('auth-section').appendChild(successDiv);

  // Auto-remove after 3 seconds
  setTimeout(() => successDiv.remove(), 3000);
}

// Handle API test button clicks
async function handleApiTest(apiFunction, resultElement, buttonElement) {
  try {
    buttonElement.disabled = true;
    const originalText = buttonElement.textContent;
    buttonElement.textContent = 'Loading...';
    resultElement.textContent = 'Loading...';

    const data = await apiFunction();
    resultElement.textContent = formatResponse(data);
    resultElement.style.color = '#51cf66';

    buttonElement.textContent = originalText;
  } catch (error) {
    resultElement.textContent = 'Error: ' + error.message;
    resultElement.style.color = '#ff6b6b';
    console.error('API test failed:', error);
  } finally {
    buttonElement.disabled = false;
  }
}

// Tab switching
tabBtns.forEach(btn => {
  btn.addEventListener('click', () => {
    const targetTab = btn.dataset.tab;

    // Update active tab button
    tabBtns.forEach(b => b.classList.remove('active'));
    btn.classList.add('active');

    // Update active tab content
    tabContents.forEach(content => {
      content.classList.remove('active');
    });

    const targetContent = document.getElementById(`${targetTab}-tab`);
    if (targetContent) {
      targetContent.classList.add('active');
    }

    // Load data if switching to certain tabs
    if (targetTab === 'tokens') {
      updateTokenTab();
    }
  });
});

// Event Listeners - Login buttons
popupLoginBtn.addEventListener('click', async () => {
  try {
    console.log('🪟 Popup login button clicked');
    popupLoginBtn.disabled = true;
    popupLoginBtn.textContent = 'Opening popup...';

    await loginWithPopup();
    updateUI();
    displaySuccess('Login successful!');
  } catch (error) {
    console.error('Popup login failed:', error);
    displayError('Popup login failed: ' + error.message);
  } finally {
    popupLoginBtn.disabled = false;
    popupLoginBtn.textContent = '🪟 Popup Login';
  }
});

redirectLoginBtn.addEventListener('click', async () => {
  try {
    console.log('↗️ Redirect login button clicked');
    await login();
  } catch (error) {
    console.error('Redirect login failed:', error);
    displayError('Redirect login failed: ' + error.message);
  }
});

// Event Listeners - Logout
logoutBtn.addEventListener('click', async () => {
  console.log('👋 Logout button clicked');
  await logout();
});

// Event Listeners - Claims management
loadClaimsBtn.addEventListener('click', loadDynamicClaims);
addClaimBtn.addEventListener('click', showAddClaimForm);
saveClaimBtn.addEventListener('click', handleSaveClaim);
cancelClaimBtn.addEventListener('click', hideAddClaimForm);

// Event Listeners - API Test Buttons
testPublicBtn.addEventListener('click', () => {
  handleApiTest(testPublic, publicResult, testPublicBtn);
});

testProtectedBtn.addEventListener('click', () => {
  handleApiTest(testProtected, protectedResult, testProtectedBtn);
});

testMeBtn.addEventListener('click', () => {
  handleApiTest(getUserProfile, meResult, testMeBtn);
});

testWeatherBtn.addEventListener('click', () => {
  handleApiTest(getWeather, weatherResult, testWeatherBtn);
});

// Start the application
initApp();
