// Maps technical service IDs to human-readable display names.
// If the service isn't in this map, we clean up the raw ID gracefully.
const SERVICE_NAMES = {
  'api-gateway':         'API Gateway',
  'order-service':       'Order Processing',
  'payment-service':     'Payment Service',
  'auth-service':        'Authentication',
  'user-service':        'User Accounts',
  'notification-service':'Notifications',
  'search-service':      'Search',
  'catalog-service':     'Product Catalog',
  'inventory-service':   'Inventory',
  'recommendation-service': 'Recommendations',
  'chaosgen':            'Chaos Generator (Test)',
}

const SERVICE_DESC = {
  'api-gateway':    'Handles all incoming traffic and routes requests to backend services',
  'order-service':  'Manages order creation, updates, and fulfillment workflows',
  'payment-service':'Processes payments and financial transactions',
  'auth-service':   'Handles login, tokens, and access permissions',
  'chaosgen':       'Test workload generator — not a production service',
}

export function serviceName(id) {
  if (!id) return 'Unknown Service'
  if (SERVICE_NAMES[id]) return SERVICE_NAMES[id]
  // Clean up raw IDs: "my-cool-svc" → "My Cool Service"
  return id
    .replace(/-/g, ' ')
    .replace(/\b\w/g, c => c.toUpperCase())
    .replace(/\bSvc\b/, 'Service')
}

export function serviceDesc(id) {
  return SERVICE_DESC[id] || `Connected service: ${id}`
}

export function isTestService(id) {
  return id === 'chaosgen' || id?.includes('test') || id?.includes('chaos')
}

// Zone → human explanation
export const ZONE_TEXT = {
  safe:     'Operating normally — well within capacity',
  warning:  'Under pressure — approaching limits',
  collapse: 'Critical — at or beyond safe capacity',
  '':       'Status unknown',
}

// Metric explanations for operators
export const METRIC_HELP = {
  rps:   'How many requests this service is receiving per second right now',
  lat:   'How long requests are waiting in queue before being processed',
  queue: 'Number of requests waiting to be handled — higher means more backlog',
  rho:   'How busy the service is as a percentage of its capacity (100% = fully loaded)',
  risk:  'The engine\'s estimated probability this service could stop responding',
  burst: 'How much traffic spikes are amplifying beyond normal baseline',
}

// Stage → what it means to the operator
export const STAGE_HUMAN = {
  detected:        'Problem detected by monitoring',
  analyzing:       'Engine is analyzing root cause',
  predicted:       'Engine has predicted what will happen next',
  action_selected: 'Engine has chosen a response action',
  executing:       'Response action is being applied',
  stabilizing:     'Conditions are improving — watching for recovery',
  resolved:        'Issue is resolved — system stable',
  failed:          'Response action did not resolve the issue',
  overridden:      'Closed by operator',
}