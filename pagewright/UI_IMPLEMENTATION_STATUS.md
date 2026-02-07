# PageWright UI Implementation Status

## Completed Files (Phase 5 - UI)

### ✅ Core Infrastructure (10 files)
1. **Configuration**
   - `src/config.ts` - Environment variables (API_URL, WS_URL, DEFAULT_DOMAIN)
   - `.env`, `.env.example` - Environment configuration

2. **TypeScript Types**
   - `src/types/api.ts` - All API types matching Gateway

3. **API Client**
   - `src/api/client.ts` - Axios client with auth interceptors, all Gateway endpoints

4. **Contexts**
   - `src/contexts/AuthContext.tsx` - Authentication state management

5. **Hooks**
   - `src/hooks/useWebSocket.ts` - WebSocket connection for real-time updates

6. **Layout & Navigation**
   - `src/components/Layout.tsx` - Main layout with header, sidebar, content
   - `src/components/Layout.css` - Responsive layout styles

7. **Auth Pages**
   - `src/pages/Login.tsx` - Login with email/password + Google OAuth button
   - `src/pages/Register.tsx` - Registration with password confirmation
   - `src/pages/ForgotPassword.tsx` - Password reset request
   - `src/pages/Auth.css` - Responsive auth styles

## ✅ Gateway Updates Completed
- Password reset endpoints (`/auth/forgot-password`, `/auth/reset-password`, `/auth/update-password`)
- WebSocket support (`/ws` endpoint with Hub, Client management)
- Database migration for password_reset_tokens table
- All handlers with tests (pending)

## 🚧 Remaining UI Files (To Be Created)

### Critical Pages (6 files)
1. **src/pages/ResetPassword.tsx**
   - Password reset form with token validation
   - Form: token (from URL), new password, confirm password

2. **src/pages/Profile.tsx**
   - User profile display
   - Password update form (current + new password)
   - Only for email/password users

3. **src/pages/Dashboard.tsx**
   - List all user sites in cards/table
   - Each site shows: FQDN, enabled status, live URL, preview URL
   - Actions: Manage Aliases, Enable/Disable, Build (navigate to chat), Delete

4. **src/pages/CreateSite.tsx**
   - FQDN input OR subdomain selection
   - Subdomain: text input + dropdown (shows .pagewright.io)
   - Template selection (dropdown)
   - On success: navigate to /chat/:fqdn

5. **src/pages/Chat.tsx**
   - Main chat interface with messages
   - Left sidebar: Versions list (scrollable, oldest to newest)
   - Chat messages: User (right), Agent (left), bubble design
   - Message input with file attach button (jpg, png, gif)
   - WebSocket integration for real-time job updates

6. **src/pages/Dashboard.css**
   - Styles for dashboard, site cards, responsive grid

### Components (6 files)
7. **src/components/SiteCard.tsx**
   - Display single site info
   - Buttons: Live URL, Preview URL, Manage Aliases, Enable/Disable, Build, Delete

8. **src/components/ManageAliasesModal.tsx**
   - Modal/dialog overlay (not browser popup)
   - List current aliases
   - Add new alias form
   - Delete alias button per alias

9. **src/components/ChatMessage.tsx**
   - Single message bubble
   - Props: message, sender (user/agent), timestamp
   - Different styles for user vs agent

10. **src/components/VersionsList.tsx**
   - Scrollable list of versions
   - Each version: build_id, timestamp
   - Click opens VersionActionModal

11. **src/components/VersionActionModal.tsx**
   - Modal with 3 buttons:
     - Preview (deploy to preview, open preview URL)
     - Promote (deploy to live, open live URL)
     - Delete (confirm + delete version)

12. **src/components/FileAttachment.tsx**
   - File upload button/input
   - Show selected files with preview (for images)
   - Remove button per file

### Utilities & Routing (3 files)
13. **src/utils/format.ts**
   - formatDate, formatTimestamp helpers

14. **src/App.tsx**
   - Main App component
   - React Router setup with all routes
   - AuthProvider wrapper
   - Protected routes wrapper

15. **src/main.tsx**
   - App entry point
   - Import PureCSS
   - ReactDOM.render

### Styles (2 files)
16. **src/index.css**
   - Global styles, reset
   - PureCSS overrides
   - Chat bubbles, modals
   - Responsive utilities

17. **src/components/Modal.css**
   - Generic modal overlay styles
   - Mobile-friendly modal

