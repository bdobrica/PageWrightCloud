# UI Service

**Port**: 5173 (development), 80 (production)

React/TypeScript user interface with chat-based site editing and real-time updates.

## Tech Stack

- **Framework**: React 18 + TypeScript
- **Build Tool**: Vite 5
- **Styling**: CSS Modules + Global CSS
- **HTTP Client**: Axios with interceptors
- **WebSocket**: Native WebSocket API
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
│   ├── ManageAliasesModal.tsx
│   ├── ChatMessage.tsx
│   ├── VersionsList.tsx
│   ├── VersionActionModal.tsx
│   └── FileAttachment.tsx
├── contexts/
│   └── AuthContext.tsx    # Authentication state management
├── hooks/
│   └── useWebSocket.ts    # WebSocket connection hook
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
- ✅ Google OAuth (button only)
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
- [ ] CreateSite: FQDN input and template selection
- [ ] Chat: Message interface with WebSocket
- [ ] Profile: User settings and password change
- [ ] ResetPassword: Token-based password reset

### Components
- [ ] SiteCard: Display site info with action buttons
- [ ] ManageAliasesModal: Add/remove domain aliases
- [ ] ChatMessage: User vs agent message bubbles
- [ ] VersionsList: Version history browser
- [ ] VersionActionModal: Deploy/preview/rollback actions
- [ ] FileAttachment: Multi-file upload widget

### Integration
- [ ] WebSocket connection for real-time updates
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

// Sites
api.post('/sites', { fqdn, template_id })
api.get('/sites', { params: { page, page_size } })
api.get(`/sites/${fqdn}`)
api.delete(`/sites/${fqdn}`)
api.post(`/sites/${fqdn}/enable`)
api.post(`/sites/${fqdn}/disable`)

// Aliases
api.get(`/sites/${fqdn}/aliases`)
api.post(`/sites/${fqdn}/aliases`, { alias })
api.delete(`/sites/${fqdn}/aliases/${alias}`)

// Versions
api.get(`/sites/${fqdn}/versions`)
api.post(`/sites/${fqdn}/versions/${versionId}/deploy`, { target })
api.delete(`/sites/${fqdn}/versions/${versionId}`)
api.get(`/sites/${fqdn}/versions/${versionId}/download`)

// Build
api.post(`/sites/${fqdn}/build`, { message, conversation_id? })
```

## WebSocket Integration

### Connection Hook

Located at `src/hooks/useWebSocket.ts`:

```typescript
const { messages, sendMessage, isConnected } = useWebSocket(
  config.WS_URL,
  token
);
```

### Message Format

**From Server:**
```json
{
  "type": "job_status",
  "data": {
    "job_id": "uuid",
    "status": "running",
    "progress": 45
  }
}
```

**To Server:**
```json
{
  "type": "subscribe",
  "site_id": "blog-example-com"
}
```

## Configuration

Environment variables (`.env` file):

```bash
VITE_API_URL=http://localhost:8085
VITE_WS_URL=ws://localhost:8085/ws
VITE_DEFAULT_DOMAIN=pagewright.dev
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

    # WebSocket proxy
    location /ws {
        proxy_pass http://gateway:8085/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "Upgrade";
        proxy_set_header Host $host;
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
      - VITE_WS_URL=ws://gateway:8085/ws
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
