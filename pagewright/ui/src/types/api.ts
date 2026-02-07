// API Types matching Gateway

export interface User {
  id: string;
  email: string;
  created_at: string;
}

export interface AuthResponse {
  token: string;
  expires_in: number;
  user: User;
}

export interface Site {
  id: string;
  fqdn: string;
  user_id: string;
  template_id: string;
  live_version_id?: string;
  preview_version_id?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface SiteAlias {
  id: string;
  site_id: string;
  alias: string;
  created_at: string;
}

export interface Version {
  id: string;
  site_id: string;
  build_id: string;
  status: string;
  created_at: string;
}

export interface PaginatedResponse<T> {
  data: T[];
  page: number;
  page_size: number;
  total_count: number;
  total_pages: number;
}

export interface JobStatusUpdate {
  job_id: string;
  site_id: string;
  status: 'queued' | 'running' | 'success' | 'failed';
  build_id?: string;
  message?: string;
  timestamp: string;
}

// Request Types

export interface RegisterRequest {
  email: string;
  password: string;
}

export interface LoginRequest {
  email: string;
  password: string;
}

export interface ForgotPasswordRequest {
  email: string;
}

export interface ResetPasswordRequest {
  token: string;
  password: string;
}

export interface UpdatePasswordRequest {
  current_password: string;
  new_password: string;
}

export interface CreateSiteRequest {
  fqdn: string;
  template_id: string;
}

export interface AddAliasRequest {
  alias: string;
}

export interface DeployVersionRequest {
  target: 'live' | 'preview';
}

export interface BuildRequest {
  message: string;
  conversation_id?: string;
  files?: File[];
}

export interface BuildResponse {
  job_id?: string;
  question?: string;
  conversation_id?: string;
}

export interface ErrorResponse {
  error: string;
  message?: string;
}
