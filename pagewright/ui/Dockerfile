# Build stage
FROM node:24.11.1-alpine AS build

WORKDIR /app

# Copy package files
COPY package.json package-lock.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Build arguments for Vite
ARG VITE_PAGEWRIGHT_API_URL=http://localhost:8085

# Set environment variables for build
ENV VITE_PAGEWRIGHT_API_URL=$VITE_PAGEWRIGHT_API_URL

# Build the application
RUN npm run build

# Production stage
FROM nginx:alpine

# Copy custom nginx config
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Copy built files from build stage
COPY --from=build /app/dist /usr/share/nginx/html

# Expose port
EXPOSE 80

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
