# UI Service

**Port**: 5173 (development), 80 (production)

React/TypeScript user interface with chat-based site editing and real-time updates.

The supported UI is the [text-only MVP](../../docs/adr/0022-architecture-decisions-mvp-capabilities.md).
See [draft recovery and publication feedback](../../docs/adr/0023-architecture-decisions-draft-recovery.md) for
same-tab re-authentication recovery, retry identity and the focused browser test.
See [keyboard/focus and responsive acceptance](../../docs/UI_ACCESSIBILITY.md) for
the five-viewport audit, native modal behavior and verification boundaries.
Attachments, alias management, custom-domain creation, Google sign-in and site
deletion are unavailable; historical feature descriptions below are not release
acceptance claims. Site creation reads the gateway's configured domain at runtime.

## Tech Stack

- **Framework**: React 18 + TypeScript
- **Build Tool**: Vite 5
- **Styling**: CSS Modules + Global CSS
- **HTTP Client**: Axios with interceptors
- **Build updates**: Bounded owner-checked HTTP polling (WebSockets disabled)
- **Authentication**: JWT tokens in localStorage
- **Router**: React Router v6

## Project Structure

```
src/
├── api/
│   └── client.ts          # Axios client with auth interceptors
├── components/
│   ├── Layout.tsx         # Main layout with header/sidebar
│   ├── Layout.css
│   ├── SiteCard.tsx       # Site display component
│   ├── ChatMessage.tsx
│   ├── VersionsList.tsx
│   ├── VersionActionModal.tsx
│   └── FileAttachment.tsx
├── contexts/
│   └── AuthContext.tsx    # Authentication state management
├── pages/
│   ├── Login.tsx
│   ├── Register.tsx
│   ├── ResetPassword.tsx
│   ├── Profile.tsx
│   ├── Dashboard.tsx      # Site listing
│   ├── CreateSite.tsx
│   ├── Chat.tsx           # Build interface
│   ├── Auth.css
│   └── Dashboard.css
├── types/
│   └── api.ts             # TypeScript types matching Gateway API
├── utils/
│   └── format.ts          # Date/time formatters
├── App.tsx                # Router configuration
├── main.tsx               # Application entry point
├── config.ts              # Environment variables
└── index.css              # Global styles
```

## Implemented Features

### Authentication
- ✅ Email/password login
- ✅ Email/password registration
- Google OAuth is disabled (UI and gateway).
- ✅ JWT token management
- ✅ Auto-redirect on auth failure
- ✅ Protected routes

### Layout & Navigation
- ✅ Responsive header with user menu
- ✅ Sidebar navigation
- ✅ Mobile-friendly design

## Remaining Features

### Pages
- [ ] Dashboard: Site cards with actions
- [x] CreateSite: Gateway-configured platform subdomain and starter template
- [x] Chat: Message interface with bounded build-history polling
- [ ] Profile: User settings and password change
- [ ] ResetPassword: Token-based password reset

### Components
- [ ] SiteCard: Display site info with action buttons
- Alias controls are removed; gateway alias routes return 501.
- [ ] ChatMessage: User vs agent message bubbles
- [ ] VersionsList: Version history browser
- [ ] VersionActionModal: Deploy/preview/rollback actions
- [ ] FileAttachment: Multi-file upload widget

### Integration
- [x] WebSocket integration disabled for the polling MVP
- [ ] Build status notifications
- [ ] Error handling and toast messages
- [ ] Loading states and skeletons

## API Integration

### API Client

Located at `src/api/client.ts`:

```typescript
const api = axios.create({
  baseURL: config.API_URL,
  headers: {
    'Content-Type': 'application/json'
  }
});

// Auto-attach JWT token
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Auto-redirect on 401
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);
```

### Available API Methods

```typescript
// Authentication
api.post('/auth/register', { email, password })
api.post('/auth/login', { email, password })
api.post('/auth/forgot-password', { email })
api.post('/auth/reset-password', { token, new_password })
api.post('/auth/update-password', { current_password, new_password })

// Runtime platform domain
api.get('/capabilities')

// Sites (site deletion is disabled)
api.post('/sites', { fqdn, template_id })
api.get('/sites', { params: { page, page_size } })
api.get(`/sites/${fqdn}`)
api.post(`/sites/${fqdn}/enable`)
api.post(`/sites/${fqdn}/disable`)

// Alias endpoints are disabled (501).

// Versions
api.get(`/sites/${fqdn}/versions`)
api.post(`/sites/${fqdn}/versions/${versionId}/deploy`, { target })
api.delete(`/sites/${fqdn}/versions/${versionId}`)
api.get(`/sites/${fqdn}/versions/${versionId}/download`)

// Build
api.post(`/sites/${fqdn}/build`, { message, conversation_id? })
```

## Build updates

The MVP uses owner-checked job history and bounded HTTP polling, not WebSockets.
See [the history/polling contract and WebSocket re-enable gate](../../docs/adr/0017-architecture-decisions-job-history-api.md).
No socket connection, query-token transport, subscription or reconnect timer is
created by the UI. The retired gateway `/ws` endpoint returns HTTP 501.

## Configuration

Environment variables (`.env` file):

```bash
VITE_API_URL=http://localhost:8085
# Site domain is supplied by the gateway /capabilities endpoint.
# Set PAGEWRIGHT_SITE_DOMAIN=pagewright.dev on the gateway.
```

## Development

### Install Dependencies
```bash
cd pagewright/ui
npm install
```

### Run Dev Server
```bash
npm run dev
# Opens http://localhost:5173
```

### Build for Production
```bash
npm run build
# Output: dist/
```

### Preview Production Build
```bash
npm run preview
```

### Linting
```bash
npm run lint
```

## Docker Deployment

### Dockerfile

```dockerfile
# Build stage
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Serve stage
FROM nginx:alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

### nginx.conf

```nginx
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # SPA routing
    location / {
        try_files $uri $uri/ /index.html;
    }

    # API proxy (optional)
    location /api/ {
        proxy_pass http://gateway:8085/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }

}
```

### docker-compose.yaml

```yaml
version: '3.8'

services:
  ui:
    build: .
    ports:
      - "3000:80"
    environment:
      - VITE_API_URL=http://gateway:8085
    depends_on:
      - gateway
```

## Styling Guidelines

### CSS Variables

Defined in `src/index.css`:

```css
:root {
  --color-primary: #007bff;
  --color-success: #28a745;
  --color-danger: #dc3545;
  --color-warning: #ffc107;
  --color-text: #333;
  --color-bg: #f5f5f5;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --border-radius: 4px;
}
```

### Component Styles

Use CSS Modules for component-specific styles:

```tsx
import styles from './SiteCard.module.css';

function SiteCard() {
  return <div className={styles.card}>...</div>;
}
```

### Responsive Breakpoints

```css
/* Mobile: < 768px */
@media (max-width: 767px) { }

/* Tablet: 768px - 1023px */
@media (min-width: 768px) and (max-width: 1023px) { }

/* Desktop: >= 1024px */
@media (min-width: 1024px) { }
```

## Testing

### Unit Tests (Not Yet Implemented)

```bash
npm run test
```

### E2E Tests (Not Yet Implemented)

```bash
npm run test:e2e
```

## Build Optimization

- Tree shaking (Vite default)
- Code splitting by route
- Lazy loading for modals and heavy components
- Image optimization
- Minification and compression

## Browser Support

- Chrome/Edge (last 2 versions)
- Firefox (last 2 versions)
- Safari (last 2 versions)
- No IE11 support
