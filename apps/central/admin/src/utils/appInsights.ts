import { ApplicationInsights } from '@microsoft/applicationinsights-web';

let appInsights: ApplicationInsights | null = null;

/**
 * Initializes Application Insights if connection string is provided
 * This is completely optional - the application will work fine without it
 */
export function initializeAppInsights(): ApplicationInsights | null {
  // Check if connection string is provided via environment variable
  const connectionString = import.meta.env.VITE_APPLICATIONINSIGHTS_CONNECTION_STRING;

  if (!connectionString) {
    console.info('Application Insights: Not configured (optional)');
    return null;
  }

  try {
    appInsights = new ApplicationInsights({
      config: {
        connectionString: connectionString,
        enableAutoRouteTracking: true, // Track page views automatically
        enableCorsCorrelation: true, // Correlate frontend and backend requests
        enableRequestHeaderTracking: true,
        enableResponseHeaderTracking: true,
        enableAjaxErrorStatusText: true, // Capture detailed error messages
      },
    });

    appInsights.loadAppInsights();
    appInsights.trackPageView(); // Manually track the initial page view

    console.info('Application Insights: Initialized successfully');
    return appInsights;
  } catch (error) {
    console.error('Application Insights: Failed to initialize', error);
    return null;
  }
}

/**
 * Gets the Application Insights instance
 */
export function getAppInsights(): ApplicationInsights | null {
  return appInsights;
}

/**
 * Tracks a custom event
 */
export function trackEvent(name: string, properties?: Record<string, any>) {
  if (appInsights) {
    appInsights.trackEvent({ name }, properties);
  }
}

/**
 * Tracks an exception
 */
export function trackException(error: Error, properties?: Record<string, any>) {
  if (appInsights) {
    appInsights.trackException({ exception: error }, properties);
  }
}

/**
 * Tracks a custom metric
 */
export function trackMetric(name: string, average: number, properties?: Record<string, any>) {
  if (appInsights) {
    appInsights.trackMetric({ name, average }, properties);
  }
}