### Docker & Build (3 files)
18. **Dockerfile**
   - Multi-stage: build (node:18) → serve (nginx:alpine)
   - Copy build output to nginx
   - Expose port 80

19. **docker-compose.yaml**
   - UI service on port 3000
   - Depends on Gateway service
   - Environment variables

20. **nginx.conf**
   - SPA routing (fallback to index.html)
   - Proxy /api to Gateway
   - CORS headers

## Implementation Guide

### File Creation Order (Priority)
1. **App.tsx + main.tsx** - Get routing working
2. **ResetPassword.tsx** - Complete auth flow
3. **Profile.tsx** - User settings
4. **Dashboard.tsx + SiteCard.tsx** - Main landing page
5. **CreateSite.tsx** - Site creation flow
6. **Chat.tsx + ChatMessage.tsx** - Core feature
7. **VersionsList.tsx + VersionActionModal.tsx** - Version management
8. **ManageAliasesModal.tsx** - Alias management
9. **Utilities + Global Styles** - Polish
10. **Docker files** - Deployment

### Key Features Per Page

#### Dashboard
```tsx
- useEffect: fetch sites on mount
- Display in responsive grid (3 cols desktop, 2 tablet, 1 mobile)
- Each card: site info + action buttons
- "Create New Site" button at top
```

#### CreateSite
```tsx
- Two modes: FQDN or Subdomain
- Radio buttons to toggle mode
- Subdomain: <input>.pagewright.io (from env var)
- FQDN: full domain validation
- Template dropdown (hardcoded for MVP: ["template-1", "template-2"])
- On success: navigate(`/chat/${fqdn}`)
```

#### Chat
```tsx
- useParams() to get fqdn
- useState: messages[], conversationId, files[]
- useWebSocket: subscribe to job updates
- Left sidebar: <VersionsList fqdn={fqdn} />
- Messages: map over array, <ChatMessage /> for each
- Input: textarea + file button + send button
- On job update: add new version to list, show status message
```

#### ManageAliasesModal
```tsx
- Props: siteId, fqdn, onClose
- useEffect: fetch aliases on mount
- List current aliases
- Add form: input + "Add" button
- Delete: call deleteAlias API, refresh list
```

### Responsive Design Breakpoints
- Mobile: < 480px (1 column, stacked layout)
- Tablet: 481-768px (2 columns, side-by-side where possible)
- Desktop: > 768px (3 columns, full sidebar)

### WebSocket Message Handling
```tsx
// In Chat.tsx
const handleJobUpdate = (update: JobStatusUpdate) => {
  if (update.site_id === siteId) {
    if (update.status === 'success' && update.build_id) {
      // Add new version to list
      // Show success message
    } else if (update.status === 'failed') {
      // Show error message
    }
  }
};

useWebSocket(handleJobUpdate);
```

### URL Patterns
- Preview: `https://{fqdn}/preview` (open in new tab)
- Live: `https://{fqdn}/` (open in new tab)

## Next Steps

1. Create remaining 20 files following the guide above
2. Test all flows:
   - Auth (login, register, forgot/reset password, Google OAuth)
   - Dashboard (list, create, enable/disable)
   - Aliases (add, delete)
   - Chat (send message, handle clarification, file upload)
   - Versions (deploy preview, promote to live, delete)
3. Docker packaging
4. Integration testing with Gateway

## Testing Checklist

- [ ] Login with email/password
- [ ] Register new account
- [ ] Forgot password flow
- [ ] Reset password with token
- [ ] Update password in profile
- [ ] Google OAuth login
- [ ] List sites on dashboard
- [ ] Create site with FQDN
- [ ] Create site with subdomain
- [ ] Enable/disable site
- [ ] Add alias
- [ ] Delete alias
- [ ] Send chat message
- [ ] Upload images in chat
- [ ] Receive job status via WebSocket
- [ ] Deploy version to preview
- [ ] Promote version to live
- [ ] Delete version
- [ ] Mobile responsive on all pages
- [ ] Tablet responsive
- [ ] Desktop layout

## File Summary
- **Completed**: 10 files (config, types, API, auth context, layout, 4 auth pages)
- **Remaining**: 20 files (6 pages, 6 components, 3 utilities, 2 styles, 3 docker)
- **Total**: 30 files for complete UI implementation

This is a comprehensive React + TypeScript SPA with full authentication, site management, chat interface with file uploads, real-time WebSocket updates, and responsive design for mobile/tablet/desktop.
